package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newTruncateTestRouter 创建回滚端点测试路由引擎：装配与同目录会话 / 交互视图测试同源（i18n、日志、
// 核心库、命名空间、当前用户、AI 运行时），注册回滚（AgentMessageTruncate）与会话详情
// （AgentSessionDetailGet）两个端点——后者供成功回滚后断言重算出的交互状态五项。
func newTruncateTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group := r.Group("/ai/session")
	group.POST("/:id/messages/truncate", AgentMessageTruncate)
	group.POST("/:id/get", AgentSessionDetailGet)
	return r
}

// truncatePath 拼接会话回滚路径。
func truncatePath(id uint) string {
	return "/ai/session/" + strconv.FormatUint(uint64(id), 10) + "/messages/truncate"
}

// sessionDetailPath 拼接会话详情路径。
func sessionDetailPath(id uint) string {
	return "/ai/session/" + strconv.FormatUint(uint64(id), 10) + "/get"
}

// truncateMessageCount 读取会话内的消息条数（含全部角色），供回滚被拒时断言消息数不变。
func truncateMessageCount(t *testing.T, store *ai.SessionStore, sessionID uint) int {
	t.Helper()
	messages, err := store.ListMessages(context.Background(), sessionID, sessionTestUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	return len(messages)
}

// decodeTruncateCode 解析回滚响应体中的业务码，供被拒用例断言 1125。
func decodeTruncateCode(t *testing.T, body []byte) int {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, string(body))
	}
	return resp.Code
}

