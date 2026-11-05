package agentkit

import (
	"strings"

	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// 提示词模板与随模板注入的固定段落集中声明于此：可编辑提示词正文、业务现场、会话摘要、
// 长期记忆与历史消息一律作为模板变量注入，代码侧的固定段落（能力发现指引、来源标注约定、
// 内联下一步约定）只在模板内按条件出现。模板由 eino 的 GoTemplate 渲染，调用处不再以字符串
// 拼接方式组装消息；注入的变量值不会被二次解析，故管理员编辑的提示词即使包含 JSON 花括号也
// 不会被误当作模板语法。
const (
	// agentSummarySectionTitle 是「会话滚动摘要」段的标题常量（记忆分节位于摘要之后、
	// 窗口历史之前）。
	agentSummarySectionTitle = "## 会话摘要"
	// agentMemorySectionTitle 是「长期记忆」段的标题常量：与摘要、业务现场、提问段标题同类，
	// 只承担提示词排版，代码不解析。
	agentMemorySectionTitle = "## 长期记忆"
	// agentBusinessSectionTitle 是用户消息中「当前业务现场」段的标题常量。
	agentBusinessSectionTitle = "## 当前业务现场"
	// agentQuestionSectionTitle 是用户消息中「用户提问」段的标题常量。
	agentQuestionSectionTitle = "## 用户提问"

	// agentCapabilityHint 是随系统提示一并注入的能力发现指引：能力知识位于 capability 分区，
	// 不再把整张能力地图常驻上下文。
	agentCapabilityHint = "判断平台是否具备某项能力、或用户意图对应哪个原生接口时，调用 knowledge_search 并传 format=capability；需要接口的精确入参、出参与字段含义时，传 format=native_api。"

	// agentCitationHint 是随系统提示一并注入的来源标注约定：knowledge_search 每条命中带
	// citation 来源编号，正文须以 [n] 标注。
	agentCitationHint = "引用知识库检索结果时，使用本轮命中条目的 citation 字段作为来源编号，在正文相应句或段末尾标注 [n]（多处来源写作 [1][2]）；编号只在本轮 knowledge_search 返回的命中中分配，不得自行编号、不得复用历史回答里的编号，也不得引用未检索到的来源。"

	// agentNextStepsHint 是随 general 助手系统提示一并注入的内联下一步输出约定：模型在最终回答
	// 正文之后、整段回答的最末尾输出一个 HTML 注释包裹的 JSON 段。
	agentNextStepsHint = `回答末尾的推荐下一步（机器可读，用户不可见）：
- 在最终回答正文之后另起一行，输出由 <!--next_steps 与 --> 包裹的 JSON，形如
<!--next_steps
{"steps":[{"title":"标题","reason":"推荐理由","text":"可直接发送的完整诉求"}]}
-->。
- 仅在最终回答（不再调用工具）时输出一次，必须位于整段回答的最末尾；正文其他位置不得出现该标记。
- 输出 1-3 条，按推荐程度从高到低排列；title 不超过 24 个字符，reason 不超过 64 个字符，text 自包含、具体、可执行。
- 没有有意义的后续动作时输出 {"steps":[]}；不要向用户解释该段。`
)

// 消息模板：固定段落按条件插入，可编辑提示词正文只作为数据注入。
const (
	// agentSystemPromptTemplate 是系统提示模板：可编辑的提示词正文在前，「当前技能」段仅在本轮选用
	// 技能时插入，能力发现指引与来源标注约定恒随其后，内联下一步约定仅在本次运行启用内联路径时追加，
	// 调用方附加的系统级约束（SystemHint）置于最末。
	agentSystemPromptTemplate = `{{.BasePrompt}}{{if .SkillName}}

## 当前技能
名称：{{.SkillName}}{{if .SkillDescription}}
说明：{{.SkillDescription}}{{end}}
请在本轮回答中严格遵循以下技能指引，且不得违反系统安全规范与既有约束：
{{.SkillContent}}{{end}}

{{.CapabilityHint}}
{{.CitationHint}}{{if .InlineNextSteps}}

{{.NextStepsHint}}{{end}}{{if .SystemHint}}

{{.SystemHint}}{{end}}`

	// agentSummaryPromptTemplate 是会话滚动摘要分节模板：段标题 + 摘要正文。
	agentSummaryPromptTemplate = `{{.Title}}
{{.Summary}}`

	// agentMemoryPromptTemplate 是长期记忆分节模板：段标题 + 逐条记忆；条目的标题可选。
	agentMemoryPromptTemplate = `{{.Title}}{{range .Items}}
- [{{.Kind}}] {{if .Title}}{{.Title}}: {{end}}{{.Content}}{{end}}`

	// agentUserPromptTemplate 是用户消息模板：业务现场段（可选）在前，提问段恒在末尾。
	agentUserPromptTemplate = `{{if .BusinessData}}{{.BusinessSectionTitle}}
{{.BusinessData}}

{{end}}{{.QuestionSectionTitle}}
{{.Query}}`

	// agentBusinessDataTemplate 把业务现场 JSON 包成代码块。
	agentBusinessDataTemplate = "```json\n{{.JSON}}\n```"

	// nextStepsPromptTemplate 是推荐下一步请求的用户消息模板：摘要、最近对话、本轮提问
	// 与回答各按条件成段。
	nextStepsPromptTemplate = `{{if .Summary}}## 会话摘要
{{.Summary}}

{{end}}{{if .Messages}}## 最近对话
{{range .Messages}}{{.Role}}：{{.Content}}
{{end}}
{{end}}{{if .Query}}## 本轮提问
{{.Query}}

{{end}}## 本轮回答
{{.Answer}}`

	// nextStepsRetryPromptTemplate 是推荐输出不合法时回传给模型的重试消息模板：附上本次原始
	// 校验错误与各字段上限，要求重新输出完整 JSON，而不是由代码截断或改写模型输出。
	nextStepsRetryPromptTemplate = `上一条推荐不合法：{{.Cause}}
请重新输出完整的 JSON（形如 {"steps":[{"title":"...","reason":"...","text":"..."}]}），不要解释：
- 每项必须包含非空 text 与 reason；
- title 不超过 {{.TitleMax}} 个字符，reason 不超过 {{.ReasonMax}} 个字符，text 不超过 {{.TextMax}} 个字符；
- 不要截断、省略或使用省略号代替内容。`
)

// promptKeyForAgentKind 按 Agent 类型选取可编辑的系统提示词键：general 取纯 Markdown 正文，
// 其余取结构化 JSON 信封版。
func promptKeyForAgentKind(kind modelsai.AgentKind) string {
	if kind == modelsai.AgentKindGeneral {
		return "agent.generalSystemPrompt"
	}
	return "agent.systemPrompt"
}

// inlineNextStepsEnabled 判断本次运行是否走「正文与下一步同一次调用」的内联路径：仅真实会话、
// general 助手且开关开启时启用；其余 Agent 类型仍走独立辅助调用。
func inlineNextStepsEnabled(params *config.Params, req Request) bool {
	if params == nil {
		return false
	}
	return req.SessionID != 0 && req.AgentKind == modelsai.AgentKindGeneral && params.Bool("agent.nextStepsEnabled")
}

// agentMemoryPromptItem 是记忆分节模板的单条数据。
type agentMemoryPromptItem struct {
	// Kind 记忆类别（decision / fact 等），原样写入条目前缀
	Kind string
	// Title 记忆标题（可空）；非空时在正文前渲染为 "title: "
	Title string
	// Content 记忆正文（已去首尾空白，非空）
	Content string
}

// memoryPromptItems 把长期记忆命中转换为分节模板数据：正文为空（去空白后）的命中跳过，
// 标题去空白后作为可选前缀。
func memoryPromptItems(memories []contract.MemoryHit) []agentMemoryPromptItem {
	items := make([]agentMemoryPromptItem, 0, len(memories))
	for _, hit := range memories {
		content := strings.TrimSpace(hit.Content)
		if content == "" {
			continue
		}
		items = append(items, agentMemoryPromptItem{
			Kind:    string(hit.Kind),
			Title:   strings.TrimSpace(hit.Title),
			Content: content,
		})
	}
	return items
}
