// 本文件承载会话交互状态的单一权威推导与过渡守卫。交互状态不是新的可变状态列，而是既有事实
// （在途活动、挂起现场、会话消息、轮次上限、最近助手终局）的纯函数：StateOf 是全函数，任意
// 输入必落且仅落一个主状态，因此不会出现「未定义」或「同时命中两个状态」的空档。会话详情、
// 会话列表、在途运行快照与流式 state 事件一律复用这里的出参结构，前端不再自行拼状态。五个
// 守卫（CanSend / CanCancel / CanRollback / CanCompact / CanRerunUserInput）同样只依据 StateOf
// 的推导结果判定，避免各处重写「能不能操作」的判断逻辑。

package interaction

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 交互状态守卫的包级错误哨兵，供视图层 errors.Is 映射业务码（1305 / 1306），不依赖
// internal/manager/response：领域层只暴露语义，不关心 HTTP 状态码与文案。
var (
	// ErrInteractionLocked 会话已达轮次上限，拒绝新提问（视图层映射为业务码 1305）
	ErrInteractionLocked = errors.New("agent interaction locked")
	// ErrInteractionBusy 当前交互状态不允许该操作（视图层映射为业务码 1306）
	ErrInteractionBusy = errors.New("agent interaction busy")
)

// InteractionFacts 是状态推导的只读事实快照：由调用方（视图层 / 会话服务）按会话当前事实
// 填齐后传入，推导函数不查库、不持锁，故同一份事实必得同一份状态。它只承载「既有事实」，
// 不承载任何可变状态，也不承担缓存职责。
type InteractionFacts struct {
	// LiveRunning 会话内是否存在 running 的在途活动（生成或上下文压缩），由调用方查内存登记
	// （RunHub 的活动表）得出；true 时会话处于运行中，优先于挂起现场与锁定。
	LiveRunning bool
	// LiveSubstate 在途活动当前上报的子状态（generating/thinking/tool_calling/compacting），
	// 仅当 LiveRunning == true 时有意义；取值不在合法集内（含空串）时按 generating 兜底。
	LiveSubstate ai.AgentInteractionSubstate
	// PendingKind 挂起现场的种类：answer（问卷类 ask_user）或 approval（工具授权），空串表示
	// 无挂起现场。「存在挂起活动或 pending 运行现场」统一由该字段非空表达，由调用方查
	// agent_runs 的 pending 现场与内存挂起活动得出。
	PendingKind ai.AgentInteractionSubstate
	// UserTurns 有效轮次：会话内当前存在的 role=user 消息条数（被回滚删除的提问不计入），
	// 由调用方按 agent_messages 统计得出。
	UserTurns int
	// MaxUserTurns 轮次上限（来源为全局参数 session.maxUserTurns）；<=0 视为不设上限，永不锁定。
	MaxUserTurns int
	// LastAssistantPhase 会话内最近一条助手消息的终局阶段（done/failed/cancelled）；空串表示
	// 无助手消息或该消息尚未终结（例如仍在生成中），两者在推导时同归为「成功」。
	LastAssistantPhase ai.AgentRunPhase
}

// InteractionSnapshot 是状态推导的稳定出参：state / substate / locked / userTurns / maxUserTurns
// 供会话详情、会话列表、在途运行快照与流式 state 事件共用。故意不带 JSON 标签：序列化形态由
// 视图层决定，领域层只保证取值口径唯一。已锁定时 Substate 为空串（已锁定无子状态）。
type InteractionSnapshot struct {
	// State 主状态，必为四个取值之一（推导全函数保证无空档）
	State ai.AgentInteractionState
	// Substate 子状态；已锁定时为空串，其余主状态随事实映射
	Substate ai.AgentInteractionSubstate
	// Locked 恒等于 State == AgentInteractionStateLocked，保证「是否锁定」只有一处来源
	Locked bool
	// UserTurns 透传的有效轮次，供视图层展示与判定
	UserTurns int
	// MaxUserTurns 透传的轮次上限，供视图层展示
	MaxUserTurns int
}

// StateOf 是交互状态的唯一权威推导（全函数）：任一时刻必落且仅落一个主状态，输入仅为既有事实。
// 推导按严格优先级短路——在途活动 > 挂起现场 > 轮次上限 > 暂停，因此不会产生并列或歧义；
// 所有子状态都经归一出参，保证返回值永远是合法枚举值。
func StateOf(facts InteractionFacts) InteractionSnapshot {
	snapshot := InteractionSnapshot{UserTurns: facts.UserTurns, MaxUserTurns: facts.MaxUserTurns}
	switch {
	case facts.LiveRunning:
		snapshot.State = ai.AgentInteractionStateRunning
		snapshot.Substate = normalizeRunningSubstate(facts.LiveSubstate)
	case facts.PendingKind != "":
		snapshot.State = ai.AgentInteractionStateWaiting
		snapshot.Substate = normalizePendingSubstate(facts.PendingKind)
	case facts.MaxUserTurns > 0 && facts.UserTurns >= facts.MaxUserTurns:
		snapshot.State = ai.AgentInteractionStateLocked
		snapshot.Locked = true
	default:
		snapshot.State = ai.AgentInteractionStatePaused
		snapshot.Substate = pausedSubstateOf(facts.LastAssistantPhase)
	}
	return snapshot
}

