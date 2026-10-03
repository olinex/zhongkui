package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// rrfK 是 RRF（Reciprocal Rank Fusion）融合的固定常数 k。
const rrfK = 60

// RetrievalResult 是一条检索结果的视图层载体（json 序列化给 view 层使用）。
type RetrievalResult struct {
	// DocumentID 所属知识文档 ID（字符串形式）
	DocumentID string `json:"documentID"`
	// Title 所属文档标题
	Title string `json:"title"`
	// ChunkIndex 分块在文档内的序号
	ChunkIndex int `json:"chunkIndex"`
	// Content 该块的检索文本（标题路径 + 正文）
	Content string `json:"content"`
	// Score 检索相关度分数（融合后得分）
	Score float64 `json:"score"`
}

// chunkKey 是融合打分时识别同一分块的键。
type chunkKey struct {
	docID string // 文档 ID（字符串形式）
	index int    // 分块序号
}

// embedder 是知识库向量化所需的最小接口；*EmbedClient 与 eino 实现均满足。
type embedder interface {
	EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)
}

// chatModelNamer 是能报告自身模型名的对话客户端的最小接口（*llm.ChatClient 天然满足）。
type chatModelNamer interface {
	ModelName() string
}

// chatGenerator 是知识库所需的最小对话接口；*ChatClient 与 eino 实现均满足。
// 重排使用非流式 Generate；dry-run 的评估链路使用流式 Stream 以实时透传模型输出。
type chatGenerator interface {
	Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
	Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
}

// chatModelName 尽力取回 chat 客户端的模型名（客户端不支持时返回空串），用于
// dry-run 结果的调用明细。
func chatModelName(chat chatGenerator) string {
	if namer, ok := chat.(chatModelNamer); ok {
		return namer.ModelName()
	}
	return ""
}

// Knowledge 是知识库索引的入口：负责文档切块、向量化落库（knowledge_chunks）与
// 文档/分块删除。持有一个 gorm DB、向量化客户端与切片器。
type Knowledge struct {
	// db core 库连接（knowledge_chunks / knowledge_documents 读写）。
	db *database.DB
	// embed 向量化客户端（把分块检索文本转为向量）。
	embed  embedder
	params *Params // 行为参数解析器（切片长度与重叠）
	// chunker Markdown 切片器（按参数长度与重叠切块）。
	chunker *llm.MarkdownChunker
}

// NewKnowledge 构造 Knowledge；db/embed/params 任一为空时返回英文错误。
func NewKnowledge(db *database.DB, embed embedder, params *Params) (*Knowledge, error) {
	if db == nil {
		return nil, errors.New("knowledge: nil database")
	}
	if embed == nil {
		return nil, errors.New("knowledge: nil embedder")
	}
	if params == nil {
		return nil, errors.New("knowledge: nil params")
	}
	return &Knowledge{
		db:      db,
		embed:   embed,
		params:  params,
		chunker: llm.NewMarkdownChunker(params.Int("chunking.size"), params.Int("chunking.overlap")),
	}, nil
}

