// 本文件声明知识文档处理状态（KnowledgeDocStatus）、Agent 类型（AgentKind）、运行现场状态
// （AgentRunStatus）、会话状态（SessionStatus）、消息角色（MessageRole）、记忆类别（MemoryKind）、
// 记忆状态（MemoryStatus）、工具来源（ToolSource）与模型档位（Slot）的取值空间，并把这些取值空间
// 注册为枚举类 knowledgeDocStatus / agentKind / agentRunStatus / memoryKind / memoryStatus /
// llmToolSource / llmSlot。取值的唯一来源是本文件内的类型化常量，取值序列就地复用为条目来源，
// 条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的 EnumClass 成员、i18n 键
// enum.<类名>.<key> 以及消费方引用一致。档位枚举类 llmSlot 的取值空间 Slot / AllSlots 与全部
// 解析/校验 helper 一并声明于本文件（原先声明在 internal/models/core，因 Slot 被本包的持久化模型
// LlmBinding 用作 GORM 字段类型而随 llm.go 一并迁入本包）。包内文档注释见 document.go。
package ai

import (
	// 标准库
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// KnowledgeDocStatus 是知识文档的处理状态标识：文档从导入到可被检索的流转状态。
type KnowledgeDocStatus string

// 知识文档处理状态常量。
const (
	// KnowledgeDocStatusPending 待处理（已入库，尚未开始解析）
	KnowledgeDocStatusPending KnowledgeDocStatus = "pending"
	// KnowledgeDocStatusParsing 解析中
	KnowledgeDocStatusParsing KnowledgeDocStatus = "parsing"
	// KnowledgeDocStatusReady 就绪（解析完成，可被检索）
	KnowledgeDocStatusReady KnowledgeDocStatus = "ready"
	// KnowledgeDocStatusFailed 解析失败（失败原因见 ParseError）
	KnowledgeDocStatusFailed KnowledgeDocStatus = "failed"
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

// AgentRunStatus 是 Agent 运行现场状态的标识：待恢复（pending）、已消费（consumed）与
// 已过期（expired），取值与 agent_runs.status 列的取值一致。运行现场在命中客户端工具
// 时挂起并落库，仅处于 pending 且未过期的现场可被恢复。
type AgentRunStatus string

// 运行现场状态常量。
const (
	// AgentRunStatusPending 待恢复：运行已挂起、等待前端回传审批决定与执行结果
	AgentRunStatusPending AgentRunStatus = "pending"
	// AgentRunStatusConsumed 已消费：运行现场已被成功恢复，不可再次恢复
	AgentRunStatusConsumed AgentRunStatus = "consumed"
	// AgentRunStatusExpired 已过期：运行现场超过存活期限，不可恢复
	AgentRunStatusExpired AgentRunStatus = "expired"
)

// SessionStatus 是 Agent 会话状态的标识：活跃（active）与归档（archived），取值与
// agent_sessions.status 列的取值一致（与长期记忆状态 MemoryStatus 是不同概念，勿混用）。
// 此处只声明类型与取值常量，不注册枚举类：会话状态原本就不是前端下拉的枚举类。
type SessionStatus string

// 会话状态常量。
const (
	// SessionStatusActive 会话状态：活跃（默认）
	SessionStatusActive SessionStatus = "active"
	// SessionStatusArchived 会话状态：归档（不再展示/参与上下文）
	SessionStatusArchived SessionStatus = "archived"
)

// MessageRole 是 Agent 会话消息角色的标识：用户（user）、助手（assistant）与系统（system），
// 取值与 agent_messages.role 列的取值一致。此处只声明类型与取值常量，不注册枚举类：消息角色
// 原本就不是前端下拉的枚举类。
type MessageRole string

// 消息角色常量。
const (
	// MessageRoleUser 用户消息
	MessageRoleUser MessageRole = "user"
	// MessageRoleAssistant 助手消息
	MessageRoleAssistant MessageRole = "assistant"
	// MessageRoleSystem 系统消息
	MessageRoleSystem MessageRole = "system"
)

// MemoryKind 是长期记忆类别的标识：事实（fact）、偏好（preference）、决策（decision）与产物
// （artifact），取值与 agent_memories.kind 列的取值一致。
// 类型与取值空间声明在 internal/models/ai，是因为它与 AgentKind 同属「跨层共享的 AI 归属枚举」：
// 模型层、运行时层、工具层与视图层统一引用这里声明的取值，避免各自就地声明字符串常量。
type MemoryKind string

// 记忆类别常量。
const (
	// MemoryKindFact 事实类记忆：客观陈述与结论
	MemoryKindFact MemoryKind = "fact"
	// MemoryKindPreference 偏好类记忆：用户的倾向与约定
	MemoryKindPreference MemoryKind = "preference"
	// MemoryKindDecision 决策类记忆：已确定的选择与取舍
	MemoryKindDecision MemoryKind = "decision"
	// MemoryKindArtifact 产物类记忆：可复用的片段与产出
	MemoryKindArtifact MemoryKind = "artifact"
)

// MemoryStatus 是长期记忆状态的标识：生效（active）与归档（archived），取值与
// agent_memories.status 列的取值一致（与会话状态 SessionStatus 是不同概念，勿混用）。
type MemoryStatus string

// 记忆状态常量。
const (
	// MemoryStatusActive 记忆状态：生效（参与检索与注入）
	MemoryStatusActive MemoryStatus = "active"
	// MemoryStatusArchived 记忆状态：归档（不参与检索与注入）
	MemoryStatusArchived MemoryStatus = "archived"
)

// ToolSource 是工具来源的取值类型（llm_tools.source 与视图层 source 字段）。
type ToolSource string

// 工具来源常量。
const (
	// ToolSourceBuiltin 内置工具：由代码内实现注册表声明，生命周期由代码掌握
	ToolSourceBuiltin ToolSource = "builtin"
	// ToolSourceMCP MCP 工具：由所属 MCP 服务器同步而来的远端工具
	ToolSourceMCP ToolSource = "mcp"
	// ToolSourceClient 客户端工具：由前端在用户凭据下执行，后端只下发请求描述符并等待审批结果
	ToolSourceClient ToolSource = "client"
)

// Slot 是模型档位槽位标识：flash（轻量）、standard（标准）、advanced（增强）三档对话模型
// 与 embedding（向量化）一个固定槽位。与 llm_bindings.slot 列的取值一致。
type Slot string

// 档位常量。
const (
	// SlotFlash 轻量档：低时延、低成本的对话档位
	SlotFlash Slot = "flash"
	// SlotStandard 标准档：日常问答的默认对话档位
	SlotStandard Slot = "standard"
	// SlotAdvanced 增强档：复杂推理与长输出的对话档位
	SlotAdvanced Slot = "advanced"
	// SlotEmbedding 向量化档位：知识库索引与向量检索使用
	SlotEmbedding Slot = "embedding"
)

// AllSlots 是全部固定档位的清单，声明顺序即前端展示顺序：三档对话档位在前，向量化槽位在后。
// 档位枚举的成员集合只在此处声明，视图层与 ai 运行时一律引用本清单，不自行列举档位。
var AllSlots = []Slot{SlotFlash, SlotStandard, SlotAdvanced, SlotEmbedding}

// ParseSlot 解析并校验档位取值（忽略首尾空白；不区分大小写）：四个固定档位均为合法取值，
// 其余取值返回英文错误。档位清单与取值规范化只在本包声明，调用方不再各自校验并拼错误文案。
func ParseSlot(value Slot) (Slot, error) {
	slot := Slot(strings.ToLower(strings.TrimSpace(string(value))))
	for _, known := range AllSlots {
		if known == slot {
			return slot, nil
		}
	}
	return "", errors.Newf("invalid slot %q: expected one of %s", value, slotNames(AllSlots))
}

// chatSlots 是允许作为对话档位的槽位清单（embedding 不在其列），由 AllSlots 按档位所要求的
// 模型类型过滤得出：档位成员仍只声明在 AllSlots 一处，顺序与其一致。
var chatSlots = func() []Slot {
	slots := make([]Slot, 0, len(AllSlots))
	for _, slot := range AllSlots {
		if SlotKind(slot) == llm.ModelKindChat {
			slots = append(slots, slot)
		}
	}
	return slots
}()

// ChatSlotValues 把对话档位清单展开为字面量清单（embedding 不在其列），返回新切片，调用方
// 修改不影响内部档位清单。档位成员仍只声明在 AllSlots 一处，清单由此派生，新增档位无需同步
// 修改消费方（如参数注册表 agent.defaultSlot 的封闭取值清单）。
func ChatSlotValues() []string {
	values := make([]string, 0, len(chatSlots))
	for _, slot := range chatSlots {
		values = append(values, string(slot))
	}
	return values
}

// slotNames 把档位清单渲染为错误文案中的枚举串（如 "flash, standard, advanced"）：文案与
// 档位清单同源，新增档位时无需再同步修改任何错误消息。
func slotNames(slots []Slot) string {
	names := make([]string, len(slots))
	for i, slot := range slots {
		names[i] = string(slot)
	}
	return strings.Join(names, ", ")
}

// InvalidChatSlotError 返回「对话档位取值不合法」的英文错误，枚举文案与档位清单同源；供
// 视图层在 ParseChatSlot 返回 false 时直接使用，各处不再重复拼写档位枚举。
func InvalidChatSlotError(value Slot) error {
	return chatSlotError("invalid chat slot", value)
}

// chatSlotError 拼装「对话档位取值不合法」的英文错误：subject 是错误主语，用于区分取值
// 来源（显式指定 / 调用点声明 / 注册表默认），枚举文案取自 chatSlots。
func chatSlotError(subject string, value Slot) error {
	return errors.Newf("%s %q: expected one of %s", subject, value, slotNames(chatSlots))
}

// ParseChatSlot 解析并校验对话档位取值（忽略首尾空白；不区分大小写）：
// 仅 flash / standard / advanced 合法，embedding 或未知取值返回 ok=false。
func ParseChatSlot(value Slot) (Slot, bool) {
	slot := Slot(strings.ToLower(strings.TrimSpace(string(value))))
	for _, known := range chatSlots {
		if known == slot {
			return slot, true
		}
	}
	return "", false
}

// SlotKind 返回档位要求的模型类型：embedding 档位要求向量模型，其余对话档位要求对话模型。
func SlotKind(slot Slot) llm.ModelKind {
	if slot == SlotEmbedding {
		return llm.ModelKindEmbedding
	}
	return llm.ModelKindChat
}

// ResolveSlot 按「显式指定 → 调用点声明 → 注册表默认」的优先级解析本次调用使用的对话档位：
// requested 为调用方显式指定（如请求体字段），declared 为调用点声明的建议档位（如视图层
// 按业务场景推荐），fallback 为参数注册表 agent.defaultSlot 的默认档位。三者按序取第一个
// 非空值；任一非空取值不合法（embedding 或未知档位）或全部为空时返回英文错误。
func ResolveSlot(requested, declared, fallback Slot) (Slot, error) {
	if requested != "" {
		slot, ok := ParseChatSlot(requested)
		if !ok {
			return "", InvalidChatSlotError(requested)
		}
		return slot, nil
	}
	if declared != "" {
		slot, ok := ParseChatSlot(declared)
		if !ok {
			return "", chatSlotError("invalid declared chat slot", declared)
		}
		return slot, nil
	}
	slot, ok := ParseChatSlot(fallback)
	if !ok {
		return "", chatSlotError("invalid default chat slot", fallback)
	}
	return slot, nil
}

// knowledgeDocStatusValues 是 knowledgeDocStatus 枚举类的取值空间，声明顺序即枚举类条目顺序。
var knowledgeDocStatusValues = []KnowledgeDocStatus{
	KnowledgeDocStatusPending,
	KnowledgeDocStatusParsing,
	KnowledgeDocStatusReady,
	KnowledgeDocStatusFailed,
}

// agentKindValues 是 agentKind 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var agentKindValues = []AgentKind{AgentKindKnowledge, AgentKindRego}

// agentRunStatusValues 是 agentRunStatus 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var agentRunStatusValues = []AgentRunStatus{
	AgentRunStatusPending,
	AgentRunStatusConsumed,
	AgentRunStatusExpired,
}

// memoryKindValues 是 memoryKind 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var memoryKindValues = []MemoryKind{
	MemoryKindFact,
	MemoryKindPreference,
	MemoryKindDecision,
	MemoryKindArtifact,
}