// TestAgentMessageTruncate_RejectsRunning 端到端覆盖「运行中 → 回滚被拒 1125」：为该会话起一次阻塞的
// 在途活动（内存登记为 running，agentInteractionFacts 视作真在途），调用回滚端点断言返回 409 + 业务码
// 1125，且截断未发生（消息数不变）。用例走 PostgreSQL 测试库（testdb.Core）：起活动会登记 agent_live_runs
// （含 TIMESTAMPTZ 列），sqlite 方言无法扫描。
func TestAgentMessageTruncate_RejectsRunning(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newTruncateTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "运行中回滚")
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	before := truncateMessageCount(t, store, session.ID)

	// 起一次阻塞在 ctx.Done() 的在途活动：登记为 running 且本进程内存中有对应活动，即「运行中」。
	ctx := context.Background()
	if _, err := rt.RunHub().StartActivity(ai.NewDetachedContext(ctx), ai.ActivityInput{
		SessionID: session.ID, UserID: sessionTestUserID, Namespace: knowledgeTestNamespace,
		Run: func(ctx context.Context, report func(typ modelsai.AgentRunEventType, payload any) error) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}, nil); err != nil {
		t.Fatalf("StartActivity error = %v", err)
	}
	// 用例结束前取消在途活动，避免阻塞 goroutine 残留到测试库回滚之后。
	defer func() {
		_ = rt.RunHub().Cancel(context.Background(), session.ID, sessionTestUserID, knowledgeTestNamespace)
	}()

	w := doKnowledgeJSONRequest(r, http.MethodPost, truncatePath(session.ID), map[string]any{"userMessageIndex": 1})
	if w.Code != http.StatusConflict {
		t.Fatalf("truncate(running) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeTruncateCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("truncate(running) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if got := truncateMessageCount(t, store, session.ID); got != before {
		t.Errorf("message count after rejected rollback = %d, want unchanged %d", got, before)
	}
}

// TestAgentMessageTruncate_RejectsWaiting 端到端覆盖「等待中 → 回滚被拒 1125」：为该会话播一次真实挂起
// 现场——agent_runs 待恢复登记（status=pending、未过期）+ 对应的 agent_live_runs 登记（phase=suspended、
// 无内存活动），agentInteractionFacts 视作挂起现场（无 PendingCallsJSON 缺省为 approval），调用回滚端点
// 断言返回 409 + 业务码 1125，且截断未发生（消息数不变）。
func TestAgentMessageTruncate_RejectsWaiting(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newTruncateTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "等待中回滚")
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	before := truncateMessageCount(t, store, session.ID)

	// 挂起现场由「agent_runs 待恢复登记 + agent_live_runs 挂起登记」共同表达（真实挂起链路的落库形态）。
	// 省略关联的命名空间实体：NamespaceName 已指向已播种的行，否则零值关联会被 GORM 执行 upsert。
	if err := db.Omit("Namespace").Create(&modelsai.AgentRun{
		NamespaceName: knowledgeTestNamespace,
		UserID:        sessionTestUserID,
		SessionID:     session.ID,
		Status:        modelsai.AgentRunStatusPending,
		ExpiresAt:     time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed pending agent run error = %v", err)
	}
	if err := db.Omit("Namespace").Create(&modelsai.AgentLiveRun{
		NamespaceName: knowledgeTestNamespace,
		UserID:        sessionTestUserID,
		RunID:         serializer.NewHexUUIDv4(),
		SessionID:     session.ID,
		Phase:         modelsai.AgentRunPhaseSuspended,
		ExpiresAt:     time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed suspended live run error = %v", err)
	}

	w := doKnowledgeJSONRequest(r, http.MethodPost, truncatePath(session.ID), map[string]any{"userMessageIndex": 1})
	if w.Code != http.StatusConflict {
		t.Fatalf("truncate(waiting) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeTruncateCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("truncate(waiting) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if got := truncateMessageCount(t, store, session.ID); got != before {
		t.Errorf("message count after rejected rollback = %d, want unchanged %d", got, before)
	}
}

// TestAgentMessageTruncate_AllowsLockedAndRecalculates 覆盖「已锁定回滚放行并重算状态」：把轮次上限设为 1，
// 一条提问即达上限（已锁定）；回滚到第 1 条提问发送前应放行（200）、删除该提问，并由 broadcastInteractionState
// 重算交互状态——有效轮次降到上限以下，会话由 已锁定 回落 暂停、解锁。以会话详情出参验证重算结果。
func TestAgentMessageTruncate_AllowsLockedAndRecalculates(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "1")
	rt := newEnabledAiRuntime(t, db)
	r := newTruncateTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "锁定回滚")
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	// 前置：一条提问、上限 1 → 已锁定（回滚守卫对已锁定放行）。
	if snapshot := ai.StateOf(ai.InteractionFacts{UserTurns: 1, MaxUserTurns: 1}); snapshot.State != modelsai.AgentInteractionStateLocked {
		t.Fatalf("precondition StateOf = %q, want locked", snapshot.State)
	}

	w := doKnowledgeJSONRequest(r, http.MethodPost, truncatePath(session.ID), map[string]any{"userMessageIndex": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("truncate(locked) status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if got := truncateMessageCount(t, store, session.ID); got != 0 {
		t.Errorf("message count after rollback = %d, want 0", got)
	}

	// 重算后的状态经会话详情出参暴露：已锁定 → 暂停，解锁，有效轮次归零。
	detail := doKnowledgeJSONRequest(r, http.MethodPost, sessionDetailPath(session.ID), nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("session detail status = %d, want %d (body=%s)", detail.Code, http.StatusOK, detail.Body.String())
	}
	var resp struct {
		Data *AgentSessionItemOut `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal session detail error = %v (body=%s)", err, detail.Body.String())
	}
	if resp.Data == nil {
		t.Fatalf("session detail data is nil (body=%s)", detail.Body.String())
	}
	if resp.Data.InteractionState != modelsai.AgentInteractionStatePaused || resp.Data.Locked {
		t.Errorf("state after rollback = %q locked=%v, want paused locked=false", resp.Data.InteractionState, resp.Data.Locked)
	}
	if resp.Data.UserTurns != 0 || resp.Data.MaxUserTurns != 1 {
		t.Errorf("turns after rollback = %d/%d, want 0/1", resp.Data.UserTurns, resp.Data.MaxUserTurns)
	}
}
