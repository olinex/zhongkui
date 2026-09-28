package llm

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// MemoryKind 是长期记忆类别的标识：事实（fact）、偏好（preference）、决策（decision）与产物
// （artifact），取值与 agent_memories.kind 列的取值一致。
// 类型与取值空间声明在本包，是因为它与 AgentKind 同属「跨层共享的 LLM 归属枚举」：模型层、
// 运行时层、工具层与视图层统一引用本包，避免各自就地声明字符串常量。
// MemoryKind is the identifier of a long-term memory kind: fact, preference, decision and artifact;
// the values match those of the agent_memories.kind column. The type and its value space live in
// this package because it belongs, together with AgentKind, to the LLM scope enums shared across
// layers: the model, runtime, tool and view layers all reference this package instead of declaring
// their own string constants.
type MemoryKind string

// 记忆类别常量。
// Memory kind constants.
const (
	// MemoryKindFact 事实类记忆：客观陈述与结论
	// MemoryKindFact a fact memory: objective statements and conclusions
	MemoryKindFact MemoryKind = "fact"
	// MemoryKindPreference 偏好类记忆：用户的倾向与约定
	// MemoryKindPreference a preference memory: user tendencies and conventions
	MemoryKindPreference MemoryKind = "preference"
	// MemoryKindDecision 决策类记忆：已确定的选择与取舍
	// MemoryKindDecision a decision memory: settled choices and trade-offs
	MemoryKindDecision MemoryKind = "decision"
	// MemoryKindArtifact 产物类记忆：可复用的片段与产出
	// MemoryKindArtifact an artifact memory: reusable snippets and outputs
	MemoryKindArtifact MemoryKind = "artifact"
)

// MemoryStatus 是长期记忆状态的标识：生效（active）与归档（archived），取值与
// agent_memories.status 列的取值一致（与会话状态 SessionStatus 是不同概念，勿混用）。
// MemoryStatus is the identifier of a long-term memory status: active and archived; the values
// match those of the agent_memories.status column (a different concept from SessionStatus, do not
// mix them).
type MemoryStatus string

// 记忆状态常量。
// Memory status constants.
const (
	// MemoryStatusActive 记忆状态：生效（参与检索与注入）
	// MemoryStatusActive the active memory status (retrievable and injectable)
	MemoryStatusActive MemoryStatus = "active"
	// MemoryStatusArchived 记忆状态：归档（不参与检索与注入）
	// MemoryStatusArchived the archived memory status (excluded from retrieval and injection)
	MemoryStatusArchived MemoryStatus = "archived"
)

// memoryKindValues 是 memoryKind 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
// memoryKindValues is the value space of the memoryKind enum class; the declaration order is the entry
// order of the enum class and the display order of the frontend select.
var memoryKindValues = []MemoryKind{
	MemoryKindFact,
	MemoryKindPreference,
	MemoryKindDecision,
	MemoryKindArtifact,
}

// memoryStatusValues 是 memoryStatus 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
// memoryStatusValues is the value space of the memoryStatus enum class; the declaration order is the
// entry order of the enum class and the display order of the frontend select.
var memoryStatusValues = []MemoryStatus{
	MemoryStatusActive,
	MemoryStatusArchived,
}

// AllMemoryKinds 返回全部记忆类别的副本，声明顺序即枚举类条目顺序与前端展示顺序；供记忆服务、
// 工具层与视图层复用，调用方修改返回值不影响本包取值空间。
// AllMemoryKinds returns a copy of every memory kind, with the declaration order as the enum entry
// order and the frontend display order; it is reused by the memory service, the tool layer and the
// view layer, and mutating the returned slice does not affect the value space of this package.
func AllMemoryKinds() []MemoryKind {
	kinds := make([]MemoryKind, len(memoryKindValues))
	copy(kinds, memoryKindValues)
	return kinds
}

// AllMemoryStatuses 返回全部记忆状态的副本，声明顺序即枚举类条目顺序与前端展示顺序；供记忆服务
// 与视图层复用，调用方修改返回值不影响本包取值空间。
// AllMemoryStatuses returns a copy of every memory status, with the declaration order as the enum entry
// order and the frontend display order; it is reused by the memory service and the view layer, and
// mutating the returned slice does not affect the value space of this package.
func AllMemoryStatuses() []MemoryStatus {
	statuses := make([]MemoryStatus, len(memoryStatusValues))
	copy(statuses, memoryStatusValues)
	return statuses
}

// ParseMemoryKind 解析并校验记忆类别：空串取缺省值 fact；非空但不在取值空间内的取值返回英文
// 错误；合法取值原样返回。取值清单只在本包声明，调用方不再各自校验并拼错误文案。
// ParseMemoryKind parses and validates a memory kind: an empty value yields the default fact; a
// non-empty value outside the value space yields an English error; a valid value is returned
// unchanged. The value catalog is declared only in this package, so callers no longer validate on
// their own nor assemble the error text themselves.
func ParseMemoryKind(raw MemoryKind) (MemoryKind, error) {
	if raw == "" {
		return MemoryKindFact, nil
	}
	for _, known := range memoryKindValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid memory kind %q", raw)
}

// ParseMemoryStatus 解析并校验记忆状态：空串或不在取值空间内的取值返回英文错误；合法取值原样
// 返回。状态没有缺省值（写入路径显式使用 active），故空串视为非法。
// ParseMemoryStatus parses and validates a memory status: an empty value or one outside the value
// space yields an English error; a valid value is returned unchanged. The status has no default (the
// write path uses active explicitly), so an empty value is treated as invalid.
func ParseMemoryStatus(raw MemoryStatus) (MemoryStatus, error) {
	for _, known := range memoryStatusValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid memory status %q", raw)
}
