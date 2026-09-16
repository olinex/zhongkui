package ai

import (
	// stdlib
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	// third-party
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// rrfK 是 RRF（Reciprocal Rank Fusion）融合的固定常数 k。
// rrfK is the fixed constant k of the RRF (Reciprocal Rank Fusion).
const rrfK = 60

// RetrievalResult 是一条检索结果的视图层载体（json 序列化给 view 层使用）。
// RetrievalResult is the view-layer carrier of a single retrieval hit (serialized as JSON
// for the view layer).
type RetrievalResult struct {
	// DocumentID 所属知识文档 ID（字符串形式）
	// DocumentID the ID of the owning knowledge document (string form)
	DocumentID string `json:"documentID"`
	// Title 所属文档标题
	// Title the title of the owning document
	Title string `json:"title"`
	// ChunkIndex 分块在文档内的序号
	// ChunkIndex the chunk index within the document
	ChunkIndex int `json:"chunkIndex"`
	// Content 该块的检索文本（标题路径 + 正文）
	// Content the retrieval text of the chunk (heading path plus body)
	Content string `json:"content"`
	// Score 检索相关度分数（融合后得分）
	// Score the retrieval relevance score (post-fusion score)
	Score float64 `json:"score"`
}

// chunkKey 是融合打分时识别同一分块的键。
// chunkKey identifies the same chunk during fusion scoring.
type chunkKey struct {
	docID string // 文档 ID（字符串形式）
	index int    // 分块序号
}

// embedder 是知识库向量化所需的最小接口；*EmbedClient 与 eino 实现均满足。
// embedder is the minimal embedding interface needed by the knowledge base; both
// *EmbedClient and eino implementations satisfy it.
type embedder interface {
	EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)
}