// IndexDocument 对文档执行切片与向量化，并在单事务内删除该文档旧分块后插入新分块，
// 最后更新 doc.ChunkCount 与 doc.Hash。分块正文的全文检索表示经
// to_tsvector('simple', ...) SQL 表达式写入 search 列。
func (k *Knowledge) IndexDocument(ctx context.Context, doc *ai.KnowledgeDocument) error {
	if doc == nil {
		return errors.New("knowledge: nil document")
	}
	plaintext := string(doc.Content.Value())
	chunks, err := k.chunker.Transform(ctx, []*schema.Document{{
		ID:      strconv.FormatUint(uint64(doc.ID), 10),
		Content: plaintext,
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
	// 内容摘要为带密钥的 HMAC-SHA256：明文不落库，同时摘要确定可比，供过期判定使用
	hash, err := crypto.DigestForField(&ai.KnowledgeDocument{}, "hash", "json", []byte(plaintext))
	if err != nil {
		return errors.WithStack(err)
	}
	return k.db.Transaction(func(tx *gorm.DB) error {
		// 单事务内先清旧块再插入新块，保证索引原子替换
		if err := tx.Where("document_id = ?", doc.ID).Delete(&ai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		for i, c := range chunks {
			path, _ := c.MetaData["headingPath"].(string)
			// 标题路径与正文经 crypto 序列化器加密落库（Create 走结构体路径，序列化器生效）
			chunk := &ai.KnowledgeChunk{
				DocumentID:    doc.ID,
				NamespaceName: doc.NamespaceName,
				ChunkIndex:    i,
				HeadingPath:   serializer.NewCrypto([]byte(path)),
				Content:       serializer.NewCrypto([]byte(c.Content)),
				Embedding:     toFloat32(vectors[i]),
			}
			if err := tx.Create(chunk).Error; err != nil {
				return errors.WithStack(err)
			}
			// search 列为 tsvector，须经 SQL 表达式写入
			if err := tx.Model(&ai.KnowledgeChunk{}).
				Where("id = ?", chunk.ID).
				Updates(map[string]any{"search": gorm.Expr("to_tsvector('simple', ?)", texts[i])}).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		// 采用结构体 + Select 更新而非 map：hash 经 hash 序列化器落库，
		// map 更新走 clause.Assignment 直写会绕过序列化器
		return tx.Model(&ai.KnowledgeDocument{}).
			Where("id = ?", doc.ID).
			Select("chunk_count", "hash").
			Updates(&ai.KnowledgeDocument{
				ChunkCount: len(chunks),
				Hash:       serializer.NewHashDigest(hash),
			}).Error
	})
}

// Reindex 重新切片并重建文档的分块索引。IndexDocument 本身即在事务内先清旧块，
// 因此 Reindex 直接复用 IndexDocument。
func (k *Knowledge) Reindex(ctx context.Context, doc *ai.KnowledgeDocument) error {
	return k.IndexDocument(ctx, doc)
}

// ReindexNamespace 以当前 embedding 模型为准，重建命名空间下全部知识文档的分块与向量索引：
// 逐篇读取文档正文（经 crypto 序列化器解密）后复用 IndexDocument 重新切片、向量化与落库，
// 供更换 embedding 模型（维度或语义变化）后让 knowledge_chunks 与当前模型对齐。仅处理未软删除
// 的文档，不改文档正文；返回实际重建的文档篇数，中途失败即返回该错误（已处理的文档不回滚，
// 再次执行按同一全集继续收敛）。namespaceName 为空时返回英文错误。
func (k *Knowledge) ReindexNamespace(ctx context.Context, namespaceName string) (int, error) {
	if strings.TrimSpace(namespaceName) == "" {
		return 0, errors.New("knowledge: empty namespace for reindex")
	}
	var docs []ai.KnowledgeDocument
	if err := k.db.WithContext(ctx).
		Select("id", "namespace_name", "title", "content").
		Where("namespace_name = ?", namespaceName).
		Find(&docs).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	for i := range docs {
		if err := k.Reindex(ctx, &docs[i]); err != nil {
			return i, err
		}
	}
	return len(docs), nil
}

// DeleteDocument 删除文档及其分块：先显式删除分块（文档为软删除，DB 级联不生效），
// 再软删除文档本身，两步在同一事务内完成。
func (k *Knowledge) DeleteDocument(ctx context.Context, docID uint) error {
	return k.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ?", docID).Delete(&ai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ?", docID).Delete(&ai.KnowledgeDocument{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// FusionWeights 是 weighted 融合的两路权重。
type FusionWeights struct {
	// Vector 向量路权重
	Vector float64
	// Keyword 关键词路权重
	Keyword float64
}

// RetrieveOptions 是 Retriever.Retrieve 的实现专属调用参数，经 retriever.Option 注入。
type RetrieveOptions struct {
	// NamespaceName 限定检索的命名空间（多租户隔离）
	NamespaceName string
	// DocIDs 限定检索的文档 ID 列表，为空表示不限定
	DocIDs []uint
	// TopK LLM 重排后保留的最终条数
	TopK int
	// Candidates 融合后送入重排的候选条数
	Candidates int
	// Fusion 融合策略：rrf 或 weighted
	Fusion string
	// Weights weighted 融合权重
	Weights FusionWeights
}

// WithNamespace 设置检索命名空间。
func WithNamespace(namespaceName string) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.NamespaceName = namespaceName
	})
}

// WithDocIDs 设置限定检索的文档 ID 列表。
func WithDocIDs(docIDs []uint) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.DocIDs = docIDs
	})
}

// WithRetrieveTopK 设置重排后保留的最终条数。
func WithRetrieveTopK(topK int) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.TopK = topK
	})
}