// memoryStatusValues 是 memoryStatus 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var memoryStatusValues = []MemoryStatus{
	MemoryStatusActive,
	MemoryStatusArchived,
}

// toolSourceValues 是 llmToolSource 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var toolSourceValues = []ToolSource{ToolSourceBuiltin, ToolSourceMCP, ToolSourceClient}

// AllAgentKinds 返回全部 Agent 类型的副本，声明顺序即枚举类条目顺序与前端展示顺序；供展示与
// 校验遍历使用，调用方修改返回值不影响该取值空间。
func AllAgentKinds() []AgentKind {
	kinds := make([]AgentKind, len(agentKindValues))
	copy(kinds, agentKindValues)
	return kinds
}

// ParseAgentKind 解析并校验 Agent 类型取值：空串取缺省值 knowledge；非空但不在取值空间内的
// 取值返回英文错误；合法取值原样返回。取值清单只在 internal/models/ai 声明，调用方不再各自
// 校验并拼错误文案。
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

// AllAgentRunStatuses 返回全部运行现场状态的副本，声明顺序即枚举类条目顺序与前端展示顺序；
// 供运行现场服务与视图层复用，调用方修改返回值不影响该取值空间。
func AllAgentRunStatuses() []AgentRunStatus {
	statuses := make([]AgentRunStatus, len(agentRunStatusValues))
	copy(statuses, agentRunStatusValues)
	return statuses
}

