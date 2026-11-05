package agentkit

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/audit"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

const (
	// auditRunStateKey 是工具审核决策在 run-local 状态中的键。
	auditRunStateKey = "agentkit.audit"
	// declinedToolCallText 是用户拒绝执行一次挂起调用时回灌给模型的固定英文文案。
	declinedToolCallText = "the user declined to execute this call; do not retry it without a clear reason"
	// noToolResultText 是客户端未回传结果时回灌给模型的固定英文文案。
	noToolResultText = "no result was provided for this call"
)

// auditDecision 是一条工具调用的审核结论（run-local，随 checkpoint 往返）。
type auditDecision struct {
	// CallID 模型 tool call id
	CallID string
	// ToolName 工具名
	ToolName string
	// AuditLevel 审核重要级
	AuditLevel contract.AuditLevel
	// ExecOwner 执行归属
	ExecOwner contract.ToolExecOwner
	// ApprovalRequired 最终是否需人工审核
	ApprovalRequired bool
	// Round 所属工具轮次（从 1 起）
	Round int
}

// auditRunState 是一次运行内累积的审核结论。它是 gob 可序列化的普通结构（无 map[string]any），
// 因而可经 adk.SetRunLocalValue 随 checkpoint 往返。
type auditRunState struct {
	// Round 已处理的工具轮次
	Round int
	// Slot 本次运行档位
	Slot modelsai.Slot
	// Valve 会话审核阀门
	Valve modelsai.ReviewValve
	// Decisions 按产生顺序累积的逐调用结论
	Decisions []auditDecision
}

// init 注册会随 checkpoint gob 往返的类型。
func init() {
	schema.RegisterName[*auditRunState]("agentkit_audit_run_state")
	schema.RegisterName[*contract.ApprovalRequest]("agentkit_approval_request")
	schema.RegisterName[*contract.QuestionnaireRequest]("agentkit_questionnaire_request")
	schema.RegisterName[*contract.ToolResumeResult]("agentkit_tool_resume_result")
}

// auditMiddleware 在每次模型调用后批量评估本轮工具调用的重要级并写入 run-local 状态；
// 对后端工具在 must 级时以工具级中断挂起等待人工审批。
type auditMiddleware[M adk.MessageType] struct {
	*adk.TypedBaseChatModelAgentMiddleware[M]
	evaluator *audit.Evaluator
	logger    *logging.Logger
}

// newAuditMiddleware 构造审核中间件。
func newAuditMiddleware[M adk.MessageType](evaluator *audit.Evaluator, logger *logging.Logger) adk.TypedChatModelAgentMiddleware[M] {
	return &auditMiddleware[M]{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[M]{},
		evaluator:                         evaluator,
		logger:                            logger,
	}
}

// AfterModelRewriteState 从最新的助手消息提取工具调用（排除 ask_user），批量评估审核重要级，
// 把逐调用结论写入 run-local 状态并把轮次加一。
func (m *auditMiddleware[M]) AfterModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[M], _ *adk.TypedModelContext[M]) (context.Context, *adk.TypedChatModelAgentState[M], error) {
	if state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	calls := msgops.ToolCalls(state.Messages[len(state.Messages)-1])
	if len(calls) == 0 {
		return ctx, state, nil
	}

	previous, _ := loadAuditState(ctx)
	round := previous.Round + 1
	slot := previous.Slot
	valve := previous.Valve
	if rc, ok := RunContextFrom[M](ctx); ok {
		slot = rc.Request.Slot
		valve = rc.Request.ReviewValve
	}
	if valve == "" {
		valve = modelsai.ReviewValveEach
	}

	decisions := m.evaluate(ctx, slot, valve, calls, round)
	next := &auditRunState{
		Round:     round,
		Slot:      slot,
		Valve:     valve,
		Decisions: append(previous.Decisions, decisions...),
	}
	if err := adk.SetRunLocalValue(ctx, auditRunStateKey, next); err != nil {
		m.warnf(ctx, "agentkit: storing the tool audit decisions failed: %v", err)
	}
	return ctx, state, nil
}