// WithCandidates 设置融合后送入重排的候选条数。
func WithCandidates(candidates int) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Candidates = candidates
	})
}

// WithFusion 设置融合策略（rrf 或 weighted）。
func WithFusion(fusion string) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Fusion = fusion
	})
}

// WithWeights 设置 weighted 融合权重。
func WithWeights(weights FusionWeights) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Weights = weights
	})
}

// Retriever 是知识库的混合检索器，实现 eino retriever.Retriever 接口：
// 向量路（pgvector 余弦距离）与关键词路（tsvector 全文检索）各取 candidates 条，
// 经 RRF / weighted 融合后送入 LLM 重排取 TopK。重排按调用点声明的 flash 档位经
// resolver 解析对话客户端；resolver 缺失（未注入）时跳过重排，直接返回融合排序前
// TopK，flash 档位未绑定时返回明确英文错误而不切换到其他档位（见 Retrieve 注释）。
// 每次检索命中后异步回写命中文档的使用统计（use_count / last_used_at），供知识库
// 运营判断哪些文档真正被用到。
type Retriever struct {
	// db core 库连接（knowledge_chunks / knowledge_documents 读取）。
	db *database.DB
	// embed 向量化客户端（把查询文本转为向量）。
	embed    embedder
	resolver ChatResolver    // 可空：为空时跳过 LLM 重排与评估
	params   *Params         // 行为参数解析器（检索条数、融合、重排提示词与温度）
	logger   *logging.Logger // 可空：为 nil 时跳过告警日志
}

// NewRetriever 构造 Retriever；db/embed/params 任一为空时返回英文错误。resolver 可为
// nil，此时检索跳过 LLM 重排（见 Retrieve 的说明）；logger 可为 nil（内部判空后再用）。
func NewRetriever(db *database.DB, embed embedder, resolver ChatResolver, params *Params, logger *logging.Logger) (*Retriever, error) {
	if db == nil {
		return nil, errors.New("retriever: nil database")
	}
	if embed == nil {
		return nil, errors.New("retriever: nil embedder")
	}
	if params == nil {
		return nil, errors.New("retriever: nil params")
	}
	return &Retriever{db: db, embed: embed, resolver: resolver, params: params, logger: logger}, nil
}

// resolveRerankChat 按调用点声明的低档位（rerankSlot，即 flash）解析重排与评估使用的
// 对话客户端：resolver 未注入时返回 (nil, nil)，调用方跳过重排与评估；档位未绑定或绑定
// 不可用时返回运行时（Runtime）给出的英文错误，绝不静默切换到其他档位。
func (r *Retriever) resolveRerankChat() (chatGenerator, error) {
	if r.resolver == nil {
		return nil, nil
	}
	chat, err := r.resolver(ai.SlotFlash)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if chat == nil {
		return nil, errors.Newf("retriever: no chat model resolved for slot %q", ai.SlotFlash)
	}
	return chat, nil
}

// retrieveOptions 解析本次检索实际生效的参数：先以行为参数（retrieval.*）为默认值，再叠加
// 调用方经 opts 注入的覆盖，最后做两处归一化（Candidates 非正数时取 TopK；TopK 非正数或
// 超过 Candidates 时取 Candidates）。Retrieve 与 DryRun 共用本方法，保证 dry-run 的改进建议
// 引用的参数与实际检索完全一致。
func (r *Retriever) retrieveOptions(opts ...retriever.Option) *RetrieveOptions {
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{
		TopK:       r.params.Int("retrieval.topK"),
		Candidates: r.params.Int("retrieval.candidates"),
		Fusion:     r.params.String("retrieval.fusion"),
		Weights: FusionWeights{
			Vector:  r.params.Float("retrieval.weightVector"),
			Keyword: r.params.Float("retrieval.weightKeyword"),
		},
	}, opts...)
	if o.Candidates <= 0 {
		o.Candidates = o.TopK
	}
	if o.TopK <= 0 || o.TopK > o.Candidates {
		o.TopK = o.Candidates
	}
	return o
}

