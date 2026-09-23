package ai

import (
	// third-party
	"github.com/cloudwego/eino/components/retriever"
	"github.com/gin-gonic/gin"

	// internal
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/sse"
)

// DryRunIn 定义知识库检索试运行请求输入。
// DryRunIn defines the request input of the knowledge-base retrieval dry-run.
type DryRunIn struct {
	// Query 检索查询文本，必填
	// Query the retrieval query text, required
	Query string `json:"query" binding:"required"`
	// DocIDs 限定检索的文档 ID 列表，为空表示不限定
	// DocIDs the document IDs to restrict the retrieval to, empty means no restriction
	DocIDs []uint `json:"docIDs"`
	// TopK 重排后保留的最终条数，0 表示沿用默认
	// TopK the number of final hits kept after the rerank, 0 means the default
	TopK int `json:"topK"`
	// Candidates 融合后送入重排的候选条数，0 表示沿用默认
	// Candidates the number of candidates fed into the rerank after fusion, 0 means the
	// default
	Candidates int `json:"candidates"`
	// Fusion 融合策略（rrf/weighted），空表示沿用默认
	// Fusion the fusion strategy (rrf/weighted), empty means the default
	Fusion string `json:"fusion"`
	// Weights weighted 融合权重，全零表示沿用默认
	// Weights the weights of the weighted fusion, all-zero means the default
	Weights RetrievalWeightsIn `json:"weights"`
}

// RetrievalWeightsIn weighted 融合权重输入（视图层独立定义，不直接引用检索层权重类型）。
// RetrievalWeightsIn the input of the weighted-fusion weights (independently defined in the
// view layer rather than referencing the retrieval-layer weight type directly).
type RetrievalWeightsIn struct {
	// Vector 向量检索路权重
	// Vector the weight of the vector retrieval path
	Vector float64 `json:"vector"`
	// Keyword 关键词检索路权重
	// Keyword the weight of the keyword retrieval path
	Keyword float64 `json:"keyword"`
}

// DryRunOut 定义知识库检索试运行响应输出（与 ai.DryRunResult 同构）。
// DryRunOut defines the response output of the knowledge-base retrieval dry-run (the same
// shape as ai.DryRunResult).
type DryRunOut = ai.DryRunResult

// RetrievalWeightsOut weighted 融合权重（视图层独立定义，避免直接序列化检索层权重类型）。
// RetrievalWeightsOut the weighted-fusion weights (independently defined in the view layer
// rather than serializing the retrieval-layer weight type directly).
type RetrievalWeightsOut struct {
	// Vector 向量检索路权重
	// Vector the weight of the vector retrieval path
	Vector float64 `json:"vector"`
	// Keyword 关键词检索路权重
	// Keyword the weight of the keyword retrieval path
	Keyword float64 `json:"keyword"`
}

// RetrievalOptionsOut 定义检索试运行参数默认值响应。
// RetrievalOptionsOut defines the response of the default dry-run retrieval parameters.
type RetrievalOptionsOut struct {
	// TopK 重排后保留的最终条数
	// TopK the number of final hits kept after the rerank
	TopK int `json:"topK"`
	// Candidates 融合后送入重排的候选条数
	// Candidates the number of candidates fed into the rerank after fusion
	Candidates int `json:"candidates"`
	// Fusion 融合策略（rrf/weighted）
	// Fusion the fusion strategy (rrf/weighted)
	Fusion string `json:"fusion"`
	// Weights weighted 融合权重
	// Weights the weights of the weighted fusion
	Weights RetrievalWeightsOut `json:"weights"`
}

// RetrievalOptions 处理检索试运行参数默认值请求：从 AI 运行时参数解析器（当前 AI 组件
// 携带的参数解析器，即注册表默认值叠加 agent_params 覆盖值）读取 retrieval.* 键，
// 返回 topK/candidates/fusion/weights 默认值供前端对话框预填展示。
// RetrievalOptions handles the dry-run default parameters request: it reads the retrieval.*
// keys from the runtime parameter resolver (the resolver carried by the current AI
// components, i.e. the registry defaults layered with the agent_params overrides) and returns
// the topK/candidates/fusion/weights defaults for the frontend dialog to prefill.
func RetrievalOptions(c *gin.Context) {
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	params := comps.Params
	out := RetrievalOptionsOut{
		TopK:       params.Int("retrieval.topK"),
		Candidates: params.Int("retrieval.candidates"),
		Fusion:     params.String("retrieval.fusion"),
		Weights: RetrievalWeightsOut{
			Vector:  params.Float("retrieval.weightVector"),
			Keyword: params.Float("retrieval.weightKeyword"),
		},
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// RetrievalDryRunStream 处理知识库检索试运行请求（SSE，路由 /retrieval/dry-run）：
// 混合检索 → 基于上下文生成回答 → RAGAS 风格量化评估。所有校验在建 SSE writer 之前
// 完成，失败仍以普通 JSON 错误响应；writer 就绪后阶段事件以 stage 事件、模型增量以
// 带 stage 的 delta 事件实时推送，成功以 done 事件（完整结果含 meta）收尾，致命
// 错误以 error 事件结束。
// RetrievalDryRunStream handles the knowledge-base retrieval dry-run request (SSE, route
// /retrieval/dry-run): hybrid retrieval, context-based answer generation, then
// RAGAS-style quantitative evaluation. Every validation runs before the SSE writer is
// created, so a failure still yields a plain JSON error response; once the writer is
// ready, stage events are pushed as stage events, model deltas as stage-tagged delta
// events, a success ends with a done event (the full result including meta) and a fatal
// error ends with an error event.
func RetrievalDryRunStream(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in DryRunIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	opts := []retriever.Option{ai.WithNamespace(namespace)}
	if len(in.DocIDs) > 0 {
		opts = append(opts, ai.WithDocIDs(in.DocIDs))
	}
	if in.TopK > 0 {
		opts = append(opts, ai.WithRetrieveTopK(in.TopK))
	}
	if in.Candidates > 0 {
		opts = append(opts, ai.WithCandidates(in.Candidates))
	}
	if in.Fusion != "" {
		opts = append(opts, ai.WithFusion(in.Fusion))
	}
	if in.Weights.Vector != 0 || in.Weights.Keyword != 0 {
		opts = append(opts, ai.WithWeights(ai.FusionWeights{Vector: in.Weights.Vector, Keyword: in.Weights.Keyword}))
	}
	sseWriter := sse.NewSSEWriter(c.Writer)
	// 阶段事件映射：带增量输出的事件走 delta 事件（携带阶段标识），状态事件走 stage 事件
	// Stage mapping: events carrying a delta become delta events (tagged with the stage),
	// status events become stage events
	report := func(ev ai.StageEvent) error {
		if ev.Delta != "" {
			return sseWriter.SendStageDelta(string(ev.Stage), ev.Delta)
		}
		return sseWriter.SendStage(ev)
	}
	out, err := comps.Retriever.DryRun(c, in.Query, report, opts...)
	if err != nil {
		_ = sseWriter.SendError(err)
		return
	}
	_ = sseWriter.SendDone(out)
}