// chatGenerator 是 LLM 重排所需的最小对话接口；*ChatClient 与 eino 实现均满足。
// chatGenerator is the minimal chat interface needed by the LLM rerank; both *ChatClient
// and eino implementations satisfy it.
type chatGenerator interface {
	Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// Knowledge 是知识库索引的入口：负责文档切块、向量化落库（knowledge_chunks）与
// 文档/分块删除。持有一个 gorm DB、向量化客户端与切片器；chat 客户端用于重排（可为空）。
// Knowledge is the entry of knowledge-base indexing: it chunks documents, embeds and
// persists them into knowledge_chunks, and deletes documents/chunks. It holds a gorm DB,
// the embedding client and the chunker; the chat client is used for rerank (may be nil).
type Knowledge struct {
	db      *database.DB
	embed   embedder
	chat    chatGenerator // 可空：为空时重排被跳过
	cfg     *pkgllm.LLMConfig
	chunker *pkgllm.MarkdownChunker
}

// NewKnowledge 构造 Knowledge；db/embed/cfg 任一为空时返回英文错误。
// NewKnowledge constructs a Knowledge; a nil db, embedder or config returns an English error.
func NewKnowledge(db *database.DB, embed embedder, chat chatGenerator, cfg *pkgllm.LLMConfig) (*Knowledge, error) {
	if db == nil {
		return nil, errors.New("knowledge: nil database")
	}
	if embed == nil {
		return nil, errors.New("knowledge: nil embedder")
	}
	if cfg == nil {
		return nil, errors.New("knowledge: nil ai config")
	}
	return &Knowledge{
		db:      db,
		embed:   embed,
		chat:    chat,
		cfg:     cfg,
		chunker: pkgllm.NewMarkdownChunker(cfg.Chunking.Size, cfg.Chunking.Overlap),
	}, nil
}

// IndexDocument 对文档执行切片与向量化，并在单事务内删除该文档旧分块后插入新分块，
// 最后更新 doc.ChunkCount 与 doc.Hash。分块正文的全文检索表示经
// to_tsvector('simple', ...) SQL 表达式写入 search 列。
// IndexDocument chunks and embeds the document, then deletes the document's old chunks
// and inserts the new ones within a single transaction, finally updating doc.ChunkCount
// and doc.Hash. The full-text search representation of each chunk body is written to the
// search column via the to_tsvector('simple', ...) SQL expression.
func (k *Knowledge) IndexDocument(ctx context.Context, doc *modelsai.KnowledgeDocument) error {
	if doc == nil {
		return errors.New("knowledge: nil document")
	}
	chunks, err := k.chunker.Transform(ctx, []*schema.Document{{
		ID:      strconv.FormatUint(uint64(doc.ID), 10),
		Content: doc.Content,
		MetaData: map[string]any{
			"namespace":  doc.NamespaceName,
			"documentID": strconv.FormatUint(uint64(doc.ID), 10),
			"title":      doc.Title,
		},
	}})
	if err != nil {
		return errors.WithStack(err)
	}
	// 每块索引文本 = 标题路径 + 正文，向量化与全文检索共用该文本
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		path, _ := c.MetaData["headingPath"].(string)
		texts[i] = strings.TrimSpace(path + "\n" + c.Content)
	}
	vectors := make([][]float64, len(chunks))
	if len(texts) > 0 {
		if vectors, err = k.embed.EmbedStrings(ctx, texts); err != nil {
			return errors.WithStack(err)
		}
	}
	hash := sha256Hex(doc.Content)
	return k.db.Transaction(func(tx *gorm.DB) error {
		// 单事务内先清旧块再插入新块，保证索引原子替换
		if err := tx.Where("document_id = ?", doc.ID).Delete(&modelsai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		for i, c := range chunks {
			path, _ := c.MetaData["headingPath"].(string)
			chunk := &modelsai.KnowledgeChunk{
				DocumentID:    doc.ID,
				NamespaceName: doc.NamespaceName,
				ChunkIndex:    i,
				HeadingPath:   path,
				Content:       c.Content,
				Embedding:     toFloat32(vectors[i]),
			}
			if err := tx.Create(chunk).Error; err != nil {
				return errors.WithStack(err)
			}
			// search 列为 tsvector，须经 SQL 表达式写入
			if err := tx.Model(&modelsai.KnowledgeChunk{}).
				Where("id = ?", chunk.ID).
				Updates(map[string]any{"search": gorm.Expr("to_tsvector('simple', ?)", texts[i])}).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		return tx.Model(&modelsai.KnowledgeDocument{}).
			Where("id = ?", doc.ID).
			Updates(map[string]any{"chunk_count": len(chunks), "hash": hash}).Error
	})
}

// Reindex 重新切片并重建文档的分块索引。IndexDocument 本身即在事务内先清旧块，
// 因此 Reindex 直接复用 IndexDocument。
// Reindex re-chunks and rebuilds the chunk index of the document. IndexDocument already
// clears the old chunks inside its transaction, so Reindex simply reuses IndexDocument.
func (k *Knowledge) Reindex(ctx context.Context, doc *modelsai.KnowledgeDocument) error {
	return k.IndexDocument(ctx, doc)
}

// DeleteDocument 删除文档及其分块：先显式删除分块（文档为软删除，DB 级联不生效），
// 再软删除文档本身，两步在同一事务内完成。
// DeleteDocument deletes a document and its chunks: chunks are deleted explicitly first
// (the document is soft-deleted so the DB cascade does not fire), then the document is
// soft-deleted, both inside a single transaction.
func (k *Knowledge) DeleteDocument(ctx context.Context, docID uint) error {
	return k.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ?", docID).Delete(&modelsai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ?", docID).Delete(&modelsai.KnowledgeDocument{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// RetrieveOptions 是 Retriever.Retrieve 的实现专属调用参数，经 retriever.Option 注入。
// RetrieveOptions carries the implementation-specific call parameters of
// Retriever.Retrieve, injected via retriever.Option.
type RetrieveOptions struct {
	// NamespaceName 限定检索的命名空间（多租户隔离）
	// NamespaceName the namespace to restrict the retrieval to (multi-tenant isolation)
	NamespaceName string
	// DocIDs 限定检索的文档 ID 列表，为空表示不限定
	// DocIDs the document IDs to restrict the retrieval to, empty means no restriction
	DocIDs []uint
	// TopK LLM 重排后保留的最终条数
	// TopK the number of final hits kept after the LLM rerank
	TopK int
	// Candidates 融合后送入重排的候选条数
	// Candidates the number of candidates fed into the rerank after fusion
	Candidates int
	// Fusion 融合策略：rrf 或 weighted
	// Fusion the fusion strategy: rrf or weighted
	Fusion string
	// Weights weighted 融合权重
	// Weights the weights of the weighted fusion
	Weights pkgllm.WeightConfig
}

// WithNamespace 设置检索命名空间。
// WithNamespace sets the retrieval namespace.
func WithNamespace(namespaceName string) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.NamespaceName = namespaceName
	})
}

// WithDocIDs 设置限定检索的文档 ID 列表。
// WithDocIDs sets the document IDs the retrieval is restricted to.
func WithDocIDs(docIDs []uint) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.DocIDs = docIDs
	})
}

