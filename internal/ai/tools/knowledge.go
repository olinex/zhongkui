// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件实现内置工具 knowledge_search：Agent 在工具循环中按需对当前命名空间的知识库做一次语义
// 检索，可依据上一轮结果改写查询多次调用，从而由模型自行编排检索时机与查询语句。工具本身无状态：
// 检索器接口 KnowledgeSearcher 与命中条目 KnowledgeHit 声明在本包，由 internal/ai 适配实现
// （internal/ai 单向依赖本包，故接口与类型必须留在本包内以避免导入环）；命名空间与文档限定由适配
// 器持有，模型入参只有 query，无法越权改变租户范围。检索器与单条内容截断长度经 ctx 注入，取不到
// 时工具返回英文错误。工具元信息（internal/ai 的 ToolSpecs）与出厂行（002_fixtures.sql）由另外两处提供，
// 本文件只补实现并在 init 期登记。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// knowledgeRuntimeKey 是知识检索运行期检索器与截断长度在 ctx 中的键。用私有空结构体作键可避免与
// 其他包或本包其他上下文的键冲突（只有本包能构造该类型的值）。
type knowledgeRuntimeKey struct{}

// KnowledgeHit 是知识检索工具的一次命中条目。
type KnowledgeHit struct {
	// DocumentID 所属知识文档 ID（字符串形式）
	DocumentID string
	// Title 所属文档标题
	Title string
	// ChunkIndex 分块在文档内的序号
	ChunkIndex int
	// Content 该块的检索文本
	Content string
	// Score 检索相关度分数
	Score float64
	// Complete 内容是否为整篇文档展开结果（small-to-big）：为 true 时不再按 SnippetLimit 截断，
	// 由上层（internal/ai）在检索结果上标记。
	Complete bool
}

// KnowledgeSearcher 是知识检索器接口：由上层（internal/ai）的适配器实现，
// 内部调用 Retriever.Retrieve（命名空间与文档限定由适配器持有，模型入参不参与）。
type KnowledgeSearcher interface {
	// Search 按查询语句检索当前运行命名空间的知识库并返回命中条目。
	Search(ctx context.Context, query string) ([]KnowledgeHit, error)
}

// KnowledgeRuntime 是 knowledge_search 运行期所需的上下文快照。由 Agent 节点 4 在派发工具前经
// WithKnowledgeRuntime 注入 ctx。
type KnowledgeRuntime struct {
	// Searcher 检索器（nil 时工具返回英文错误）
	Searcher KnowledgeSearcher
	// SnippetLimit 单条命中的内容截断长度（rune 数），非正数表示不截断
	SnippetLimit int
}

// WithKnowledgeRuntime 把知识检索运行期检索器与截断长度写入 ctx，供 knowledge_search 在工具
// 循环中取回。
func WithKnowledgeRuntime(ctx context.Context, rt KnowledgeRuntime) context.Context {
	return context.WithValue(ctx, knowledgeRuntimeKey{}, rt)
}

// knowledgeRuntimeFrom 从 ctx 取回知识检索运行期信息；未注入时返回 ok=false。
func knowledgeRuntimeFrom(ctx context.Context) (KnowledgeRuntime, bool) {
	rt, ok := ctx.Value(knowledgeRuntimeKey{}).(KnowledgeRuntime)
	return rt, ok
}

// knowledgeSearchArgs 是 knowledge_search 的 JSON 入参形态：query 必填，命名空间、文档限定与
// topK 等均不暴露给模型。
type knowledgeSearchArgs struct {
	// Query 检索查询语句（必填，去空白后非空）
	Query string `json:"query"`
}

// knowledgeSearchItem 是返回给模型的单条命中：字段与 KnowledgeHit 一一对应，字段名采用小驼峰。
type knowledgeSearchItem struct {
	// DocumentID 所属知识文档 ID（字符串形式）
	DocumentID string `json:"documentID"`
	// Title 所属文档标题
	Title string `json:"title"`
	// ChunkIndex 分块在文档内的序号
	ChunkIndex int `json:"chunkIndex"`
	// Content 该块的检索文本（按 SnippetLimit 截断后）
	Content string `json:"content"`
	// Score 检索相关度分数
	Score float64 `json:"score"`
}

// knowledgeSearchResult 是 knowledge_search 返回给模型的 JSON 载荷：results 为命中列表（无命中时
// 为空数组而非 null），message 为可选的补充说明（如空结果提示）。
type knowledgeSearchResult struct {
	// Results 命中条目列表（无命中时为空数组）
	Results []knowledgeSearchItem `json:"results"`
	// Message 补充说明（可选，空结果时为提示文案）
	Message string `json:"message,omitempty"`
}

// knowledgeSearchTool 是内置工具 knowledge_search 的无状态实现：不持有依赖，检索器与截断长度均
// 取自 ctx。
type knowledgeSearchTool struct{}

// Info 返回 knowledge_search 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述，唯一键为 query（必填）；命名空间、文档限定与 topK 由运行上下文决定，
// 不出现在入参中。
func (t *knowledgeSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "knowledge_search",
		Desc: "在用户维护的知识库中检索与查询相关的文档片段（语义与关键词混合检索）。" +
			"当回答需要引用知识库中的既有资料时调用；可用不同的查询多次调用，逐步收敛到所需信息。" +
			"检索范围（命名空间与文档）由本次运行固定，调用无法改变。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "用于检索知识库的查询语句，必须非空。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 knowledge_search：解析 JSON 入参，校验 query 非空，从 ctx 取回检索器后执行
// 一次知识库检索，返回形如 {"results":[...]} 的 JSON 字符串。任何校验失败均返回英文错误且不触发
// 检索；检索错误以 errors.WithStack 包装后返回（英文）。无命中时返回空数组并附空结果说明，且
// 不返回错误。
func (t *knowledgeSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args knowledgeSearchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("knowledge_search: query must be non-empty")
	}
	rt, ok := knowledgeRuntimeFrom(ctx)
	if !ok || rt.Searcher == nil {
		return "", errors.New("knowledge_search: no knowledge runtime in context")
	}
	hits, err := rt.Searcher.Search(ctx, query)
	if err != nil {
		return "", errors.WithStack(err)
	}
	results := make([]knowledgeSearchItem, 0, len(hits))
	for _, hit := range hits {
		// 整篇展开的命中（Complete）已是完整规格，截断只会让模型看不到字段定义，故原样返回
		content := hit.Content
		if !hit.Complete {
			content = truncateSnippet(hit.Content, rt.SnippetLimit)
		}
		results = append(results, knowledgeSearchItem{
			DocumentID: hit.DocumentID,
			Title:      hit.Title,
			ChunkIndex: hit.ChunkIndex,
			Content:    content,
			Score:      hit.Score,
		})
	}
	result := knowledgeSearchResult{Results: results}
	if len(results) == 0 {
		result.Message = "未检索到匹配的知识，请改用其他查询词重试"
	}
	out, err := json.Marshal(result)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// truncateSnippet 按 rune 把 s 截断到前 max 个 rune，发生截断时追加 "..."；max 非正数或字符串本身
// 未超长时原样返回。语义与 internal/ai 的 truncateText 一致，但本包不能引用上层包，故自行实现。
func truncateSnippet(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// init 向内置工具实现注册表登记 knowledge_search；工具无状态，不消费 Deps。
func init() {
	Register("knowledge_search", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &knowledgeSearchTool{}, nil
	})
}