// Retrieve 执行混合检索：向量路 + 关键词路 → 融合 → 内容去重 → LLM 重排。
// 档位语义：重排声明使用 flash 档位（rerankSlot），该档位未绑定时直接返回英文错误，
// 不切换到其他档位。降级策略：未注入 resolver（重排未启用）或重排调用/解析失败时，
// 回退为融合排序前 TopK（英文注释说明降级行为），检索仍可用。
func (r *Retriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	o := r.retrieveOptions(opts...)
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("retriever: empty query")
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
	// 内容去重先于候选截断与重排：重复命中会占掉 candidates/topK 名额、把真正独立的候选
	// 挤出短名单，还会让重排在同一段文本的多个副本上重复判定（见 dedupeSimilarResults）
	cands = dedupeSimilarResults(cands)
	cands = selectTopK(cands, o.Candidates)

	// 解析重排使用的对话客户端（调用点声明 flash 档位）：未注入 resolver 时跳过重排，
	// 直接返回融合排序前 TopK；档位未绑定时返回英文错误，不切换到其他档位。
	chat, err := r.resolveRerankChat()
	if err != nil {
		return nil, err
	}
	var docs []*schema.Document
	if chat == nil {
		docs = toSchemaDocuments(selectTopK(cands, o.TopK))
	} else if ranked, rerankErr := r.rerank(ctx, chat, query, cands, o.TopK); rerankErr != nil {
		// 重排失败（调用错误或 JSON 解析失败）：降级为融合排序前 TopK。
		docs = toSchemaDocuments(selectTopK(cands, o.TopK))
	} else {
		docs = toSchemaDocuments(ranked)
	}
	// 命中回写使用统计：仅统计最终返回的文档，异步执行且失败仅告警，不影响检索结果
	r.touchUsageAsync(ctx, docs)
	return docs, nil
}

// DryRun 执行知识库 RAG dry-run：混合检索（参数可覆盖）→ 基于上下文生成回答 →
// RAGAS 风格量化评估 → 改进建议。全流程经 report 实时上报阶段状态与模型增量输出（为 nil 时
// 不上报）；report 返回错误（SSE 写失败/客户端断开）时立即中止并向上返回该错误。
// languageTag 为请求语言标签（BCP-47），用于要求模型以该语言输出改进建议正文，为空时不
// 下发语言指令。检索失败或 flash 档位未绑定（重排与评估的声明档位）时返回错误；评估客户端
// 缺失或检索结果为空时返回带检索结果与 evalError 的结果（evaluation 为空）。结果中的 Meta
// 汇总检索耗时、各次调用明细、总计耗时与 token。
func (r *Retriever) DryRun(ctx context.Context, query string, report EvalReporter, languageTag string, opts ...retriever.Option) (*DryRunResult, error) {
	totalStart := time.Now()
	// reportErr 记录上报写失败（客户端断开），与「评估前置条件不满足」的降级错误区分开
	var reportErr error
	emit := func(ev StageEvent) error {
		if report == nil || reportErr != nil {
			return reportErr
		}
		reportErr = report(ev)
		return reportErr
	}

	if err := emit(StageEvent{Stage: EvalStageRetrieve, Status: EvalStatusStart}); err != nil {
		return nil, err
	}
	retrieveStart := time.Now()
	docs, err := r.Retrieve(ctx, query, opts...)
	retrieveElapsed := time.Since(retrieveStart).Milliseconds()
	if err != nil {
		_ = emit(StageEvent{Stage: EvalStageRetrieve, Status: EvalStatusFailed, ElapsedMs: retrieveElapsed, Err: err.Error()})
		return nil, errors.WithStack(err)
	}
	results := ToRetrievalResults(docs)
	if err := emit(StageEvent{Stage: EvalStageRetrieve, Status: EvalStatusDone, ElapsedMs: retrieveElapsed}); err != nil {
		return nil, err
	}

	out := &DryRunResult{
		Query:   query,
		Results: results,
		Meta: DryRunMeta{
			RetrieveElapsedMs: retrieveElapsed,
			// 空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null
			Calls: []CallMeta{},
		},
	}
	// 评估 judge 同样声明使用 flash 档位；档位未绑定时返回英文错误（不切换到其他档位），
	// 由调用方以 SSE error 事件明确告知用户。
	chat, err := r.resolveRerankChat()
	if err != nil {
		return nil, err
	}
	// 改进建议需引用本次实际生效的检索参数（与 Retrieve 走上完全相同的默认值与归一化），
	// 并需要请求语言标签
	diag := diagnosisContext{Options: *r.retrieveOptions(opts...), Language: languageTag}
	ev, calls, evalErr := evaluate(ctx, r.params, chat, r.embed, query, results, diag, emit)
	if reportErr != nil {
		return nil, reportErr
	}
	if evalErr != nil {
		out.EvalError = evalErr.Error()
	} else {
		out.Evaluation = ev
	}
	if calls != nil {
		out.Meta.Calls = calls
	}
	modelName := chatModelName(chat)
	for i := range out.Meta.Calls {
		out.Meta.Calls[i].Model = modelName
		out.Meta.Usage.PromptTokens += out.Meta.Calls[i].Usage.PromptTokens
		out.Meta.Usage.CompletionTokens += out.Meta.Calls[i].Usage.CompletionTokens
		out.Meta.Usage.TotalTokens += out.Meta.Calls[i].Usage.TotalTokens
	}
	out.Meta.TotalElapsedMs = time.Since(totalStart).Milliseconds()
	return out, nil
}