// WithRetrieveTopK 设置重排后保留的最终条数。
// WithRetrieveTopK sets the number of final hits kept after the rerank.
func WithRetrieveTopK(topK int) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.TopK = topK
	})
}

// WithCandidates 设置融合后送入重排的候选条数。
// WithCandidates sets the number of candidates fed into the rerank after fusion.
func WithCandidates(candidates int) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Candidates = candidates
	})
}

// WithFusion 设置融合策略（rrf 或 weighted）。
// WithFusion sets the fusion strategy (rrf or weighted).
func WithFusion(fusion string) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Fusion = fusion
	})
}

// WithWeights 设置 weighted 融合权重。
// WithWeights sets the weights of the weighted fusion.
func WithWeights(weights pkgllm.WeightConfig) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Weights = weights
	})
}

// Retriever 是知识库的混合检索器，实现 eino retriever.Retriever 接口：
// 向量路（pgvector 余弦距离）与关键词路（tsvector 全文检索）各取 candidates 条，
// 经 RRF / weighted 融合后送入 LLM 重排取 TopK。chat 客户端不可用时跳过重排，
// 直接返回融合排序前 TopK（见 Retrieve 注释）。
// Retriever is the hybrid retriever of the knowledge base, implementing the eino
// retriever.Retriever interface: the vector path (pgvector cosine distance) and the
// keyword path (tsvector full-text search) each fetch candidates hits, which are fused
// via RRF / weighted and then reranked by the LLM down to TopK. When the chat client is
// unavailable the rerank is skipped and the fusion-ranked TopK is returned directly (see
// the Retrieve comment).
type Retriever struct {
	db    *database.DB
	embed embedder
	chat  chatGenerator // 可空：为空时跳过 LLM 重排
	cfg   *pkgllm.LLMConfig
}

// NewRetriever 构造 Retriever；db/embed/cfg 任一为空时返回英文错误。
// NewRetriever constructs a Retriever; a nil db, embedder or config returns an English
// error.
func NewRetriever(db *database.DB, embed embedder, chat chatGenerator, cfg *pkgllm.LLMConfig) (*Retriever, error) {
	if db == nil {
		return nil, errors.New("retriever: nil database")
	}
	if embed == nil {
		return nil, errors.New("retriever: nil embedder")
	}
	if cfg == nil {
		return nil, errors.New("retriever: nil ai config")
	}
	return &Retriever{db: db, embed: embed, chat: chat, cfg: cfg}, nil
}

