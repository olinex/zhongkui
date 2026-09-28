package llm

// SessionStatus 是 Agent 会话状态的标识：活跃（active）与归档（archived），取值与
// agent_sessions.status 列的取值一致（与长期记忆状态 MemoryStatus 是不同概念，勿混用）。
// 此处只声明类型与取值常量，不注册枚举类：会话状态原本就不是前端下拉的枚举类。
// SessionStatus is the identifier of an agent session status: active and archived; the values match
// those of the agent_sessions.status column (a different concept from the long-term memory status
// MemoryStatus, do not mix them). Only the type and its constants are declared here, with no enum
// class registration: the session status was never a frontend select enum class.
type SessionStatus string

// 会话状态常量。
// Session status constants.
const (
	// SessionStatusActive 会话状态：活跃（默认）
	// SessionStatusActive the active session status (default)
	SessionStatusActive SessionStatus = "active"
	// SessionStatusArchived 会话状态：归档（不再展示/参与上下文）
	// SessionStatusArchived the archived session status (hidden from lists/context)
	SessionStatusArchived SessionStatus = "archived"
)

// MessageRole 是 Agent 会话消息角色的标识：用户（user）、助手（assistant）与系统（system），
// 取值与 agent_messages.role 列的取值一致。此处只声明类型与取值常量，不注册枚举类：消息角色
// 原本就不是前端下拉的枚举类。
// MessageRole is the identifier of an agent session message role: user, assistant and system; the
// values match those of the agent_messages.role column. Only the type and its constants are declared
// here, with no enum class registration: the message role was never a frontend select enum class.
type MessageRole string

// 消息角色常量。
// Message role constants.
const (
	// MessageRoleUser 用户消息
	// MessageRoleUser the user message
	MessageRoleUser MessageRole = "user"
	// MessageRoleAssistant 助手消息
	// MessageRoleAssistant the assistant message
	MessageRoleAssistant MessageRole = "assistant"
	// MessageRoleSystem 系统消息
	// MessageRoleSystem the system message
	MessageRoleSystem MessageRole = "system"
)
