package agentkit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
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
	wasInterrupted, hasState, storedArguments := tool.GetInterruptState[string](ctx)
	if !hasState || storedArguments == "" {
		storedArguments = argumentsInJSON
	}
	if !wasInterrupted {
		canonical, errText := prepareApiInvokeCall(argumentsInJSON, t.capabilityItems())
		if errText != "" {
			return errText, nil
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

// 问卷入参校验的固定错误。
var (
	errQuestionnaireRequiresQuestion = errors.New("ask_user requires at least one question")
	errQuestionnaireRequiresID       = errors.New("each ask_user question requires a non-empty id")
	errQuestionnaireRequiresText     = errors.New("each ask_user question requires a non-empty question text")
	errQuestionnaireOptionCount      = errors.New("each ask_user question requires 1 to " + strconv.Itoa(contract.MaxQuestionnaireOptions) + " options")
	errQuestionnaireOptionText       = errors.New("each ask_user option requires a non-empty text")
)

// askUserArgs 是 ask_user 工具入参的解析目标：只取问卷题目。
type askUserArgs struct {
	// Questions 问卷题目列表
	Questions []contract.QuestionnaireQuestion `json:"questions"`
}

// parseQuestionnaireArgs 把模型给出的 ask_user 调用入参解析为问卷载荷并做基本校验（至少 1 题，每题需
// 有非空 id / question 与 1~3 条非空 text 的候选项）。解析或校验失败返回英文错误，调用方据此按工具
// 失败回灌、让模型修正后重试，而不是挂起一个空问卷。
func parseQuestionnaireArgs(arguments string) (*contract.QuestionnairePayload, error) {
	var args askUserArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(args.Questions) == 0 {
		return nil, errQuestionnaireRequiresQuestion
	}
	for _, q := range args.Questions {
		if strings.TrimSpace(q.ID) == "" {
			return nil, errQuestionnaireRequiresID
		}
		if strings.TrimSpace(q.Question) == "" {
			return nil, errQuestionnaireRequiresText
		}
		if len(q.Options) == 0 || len(q.Options) > contract.MaxQuestionnaireOptions {
			return nil, errQuestionnaireOptionCount
		}
		for _, o := range q.Options {
			if strings.TrimSpace(o.Text) == "" {
				return nil, errQuestionnaireOptionText
			}
		}
	}
	return &contract.QuestionnairePayload{Questions: args.Questions}, nil
}