// Retrieve 执行混合检索：向量路 + 关键词路 → 融合 → LLM 重排。
// 降级策略：chat 客户端为 nil 或重排调用/解析失败时，回退为融合排序前 TopK
// （英文注释说明降级行为），检索仍可用。
// Retrieve runs the hybrid retrieval: vector path plus keyword path, fusion, then LLM
// rerank. Degradation strategy: when the chat client is nil or the rerank call/parse
// fails, it falls back to the fusion-ranked TopK (the degradation is documented here in
// English), keeping the retrieval usable.
func (r *Retriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{
		TopK:       r.cfg.Rerank.TopK,
		Candidates: r.cfg.Rerank.Candidates,
		Fusion:     r.cfg.Rerank.Fusion,
		Weights:    r.cfg.Rerank.Weights,
	}, opts...)
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("retriever: empty query")
	}
	if o.Candidates <= 0 {
		o.Candidates = o.TopK
	}
	if o.TopK <= 0 || o.TopK > o.Candidates {
		o.TopK = o.Candidates
	}

	queryVec, err := r.embed.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	vectorHits, err := r.vectorRetrieve(ctx, o, queryVec[0])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	keywordHits, err := r.keywordRetrieve(ctx, o, query)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cands := fuseRetrievalResults(vectorHits, keywordHits, o.Fusion, o.Weights)
	cands = selectTopK(cands, o.Candidates)

	// 无 chat 客户端：跳过 LLM 重排，直接返回融合排序前 TopK。
	// No chat client: the LLM rerank is skipped and the fusion-ranked TopK is returned.
	if r.chat == nil {
		return toSchemaDocuments(selectTopK(cands, o.TopK)), nil
	}
	// 重排失败（调用错误或 JSON 解析失败）：降级为融合排序前 TopK。
	// Rerank failure (call error or JSON parse failure): degrade to the fusion-ranked TopK.
	ranked, rerankErr := r.rerank(ctx, query, cands, o.TopK)
	if rerankErr != nil {
		return toSchemaDocuments(selectTopK(cands, o.TopK)), nil
	}
	return toSchemaDocuments(ranked), nil
}