// ParseAgentRunStatus 解析并校验运行现场状态：不在取值空间内的取值（含空串）返回英文错误；
// 合法取值原样返回。状态没有缺省值（写入路径显式使用 pending），故空串视为非法。
func ParseAgentRunStatus(raw AgentRunStatus) (AgentRunStatus, error) {
	for _, known := range agentRunStatusValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid agent run status %q", raw)
}

// AllMemoryKinds 返回全部记忆类别的副本，声明顺序即枚举类条目顺序与前端展示顺序；供记忆服务、
// 工具层与视图层复用，调用方修改返回值不影响该取值空间。
func AllMemoryKinds() []MemoryKind {
	kinds := make([]MemoryKind, len(memoryKindValues))
	copy(kinds, memoryKindValues)
	return kinds
}

// AllMemoryStatuses 返回全部记忆状态的副本，声明顺序即枚举类条目顺序与前端展示顺序；供记忆服务
// 与视图层复用，调用方修改返回值不影响该取值空间。
func AllMemoryStatuses() []MemoryStatus {
	statuses := make([]MemoryStatus, len(memoryStatusValues))
	copy(statuses, memoryStatusValues)
	return statuses
}

// ParseMemoryKind 解析并校验记忆类别：空串取缺省值 fact；非空但不在取值空间内的取值返回英文
// 错误；合法取值原样返回。取值清单只在 internal/models/ai 声明，调用方不再各自校验并拼错误文案。
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
func ParseMemoryStatus(raw MemoryStatus) (MemoryStatus, error) {
	for _, known := range memoryStatusValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid memory status %q", raw)
}

