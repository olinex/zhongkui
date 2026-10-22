package ai

import (
	// 标准库
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// TestMapAgentLiveRunError 测试续播 / 取消失败到业务错误码的映射：不存在与归属不符同为 404
// （避免泄露运行存在性），登记过期返回 400。
func TestMapAgentLiveRunError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.GET("/map", func(c *gin.Context) {
		switch c.Query("kind") {
		case "notfound":
			mapAgentLiveRunError(c, ai.ErrAgentLiveRunNotFound)
		case "forbidden":
			mapAgentLiveRunError(c, ai.ErrAgentLiveRunForbidden)
		case "expired":
			mapAgentLiveRunError(c, ai.ErrAgentLiveRunExpired)
		}
	})
	cases := []struct {
		kind     string
		wantHttp int
		wantCode int
	}{
		{"notfound", http.StatusNotFound, response.CodeAgentLiveRunNotFound},
		{"forbidden", http.StatusNotFound, response.CodeAgentLiveRunForbidden},
		{"expired", http.StatusBadRequest, response.CodeAgentLiveRunExpired},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/map?kind="+c.kind, nil))
		if rec.Code != c.wantHttp {
			t.Errorf("%s: status = %d, want %d", c.kind, rec.Code, c.wantHttp)
		}
		var body struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: unmarshal body error = %v (body=%s)", c.kind, err, rec.Body.String())
		}
		if body.Code != c.wantCode {
			t.Errorf("%s: code = %d, want %d", c.kind, body.Code, c.wantCode)
		}
	}
}

// TestNewAgentLiveRunOut 测试驻留运行快照到视图层输出项的映射：挂起阶段标记待审批并携带待执行
// 调用列表，其它阶段待审批为假且待执行调用为空数组；交互状态五项随 interaction 快照原样写入。
func TestNewAgentLiveRunOut(t *testing.T) {
	interaction := ai.InteractionSnapshot{
		State:        modelsai.AgentInteractionStateWaiting,
		Substate:     modelsai.AgentInteractionSubstateApproval,
		Locked:       false,
		UserTurns:    2,
		MaxUserTurns: 20,
	}
	suspended := newAgentLiveRunOut(&ai.LiveRunStatus{
		RunID:   serializer.MustHexUUIDv4FromString("11111111-1111-1111-1111-111111111111"),
		Phase:   modelsai.AgentRunPhaseSuspended,
		LastSeq: 7,
		PendingCalls: []ai.PendingCall{
			{CallID: "c1", ToolName: "api_invoke", Arguments: "{}", AuditLevel: ai.AuditLevelSuggest, ApprovalRequired: true},
		},
	}, interaction)
	if !suspended.AwaitingApproval || suspended.LastSeq != 7 || len(suspended.PendingCalls) != 1 {
		t.Fatalf("suspended snapshot = %+v, want awaiting approval with one pending call", suspended)
	}
	// 审核重要级与「是否需要人工审批」须随快照下发，供前端重建待审批面板时不丢字段
	if call := suspended.PendingCalls[0]; call.AuditLevel != string(ai.AuditLevelSuggest) || !call.ApprovalRequired {
		t.Errorf("pending call = %+v, want audit level and approval flag carried through", call)
	}
	// 交互状态五项须与会话出参同口径地随快照下发。
	if suspended.InteractionState != string(modelsai.AgentInteractionStateWaiting) ||
		suspended.InteractionSubstate != string(modelsai.AgentInteractionSubstateApproval) ||
		suspended.Locked || suspended.UserTurns != 2 || suspended.MaxUserTurns != 20 {
		t.Errorf("suspended interaction = %+v, want the interaction snapshot carried through", suspended)
	}
	running := newAgentLiveRunOut(&ai.LiveRunStatus{RunID: serializer.MustHexUUIDv4FromString("22222222-2222-2222-2222-222222222222"), Phase: modelsai.AgentRunPhaseRunning}, ai.InteractionSnapshot{
		State:    modelsai.AgentInteractionStateRunning,
		Substate: modelsai.AgentInteractionSubstateGenerating,
	})
	if running.AwaitingApproval || running.PendingCalls == nil || len(running.PendingCalls) != 0 {
		t.Errorf("running snapshot = %+v, want not awaiting approval with an empty (non-nil) call list", running)
	}
	if running.InteractionState != string(modelsai.AgentInteractionStateRunning) || running.InteractionSubstate != string(modelsai.AgentInteractionSubstateGenerating) {
		t.Errorf("running interaction = %+v, want running/generating", running)
	}
}
