package llm

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// AgentKind 是 Agent 类型的标识：知识检索/知识库运营（knowledge）与 Rego 规则生成（rego）。
// 会话与长期记忆均按「命名空间 + AgentKind」归属与隔离，取值与 agent_sessions.agent_kind、
// agent_memories.agent_kind 列的取值一致。
// AgentKind is the identifier of an agent kind: knowledge retrieval / knowledge-base operation
// (knowledge) and Rego rule generation (rego). Both sessions and long-term memories are owned and
// isolated by "namespace + AgentKind", and the values match those of the agent_sessions.agent_kind
// and agent_memories.agent_kind columns.
type AgentKind string

// Agent 类型常量。
// Agent kind constants.
const (
	// AgentKindKnowledge 知识检索/知识库运营 Agent
	// AgentKindKnowledge the knowledge retrieval / knowledge-base operation agent
	AgentKindKnowledge AgentKind = "knowledge"
	// AgentKindRego Rego 规则生成 Agent
	// AgentKindRego the Rego rule generation agent
	AgentKindRego AgentKind = "rego"
)

// agentKindValues 是 agentKind 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
// agentKindValues is the value space of the agentKind enum class; the declaration order is the entry
// order of the enum class and the display order of the frontend select.
var agentKindValues = []AgentKind{AgentKindKnowledge, AgentKindRego}

// AllAgentKinds 返回全部 Agent 类型的副本，声明顺序即枚举类条目顺序与前端展示顺序；供展示与
// 校验遍历使用，调用方修改返回值不影响本包取值空间。
// AllAgentKinds returns a copy of every agent kind, with the declaration order as the enum entry
// order and the frontend display order; it serves display and validation iteration, and mutating the
// returned slice does not affect the value space of this package.
func AllAgentKinds() []AgentKind {
	kinds := make([]AgentKind, len(agentKindValues))
	copy(kinds, agentKindValues)
	return kinds
}

// ParseAgentKind 解析并校验 Agent 类型取值：空串取缺省值 knowledge；非空但不在取值空间内的
// 取值返回英文错误；合法取值原样返回。取值清单只在本包声明，调用方不再各自校验并拼错误文案。
// ParseAgentKind parses and validates an agent kind value: an empty value yields the default
// knowledge; a non-empty value outside the value space yields an English error; a valid value is
// returned unchanged. The value catalog is declared only in this package, so callers no longer
// validate on their own nor assemble the error text themselves.
func ParseAgentKind(raw AgentKind) (AgentKind, error) {
	if raw == "" {
		return AgentKindKnowledge, nil
	}
	for _, known := range agentKindValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid agent kind %q", raw)
}
