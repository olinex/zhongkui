package ai

import "zhongkui.com/zhongkui/internal/pkg/llm"

// Components 聚合 AI 助手（RAG + Agent）运行所需的全部组件：对话执行器（Agent）、
// 知识库索引（Knowledge）、会话持久化（SessionStore）与行为配置快照（LLMConfig）。
// 由 provider 依据 [llm] 配置一次性装配并经请求中间件注入视图层；AI 未启用时除
// Config 外全部为 nil，视图层以 Enabled 统一判断。
// Components aggregates all the components the AI assistant (RAG + Agent) needs at
// runtime: the dialogue executor (Agent), the knowledge-base index (Knowledge), the
// session persistence (SessionStore) and the behavior-config snapshot (LLMConfig). It
// is assembled once by the provider from the [llm] config and injected into the view
// layer via a request middleware; when AI is disabled everything except Config is nil
// and the view layer checks Enabled uniformly.
type Components struct {
	// Agent 对话执行器（AI 未启用时为 nil）
	// Agent the dialogue executor (nil when AI is disabled)
	Agent *Agent
	// Knowledge 知识库索引（AI 未启用时为 nil）
	// Knowledge the knowledge-base index (nil when AI is disabled)
	Knowledge *Knowledge
	// Sessions 会话持久化（AI 未启用时为 nil）
	// Sessions the session persistence (nil when AI is disabled)
	Sessions *SessionStore
	// Config 行为配置快照（始终非 nil）
	// Config the behavior-config snapshot (always non-nil)
	Config *llm.LLMConfig
}

// Enabled 报告 AI 能力是否已完整装配（未启用或装配不完整时返回 false）。
// Enabled reports whether the AI capability is fully assembled (false when disabled
// or partially assembled).
func (c *Components) Enabled() bool {
	return c != nil && c.Agent != nil && c.Knowledge != nil && c.Sessions != nil && c.Config != nil
}