// vectorRetrieve 执行向量路检索：按余弦距离（embedding <=> ?）升序取 candidates 条。
// vectorRetrieve runs the vector-path retrieval: ascending cosine distance
// (embedding <=> ?) limited to candidates hits.
func (r *Retriever) vectorRetrieve(ctx context.Context, o *RetrieveOptions, vec []float64) ([]*RetrievalResult, error) {
	if o.Candidates <= 0 || len(vec) == 0 {
		return nil, nil
	}
	lit := vectorLiteral(vec)
	q := r.db.Table("knowledge_chunks AS c").
		Select("c.document_id, c.chunk_index, c.heading_path, c.content, d.title, (1 - (c.embedding <=> ?::vector)) AS score", lit).
		Joins("JOIN knowledge_documents AS d ON d.id = c.document_id AND d.deleted_at IS NULL").
		Where("c.namespace_name = ?", o.NamespaceName)
	if len(o.DocIDs) > 0 {
		q = q.Where("c.document_id IN ?", o.DocIDs)
	}
	q = q.Order(clause.OrderBy{Expression: gorm.Expr("c.embedding <=> ?::vector", lit)}).
		Limit(o.Candidates)
	var rows []struct {
		modelsai.KnowledgeChunk
		Title string
		Score float64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rowsToResults(rows), nil
}

// keywordRetrieve 执行关键词路检索：search @@ plainto_tsquery 过滤，
// 按 ts_rank 降序取 candidates 条。
// keywordRetrieve runs the keyword-path retrieval: filtered by
// search @@ plainto_tsquery and ordered by ts_rank descending, limited to candidates hits.
func (r *Retriever) keywordRetrieve(ctx context.Context, o *RetrieveOptions, query string) ([]*RetrievalResult, error) {
	if o.Candidates <= 0 {
		return nil, nil
	}
	q := r.db.Table("knowledge_chunks AS c").
		Select("c.document_id, c.chunk_index, c.heading_path, c.content, d.title, ts_rank(c.search, plainto_tsquery('simple', ?)) AS score", query).
		Joins("JOIN knowledge_documents AS d ON d.id = c.document_id AND d.deleted_at IS NULL").
		Where("c.namespace_name = ?", o.NamespaceName).
		Where("c.search @@ plainto_tsquery('simple', ?)", query)
	if len(o.DocIDs) > 0 {
		q = q.Where("c.document_id IN ?", o.DocIDs)
	}
	q = q.Order(clause.OrderBy{Expression: gorm.Expr("ts_rank(c.search, plainto_tsquery('simple', ?)) DESC", query)}).
		Limit(o.Candidates)
	var rows []struct {
		modelsai.KnowledgeChunk
		Title string
		Score float64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rowsToResults(rows), nil
}

// rerank 将候选块发给 chat 模型按相关度排序：模型须返回 JSON 数组（按相关度降序的
// 候选序号），解析失败或返回无效序号时返回错误（上层降级为融合排序）。chat 为 nil
// 时同样返回错误（上层跳过重排）。prompt 与系统指令均为英文，要求模型仅输出 JSON。
// rerank sends the candidates to the chat model for relevance ordering: the model must
// return a JSON array (candidate indices in descending relevance); a parse failure or
// invalid indices yield an error (the caller degrades to the fusion order). A nil chat
// also yields an error (the caller skips the rerank). The prompt and the system
// instruction are in English, asking the model to output JSON only.
func (r *Retriever) rerank(ctx context.Context, query string, cands []*RetrievalResult, topK int) ([]*RetrievalResult, error) {
	if r.chat == nil {
		return nil, errors.New("retriever: chat client unavailable, rerank skipped")
	}
	messages := []*schema.Message{
		schema.SystemMessage("You are a relevance reranker for retrieval-augmented generation. Rank the given chunks by relevance to the query and return ONLY a JSON array of chunk indices ordered from most to least relevant, e.g. [3,1,0]. Do not output any other text."),
		schema.UserMessage(buildRerankPrompt(query, cands)),
	}
	resp, err := r.chat.Generate(ctx, messages)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	indices, err := parseRerankIndices(resp.Content)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ranked := make([]*RetrievalResult, 0, len(indices))
	used := make(map[int]bool, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(cands) || used[idx] {
			continue
		}
		used[idx] = true
		ranked = append(ranked, cands[idx])
	}
	if len(ranked) == 0 {
		return nil, errors.New("retriever: rerank returned no valid candidate indices")
	}
	return selectTopK(ranked, topK), nil
}

// buildRerankPrompt 构造重排请求的用户消息：列出 query 与带序号的候选块。
// buildRerankPrompt builds the user message of the rerank request, listing the query and
// the numbered candidate chunks.
func buildRerankPrompt(query string, cands []*RetrievalResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nChunks:\n", query)
	for i, c := range cands {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, c.Title, truncateText(c.Content, 500))
	}
	return sb.String()
}

// parseRerankIndices 解析模型返回的 JSON 序号数组；容忍 ```json 代码围栏包裹。
// parseRerankIndices parses the JSON index array returned by the model; ```json code
// fences around the payload are tolerated.
func parseRerankIndices(content string) ([]int, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			content = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if content == "" {
		return nil, errors.New("retriever: rerank response is empty")
	}
	var indices []int
	if err := json.Unmarshal([]byte(content), &indices); err == nil {
		return indices, nil
	}
	var floats []float64
	if err := json.Unmarshal([]byte(content), &floats); err == nil {
		// 独立切片：首次 []int 解码失败时可能已部分填充 indices，不得复用
		out := make([]int, 0, len(floats))
		for _, f := range floats {
			out = append(out, int(f))
		}
		return out, nil
	}
	return nil, errors.Newf("retriever: cannot parse rerank response as an index array: %q", truncateText(content, 200))
}

// fuseRetrievalResults 按策略融合两路检索结果；未知策略默认 RRF。
// fuseRetrievalResults fuses the two retrieval paths by the given strategy; unknown
// strategies default to RRF.
func fuseRetrievalResults(vector, keyword []*RetrievalResult, fusion string, weights pkgllm.WeightConfig) []*RetrievalResult {
	switch strings.ToLower(strings.TrimSpace(fusion)) {
	case "weighted":
		return fuseWeighted(vector, keyword, weights)
	default:
		return fuseRRF(vector, keyword)
	}
}

// fuseRRF 以 RRF（sum(1/(k+rank))，k=60）融合两路结果，按融合分降序返回。
// fuseRRF fuses the two paths with RRF (sum(1/(k+rank)), k=60), returning hits sorted by
// the fused score descending.
func fuseRRF(vector, keyword []*RetrievalResult) []*RetrievalResult {
	scores := make(map[chunkKey]*RetrievalResult)
	addPath := func(path []*RetrievalResult) {
		for i, hit := range path {
			key := chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}
			entry, ok := scores[key]
			if !ok {
				cp := *hit
				cp.Score = 0
				entry = &cp
				scores[key] = entry
			}
			entry.Score += 1.0 / float64(rrfK+i+1)
		}
	}
	addPath(vector)
	addPath(keyword)
	return sortedResults(scores)
}

