package agentkit

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// askUserTool 把 ask_user 实现为 eino 一等工具：首次调用以工具级中断挂起并携带问卷载荷，
// 续跑时读取前端回传的作答结果。
type askUserTool struct{}

// Info 返回 ask_user 的入参 schema。
func (t *askUserTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return contract.NewAskUserToolInfo(), nil
}

// InvokableRun 首次调用校验入参并挂起；续跑时按 ToolResumeResult 回灌答案。
func (t *askUserTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// 后台无人值守：没有可交互的前端，直接返回固定降级提示，不校验参数、不挂起。
	if _, ok := AutomationFrom(ctx); ok {
		return AskUserUnattendedNotice, nil
	}
	wasInterrupted, hasState, storedArguments := tool.GetInterruptState[string](ctx)
	if !hasState || storedArguments == "" {
		storedArguments = argumentsInJSON
	}
	if !wasInterrupted {
		payload, err := parseQuestionnaireArgs(argumentsInJSON)
		if err != nil {
			return "invalid ask_user arguments: " + err.Error(), nil
		}
		decision, _ := lookupToolDecision(ctx, compose.GetToolCallID(ctx))
		request := &contract.QuestionnaireRequest{
			CallID:    compose.GetToolCallID(ctx),
			Questions: payload.Questions,
			Round:     decision.Round,
			Arguments: argumentsInJSON,
		}
		return "", tool.StatefulInterrupt(ctx, request, argumentsInJSON)
	}

	isTarget, hasData, data := tool.GetResumeContext[*contract.ToolResumeResult](ctx)
	if isTarget && hasData {
		return clientToolResumeResult(data), nil
	}
	if !isTarget {
		payload, err := parseQuestionnaireArgs(storedArguments)
		if err != nil {
			// 续跑参数已不可解析（不应发生）：按无结果回灌，避免再次挂起一个残缺问卷。
			return noToolResultText, nil
		}
		decision, _ := lookupToolDecision(ctx, compose.GetToolCallID(ctx))
		request := &contract.QuestionnaireRequest{
			CallID:    compose.GetToolCallID(ctx),
			Questions: payload.Questions,
			Round:     decision.Round,
			Arguments: storedArguments,
		}
		return "", tool.StatefulInterrupt(ctx, request, storedArguments)
	}
	return noToolResultText, nil
}

// apiInvokeTool 把 api_invoke 实现为 eino 一等工具：首次调用按能力白名单解析出规范入参并挂起
// 等待人工审批与前端执行，续跑时回灌前端结果。
type apiInvokeTool struct {
	capabilities func() []capability.Capability
}

// Info 返回 api_invoke 的入参 schema。
func (t *apiInvokeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return contract.NewApiInvokeToolInfo(), nil
}

// InvokableRun 首次调用校验并规范化入参后挂起；续跑时按 ToolResumeResult 回灌结果。
func (t *apiInvokeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// 后台无人值守：没有前端可审批与执行，改由服务端回调执行并回灌结果，不产生挂起。
	if automation, ok := AutomationFrom(ctx); ok {
		return t.invokeUnattended(ctx, automation, argumentsInJSON), nil
	}
	wasInterrupted, hasState, storedArguments := tool.GetInterruptState[string](ctx)
	if !hasState || storedArguments == "" {
		storedArguments = argumentsInJSON
	}
	if !wasInterrupted {
		canonical, err := prepareApiInvokeCall(argumentsInJSON, t.capabilityItems())
		if err != nil {
			return err.Error(), nil
		}
		callID := compose.GetToolCallID(ctx)
		decision, _ := lookupToolDecision(ctx, callID)
		return "", tool.StatefulInterrupt(ctx, apiInvokeApprovalRequest(decision, callID, canonical), canonical)
	}

	isTarget, hasData, data := tool.GetResumeContext[*contract.ToolResumeResult](ctx)
	if isTarget && hasData {
		return clientToolResumeResult(data), nil
	}
	if !isTarget {
		callID := compose.GetToolCallID(ctx)
		decision, _ := lookupToolDecision(ctx, callID)
		return "", tool.StatefulInterrupt(ctx, apiInvokeApprovalRequest(decision, callID, storedArguments), storedArguments)
	}
	return noToolResultText, nil
}

