package llm

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// AgentKind 是 Agent 类型的标识：知识检索/知识库运营（knowledge）与 Rego 规则生成（rego）。
// 会话与长期记忆均按「命名空间 + AgentKind」归属与隔离，取值与 agent_sessions.agent_kind、
// agent_memories.agent_kind 列的取值一致。
type AgentKind string

// Agent 类型常量。
const (
	// AgentKindKnowledge 知识检索/知识库运营 Agent
	AgentKindKnowledge AgentKind = "knowledge"
	// AgentKindRego Rego 规则生成 Agent
	AgentKindRego AgentKind = "rego"
)

// agentKindValues 是 agentKind 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var agentKindValues = []AgentKind{AgentKindKnowledge, AgentKindRego}

// AllAgentKinds 返回全部 Agent 类型的副本，声明顺序即枚举类条目顺序与前端展示顺序；供展示与
// 校验遍历使用，调用方修改返回值不影响本包取值空间。
func AllAgentKinds() []AgentKind {
	kinds := make([]AgentKind, len(agentKindValues))
	copy(kinds, agentKindValues)
	return kinds
}

// ParseAgentKind 解析并校验 Agent 类型取值：空串取缺省值 knowledge；非空但不在取值空间内的
// 取值返回英文错误；合法取值原样返回。取值清单只在本包声明，调用方不再各自校验并拼错误文案。
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