// vectorRetrieve 执行向量路检索：按余弦距离（embedding <=> ?）升序取 candidates 条。
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
		ai.KnowledgeChunk
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
		ai.KnowledgeChunk
		Title string
		Score float64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rowsToResults(rows), nil
}

// rerank 将候选块发给 chat 模型按相关度排序：模型须返回 JSON 数组（按相关度降序的
// 候选序号），解析失败或返回无效序号时返回错误（上层降级为融合排序）。chat 由调用方
// 经 resolveRerankChat 按声明的 flash 档位解析后传入。prompt 与系统指令均为中文，
// 要求模型仅输出 JSON。
func (r *Retriever) rerank(ctx context.Context, chat chatGenerator, query string, cands []*RetrievalResult, topK int) ([]*RetrievalResult, error) {
	messages := []*schema.Message{
		schema.SystemMessage(r.params.Text("retrieval.rerankSystemPrompt")),
		schema.UserMessage(buildRerankPrompt(query, cands, r.params.Int("retrieval.rerankSnippetLimit"))),
	}
	// 重排使用低温度（参数 retrieval.rerankTemperature），使排序结果更确定
	resp, err := chat.Generate(ctx, messages, model.WithTemperature(float32(r.params.Float("retrieval.rerankTemperature"))))
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

// buildRerankPrompt 构造重排请求的用户消息：列出 query 与带序号的候选块（正文截断到
// snippetLimit 个字符）。
func buildRerankPrompt(query string, cands []*RetrievalResult, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "查询：%s\n\n候选分块：\n", query)
	for i, c := range cands {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, c.Title, truncateText(c.Content, snippetLimit))
	}
	return sb.String()
}

// parseRerankIndices 解析模型返回的 JSON 序号数组；容忍 ```json 代码围栏包裹。
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
func fuseRetrievalResults(vector, keyword []*RetrievalResult, fusion string, weights FusionWeights) []*RetrievalResult {
	switch strings.ToLower(strings.TrimSpace(fusion)) {
	case FusionWeighted:
		return fuseWeighted(vector, keyword, weights)
	default:
		return fuseRRF(vector, keyword)
	}
}

// fuseRRF 以 RRF（sum(1/(k+rank))，k=60）融合两路结果，按融合分降序返回。
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
func fuseWeighted(vector, keyword []*RetrievalResult, weights FusionWeights) []*RetrievalResult {
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
func lessResult(a, b *RetrievalResult) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.DocumentID != b.DocumentID {
		return a.DocumentID < b.DocumentID
	}
	return a.ChunkIndex < b.ChunkIndex
}

// 内容去重的字符 n-gram 长度与 Jaccard 相似度阈值。阈值取高值，只拦「内容实质相同」的命中：
// 分片接缝处的重叠复读（默认 overlap=100 对 size=800）只占约一成，远低于阈值，故相邻块
// 不会被误删；而同一篇自重复文本被切成相邻块、同一份内容被重复索引等真重复会被拦下。
const (
	dedupeShingleSize      = 4
	dedupeSimilarityCutoff = 0.8
)

// dedupeSimilarResults 去掉融合排序结果中内容实质重复的命中：按排序自前向后逐条保留，
// 某条与任一已保留条目的字符 n-gram Jaccard 相似度不低于 dedupeSimilarityCutoff 时丢弃。
// 保留排序更靠前的一条——重排与融合都认定它更相关，分数更高且信息不劣于后者。输入不足
// 两条时原样返回。
func dedupeSimilarResults(hits []*RetrievalResult) []*RetrievalResult {
	return dedupeSimilarResultsWithCutoff(hits, dedupeSimilarityCutoff)
}