// invokeUnattended 在无人值守运行中执行一次 api_invoke：先按能力白名单解析规范入参，再交给服务端
// 回调执行。任何失败都只作为工具结果文本回灌给模型，不产生中断。
func (t *apiInvokeTool) invokeUnattended(ctx context.Context, automation *Automation, argumentsInJSON string) string {
	canonicalJSON, err := prepareApiInvokeCall(argumentsInJSON, t.capabilityItems())
	if err != nil {
		return err.Error()
	}
	var canonical apiInvokeCanonicalArgs
	if err := json.Unmarshal([]byte(canonicalJSON), &canonical); err != nil {
		return ApiInvokeFailedPrefix + err.Error()
	}
	if automation.InvokeAPI == nil {
		return ApiInvokeUnavailableNotice
	}
	result, err := automation.InvokeAPI(ctx, canonical.Method, canonical.Path, canonical.Params)
	if err != nil {
		return ApiInvokeFailedPrefix + err.Error()
	}
	return result
}

// capabilityItems 返回当前能力快照；为空时视为无能力。
func (t *apiInvokeTool) capabilityItems() []capability.Capability {
	if t == nil || t.capabilities == nil {
		return nil
	}
	return t.capabilities()
}

// apiInvokeApprovalRequest 构造 api_invoke 的审批载荷；审核结论缺失时默认 must / 无需审批。
func apiInvokeApprovalRequest(decision auditDecision, callID, canonicalArguments string) *contract.ApprovalRequest {
	level := decision.AuditLevel
	if level == "" {
		level = contract.AuditLevelMust
	}
	return &contract.ApprovalRequest{
		CallID:           callID,
		ToolName:         contract.ApiInvokeToolName,
		Arguments:        canonicalArguments,
		AuditLevel:       level,
		ApprovalRequired: decision.ApprovalRequired,
		ExecOwner:        contract.ToolExecOwnerClient,
		Round:            decision.Round,
	}
}

// clientToolResumeResult 把前端回传的工具执行结果转成回灌给模型的文本。
func clientToolResumeResult(result *contract.ToolResumeResult) string {
	switch {
	case result == nil:
		return noToolResultText
	case !result.Approved:
		return declinedToolCallText
	case result.Error != "":
		return result.Error
	case result.NoResult:
		return noToolResultText
	default:
		return result.Result
	}
}

// askUserOptionRule 是问卷候选项的校验视图。
type askUserOptionRule struct {
	// Text 候选答案文本
	Text string `json:"text" binding:"notBlank"`
	// Detail 候选答案的可选说明
	Detail string `json:"detail"`
}

// askUserQuestionRule 是单个问卷题目的校验视图。
type askUserQuestionRule struct {
	// ID 题目在本轮问卷内的唯一标识
	ID string `json:"id" binding:"notBlank"`
	// Question 题面
	Question string `json:"question" binding:"notBlank"`
	// Options 候选答案，1~3 条；max=3 与 contract.MaxQuestionnaireOptions 保持一致
	Options []askUserOptionRule `json:"options" binding:"required,min=1,max=3,dive"`
}

// askUserArgs 是 ask_user 工具入参的校验视图。
type askUserArgs struct {
	// Questions 问卷题目，至少 1 题
	Questions []askUserQuestionRule `json:"questions" binding:"required,min=1,dive"`
}

// parseQuestionnaireArgs 把模型给出的 ask_user 调用入参解析为问卷载荷并按校验视图校验（至少 1 题，每题需
// 有非空 id / question 与 1~3 条非空 text 的候选项）。解析或校验失败返回英文错误，调用方据此按工具
// 失败回灌、让模型修正后重试，而不是挂起一个空问卷。
func parseQuestionnaireArgs(arguments string) (*contract.QuestionnairePayload, error) {
	var args askUserArgs
	if err := validator.DecodeObject(arguments, &args); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := validator.Check(&args); err != nil {
		return nil, errors.Newf("ask_user questionnaire is invalid: %s", validator.Describe(err))
	}
	questions := make([]contract.QuestionnaireQuestion, 0, len(args.Questions))
	for _, q := range args.Questions {
		options := make([]contract.QuestionnaireOption, 0, len(q.Options))
		for _, o := range q.Options {
			options = append(options, contract.QuestionnaireOption{Text: o.Text, Detail: o.Detail})
		}
		questions = append(questions, contract.QuestionnaireQuestion{
			ID:       q.ID,
			Question: q.Question,
			Options:  options,
		})
	}
	return &contract.QuestionnairePayload{Questions: questions}, nil
}