// normalizeRunningSubstate 把在途活动上报的子状态归一到运行中的合法子状态集：不在
// {generating, thinking, tool_calling, compacting} 内（含空串与未知值）一律兜底为 generating，
// 使「活动存在」这一事实即使伴随残缺的阶段上报也仍是合法状态。
func normalizeRunningSubstate(substate ai.AgentInteractionSubstate) ai.AgentInteractionSubstate {
	switch substate {
	case ai.AgentInteractionSubstateGenerating,
		ai.AgentInteractionSubstateThinking,
		ai.AgentInteractionSubstateToolCalling,
		ai.AgentInteractionSubstateCompacting:
		return substate
	default:
		return ai.AgentInteractionSubstateGenerating
	}
}

// normalizePendingSubstate 把挂起现场种类归一到等待中的合法子状态集：不在 {answer, approval} 内
// （例如残缺或未知种类）一律兜底为 approval——把未知挂起保守地当作需人工授权的工具调用，
// 避免误当作问卷而错开输入面板。
func normalizePendingSubstate(kind ai.AgentInteractionSubstate) ai.AgentInteractionSubstate {
	switch kind {
	case ai.AgentInteractionSubstateAnswer, ai.AgentInteractionSubstateApproval:
		return kind
	default:
		return ai.AgentInteractionSubstateApproval
	}
}

// pausedSubstateOf 把最近助手消息的终局阶段映射为暂停子状态：done→成功、failed→失败、
// cancelled→用户中止；无助手消息、尚未终结（如仍在生成中）或未知阶段一律归为成功，与 spec
// 「空 → 成功」的口径一致（空会话即暂停/成功）。
func pausedSubstateOf(phase ai.AgentRunPhase) ai.AgentInteractionSubstate {
	switch phase {
	case ai.AgentRunPhaseFailed:
		return ai.AgentInteractionSubstateFailed
	case ai.AgentRunPhaseCancelled:
		return ai.AgentInteractionSubstateAborted
	default:
		return ai.AgentInteractionSubstateSuccess
	}
}

// CanSend 判定当前交互状态是否允许发送新提问：仅暂停允许；已锁定返回 ErrInteractionLocked
// （视图层映射 1305），运行中 / 等待中返回 ErrInteractionBusy（视图层映射 1306）。判定只依据
// StateOf 的结果，与前端输入闸门口径同源。
func CanSend(facts InteractionFacts) error {
	switch StateOf(facts).State {
	case ai.AgentInteractionStatePaused:
		return nil
	case ai.AgentInteractionStateLocked:
		return ErrInteractionLocked
	default:
		return ErrInteractionBusy
	}
}

// CanCancel 判定当前交互状态是否允许中止：仅运行中（任一子状态）与等待中允许，其余返回
// ErrInteractionBusy。需注意本守卫表达的是「UI 上停止按钮是否可用」的等价口径；取消端点在
// 运行中 / 等待中以外的状态下由端点自行保持幂等语义（例如重复取消不报错），本守卫不负责该语义。
func CanCancel(facts InteractionFacts) error {
	switch StateOf(facts).State {
	case ai.AgentInteractionStateRunning, ai.AgentInteractionStateWaiting:
		return nil
	default:
		return ErrInteractionBusy
	}
}

// CanRollback 判定当前交互状态是否允许「回滚到某条用户提问发送前」：仅运行中（无稳定截断对象）返回
// ErrInteractionBusy；暂停 / 已锁定 / 等待中均允许。等待中回滚是合法操作——它直接放弃当前待输入回合，
// 截断时会把该会话的 pending 现场置为已消费。已锁定非死路，回滚使其轮次降到上限以下即自动解锁。
func CanRollback(facts InteractionFacts) error {
	switch StateOf(facts).State {
	case ai.AgentInteractionStatePaused, ai.AgentInteractionStateLocked, ai.AgentInteractionStateWaiting:
		return nil
	default:
		return ErrInteractionBusy
	}
}

// CanCompact 判定当前交互状态是否允许发起手动上下文压缩：仅暂停与已锁定允许；压缩本身是一次在途活动，
// 运行中 / 等待中（含压缩中）返回 ErrInteractionBusy。等待中不压缩——此时应先作答 / 授权，或先回滚。
func CanCompact(facts InteractionFacts) error {
	switch StateOf(facts).State {
	case ai.AgentInteractionStatePaused, ai.AgentInteractionStateLocked:
		return nil
	default:
		return ErrInteractionBusy
	}
}

// CanRerunUserInput 判定当前交互状态是否允许「回退到某条助手待输入回合（问卷 / 授权）」：仅运行中返回
// ErrInteractionBusy；暂停 / 已锁定 / 等待中均允许。等待中回退重开同样是合法操作——回退会截断该锚点
// 之后的回合并作废更晚的 pending 现场，再以所选回合的现场重新等待用户输入。
func CanRerunUserInput(facts InteractionFacts) error {
	switch StateOf(facts).State {
	case ai.AgentInteractionStatePaused, ai.AgentInteractionStateLocked, ai.AgentInteractionStateWaiting:
		return nil
	default:
		return ErrInteractionBusy
	}
}
