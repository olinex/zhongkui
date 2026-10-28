// 本文件是「会话交互状态」在视图层的**唯一事实装配入口**：把 turns.Manager.State 作为唯一在途/挂起
// 事实来源，辅以会话有效轮次上限与挂起交互种类，交领域层的全函数 StateOf 推导出状态。会话详情 /
// 列表 / 回合状态快照以及发送 / 回滚 / 压缩类守卫一律复用这里的入口，不得各写一套读库逻辑，否则
// 状态出参与守卫判定会沿不同口径漂移。

package ai

import (
	// 标准库
	"net/http"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	interaction "zhongkuisec.com/zhongkui/internal/ai/interaction"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// agentTurnState 读取某会话的回合生命周期快照（turns.Manager.State）。组件未装配、会话为空或读库
// 失败时返回 false；调用方据此按「无在途、无挂起」的零值事实推导状态，绝不让状态读取失败中断请求。
func agentTurnState(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) (turns.State, bool) {
	if comps == nil || comps.Turns == nil || session == nil {
		return turns.State{}, false
	}
	state, err := comps.Turns.State(c.Request.Context(), session.ID, session.UserID, session.NamespaceName)
	if err != nil {
		return turns.State{}, false
	}
	return state, true
}

// pendingInteractionKind 判定挂起现场的种类：会话内存在 pending 问卷交互时为 answer，其余（含无法
// 读取）一律按 approval 保守处理，避免把需授权的调用误渲染成问卷。
func pendingInteractionKind(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) modelsai.AgentInteractionSubstate {
	if comps == nil || comps.Sessions == nil || session == nil {
		return modelsai.AgentInteractionSubstateApproval
	}
	items, err := comps.Sessions.ListInteractions(c.Request.Context(), session.ID, session.UserID)
	if err != nil {
		return modelsai.AgentInteractionSubstateApproval
	}
	for _, item := range items {
		if item == nil || item.Status != modelsai.InteractionStatusPending {
			continue
		}
		if item.Kind == modelsai.InteractionKindQuestionnaire {
			return modelsai.AgentInteractionSubstateAnswer
		}
	}
	return modelsai.AgentInteractionSubstateApproval
}

// agentInteractionFacts 按会话当前事实装配 interaction.InteractionFacts（StateOf 的唯一输入）：
//   - LiveRunning：turns.Manager.State.Running（本进程存在执行中的回合）。
//   - PendingKind：State.Interrupted 为 true 时按 agent_interactions 的 pending 种类给出 answer / approval。
//   - UserTurns：turns.Manager.State.UserTurns（会话内当前存在的用户提问条数）。
//   - MaxUserTurns：全局参数 session.maxUserTurns。
//   - LastAssistantPhase：State.LastErr 非空时视为 failed，否则视为成功（空串）。
//
// 读库失败一律按「该事实缺省为零值」处理而不中断请求：状态推导是全函数，零值事实仍必落一个合法状态。
func agentInteractionFacts(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) interaction.InteractionFacts {
	if comps == nil {
		return interaction.InteractionFacts{}
	}
	maxUserTurns := comps.Params.Int("session.maxUserTurns")
	if session == nil {
		return interaction.InteractionFacts{MaxUserTurns: maxUserTurns}
	}
	state, ok := agentTurnState(c, comps, session)
	if !ok {
		return interaction.InteractionFacts{MaxUserTurns: maxUserTurns}
	}
	var pending modelsai.AgentInteractionSubstate
	if state.Interrupted {
		pending = pendingInteractionKind(c, comps, session)
	}
	return turnStateFacts(state, maxUserTurns, pending)
}

// turnStateFacts 把 turns.Manager.State 映射为交互事实：Running → LiveRunning，Interrupted 且给出
// PendingKind → 等待中，LastErr 非空 → 最近回合失败，其余（含空串）按成功处理。纯函数，便于按状态
// 口径直接断言。
func turnStateFacts(state turns.State, maxUserTurns int, pending modelsai.AgentInteractionSubstate) interaction.InteractionFacts {
	facts := interaction.InteractionFacts{
		LiveRunning:  state.Running,
		UserTurns:    state.UserTurns,
		MaxUserTurns: maxUserTurns,
	}
	if state.Interrupted {
		facts.PendingKind = pending
	}
	if state.LastErr != "" {
		facts.LastAssistantPhase = modelsai.AgentRunPhaseFailed
	}
	return facts
}

// interactionSnapshot 是会话交互状态推导在视图层的唯一调用点：装配事实后交领域层全函数 StateOf 得到
// 快照。会话详情 / 列表 / 回合快照与各守卫一律复用它，保证同一口径。session 为空时（如新建 / 分支
// 场景尚无需事实）得到暂停态快照。
func interactionSnapshot(c *gin.Context, comps *ai.Components, session *modelsai.AgentSession) interaction.InteractionSnapshot {
	return interaction.StateOf(agentInteractionFacts(c, comps, session))
}

// rejectIfSessionLocked 依据交互状态拒绝已达轮次上限的会话发起新提问（400 + 业务码 1305）。
// 运行中 / 等待中不在此处拒绝：新一轮会抢占旧回合或作废挂起现场，是 spec 明确允许的操作。
func rejectIfSessionLocked(c *gin.Context, snapshot interaction.InteractionSnapshot) bool {
	if !snapshot.Locked {
		return false
	}
	response.CreateCodeError(http.StatusBadRequest, response.CodeAgentSessionLocked, nil).AbortWithGinH(c)
	return true
}

// rejectGuardError 把交互状态守卫（CanSend / CanRollback / CanCompact 等）的拒绝哨兵映射为本地化
// 业务错误并中断请求，返回 true 表示已中断：
//   - 已锁定（ErrInteractionLocked）→ 400 + 业务码 1305「会话已达轮次上限，请回滚后再试」；
//   - 运行中 / 等待中（ErrInteractionBusy）→ 409 + 业务码 1306「当前会话状态不允许该操作」。
func rejectGuardError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, interaction.ErrInteractionLocked):
		response.CreateCodeError(http.StatusBadRequest, response.CodeAgentSessionLocked, nil).AbortWithGinH(c)
	default:
		response.CreateCodeError(http.StatusConflict, response.CodeAgentInteractionBusy, nil).AbortWithGinH(c)
	}
	return true
}
