// 本文件是「交互状态的单一权威推导」（internal/ai/interaction.go 的 StateOf）在视图层的**唯一事实
// 装配入口**：把会话当前的既有事实读齐（在途活动、挂起现场、有效轮次、轮次上限、最近助手终局），
// 交领域层的全函数推导出状态。后续 Task（会话详情 / 列表 / 在途快照出参、流式 state 事件、发送 / 回滚
// / 压缩 / 回退类守卫）一律复用 agentInteractionFacts 与 rejectIfInteractionBusy，不得各写一套读库
// 逻辑，否则状态出参与守卫判定会沿不同口径漂移。

package ai

import (
	// 标准库
	"net/http"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// agentInteractionFacts 按会话当前事实装配 ai.InteractionFacts（StateOf 的唯一输入），六个字段口径：
//   - LiveRunning / LiveSubstate：经 RunHub.LiveRun 取当前运行快照，**仅当取自内存且 phase=running** 时视作
//     真在途，子状态取运行上报值（空串 / 未知值由 StateOf 归一兜底为 generating）。
//   - PendingKind：经 SessionStore 读取该会话最近一条未过期的 pending 运行现场，问卷类 ask_user → answer，
//     其余 → approval。
//   - UserTurns：会话内当前存在的用户提问条数（SessionStore.CountUserTurns，被回滚删除的不计入）。
//   - MaxUserTurns：全局参数 session.maxUserTurns。
//   - LastAssistantPhase：会话内最近一条助手消息的终局阶段。
//
// 存活性兜底（spec「在途运行存活性收敛」）：登记为 running / suspended 的运行在本进程内存中已无对应活动
// 时即为孤儿（进程重启、goroutine 丢失，或挂起现场已过期 / 已作废）——读取即按已收敛处理，置该登记与会话
// 最近助手消息为 failed，使会话派生回落 `暂停/失败`，杜绝「永远转圈」。唯一的例外是锚点已置 cancelled
// （用户在 `等待中` 主动中止），此时按 `暂停/用户中止` 呈现，不覆盖为失败。
//
// 读库失败一律按「该事实缺省为零值」处理而不中断请求：状态推导是全函数，零值事实仍必落一个合法状态，
// 与真实状态最多存在短暂偏差，且会在下一次状态重算时纠正；这比在读取路径上抛出 500 更可用。
func agentInteractionFacts(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) ai.InteractionFacts {
	facts := ai.InteractionFacts{MaxUserTurns: comps.Params.Int("session.maxUserTurns")}
	if session == nil {
		return facts
	}
	ctx := c.Request.Context()

	// 在途活动：内存运行优先，无内存登记时 RunHub 回落数据库快照（不存在 / 已过期均按「无在途活动」）。
	var live *ai.LiveRunStatus
	if comps.Hub != nil {
		if st, err := comps.Hub.LiveRun(ctx, session.ID, session.UserID); err == nil && st != nil {
			live = st
		}
	}

	// 挂起现场：种类由 agent_runs 的 pending 现场判定（问卷类 / 工具授权）。
	var pendingKind modelsai.AgentInteractionSubstate
	if kind, err := comps.Sessions.PendingRunSubstate(ctx, session.ID, session.UserID); err == nil {
		pendingKind = kind
	}

	// 有效轮次与最近助手终局：锁定与暂停子状态均按此推导，不写任何状态列。
	if turns, err := comps.Sessions.CountUserTurns(ctx, session.ID, session.UserID); err == nil {
		facts.UserTurns = turns
	}
	if msg, err := comps.Sessions.LatestAssistantMessage(ctx, session.ID, session.UserID); err == nil {
		facts.LastAssistantPhase = msg.Phase
	}

	switch {
	case live != nil && live.Phase == modelsai.AgentRunPhaseRunning && live.InMemory:
		// 真在途：本进程内存中有对应活动；子状态取运行上报值（空串 / 未知值由 StateOf 归一兜底）。
		facts.LiveRunning = true
		facts.LiveSubstate = live.Substate
	case live != nil && (live.Phase == modelsai.AgentRunPhaseRunning || live.Phase == modelsai.AgentRunPhaseSuspended):
		switch {
		case pendingKind != "":
			// 挂起且现场仍有效：等待中（问卷 / 授权）。
			facts.PendingKind = pendingKind
		case facts.LastAssistantPhase == modelsai.AgentRunPhaseCancelled:
			// 用户已在等待中主动中止：现场已作废、锚点已置 cancelled，无在途活动。
		default:
			// 孤儿运行：内存无对应活动且无有效挂起现场，读取即按已收敛处理（写回 failed 使后续读取一致）。
			if err := comps.Sessions.ConvergeOrphanLiveRun(ctx, live.RunID, session.ID, session.UserID); err == nil {
				facts.LastAssistantPhase = modelsai.AgentRunPhaseFailed
			}
		}
	}
	return facts
}

// rejectIfInteractionBusy 依据发送守卫 ai.CanSend 判定当前交互状态是否允许发起新提问，被拒时以本地化
// 业务错误中断请求并返回 true（调用方据此提前 return，绝不启动运行）。
func rejectIfInteractionBusy(c *gin.Context, facts ai.InteractionFacts) bool {
	return rejectGuardError(c, ai.CanSend(facts))
}

// interactionSnapshot 是会话交互状态推导在视图层的唯一调用点：装配事实（agentInteractionFacts）后交
// 领域层全函数 StateOf 得到快照。会话详情 / 列表 / 在途运行的出参一律复用它，保证各出参与守卫判定
// 沿同一口径推导，不各自读库拼状态。session 为空时（如新建 / 分支场景尚无需事实）得到暂停态快照。
func interactionSnapshot(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) ai.InteractionSnapshot {
	return ai.StateOf(agentInteractionFacts(c, comps, session))
}

// broadcastInteractionState 在事实变更操作完成后重算交互状态并向该会话的内存在途运行广播一条 state
// 事件：无在途运行时 Hub 侧为 no-op（暂停 / 已锁定 / 等待中后端此时无生成 goroutine），此时状态随调用
// 方 HTTP 出参下发。广播为尽力而为，失败不影响调用方返回（出参已是权威状态）。
func broadcastInteractionState(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) {
	if comps == nil || comps.Hub == nil || session == nil {
		return
	}
	snap := interactionSnapshot(c, comps, session)
	_ = comps.Hub.BroadcastInteractionState(c.Request.Context(), session.ID, session.UserID, ai.InteractionStateEvent{
		State:    snap.State,
		Substate: snap.Substate,
	})
}

// rejectGuardError 把交互状态守卫（CanSend / CanRollback / CanCompact / CanRerunUserInput 等）的拒绝
// 哨兵映射为本地化业务错误并中断请求，返回 true 表示已中断：
//   - 已锁定（ErrInteractionLocked）→ 400 + 业务码 1124「会话已达轮次上限，请回滚后再试」；
//   - 运行中 / 等待中（ErrInteractionBusy）→ 409 + 业务码 1125「当前会话状态不允许该操作」。
//
// 领域层只暴露语义哨兵，HTTP 状态码与业务码到文案的映射收敛在本函数一处：各类操作（发送 / 回滚 /
// 压缩 / 回退）的守卫判定各自不同，但**拒绝后的错误表达完全相同**，故映射只需一处，避免各端点各写
// 一套状态码与业务码。
func rejectGuardError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ai.ErrInteractionLocked):
		response.CreateCodeError(http.StatusBadRequest, response.CodeAgentSessionLocked, nil).AbortWithGinH(c)
	default:
		response.CreateCodeError(http.StatusConflict, response.CodeAgentInteractionBusy, nil).AbortWithGinH(c)
	}
	return true
}
