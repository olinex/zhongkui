// 本文件声明知识文档处理状态（KnowledgeDocStatus）、Agent 类型（AgentKind）、运行现场状态
// （AgentRunStatus）、运行生命周期阶段（AgentRunPhase）、会话交互状态（AgentInteractionState）、
// 交互子状态（AgentInteractionSubstate）、会话状态（SessionStatus）、消息角色（MessageRole）、
// 记忆类别（MemoryKind）、记忆状态（MemoryStatus）、工具来源（ToolSource）、技能来源（SkillSource）、
// 模型档位（Slot）与会话审核阀门（ReviewValve）的取值空间，并把这些取值空间注册为枚举类
// knowledgeDocStatus / agentKind / agentRunStatus / agentRunPhase / agentInteractionState /
// agentInteractionSubstate / memoryKind / memoryStatus / llmToolSource / agentSkillSource /
// llmSlot / reviewValve
// （审核阀门的取值常量与取值序列在 valve.go）。取值的唯一来源是类型化常量，取值序列就地复用为
// 条目来源，条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的 EnumClass 成员、i18n 键
// enum.<类名>.<key> 以及消费方引用一致。消息原生表示不再在本包声明：其取值空间与对话模型接口
// 形态同源，统一为 internal/pkg/llm 的 APIMode（枚举类 llmAPIMode）。档位枚举类 llmSlot 的取值
// 空间 Slot / AllSlots 与全部解析/校验 helper 一并声明于本文件（原先声明在 internal/models/core，
// 因 Slot 被本包的持久化模型 LlmBinding 用作 GORM 字段类型而随 llm.go 一并迁入本包）。包内文档
// 注释见 document.go。

package ai