// dedupeSimilarResultsWithCutoff 是 dedupeSimilarResults 的阈值可变体：相似度阈值由调用方
// 指定（长期记忆检索去重用参数 memory.dedupeThreshold），其余行为与 dedupeSimilarResults
// 完全一致。既有 dedupeSimilarResults 以 dedupeSimilarityCutoff 委托本函数，行为不变。
func dedupeSimilarResultsWithCutoff(hits []*RetrievalResult, cutoff float64) []*RetrievalResult {
	if len(hits) < 2 {
		return hits
	}
	kept := make([]*RetrievalResult, 0, len(hits))
	keptShingles := make([]map[string]struct{}, 0, len(hits))
	for _, hit := range hits {
		shingles := contentShingles(hit.Content)
		duplicated := false
		for _, prev := range keptShingles {
			if jaccardSimilarityWithCutoff(shingles, prev, cutoff) >= cutoff {
				duplicated = true
				break
			}
		}
		if duplicated {
			continue
		}
		kept = append(kept, hit)
		keptShingles = append(keptShingles, shingles)
	}
	return kept
}

// contentShingles 把分块正文归一化后切成字符 n-gram 集合用作相似度指纹：归一化先折叠
// 全部空白，使标题路径拼接、段落分隔等排版差异不影响判定；正文短于 n-gram 长度时整体
// 作为一个指纹，保证「同样短小」也能互相判为重复。
func contentShingles(content string) map[string]struct{} {
	runes := []rune(strings.Join(strings.Fields(content), ""))
	if len(runes) == 0 {
		return nil
	}
	if len(runes) < dedupeShingleSize {
		return map[string]struct{}{string(runes): {}}
	}
	out := make(map[string]struct{}, len(runes)-dedupeShingleSize+1)
	for i := 0; i+dedupeShingleSize <= len(runes); i++ {
		out[string(runes[i:i+dedupeShingleSize])] = struct{}{}
	}
	return out
}

// jaccardSimilarity 计算两个 n-gram 集合的 Jaccard 相似度；任一集合为空时返回 0。
// 先做上限剪枝：Jaccard 至多为 min(|A|,|B|)/max(|A|,|B|)，达不到阈值的组合直接返回 0，
// 免去全量交集计算（正文长度悬殊的条目占绝大多数）。
func jaccardSimilarity(a, b map[string]struct{}) float64 {
	return jaccardSimilarityWithCutoff(a, b, dedupeSimilarityCutoff)
}

// jaccardSimilarityWithCutoff 是 jaccardSimilarity 的阈值可变体：上限剪枝所用的阈值由调用方
// 指定（长期记忆检索去重用参数 memory.dedupeThreshold），其余计算与 jaccardSimilarity 完全
// 一致。既有 jaccardSimilarity 以 dedupeSimilarityCutoff 委托本函数，行为不变。
func jaccardSimilarityWithCutoff(a, b map[string]struct{}, cutoff float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	small, large := len(a), len(b)
	if small > large {
		small, large = large, small
	}
	if float64(small)/float64(large) < cutoff {
		return 0
	}
	var overlap int
	for shingle := range a {
		if _, ok := b[shingle]; ok {
			overlap++
		}
	}
	return float64(overlap) / float64(len(a)+len(b)-overlap)
}

// selectTopK 取融合排序后的前 topK 条（重排不可用/失败时的降级结果）；topK 非正数时
// 返回全部。
func selectTopK(cands []*RetrievalResult, topK int) []*RetrievalResult {
	if topK <= 0 || len(cands) <= topK {
		return cands
	}
	return cands[:topK]
}

// toSchemaDocuments 将检索结果转为 eino schema.Document，MetaData 携带 documentID、
// title、chunkIndex、score，Content 为该块检索文本。
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

