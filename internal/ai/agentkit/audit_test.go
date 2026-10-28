package agentkit

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/audit"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

func TestAuditEvaluateAlignsLevelsAndApproval(t *testing.T) {
	chat := &fakeChat{script: []*schema.Message{{
		Role: schema.Assistant,
		Content: `{"levels":[
			{"callID":"c1","level":"auto"},
			{"callID":"c2","level":"suggest"},
			{"callID":"c4","level":"none"}
		]}`,
	}}}
	evaluator := audit.New(audit.Deps{
		Chats:  func(modelsai.Slot) (model.BaseChatModel, error) { return chat, nil },
		Params: testParams(nil),
	})
	middleware := &auditMiddleware[*schema.Message]{evaluator: evaluator}
	calls := []msgops.ToolCall{
		{ID: "c1", Name: "add", Args: "{}"},
		{ID: "c2", Name: contract.ApiInvokeToolName, Args: "{}"},
		{ID: "c3", Name: contract.AskUserToolName, Args: "{}"},
		{ID: "c4", Name: "datetime_calc", Args: "{}"},
	}

	decisions := middleware.evaluate(context.Background(), modelsai.SlotStandard, modelsai.ReviewValveAuto, calls, 2)
	if len(decisions) != 4 {
		t.Fatalf("decisions len = %d, want 4", len(decisions))
	}
	if decisions[0].AuditLevel != contract.AuditLevelAuto || decisions[0].ApprovalRequired {
		t.Errorf("backend auto decision = %+v, want auto / no approval", decisions[0])
	}
	if decisions[1].AuditLevel != contract.AuditLevelSuggest || !decisions[1].ApprovalRequired {
		t.Errorf("client suggest decision = %+v, want suggest / approval", decisions[1])
	}
	if decisions[1].ExecOwner != contract.ToolExecOwnerClient {
		t.Errorf("api_invoke owner = %q, want client", decisions[1].ExecOwner)
	}
	if decisions[2].AuditLevel != contract.AuditLevelNone || decisions[2].ApprovalRequired {
		t.Errorf("ask_user decision = %+v, want none / no approval", decisions[2])
	}
	if decisions[3].AuditLevel != contract.AuditLevelNone || decisions[3].ApprovalRequired {
		t.Errorf("backend none decision = %+v, want none / no approval", decisions[3])
	}
	for i, decision := range decisions {
		if decision.Round != 2 {
			t.Errorf("decision[%d].Round = %d, want 2", i, decision.Round)
		}
	}
}

func TestAuditEvaluateBackendMustRequiresApproval(t *testing.T) {
	chat := &fakeChat{script: []*schema.Message{{
		Role:    schema.Assistant,
		Content: `{"levels":[{"callID":"c1","level":"must"}]}`,
	}}}
	evaluator := audit.New(audit.Deps{
		Chats:  func(modelsai.Slot) (model.BaseChatModel, error) { return chat, nil },
		Params: testParams(nil),
	})
	middleware := &auditMiddleware[*schema.Message]{evaluator: evaluator}
	decisions := middleware.evaluate(context.Background(), modelsai.SlotStandard, modelsai.ReviewValveNone,
		[]msgops.ToolCall{{ID: "c1", Name: "add", Args: "{}"}}, 1)
	if decisions[0].AuditLevel != contract.AuditLevelMust || !decisions[0].ApprovalRequired {
		t.Fatalf("backend must decision = %+v", decisions[0])
	}
	request := approvalRequest(decisions[0], "add", "{}")
	if request.AuditLevel != contract.AuditLevelMust || !request.ApprovalRequired ||
		request.ExecOwner != contract.ToolExecOwnerBackend || request.Round != 1 {
		t.Fatalf("approval request = %+v", request)
	}
}

func TestAuditEvaluateFallsBackToMust(t *testing.T) {
	chat := &fakeChat{script: []*schema.Message{{Role: schema.Assistant, Content: ""}}}
	evaluator := audit.New(audit.Deps{
		Chats:  func(modelsai.Slot) (model.BaseChatModel, error) { return chat, nil },
		Params: testParams(nil),
	})
	middleware := &auditMiddleware[*schema.Message]{evaluator: evaluator}
	decisions := middleware.evaluate(context.Background(), modelsai.SlotStandard, modelsai.ReviewValveEach,
		[]msgops.ToolCall{{ID: "c1", Name: "add", Args: "{}"}}, 1)
	if decisions[0].AuditLevel != contract.AuditLevelMust || !decisions[0].ApprovalRequired {
		t.Fatalf("fallback decision = %+v, want must / approval", decisions[0])
	}
}
