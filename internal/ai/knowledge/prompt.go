package knowledge

import (
	// 标准库
	"context"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 本文件集中声明检索与检索试运行链路中送入模型的提示词与用户消息模板：管理员可编辑的
// 提示词正文一律作为模板变量注入，代码侧的固定段落（语言约束、段标题、分块清单）只在
// 模板内按条件或循环出现。模板由 eino 的 prompt 组件按 schema.GoTemplate（底层 text/template）
// 渲染，调用处不再拼接字符串；注入的变量值不会被二次解析，故管理员编辑的提示词即使包含
// JSON 花括号也不会被误当作模板语法。

// suggestionsSystemPromptTemplate 是改进建议阶段系统提示词的组装模板：管理员可编辑的提示词
// 正文作为数据注入，语言约束只在请求带有语言标签时追加，避免管理员编辑提示词时丢失该约束。
const suggestionsSystemPromptTemplate = `{{.Base}}{{if .LanguageTag}}

请用 BCP-47 语言标签 "{{.LanguageTag}}" 指定的语言书写每一条 "detail"；JSON 键与类别标识保持英文。{{end}}`

// suggestionsPromptTemplate 是改进建议请求的用户消息模板：查询、生效参数、回答、指标、
// 分块清单与降级警告均由变量注入，回答缺失时在模板内切换到「生成失败」文案。
const suggestionsPromptTemplate = `查询：{{.Query}}

实际生效的检索参数：topK={{.TopK}}, candidates={{.Candidates}}, fusion={{.Fusion}}, weightVector={{printf "%g" .WeightVector}}, weightKeyword={{printf "%g" .WeightKeyword}}

{{if .HasAnswer}}回答：
{{.Answer}}{{else}}回答：（生成失败，未产出回答）{{end}}

指标：contextPrecision={{printf "%g" .ContextPrecision}}, faithfulness={{printf "%g" .Faithfulness}}, answerRelevance={{printf "%g" .AnswerRelevance}}, contextUtilization={{printf "%g" .ContextUtilization}}, relevantChunks={{.RelevantCount}}/{{.TotalChunks}}

上下文分块：
{{range .Chunks}}{{if .Judged}}[{{.Index}}] {{.Title}}（relevant={{.Relevant}}, score={{printf "%g" .Score}}）
{{.Snippet}}{{else}}[{{.Index}}] {{.Title}}（未评判）
{{.Snippet}}{{end}}

{{end}}{{if .Warnings}}告警：
{{range .Warnings}}- {{.}}
{{end}}{{end}}`

// evalChunkPromptTemplate 是回答生成与分块相关性判定共用的用户消息模板。
const evalChunkPromptTemplate = `查询：{{.Query}}

上下文分块：
{{range .Chunks}}[{{.Index}}] {{.Title}}
{{.Snippet}}

{{end}}`

// claimsPromptTemplate 是声明判定请求的用户消息模板。
const claimsPromptTemplate = `回答：
{{.Answer}}

上下文分块：
{{range .Chunks}}[{{.Index}}] {{.Title}}
{{.Snippet}}

{{end}}`

// reverseQuestionsPromptTemplate 是反推问题请求的用户消息模板。
const reverseQuestionsPromptTemplate = `回答：
{{.Answer}}`

// rerankPromptTemplate 是重排请求的用户消息模板。
const rerankPromptTemplate = `查询：{{.Query}}

候选分块：
{{range .Candidates}}[{{.Index}}] {{.Title}}
{{.Snippet}}

{{end}}`

// promptChunk 是提示词模板中的单条检索分块数据。
type promptChunk struct {
	// Index 分块在检索结果中的下标
	Index int
	// Title 分块标题
	Title string
	// Snippet 按 snippetLimit 截断后的正文
	Snippet string
	// Judged 是否已带相关性判定（仅改进建议模板使用）
	Judged bool
	// Relevant 相关性判定结果（Judged 为 true 时有效）
	Relevant bool
	// Score 相关性分数（Judged 为 true 时有效）
	Score float64
}

// suggestionsSystemPrompt 渲染改进建议阶段的系统提示词：以管理员可编辑的
// eval.suggestionsSystemPrompt 出厂值为主体，按模板追加以请求语言输出建议正文的指令。语言指令
// 由代码侧注入而不是写进提示词，避免管理员编辑提示词时丢失语言约束。
func suggestionsSystemPrompt(ctx context.Context, params *config.Params, languageTag string) (*schema.Message, error) {
	messages, err := schema.SystemMessage(suggestionsSystemPromptTemplate).Format(ctx, map[string]any{
		"Base":        params.Text("eval.suggestionsSystemPrompt"),
		"LanguageTag": strings.TrimSpace(languageTag),
	}, schema.GoTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return messages[0], nil
}

// buildSuggestionsPrompt 构造改进建议请求的用户消息：查询、生效参数、回答、量化指标、
// 带判定与未判定标记的分块清单与降级警告均经模板渲染。
func buildSuggestionsPrompt(ctx context.Context, query string, ev *DryRunEvaluation, results []retrieval.RetrievalResult, opts RetrieveOptions, snippetLimit int) (*schema.Message, error) {
	judged := make(map[int]ChunkJudgment, len(ev.ChunkJudgments))
	for _, judgment := range ev.ChunkJudgments {
		judged[judgment.Index] = judgment
	}
	chunks := make([]promptChunk, 0, len(results))
	for i, result := range results {
		chunk := promptChunk{Index: i, Title: result.Title, Snippet: evalSnippet(result, snippetLimit)}
		if judgment, ok := judged[i]; ok {
			chunk.Judged = true
			chunk.Relevant = judgment.Relevant
			chunk.Score = judgment.Score
		}
		chunks = append(chunks, chunk)
	}
	return renderUserPrompt(ctx, suggestionsPromptTemplate, map[string]any{
		"Query":              query,
		"TopK":               opts.TopK,
		"Candidates":         opts.Candidates,
		"Fusion":             opts.Fusion,
		"WeightVector":       opts.Weights.Vector,
		"WeightKeyword":      opts.Weights.Keyword,
		"Answer":             ev.Answer,
		"HasAnswer":          ev.Answer != "",
		"ContextPrecision":   ev.Metrics.ContextPrecision,
		"Faithfulness":       ev.Metrics.Faithfulness,
		"AnswerRelevance":    ev.Metrics.AnswerRelevance,
		"ContextUtilization": ev.Metrics.ContextUtilization,
		"RelevantCount":      ev.RelevantCount,
		"TotalChunks":        ev.TotalChunks,
		"Chunks":             chunks,
		"Warnings":           ev.Warnings,
	})
}

// buildEvalChunkPrompt 构造回答生成/相关性判定共用的用户消息：列出 query 与带序号的检索分块。
func buildEvalChunkPrompt(ctx context.Context, query string, results []retrieval.RetrievalResult, snippetLimit int) (*schema.Message, error) {
	return renderUserPrompt(ctx, evalChunkPromptTemplate, map[string]any{
		"Query":  query,
		"Chunks": promptChunks(results, snippetLimit),
	})
}

// buildClaimsPrompt 构造声明判定请求的用户消息：列出 answer 与带序号的检索分块。
func buildClaimsPrompt(ctx context.Context, answer string, results []retrieval.RetrievalResult, snippetLimit int) (*schema.Message, error) {
	return renderUserPrompt(ctx, claimsPromptTemplate, map[string]any{
		"Answer": answer,
		"Chunks": promptChunks(results, snippetLimit),
	})
}

// buildReverseQuestionsPrompt 构造反推问题请求的用户消息：只给出待反推的回答正文，问题条数
// 由 eval.reverseQuestionCount 在解析侧限制。
func buildReverseQuestionsPrompt(ctx context.Context, answer string) (*schema.Message, error) {
	return renderUserPrompt(ctx, reverseQuestionsPromptTemplate, map[string]any{"Answer": answer})
}

// buildRerankPrompt 构造重排请求的用户消息：列出 query 与带序号的候选块。
func buildRerankPrompt(ctx context.Context, query string, cands []*retrieval.RetrievalResult, snippetLimit int) (*schema.Message, error) {
	chunks := make([]promptChunk, 0, len(cands))
	for i, candidate := range cands {
		if candidate == nil {
			continue
		}
		chunks = append(chunks, promptChunk{
			Index:   i,
			Title:   candidate.Title,
			Snippet: retrieval.TruncateText(candidate.Content, snippetLimit),
		})
	}
	return renderUserPrompt(ctx, rerankPromptTemplate, map[string]any{
		"Query":      query,
		"Candidates": chunks,
	})
}

// renderUserPrompt 按 GoTemplate 渲染单条 user 消息；模板只引用受控变量，渲染失败时返回错误。
func renderUserPrompt(ctx context.Context, tpl string, vars map[string]any) (*schema.Message, error) {
	messages, err := schema.UserMessage(tpl).Format(ctx, vars, schema.GoTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return messages[0], nil
}

// evalSnippet 返回分块在评估提示词中的文本：整篇展开的命中（Complete）原样返回，其余按
// snippetLimit 截断。评估必须看到与实际检索一致的完整内容，否则会把「整篇已展开」误判为覆盖不足。
func evalSnippet(r retrieval.RetrievalResult, snippetLimit int) string {
	if r.Complete {
		return r.Content
	}
	return retrieval.TruncateText(r.Content, snippetLimit)
}

// promptChunks 把检索结果转换为提示词模板数据（保持原顺序与下标）。
func promptChunks(results []retrieval.RetrievalResult, snippetLimit int) []promptChunk {
	chunks := make([]promptChunk, 0, len(results))
	for i, result := range results {
		chunks = append(chunks, promptChunk{Index: i, Title: result.Title, Snippet: evalSnippet(result, snippetLimit)})
	}
	return chunks
}