// evaluate 对本轮调用做一次批量审核评估：ask_user 豁免评估（none / 无需审批），其余调用按键
// 与顺序对齐回填。评估失败时 audit.Evaluator 已保守回落 must。
func (m *auditMiddleware[M]) evaluate(ctx context.Context, slot modelsai.Slot, valve modelsai.ReviewValve, calls []msgops.ToolCall, round int) []auditDecision {
	decisions := make([]auditDecision, len(calls))
	evalCalls := make([]contract.PendingCall, 0, len(calls))
	evalIndex := make([]int, 0, len(calls))
	for i, call := range calls {
		owner := contract.ToolExecOwnerBackend
		if contract.IsClientTool(call.Name) {
			owner = contract.ToolExecOwnerClient
		}
		decisions[i] = auditDecision{
			CallID:    call.ID,
			ToolName:  call.Name,
			ExecOwner: owner,
			Round:     round,
		}
		if contract.IsQuestionnaireTool(call.Name) {
			decisions[i].AuditLevel = contract.AuditLevelNone
			decisions[i].ApprovalRequired = false
			continue
		}
		evalCalls = append(evalCalls, contract.PendingCall{
			CallID:    call.ID,
			ToolName:  call.Name,
			Arguments: call.Args,
			ExecOwner: owner,
		})
		evalIndex = append(evalIndex, i)
	}
	if len(evalCalls) > 0 && m.evaluator != nil {
		for levelIndex, level := range m.evaluator.Levels(ctx, slot, evalCalls) {
			if levelIndex >= len(evalIndex) {
				break
			}
			decisions[evalIndex[levelIndex]].AuditLevel = level
		}
	}
	for i := range decisions {
		decision := &decisions[i]
		if decision.AuditLevel == "" {
			decision.AuditLevel = contract.AuditLevelMust
		}
		switch decision.ExecOwner {
		case contract.ToolExecOwnerBackend:
			decision.ApprovalRequired = decision.AuditLevel == contract.AuditLevelMust
		default:
			decision.ApprovalRequired = audit.NeedsHumanReview(decision.AuditLevel, valve)
		}
		if contract.IsQuestionnaireTool(decision.ToolName) {
			decision.AuditLevel = contract.AuditLevelNone
			decision.ApprovalRequired = false
		}
	}
	// 无人值守运行没有前端可审批：保留 AuditLevel 供日志 / 评估，但一律不要求人工审核。
	if _, ok := AutomationFrom(ctx); ok {
		for i := range decisions {
			decisions[i].ApprovalRequired = false
		}
	}
	return decisions
}

// WrapInvokableToolCall 对非客户端工具在 must 级时插入人工审批中断。客户端工具（ask_user /
// api_invoke）自带中断逻辑，此处直接放行。
func (m *auditMiddleware[M]) WrapInvokableToolCall(_ context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	if tCtx == nil || contract.IsClientTool(tCtx.Name) {
		return endpoint, nil
	}
	callID := tCtx.CallID
	name := tCtx.Name
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		// 无人值守运行没有前端可审批：直接执行，不插入审批中断。
		if _, ok := AutomationFrom(ctx); ok {
			return endpoint(ctx, argumentsInJSON, opts...)
		}
		wasInterrupted, hasState, storedArguments := tool.GetInterruptState[string](ctx)
		if !hasState || storedArguments == "" {
			storedArguments = argumentsInJSON
		}
		if !wasInterrupted {
			decision, ok := lookupToolDecision(ctx, callID)
			if !ok || decision.AuditLevel != contract.AuditLevelMust {
				return endpoint(ctx, argumentsInJSON, opts...)
			}
			return "", tool.StatefulInterrupt(ctx, approvalRequest(decision, name, storedArguments), storedArguments)
		}

		isTarget, hasData, data := tool.GetResumeContext[*contract.ToolResumeResult](ctx)
		if isTarget && hasData {
			if data.Approved {
				return endpoint(ctx, storedArguments, opts...)
			}
			return declinedToolCallText, nil
		}
		if !isTarget {
			decision, _ := lookupToolDecision(ctx, callID)
			return "", tool.StatefulInterrupt(ctx, approvalRequest(decision, name, storedArguments), storedArguments)
		}
		// 目标是本次调用但没有携带数据：按隐式批准继续执行。
		return endpoint(ctx, storedArguments, opts...)
	}, nil
}

// approvalRequest 构造一次后端审批中断的载荷。
func approvalRequest(decision auditDecision, toolName, arguments string) *contract.ApprovalRequest {
	level := decision.AuditLevel
	if level == "" {
		level = contract.AuditLevelMust
	}
	return &contract.ApprovalRequest{
		CallID:           decision.CallID,
		ToolName:         toolName,
		Arguments:        arguments,
		AuditLevel:       level,
		ApprovalRequired: true,
		ExecOwner:        contract.ToolExecOwnerBackend,
		Round:            decision.Round,
	}
}

// loadAuditState 读取当前 run-local 审核状态；不存在或读取失败时返回零值。
func loadAuditState(ctx context.Context) (*auditRunState, bool) {
	value, ok, err := adk.GetRunLocalValue(ctx, auditRunStateKey)
	if err != nil || !ok {
		return &auditRunState{}, false
	}
	state, ok := value.(*auditRunState)
	if !ok || state == nil {
		return &auditRunState{}, false
	}
	return state, true
}

// lookupToolDecision 按 callID 取回本调用的审核结论。
func lookupToolDecision(ctx context.Context, callID string) (auditDecision, bool) {
	state, ok := loadAuditState(ctx)
	if !ok {
		return auditDecision{}, false
	}
	for i := len(state.Decisions) - 1; i >= 0; i-- {
		if state.Decisions[i].CallID == callID {
			return state.Decisions[i], true
		}
	}
	return auditDecision{}, false
}

// warnf 在 logger 非空时输出告警。
func (m *auditMiddleware[M]) warnf(ctx context.Context, format string, args ...any) {
	if m.logger == nil {
		return
	}
	m.logger.Ctx(ctx).Warnf(format, args...)
}