// fuseWeighted 以归一化加权融合两路结果：每路得分先 min-max 归一化（最大值等于最小值
// 时取 0.5），再按 vector/keyword 权重线性加权，最后按融合分降序返回。
// fuseWeighted fuses the two paths with normalized weights: each path score is min-max
// normalized first (0.5 when the max equals the min), then linearly weighted by the
// vector/keyword weights, finally returned sorted by the fused score descending.
func fuseWeighted(vector, keyword []*RetrievalResult, weights pkgllm.WeightConfig) []*RetrievalResult {
	scores := make(map[chunkKey]*RetrievalResult)
	normalized := func(path []*RetrievalResult) map[chunkKey]float64 {
		out := make(map[chunkKey]float64, len(path))
		if len(path) == 0 {
			return out
		}
		minS, maxS := path[0].Score, path[0].Score
		for _, hit := range path {
			if hit.Score < minS {
				minS = hit.Score
			}
			if hit.Score > maxS {
				maxS = hit.Score
			}
		}
		for _, hit := range path {
			norm := 0.5
			if maxS > minS {
				norm = (hit.Score - minS) / (maxS - minS)
			}
			out[chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}] = norm
		}
		return out
	}
	vecNorm := normalized(vector)
	kwNorm := normalized(keyword)
	keyOf := func(hit *RetrievalResult) chunkKey {
		return chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}
	}
	merge := func(path []*RetrievalResult, w float64, norm map[chunkKey]float64) {
		for _, hit := range path {
			key := keyOf(hit)
			if _, ok := scores[key]; !ok {
				cp := *hit
				cp.Score = 0
				scores[key] = &cp
			}
			// 每路只叠加本路的加权归一化分，避免两路同现的命中被重复计分
			scores[key].Score += w * norm[key]
		}
	}
	merge(vector, weights.Vector, vecNorm)
	merge(keyword, weights.Keyword, kwNorm)
	return sortedResults(scores)
}

// sortedResults 将融合得分表转为按分数降序的切片（分数相同按 docID/chunkIndex 稳定排序）。
// sortedResults converts the fusion score map into a slice ordered by score descending
// (ties broken deterministically by docID/chunkIndex).
func sortedResults(scores map[chunkKey]*RetrievalResult) []*RetrievalResult {
	out := make([]*RetrievalResult, 0, len(scores))
	for _, hit := range scores {
		out = append(out, hit)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if lessResult(out[j], out[j-1]) {
				out[j], out[j-1] = out[j-1], out[j]
			} else {
				break
			}
		}
	}
	return out
}

// lessResult 判断 a 是否应排在 b 之前（分数更高，或同分时 docID/chunkIndex 更小）。
// lessResult reports whether a should precede b (higher score, or smaller
// docID/chunkIndex on a tie).
func lessResult(a, b *RetrievalResult) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.DocumentID != b.DocumentID {
		return a.DocumentID < b.DocumentID
	}
	return a.ChunkIndex < b.ChunkIndex
}

