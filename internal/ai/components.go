// Package ai 是 AI 助手（RAG + Agent）的完整实现层：知识库索引与混合检索、会话持久化、
// 对话执行器（Agent）与质量评估，以及运行时装配（以数据库模型注册表 llm_models 与档位绑定
// llm_bindings 作为 LLM 接入配置的唯一事实来源，收敛「档位 → 模型端点配置」的映射、行为参数
// （agent_params，出厂值由 migrate fixture 注入）、可用组件的组装、变更热重载与端点探活），
// 供 HTTP 视图层与依赖状态探测共用。本包不读取任何 LLM 配置文件。
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
type Components struct {
	// Agent 对话执行器（AI 未启用时为 nil）
	Agent *Agent
	// Knowledge 知识库索引（AI 未启用时为 nil）
	Knowledge *Knowledge
	// Retriever 知识库混合检索器（AI 未启用时为 nil）
	Retriever *Retriever
	// Sessions 会话持久化（AI 未启用时为 nil）
	Sessions *SessionStore
	// Memory 会话记忆服务（滚动摘要 / 会话压缩 / 长期记忆检索；AI 未启用时为 nil）
	Memory *Memory
	// Params 行为参数解析器（始终非 nil）
	Params *llm.Params
	// Tools 已启用的工具（AI 未启用时为空），视图层 / Agent 由它取得工具清单与 schema；
	// AI 启用但无启用工具时为空切片（非 nil）
	Tools []tool.BaseTool
}

// Enabled 报告 AI 能力是否已完整装配（未启用或装配不完整时返回 false）。
func (c *Components) Enabled() bool {
	return c != nil && c.Agent != nil && c.Knowledge != nil && c.Sessions != nil && c.Memory != nil && c.Params != nil && c.Tools != nil
}