// TouchUsage 递增给定知识文档的 use_count 与 last_used_at；ids 为空直接返回 nil。
// 语义与长期记忆的 MemoryStore.TouchUsage 一致，供检索命中后回写使用统计。
func (r *Retriever) TouchUsage(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&ai.KnowledgeDocument{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"use_count":    gorm.Expr("use_count + 1"),
			"last_used_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// touchUsageAsync 以命中文档 ID 异步回写使用统计（每篇命中文档一次自增）：无 ID 时直接返回，
// 写入失败仅告警、绝不中断或影响检索结果。长期记忆与知识库采用同一策略：使用统计是增强信息，
// 不能因它失败而让生成/检索失败。
func (r *Retriever) touchUsageAsync(ctx context.Context, docs []*schema.Document) {
	ids := documentIDsOf(docs)
	if len(ids) == 0 {
		return
	}
	go func() {
		if err := r.TouchUsage(ctx, ids); err != nil {
			r.warnf(ctx, "retriever: touch document usage failed: %v", err)
		}
	}()
}

// documentIDsOf 从检索结果中提取去重后的文档 ID：MetaData 的 documentID 为字符串形式，
// 解析失败或缺失的条目直接跳过，保证非数字 ID 不会污染统计。
func documentIDsOf(docs []*schema.Document) []uint {
	ids := make([]uint, 0, len(docs))
	seen := make(map[uint]struct{}, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.MetaData == nil {
			continue
		}
		raw, ok := doc.MetaData["documentID"].(string)
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, dup := seen[uint(id)]; dup {
			continue
		}
		seen[uint(id)] = struct{}{}
		ids = append(ids, uint(id))
	}
	return ids
}

// warnf 在 logger 非空时输出告警。
func (r *Retriever) warnf(ctx context.Context, format string, args ...any) {
	if r.logger == nil {
		return
	}
	r.logger.Ctx(ctx).Warnf(format, args...)
}

// ToRetrievalResults 从 eino schema.Document 列表（MetaData 携带 documentID/title/
// chunkIndex/score，Content 为检索文本）转换出 RetrievalResult 列表，供 view 层序列化。
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
func rowsToResults(rows []struct {
	ai.KnowledgeChunk
	Title string
	Score float64
}) []*RetrievalResult {
	out := make([]*RetrievalResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, &RetrievalResult{
			DocumentID: strconv.FormatUint(uint64(row.DocumentID), 10),
			Title:      row.Title,
			ChunkIndex: row.ChunkIndex,
			Content:    strings.TrimSpace(string(row.HeadingPath.Value()) + "\n" + string(row.Content.Value())),
			Score:      row.Score,
		})
	}
	return out
}

// vectorLiteral 将 []float64 序列化为 pgvector 字面量（如 "[0.1,0.2,...]"）。
func vectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// toFloat32 将 []float64 转为 []float32（向量化输出转存储维度类型）。
func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}

// chunkOverviewRow 是分块明细的读取行：内嵌 KnowledgeChunk 以复用其字段的 crypto
// 序列化器标签（否则按原始密文字节扫入 string，得到的是密文而非明文），
// 查询仅 Select 明细所需列，避免载入 embedding 大字段。
type chunkOverviewRow struct {
	ai.KnowledgeChunk
}

// ChunkOverviewSummary 是文档分块构建摘要：读时从知识文档与其分块聚合派生，不持久化。
type ChunkOverviewSummary struct {
	// ChunkCount 分块数（实际读取到的分块行数）
	ChunkCount int
	// ContentChars 文档正文字符数
	ContentChars int
	// ChunkCharsTotal 各分块正文字符数之和（含 overlap 造成的重复）
	ChunkCharsTotal int
	// ChunkCharsAvg 分块正文字符数均值（整数除法；无分块时为 0）
	ChunkCharsAvg int
	// ChunkCharsMin 分块正文字符数最小值（无分块时为 0）
	ChunkCharsMin int
	// ChunkCharsMax 分块正文字符数最大值（无分块时为 0）
	ChunkCharsMax int
	// HeadingPathCount 去重后的标题路径数（空路径不计入）
	HeadingPathCount int
	// AtomicChunkCount 含代码块或表格的分块数（超长也不切分的原子块）
	AtomicChunkCount int
	// ChunkSize 生效参数 chunking.size（params 为 nil 时为 0）
	ChunkSize int
	// ChunkOverlap 生效参数 chunking.overlap（params 为 nil 时为 0）
	ChunkOverlap int
	// Indexed 是否已建立索引（存在分块且内容摘要非空）
	Indexed bool
	// Stale 索引摘要与当前正文不一致（索引落后于正文）
	Stale bool
	// IndexedAt 最近一次索引写入时间（文档更新时间）
	IndexedAt time.Time
	// Hash 文档内容的 HMAC-SHA256 十六进制摘要
	Hash string
}