// init 注册枚举类 knowledgeDocStatus / agentKind / agentRunStatus / memoryKind / memoryStatus /
// llmToolSource / llmSlot（取值空间固定，条目在注册时快照）。agentKind 复用 agentKindValues，
// Agent 类型成员仍只在本文件一处声明；agentRunStatus 复用 agentRunStatusValues，运行现场状态成员
// 仍只在本文件一处声明；memoryKind / memoryStatus 复用本文件的两份取值序列，记忆类别/状态成员仍
// 只在本文件一处声明；llmToolSource 复用 toolSourceValues；llmSlot 的取值空间直接复用本文件的
// AllSlots 清单，档位成员仍只在该清单一处声明。
func init() {
	enum.Register(enum.Type{Name: "knowledgeDocStatus", Entries: enum.EntriesOf(knowledgeDocStatusValues)})
	enum.Register(enum.Type{Name: "agentKind", Entries: enum.EntriesOf(agentKindValues)})
	enum.Register(enum.Type{Name: "agentRunStatus", Entries: enum.EntriesOf(agentRunStatusValues)})
	enum.Register(enum.Type{Name: "memoryKind", Entries: enum.EntriesOf(memoryKindValues)})
	enum.Register(enum.Type{Name: "memoryStatus", Entries: enum.EntriesOf(memoryStatusValues)})
	enum.Register(enum.Type{Name: "llmToolSource", Entries: enum.EntriesOf(toolSourceValues)})
	enum.Register(enum.Type{Name: "llmSlot", Entries: enum.EntriesOf(AllSlots)})
}
