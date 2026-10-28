package contract

import (
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ErrInvalidAction 表示被服务端拒绝的操作名或负载。
var ErrInvalidAction = errors.New("ai: invalid action")

// IsValidAction 判断 name 是否为已知的客户端操作。
func IsValidAction(name string) bool {
	switch name {
	case ActionApprovalDecide, ActionQuestionnaireSubmit, ActionChatSend:
		return true
	default:
		return false
	}
}

// ValidateActionRequest 校验操作名及其所需的负载。
func ValidateActionRequest(request ActionRequest) error {
	if !IsValidAction(request.Action) {
		return errors.Wrapf(ErrInvalidAction, "unknown action %q", request.Action)
	}

	switch request.Action {
	case ActionApprovalDecide:
		if len(request.Decisions) == 0 {
			return errors.Wrapf(ErrInvalidAction, "%s requires decisions", ActionApprovalDecide)
		}
		for _, decision := range request.Decisions {
			if strings.TrimSpace(decision.CallID) == "" {
				return errors.Wrapf(ErrInvalidAction, "%s requires a callID per decision", ActionApprovalDecide)
			}
		}
	case ActionQuestionnaireSubmit:
		if len(request.Answers) == 0 {
			return errors.Wrapf(ErrInvalidAction, "%s requires answers", ActionQuestionnaireSubmit)
		}
		for _, answer := range request.Answers {
			if strings.TrimSpace(answer.QuestionID) == "" {
				return errors.Wrapf(ErrInvalidAction, "%s requires a questionID per answer", ActionQuestionnaireSubmit)
			}
		}
	case ActionChatSend:
		if strings.TrimSpace(request.Text) == "" {
			return errors.Wrapf(ErrInvalidAction, "%s requires text", ActionChatSend)
		}
	}

	return nil
}

// ApprovalDecisions 把审批 ActionRequest 转换为规范化决定，
// 当某条决定缺少中断 ID 时用请求中的 ID 填充。
func ApprovalDecisions(request ActionRequest) ([]ApprovalDecision, error) {
	if request.Action != ActionApprovalDecide {
		return nil, errors.Wrapf(ErrInvalidAction, "%q is not %s", request.Action, ActionApprovalDecide)
	}
	if err := ValidateActionRequest(request); err != nil {
		return nil, errors.WithStack(err)
	}

	interruptID := request.InterruptID
	if interruptID == "" && len(request.InterruptIDs) > 0 {
		interruptID = request.InterruptIDs[0]
	}

	decisions := make([]ApprovalDecision, 0, len(request.Decisions))
	for _, decision := range request.Decisions {
		decision.CallID = strings.TrimSpace(decision.CallID)
		if decision.InterruptID == "" {
			decision.InterruptID = interruptID
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// QuestionnaireAnswers 把问卷 ActionRequest 转换为规范化
// 答案。非空答案意味着 Answered；未作答的条目会丢弃其
// 文本。
func QuestionnaireAnswers(request ActionRequest) ([]QuestionnaireAnswer, error) {
	if request.Action != ActionQuestionnaireSubmit {
		return nil, errors.Wrapf(ErrInvalidAction, "%q is not %s", request.Action, ActionQuestionnaireSubmit)
	}
	if err := ValidateActionRequest(request); err != nil {
		return nil, errors.WithStack(err)
	}

	answers := make([]QuestionnaireAnswer, 0, len(request.Answers))
	for _, answer := range request.Answers {
		answer.QuestionID = strings.TrimSpace(answer.QuestionID)
		if answer.Answer != "" {
			answer.Answered = true
		}
		if !answer.Answered {
			answer.Answer = ""
			answer.Custom = false
		}
		answers = append(answers, answer)
	}
	return answers, nil
}