// selectTopK 取融合排序后的前 topK 条（重排不可用/失败时的降级结果）；topK 非正数时
// 返回全部。
// selectTopK takes the first topK hits of the fusion order (the degraded result when the
// rerank is unavailable or fails); a non-positive topK returns all hits.
func selectTopK(cands []*RetrievalResult, topK int) []*RetrievalResult {
	if topK <= 0 || len(cands) <= topK {
		return cands
	}
	return cands[:topK]
}

// toSchemaDocuments 将检索结果转为 eino schema.Document，MetaData 携带 documentID、
// title、chunkIndex、score，Content 为该块检索文本。
// toSchemaDocuments converts retrieval results into eino schema.Document values whose
// MetaData carries documentID, title, chunkIndex and score, with Content being the
// retrieval text of the chunk.
func toSchemaDocuments(results []*RetrievalResult) []*schema.Document {
	out := make([]*schema.Document, 0, len(results))
	for _, r := range results {
		out = append(out, &schema.Document{
			ID:      r.DocumentID,
			Content: r.Content,
			MetaData: map[string]any{
				"documentID": r.DocumentID,
				"title":      r.Title,
				"chunkIndex": r.ChunkIndex,
				"score":      r.Score,
			},
		})
	}
	return out
}

// ToRetrievalResults 从 eino schema.Document 列表（MetaData 携带 documentID/title/
// chunkIndex/score，Content 为检索文本）转换出 RetrievalResult 列表，供 view 层序列化。
// ToRetrievalResults converts eino schema.Document values (MetaData carrying
// documentID/title/chunkIndex/score and Content being the retrieval text) into a
// RetrievalResult list for view-layer serialization.
func ToRetrievalResults(docs []*schema.Document) []RetrievalResult {
	out := make([]RetrievalResult, 0, len(docs))
	for _, d := range docs {
		r := RetrievalResult{Content: d.Content}
		if d.MetaData != nil {
			if v, ok := d.MetaData["documentID"].(string); ok {
				r.DocumentID = v
			}
			if v, ok := d.MetaData["title"].(string); ok {
				r.Title = v
			}
			if v, ok := d.MetaData["chunkIndex"].(int); ok {
				r.ChunkIndex = v
			}
			if v, ok := d.MetaData["score"].(float64); ok {
				r.Score = v
			}
		}
		out = append(out, r)
	}
	return out
}

// rowsToResults 将检索 SQL 扫描行转换为检索结果，Content 为标题路径 + 正文的检索文本。
// rowsToResults converts retrieval SQL scan rows into retrieval results whose Content is
// the heading path plus the chunk body.
func rowsToResults(rows []struct {
	modelsai.KnowledgeChunk
	Title string
	Score float64
}) []*RetrievalResult {
	out := make([]*RetrievalResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, &RetrievalResult{
			DocumentID: strconv.FormatUint(uint64(row.DocumentID), 10),
			Title:      row.Title,
			ChunkIndex: row.ChunkIndex,
			Content:    strings.TrimSpace(row.HeadingPath + "\n" + row.Content),
			Score:      row.Score,
		})
	}
	return out
}

// vectorLiteral 将 []float64 序列化为 pgvector 字面量（如 "[0.1,0.2,...]"）。
// vectorLiteral serializes []float64 into a pgvector literal (e.g. "[0.1,0.2,...]").
func vectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// toFloat32 将 []float64 转为 []float32（向量化输出转存储维度类型）。
// toFloat32 converts []float64 into []float32 (embedding output to the storage type).
func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}

// sha256Hex 计算内容的 sha256 十六进制摘要。
// sha256Hex computes the sha256 hex digest of the content.
func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// truncateText 将文本截断到前 max 个字符（超过时追加省略号），避免重排 prompt 超长。
// truncateText truncates text to the first max characters (appending an ellipsis when
// longer), keeping the rerank prompt bounded.
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
