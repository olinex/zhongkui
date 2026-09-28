// Package ai 是 AI 助手（RAG + Agent）的完整实现层：知识库索引与混合检索、会话持久化、
// 对话执行器（Agent）与质量评估，以及运行时装配（以数据库模型注册表 llm_models 与档位绑定
// llm_bindings 作为 LLM 接入配置的唯一事实来源，收敛「档位 → 模型端点配置」的映射、行为参数
// （agent_params，出厂值由 migrate fixture 注入）、可用组件的组装、变更热重载与端点探活），
// 供 HTTP 视图层与依赖状态探测共用。本包不读取任何 LLM 配置文件。
//
// Package ai is the complete implementation layer of the AI assistant (RAG + Agent): the
// knowledge-base index and hybrid retrieval, the session persistence, the dialogue executor
// (Agent) and the quality evaluation, plus the runtime assembly (it takes the database model
// registry llm_models and the slot bindings llm_bindings as the single source of truth of the
// LLM access config and converges the "slot → model endpoint config" mapping, the behavior
// parameters (agent_params, factory values injected by the migrate fixtures), the assembly of
// the usable components, the hot reload of changes and the endpoint probing), shared by the
// HTTP view layer and the dependency status probes. This package reads no LLM config file.
package ai

import (
	"github.com/cloudwego/eino/components/tool"

	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// Components 聚合 AI 助手（RAG + Agent）运行所需的全部组件：对话执行器（Agent）、
// 知识库索引（Knowledge）、知识库检索器（Retriever）、会话持久化（SessionStore）、
// 行为参数解析器（Params）与已启用的工具清单（Tools）。由本包的运行时（Runtime）依据数据库
// 模型注册表装配，并经请求中间件注入视图层；AI 未启用时各执行组件为 nil，视图层以 Enabled
// 统一判断。
// Components aggregates all the components the AI assistant (RAG + Agent) needs at
// runtime: the dialogue executor (Agent), the knowledge-base index (Knowledge), the
// knowledge-base retriever (Retriever), the session persistence (SessionStore), the
// behavior-parameter resolver (Params) and the enabled tool list (Tools). It is assembled by the
// runtime (Runtime) of this package from the database model registry and injected into the view
// layer via a request middleware; when AI is disabled the execution components are nil and the
// view layer checks Enabled uniformly.
type Components struct {
	// Agent 对话执行器（AI 未启用时为 nil）
	// Agent the dialogue executor (nil when AI is disabled)
	Agent *Agent
	// Knowledge 知识库索引（AI 未启用时为 nil）
	// Knowledge the knowledge-base index (nil when AI is disabled)
	Knowledge *Knowledge
	// Retriever 知识库混合检索器（AI 未启用时为 nil）
	// Retriever the knowledge-base hybrid retriever (nil when AI is disabled)
	Retriever *Retriever
	// Sessions 会话持久化（AI 未启用时为 nil）
	// Sessions the session persistence (nil when AI is disabled)
	Sessions *SessionStore
	// Memory 会话记忆服务（滚动摘要 / 会话压缩 / 长期记忆检索；AI 未启用时为 nil）
	// Memory the session memory service (rolling summary / session compaction / long-term memory
	// retrieval; nil when AI is disabled)
	Memory *Memory
	// Params 行为参数解析器（始终非 nil）
	// Params the behavior-parameter resolver (always non-nil)
	Params *llm.Params
	// Tools 已启用的工具（AI 未启用时为空），视图层 / Agent 由它取得工具清单与 schema；
	// AI 启用但无启用工具时为空切片（非 nil）
	// Tools the enabled tools (empty when AI is disabled); the view layer and the Agent take the
	// tool list and schema from it. It is an empty (non-nil) slice when AI is enabled but no tool
	// is enabled
	Tools []tool.BaseTool
}

// Enabled 报告 AI 能力是否已完整装配（未启用或装配不完整时返回 false）。
// Enabled reports whether the AI capability is fully assembled (false when disabled
// or partially assembled).
func (c *Components) Enabled() bool {
	return c != nil && c.Agent != nil && c.Knowledge != nil && c.Sessions != nil && c.Memory != nil && c.Params != nil && c.Tools != nil
}