import (
	// 标准库
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
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

// AgentKind 是 Agent 类型的标识：通用对话（general）、知识检索/知识库运营（knowledge）与
// Rego 规则生成（rego）。会话与长期记忆均按「命名空间 + AgentKind」归属与隔离，取值与
// agent_sessions.agent_kind、agent_memories.agent_kind 列的取值一致。
type AgentKind string

// Agent 类型常量。
const (
	// AgentKindGeneral 通用对话 Agent（不绑定知识库运营与 Rego 规则生成场景，纯 Markdown 回答）
	AgentKindGeneral AgentKind = "general"
	// AgentKindKnowledge 知识检索/知识库运营 Agent
	AgentKindKnowledge AgentKind = "knowledge"
	// AgentKindRego Rego 规则生成 Agent
	AgentKindRego AgentKind = "rego"
)

// AgentRunStatus 是 Agent 运行现场状态的标识：待恢复（pending）与已消费（consumed），取值与
// agent_runs.status 列的取值一致。运行现场在命中客户端工具时挂起并落库，处于 pending 的现场
// 可被恢复（现场不设过期，只要会话未删除即可恢复）。
type AgentRunStatus string

// 运行现场状态常量。
const (
	// AgentRunStatusPending 待恢复：运行已挂起、等待前端回传审批决定与执行结果
	AgentRunStatusPending AgentRunStatus = "pending"
	// AgentRunStatusConsumed 已消费：运行现场已被成功恢复，不可再次恢复
	AgentRunStatusConsumed AgentRunStatus = "consumed"
)

// AgentRunPhase 是 Agent 回合生命周期阶段的标识：运行中（running）、已完成（done）、失败
// （failed）、已取消（cancelled）与挂起待审批（suspended），取值与 agent_messages.phase
// 列的取值一致。与运行现场状态 AgentRunStatus 是不同概念：AgentRunStatus 描述 agent_runs 中
// 「待恢复现场」的消费状态，AgentRunPhase 描述一次回合的生命周期阶段。
type AgentRunPhase string

// 运行生命周期阶段常量。
const (
	// AgentRunPhaseRunning 运行中：图循环正在执行
	AgentRunPhaseRunning AgentRunPhase = "running"
	// AgentRunPhaseDone 已完成：图循环正常结束
	AgentRunPhaseDone AgentRunPhase = "done"
	// AgentRunPhaseFailed 失败：图循环以错误结束
	AgentRunPhaseFailed AgentRunPhase = "failed"
	// AgentRunPhaseCancelled 已取消：被显式停止或退订宽限到点自动取消
	AgentRunPhaseCancelled AgentRunPhase = "cancelled"
	// AgentRunPhaseSuspended 挂起待审批：命中客户端工具，等待前端回传审批与执行结果
	AgentRunPhaseSuspended AgentRunPhase = "suspended"
)

// AgentInteractionState 是会话交互状态（四主状态）的标识：等待中（waiting）、运行中（running）、
// 暂停（paused）与已锁定（locked）。交互状态由既有事实（在途活动、挂起现场、会话消息与轮次上限）
// 唯一推导，不落库为独立可变列；等待中/运行中/暂停各自携带子状态（见 AgentInteractionSubstate），
// 已锁定无子状态。
type AgentInteractionState string

// 会话交互状态常量。
const (
	// AgentInteractionStateWaiting 等待中：本轮尚未结束，且必须由用户输入才能继续（等授权或等回答）
	AgentInteractionStateWaiting AgentInteractionState = "waiting"
	// AgentInteractionStateRunning 运行中：会话内存在在途活动（生成或上下文压缩），后端正在产出
	AgentInteractionStateRunning AgentInteractionState = "running"
	// AgentInteractionStatePaused 暂停：会话静默待命，最近一次运行已有终局，可发起新提问或回滚类操作
	AgentInteractionStatePaused AgentInteractionState = "paused"
	// AgentInteractionStateLocked 已锁定：有效轮次已达上限，拒绝新的提问，仍可执行回滚类操作
	AgentInteractionStateLocked AgentInteractionState = "locked"
)

// AgentInteractionSubstate 是会话交互子状态的标识：等待中（approval/answer）、运行中
// （generating/thinking/tool_calling/compacting）与暂停（success/failed/aborted）三类主状态各自的
// 细分取值，与 AgentInteractionState 一同由交互状态机推导；已锁定无子状态（其子状态以空串表达）。
type AgentInteractionSubstate string

// 交互子状态常量。
const (
	// AgentInteractionSubstateApproval 等待授权：等待用户批准/拒绝客户端工具调用
	AgentInteractionSubstateApproval AgentInteractionSubstate = "approval"
	// AgentInteractionSubstateAnswer 等待回答：等待用户提交主动提问问卷
	AgentInteractionSubstateAnswer AgentInteractionSubstate = "answer"
	// AgentInteractionSubstateGenerating 正文生成中：模型正在流式产出正文
	AgentInteractionSubstateGenerating AgentInteractionSubstate = "generating"
	// AgentInteractionSubstateThinking 思考中：模型正在产出思考/推理内容
	AgentInteractionSubstateThinking AgentInteractionSubstate = "thinking"
	// AgentInteractionSubstateToolCalling 工具调用中：正在执行工具调用
	AgentInteractionSubstateToolCalling AgentInteractionSubstate = "tool_calling"
	// AgentInteractionSubstateCompacting 上下文压缩中：正在生成滚动摘要
	AgentInteractionSubstateCompacting AgentInteractionSubstate = "compacting"
	// AgentInteractionSubstateSuccess 成功：最近一次运行正常结束
	AgentInteractionSubstateSuccess AgentInteractionSubstate = "success"
	// AgentInteractionSubstateFailed 失败：最近一次运行以错误结束
	AgentInteractionSubstateFailed AgentInteractionSubstate = "failed"
	// AgentInteractionSubstateAborted 用户中止：最近一次运行被用户（或退订宽限）中止
	AgentInteractionSubstateAborted AgentInteractionSubstate = "aborted"
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

// SessionOrigin 是 Agent 会话来由的标识：普通助手会话（chat）与内部运行会话（periodic_report），
// 取值与 agent_sessions.origin 列的取值一致。内部运行会话只在创建它的业务入口（如周期性报告的
// 历史报告）中查看，不出现在 AI 助手会话列表。此处只声明类型与取值常量，不注册枚举类。
type SessionOrigin string

// 会话来由常量。
const (
	// SessionOriginChat 普通助手会话：用户在 AI 助手中创建并展示在会话列表中
	SessionOriginChat SessionOrigin = "chat"
	// SessionOriginPeriodicReport 周期报告运行会话：每次运行独立创建，不在助手会话列表中展示
	SessionOriginPeriodicReport SessionOrigin = "periodic_report"
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

// SkillSource 是技能来源的取值类型（agent_skills.source 与视图层 source 字段）。
type SkillSource string

// 技能来源常量。
const (
	// SkillSourceCustom 自定义技能：由用户在命名空间内维护，正文加密落库
	SkillSourceCustom SkillSource = "custom"
	// SkillSourceBuiltin 内置技能：由代码注册表声明、fixture 注入注册行，正文由代码按名提供
	SkillSourceBuiltin SkillSource = "builtin"
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
var agentKindValues = []AgentKind{AgentKindGeneral, AgentKindKnowledge, AgentKindRego}

// agentRunStatusValues 是 agentRunStatus 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var agentRunStatusValues = []AgentRunStatus{
	AgentRunStatusPending,
	AgentRunStatusConsumed,
}

// agentRunPhaseValues 是 agentRunPhase 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var agentRunPhaseValues = []AgentRunPhase{
	AgentRunPhaseRunning,
	AgentRunPhaseDone,
	AgentRunPhaseFailed,
	AgentRunPhaseCancelled,
	AgentRunPhaseSuspended,
}

// agentInteractionStateValues 是 agentInteractionState 枚举类的取值空间，声明顺序即枚举类条目
// 顺序与前端展示顺序。
var agentInteractionStateValues = []AgentInteractionState{
	AgentInteractionStateWaiting,
	AgentInteractionStateRunning,
	AgentInteractionStatePaused,
	AgentInteractionStateLocked,
}

// agentInteractionSubstateValues 是 agentInteractionSubstate 枚举类的取值空间，声明顺序即枚举类
// 条目顺序与前端展示顺序。
var agentInteractionSubstateValues = []AgentInteractionSubstate{
	AgentInteractionSubstateApproval,
	AgentInteractionSubstateAnswer,
	AgentInteractionSubstateGenerating,
	AgentInteractionSubstateThinking,
	AgentInteractionSubstateToolCalling,
	AgentInteractionSubstateCompacting,
	AgentInteractionSubstateSuccess,
	AgentInteractionSubstateFailed,
	AgentInteractionSubstateAborted,
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

// skillSourceValues 是 agentSkillSource 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var skillSourceValues = []SkillSource{SkillSourceCustom, SkillSourceBuiltin}

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

// AllAgentRunPhases 返回全部运行生命周期阶段的副本，声明顺序即枚举类条目顺序与前端展示顺序；
// 供 RunHub 与视图层复用，调用方修改返回值不影响该取值空间。
func AllAgentRunPhases() []AgentRunPhase {
	phases := make([]AgentRunPhase, len(agentRunPhaseValues))
	copy(phases, agentRunPhaseValues)
	return phases
}

// ParseAgentRunPhase 解析并校验运行生命周期阶段：不在取值空间内的取值（含空串）返回英文错误；
// 合法取值原样返回。阶段没有缺省值（写入路径显式使用 running），故空串视为非法。
func ParseAgentRunPhase(raw AgentRunPhase) (AgentRunPhase, error) {
	for _, known := range agentRunPhaseValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid agent run phase %q", raw)
}

// AllAgentInteractionStates 返回全部会话交互状态（主状态）的副本，声明顺序即枚举类条目顺序与
// 前端展示顺序；供交互状态机与视图层复用，调用方修改返回值不影响该取值空间。
func AllAgentInteractionStates() []AgentInteractionState {
	states := make([]AgentInteractionState, len(agentInteractionStateValues))
	copy(states, agentInteractionStateValues)
	return states
}

// ParseAgentInteractionState 解析并校验会话交互状态（主状态）：不在取值空间内的取值（含空串）
// 返回英文错误；合法取值原样返回。主状态没有缺省值（由推导全函数必定落一个），故空串视为非法。
func ParseAgentInteractionState(raw AgentInteractionState) (AgentInteractionState, error) {
	for _, known := range agentInteractionStateValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid agent interaction state %q", raw)
}

// AllAgentInteractionSubstates 返回全部会话交互子状态的副本，声明顺序即枚举类条目顺序与前端
// 展示顺序；供交互状态机与视图层复用，调用方修改返回值不影响该取值空间。
func AllAgentInteractionSubstates() []AgentInteractionSubstate {
	substates := make([]AgentInteractionSubstate, len(agentInteractionSubstateValues))
	copy(substates, agentInteractionSubstateValues)
	return substates
}

// ParseAgentInteractionSubstate 解析并校验会话交互子状态：不在取值空间内的取值（含空串）返回
// 英文错误；合法取值原样返回。合法子状态由本取值空间穷举，已锁定主状态无子状态（由调用方以空串
// 单独表达），故空串在此视为非法。
func ParseAgentInteractionSubstate(raw AgentInteractionSubstate) (AgentInteractionSubstate, error) {
	for _, known := range agentInteractionSubstateValues {
		if known == raw {
			return raw, nil
		}
	}
	return "", errors.Newf("invalid agent interaction substate %q", raw)
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

// init 注册枚举类 knowledgeDocStatus / agentKind / agentRunStatus / agentRunPhase /
// agentInteractionState / agentInteractionSubstate / memoryKind / memoryStatus / llmToolSource /
// agentSkillSource / llmSlot / reviewValve（取值空间固定，条目在注册时快照）。agentKind 复用
// agentKindValues，Agent 类型成员仍只在本文件一处声明；agentRunStatus 复用 agentRunStatusValues，
// 运行现场状态成员仍只在本文件一处声明；agentRunPhase 复用 agentRunPhaseValues，运行生命周期
// 阶段成员仍只在本文件一处声明；agentInteractionState / agentInteractionSubstate 复用本文件的
// 两份取值序列，会话交互状态与子状态成员仍只在本文件一处声明；memoryKind / memoryStatus 复用
// 本文件的两份取值序列，记忆类别/状态成员仍只在本文件一处声明；llmToolSource 复用
// toolSourceValues；llmSlot 的取值空间直接复用本文件的 AllSlots 清单，档位成员仍只在该清单
// 一处声明；reviewValve 复用 valve.go 的 reviewValveValues，审核阀门成员仍在 valve.go 一处声明。
func init() {
	enum.Register(enum.Type{Name: "knowledgeDocStatus", Entries: enum.EntriesOf(knowledgeDocStatusValues)})
	enum.Register(enum.Type{Name: "agentKind", Entries: enum.EntriesOf(agentKindValues)})
	enum.Register(enum.Type{Name: "agentRunStatus", Entries: enum.EntriesOf(agentRunStatusValues)})
	enum.Register(enum.Type{Name: "agentRunPhase", Entries: enum.EntriesOf(agentRunPhaseValues)})
	enum.Register(enum.Type{Name: "agentInteractionState", Entries: enum.EntriesOf(agentInteractionStateValues)})
	enum.Register(enum.Type{Name: "agentInteractionSubstate", Entries: enum.EntriesOf(agentInteractionSubstateValues)})
	enum.Register(enum.Type{Name: "memoryKind", Entries: enum.EntriesOf(memoryKindValues)})
	enum.Register(enum.Type{Name: "memoryStatus", Entries: enum.EntriesOf(memoryStatusValues)})
	enum.Register(enum.Type{Name: "llmToolSource", Entries: enum.EntriesOf(toolSourceValues)})
	enum.Register(enum.Type{Name: "agentSkillSource", Entries: enum.EntriesOf(skillSourceValues)})
	enum.Register(enum.Type{Name: "llmSlot", Entries: enum.EntriesOf(AllSlots)})
	enum.Register(enum.Type{Name: "reviewValve", Entries: enum.EntriesOf(reviewValveValues)})
}
