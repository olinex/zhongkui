package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newInteractionTestRouter 创建发送守卫测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时，并注册 AgentStream 端点。装配与 session 视图测试同源，仅端点不同。
func newInteractionTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newKnowledgeTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Set(middlewares.AiRuntimeCtxName, rt)
		user := &core.User{}
		user.ID = sessionTestUserID
		c.Set(middlewares.UserCtxName, user)
		c.Next()
	})
	r.POST("/ai/agent/:sessionID/stream", AgentStream)
	return r
}

// TestRejectIfInteractionBusy 测试发送守卫到业务码的映射：已锁定 → 400 + 1124，运行中 / 等待中 →
// 409 + 1125，暂停态放行。
func TestRejectIfInteractionBusy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.GET("/guard", func(c *gin.Context) {
		var facts ai.InteractionFacts
		switch c.Query("kind") {
		case "locked":
			facts = ai.InteractionFacts{UserTurns: 5, MaxUserTurns: 5}
		case "running":
			facts = ai.InteractionFacts{LiveRunning: true, LiveSubstate: modelsai.AgentInteractionSubstateGenerating}
		case "waiting":
			facts = ai.InteractionFacts{PendingKind: modelsai.AgentInteractionSubstateApproval}
		}
		if rejectIfInteractionBusy(c, facts) {
			return
		}
		response.SimpleOK.AbortWithGinW(c, nil)
	})
	cases := []struct {
		kind     string
		wantHttp int
		wantCode int
	}{
		{"locked", http.StatusBadRequest, response.CodeAgentSessionLocked},
		{"running", http.StatusConflict, response.CodeAgentInteractionBusy},
		{"waiting", http.StatusConflict, response.CodeAgentInteractionBusy},
		{"paused", http.StatusOK, http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/guard?kind="+tc.kind, nil))
		if rec.Code != tc.wantHttp {
			t.Errorf("%s: status = %d, want %d", tc.kind, rec.Code, tc.wantHttp)
		}
		var body struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: unmarshal body error = %v (body=%s)", tc.kind, err, rec.Body.String())
		}
		if body.Code != tc.wantCode {
			t.Errorf("%s: code = %d, want %d", tc.kind, body.Code, tc.wantCode)
		}
	}
}

// TestAgentStream_RejectsLockedSession 端到端覆盖「已满 → 发送被拒 1124」：会话有效轮次已达上限时
// 调用 AgentStream，断言返回 400 + 业务码 1124，且未登记任何在途运行（发送守卫在 Hub.Start 之前生效）。
func TestAgentStream_RejectsLockedSession(t *testing.T) {
	db := createKnowledgeTestDB(t)
	// 把轮次上限调小，使「已满」只需一条提问即可构造：值随测试事务回滚，不污染基线；
	// 该键在此环境可能尚未随既有迁移种入，故按「插入或更新」补齐（default_value 一并给出以通过强校验）。
	if err := db.Exec(
		"INSERT INTO agent_params (key, value, default_value) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value",
		"session.maxUserTurns", "1", "20",
	).Error; err != nil {
		t.Fatalf("seed session.maxUserTurns override error = %v", err)
	}
	rt := newEnabledAiRuntime(t, db)
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, knowledgeTestNamespace, "t", modelsai.SlotStandard, modelsai.AgentKindKnowledge, sessionTestUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	// 一条提问即达到上限（maxUserTurns=1）。
	if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: knowledgeTestNamespace,
		UserID:        sessionTestUserID,
		Role:          modelsai.MessageRoleUser,
		Content:       serializer.NewCrypto([]byte("q1")),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}

	router := newInteractionTestRouter(db, knowledgeTestNamespace, rt)
	path := "/ai/agent/" + strconv.FormatUint(uint64(session.ID), 10) + "/stream"
	rec := doKnowledgeJSONRequest(router, http.MethodPost, path, map[string]any{"query": "q2"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("AgentStream(locked) status = %d, want %d (body=%s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body error = %v (body=%s)", err, rec.Body.String())
	}
	if body.Code != response.CodeAgentSessionLocked {
		t.Errorf("AgentStream(locked) code = %d, want %d", body.Code, response.CodeAgentSessionLocked)
	}
	// 未产生运行：发送守卫在 Hub.Start 之前返回，会话无任何在途运行登记。
	if _, err := rt.RunHub().LiveRun(ctx, session.ID, sessionTestUserID); !errors.Is(err, ai.ErrAgentLiveRunNotFound) {
		t.Errorf("LiveRun after rejected send error = %v, want ErrAgentLiveRunNotFound", err)
	}
}