// ChunkOverviewItem 是单个分块的构建明细。
type ChunkOverviewItem struct {
	// Index 分块在文档内的序号
	Index int
	// HeadingPath 所属标题路径（空表示该块不在任何标题之下）
	HeadingPath string
	// Content 分块正文
	Content string
	// Chars 正文字符数
	Chars int
	// Lines 正文行数
	Lines int
	// Kinds 识别出的结构单元类型（取值为 chunkUnitKind 枚举类的值，按声明顺序）
	Kinds []llm.ChunkUnitKind
}

// ChunkOverview 是文档分块构建明细的聚合结果。
type ChunkOverview struct {
	// Summary 文档级索引摘要
	Summary ChunkOverviewSummary
	// Chunks 逐块明细（按 chunk_index 升序；无分块时为空切片而非 nil）
	Chunks []ChunkOverviewItem
}

// BuildChunkOverview 读取文档分块并聚合构建摘要：按 chunk_index 升序读取分块（仅选取
// chunk_index/heading_path/content，不载入 embedding），逐块统计字符数/行数/结构单元类型，
// 并派生标题路径数、原子块数、索引状态与陈旧标记。本函数为包级函数（不挂 Knowledge 方法），
// 使 AI 能力未启用时也能读取明细（此时文档通常未索引，返回空分块 + Indexed=false）。
// params 为 nil 时切片长度与重叠长度记 0；db 或 doc 为 nil 时返回英文错误。
func BuildChunkOverview(ctx context.Context, db *database.DB, params *Params, doc *ai.KnowledgeDocument) (*ChunkOverview, error) {
	if db == nil {
		return nil, errors.New("knowledge: nil database")
	}
	if doc == nil {
		return nil, errors.New("knowledge: nil document")
	}
	var rows []chunkOverviewRow
	if err := db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("chunk_index, heading_path, content").
		Where("document_id = ? AND namespace_name = ?", doc.ID, doc.NamespaceName).
		Order("chunk_index ASC").
		Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	plaintext := string(doc.Content.Value())
	summary := ChunkOverviewSummary{
		ChunkCount:   len(rows),
		ContentChars: utf8.RuneCountInString(plaintext),
		IndexedAt:    doc.UpdatedAt,
		Hash:         doc.Hash.Digest(),
	}
	if params != nil {
		summary.ChunkSize = params.Int("chunking.size")
		summary.ChunkOverlap = params.Int("chunking.overlap")
	}
	chunks := make([]ChunkOverviewItem, 0, len(rows))
	paths := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		content := string(row.Content.Value())
		headingPath := string(row.HeadingPath.Value())
		chars := utf8.RuneCountInString(content)
		kinds := llm.DescribeChunk(content)
		for _, kind := range kinds {
			// 代码块与表格为原子单元（超长也不切分），单独计数供运维判断是否出现超长块
			if kind == llm.ChunkUnitCode || kind == llm.ChunkUnitTable {
				summary.AtomicChunkCount++
				break
			}
		}
		if headingPath != "" {
			paths[headingPath] = struct{}{}
		}
		summary.ChunkCharsTotal += chars
		if len(chunks) == 0 || chars < summary.ChunkCharsMin {
			summary.ChunkCharsMin = chars
		}
		if chars > summary.ChunkCharsMax {
			summary.ChunkCharsMax = chars
		}
		chunks = append(chunks, ChunkOverviewItem{
			Index:       row.ChunkIndex,
			HeadingPath: headingPath,
			Content:     content,
			Chars:       chars,
			Lines:       strings.Count(content, "\n") + 1,
			Kinds:       kinds,
		})
	}
	if len(chunks) > 0 {
		summary.ChunkCharsAvg = summary.ChunkCharsTotal / len(chunks)
	}
	summary.HeadingPathCount = len(paths)
	summary.Indexed = summary.ChunkCount > 0 && doc.Hash.Digest() != ""
	if summary.Indexed {
		// 索引失败时 IndexDocument 的整事务回滚，hash 会落后于当前正文：此即索引陈旧信号
		current, err := crypto.DigestForField(&ai.KnowledgeDocument{}, "hash", "json", []byte(plaintext))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		summary.Stale = current != doc.Hash.Digest()
	}
	return &ChunkOverview{Summary: summary, Chunks: chunks}, nil
}

// truncateText 将文本截断到前 max 个字符（超过时追加省略号），避免重排 prompt 超长。
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
