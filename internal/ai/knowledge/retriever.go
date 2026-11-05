package knowledge

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// RetrieveOptions 是 Retriever.Retrieve 的实现专属调用参数，经 retriever.Option 注入。
type RetrieveOptions struct {
	// NamespaceName 限定检索的命名空间（多租户隔离）
	NamespaceName string
	// DocIDs 限定检索的文档 ID 列表，为空表示不限定
	DocIDs []uint
	// Format 限定检索的知识文档格式分区（如 native_api / schema / capability / generated），
	// 为空表示不限定分区。
	Format docconv.Format
	// TopK LLM 重排后保留的最终条数
	TopK int
	// Candidates 融合后送入重排的候选条数
	Candidates int
	// Fusion 融合策略：rrf 或 weighted
	Fusion string
	// Weights weighted 融合权重
	Weights retrieval.FusionWeights
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

// WithFormat 限定检索的知识文档格式分区；空值表示不限定。
func WithFormat(format docconv.Format) retriever.Option {
	return retriever.WrapImplSpecificOptFn(func(o *RetrieveOptions) {
		o.Format = format
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
func WithWeights(weights retrieval.FusionWeights) retriever.Option {
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
	embed    port.Embedder
	resolver port.ChatResolver // 可空：为空时跳过 LLM 重排与评估
	params   *config.Params    // 行为参数解析器（检索条数、融合、重排提示词与温度）
	logger   *logging.Logger   // 可空：为 nil 时跳过告警日志
}

// NewRetriever 构造 Retriever；db/embed/params 任一为空时返回英文错误。resolver 可为
// nil，此时检索跳过 LLM 重排（见 Retrieve 的说明）；logger 可为 nil（内部判空后再用）。
func NewRetriever(db *database.DB, embed port.Embedder, resolver port.ChatResolver, params *config.Params, logger *logging.Logger) (*Retriever, error) {
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
func (r *Retriever) resolveRerankChat() (port.ChatGenerator, error) {
	if r.resolver == nil {
		return nil, nil
	}
	return port.FlashChat(r.resolver)
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
		Weights: retrieval.FusionWeights{
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
	cands := retrieval.FuseRetrievalResults(vectorHits, keywordHits, o.Fusion, o.Weights)
	// 内容去重先于候选截断与重排：重复命中会占掉 candidates/topK 名额、把真正独立的候选
	// 挤出短名单，还会让重排在同一段文本的多个副本上重复判定（见 dedupeSimilarResults）
	cands = retrieval.DedupeSimilarResults(cands)
	cands = retrieval.SelectTopK(cands, o.Candidates)

	// 解析重排使用的对话客户端（调用点声明 flash 档位）：未注入 resolver 时跳过重排，
	// 直接返回融合排序前 TopK；档位未绑定时返回英文错误，不切换到其他档位。
	chat, err := r.resolveRerankChat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var final []*retrieval.RetrievalResult
	if chat == nil {
		final = retrieval.SelectTopK(cands, o.TopK)
	} else if ranked, rerankErr := r.rerank(ctx, chat, query, cands, o.TopK); rerankErr != nil {
		// 重排失败（调用错误或 JSON 解析失败）：降级为融合排序前 TopK。
		final = retrieval.SelectTopK(cands, o.TopK)
	} else {
		final = ranked
	}
	// small-to-big：命中分块所属文档整篇并入结果（见 expandDocuments）。展开发生在重排之后，
	// 使向量/关键词/重排都作用于语义聚焦的小分块，仅最终返回的内容提升为整篇。
	final = r.expandDocuments(ctx, final)
	docs := toSchemaDocuments(final)
	// 命中回写使用统计：仅统计最终返回的文档，异步执行且失败仅告警，不影响检索结果
	r.touchUsageAsync(ctx, docs)
	return docs, nil
}

// rerank 将候选块发给 chat 模型按相关度排序：模型须返回 JSON 数组（按相关度降序的
// 候选序号），解析失败或返回无效序号时返回错误（上层降级为融合排序）。chat 由调用方
// 经 resolveRerankChat 按声明的 flash 档位解析后传入。prompt 与系统指令均为中文，
// 要求模型仅输出 JSON。
func (r *Retriever) rerank(ctx context.Context, chat port.ChatGenerator, query string, cands []*retrieval.RetrievalResult, topK int) ([]*retrieval.RetrievalResult, error) {
	userMessage, err := buildRerankPrompt(ctx, query, cands, r.params.Int("retrieval.rerankSnippetLimit"))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(r.params.Text("retrieval.rerankSystemPrompt")),
		userMessage,
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
	ranked := make([]*retrieval.RetrievalResult, 0, len(indices))
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
	return retrieval.SelectTopK(ranked, topK), nil
}

// parseRerankIndices 解析模型返回的 JSON 序号数组；容忍 ```json 代码围栏包裹。
func parseRerankIndices(content string) ([]int, error) {
	content = validator.StripJSONFence(content)
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
	return nil, errors.Newf("retriever: cannot parse rerank response as an index array: %q", retrieval.TruncateText(content, 200))
}

// toSchemaDocuments 将检索结果转为 eino schema.Document，MetaData 携带 documentID、
// title、chunkIndex、score 与 complete（内容是否为整篇展开结果），Content 为该块检索文本。
func toSchemaDocuments(results []*retrieval.RetrievalResult) []*schema.Document {
	out := make([]*schema.Document, 0, len(results))
	for _, r := range results {
		out = append(out, &schema.Document{
			ID:      r.DocumentID,
			Content: r.Content,
			MetaData: map[string]any{
				"documentID":    r.DocumentID,
				"namespaceName": r.NamespaceName,
				"title":         r.Title,
				"chunkIndex":    r.ChunkIndex,
				"score":         r.Score,
				"complete":      r.Complete,
			},
		})
	}
	return out
}
