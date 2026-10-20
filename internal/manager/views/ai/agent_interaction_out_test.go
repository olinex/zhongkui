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
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newInteractionOutTestRouter 创建快照出参测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时，并注册会话详情（AgentSessionDetailGet）与 AI 状态（AgentStatusGet）两个端点。装配与
// session / interaction 视图测试同源，仅端点不同。
func newInteractionOutTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group := r.Group("/ai")
	group.POST("/session/:id/get", AgentSessionDetailGet)
	group.POST("/status", AgentStatusGet)
	return r
}

// seedMaxUserTurns 把轮次上限参数（session.maxUserTurns）的生效值播入测试库：该键在此环境可能尚未随
// 既有迁移种入，故按「插入或更新」补齐（default_value 一并给出以通过强校验）。值随测试事务回滚，不污染基线。
func seedMaxUserTurns(t *testing.T, db *database.DB, value string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO agent_params (key, value, default_value) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value",
		"session.maxUserTurns", value, "20",
	).Error; err != nil {
		t.Fatalf("seed session.maxUserTurns error = %v", err)
	}
}

// seedSessionWithOneTurn 播种一个会话并写入一条用户提问（u1），返回会话；供快照出参用例使用。
func seedSessionWithOneTurn(t *testing.T, db *database.DB, title string) *modelsai.AgentSession {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), knowledgeTestNamespace, title, modelsai.SlotStandard, modelsai.AgentKindKnowledge, sessionTestUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(context.Background(), &modelsai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: knowledgeTestNamespace,
		UserID:        sessionTestUserID,
		Role:          modelsai.MessageRoleUser,
		Content:       serializer.NewCrypto([]byte("u1")),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	return session
}

// TestAgentSessionDetailInteractionOut 校验会话详情出参的交互状态五项与「事实 → StateOf」同口径：
// 播种一条提问（userTurns=1）、把轮次上限设为 7，详情出参应为 暂停/成功、未锁定、userTurns=1、
// maxUserTurns=7，且与按同一事实调用 ai.StateOf 推导出的快照逐一相等。
func TestAgentSessionDetailInteractionOut(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "7")
	rt := newEnabledAiRuntime(t, db)
	r := newInteractionOutTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "会话详情")

	path := "/ai/session/" + strconv.FormatUint(uint64(session.ID), 10) + "/get"
	w := doKnowledgeJSONRequest(r, http.MethodPost, path, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("AgentSessionDetailGet status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Data *AgentSessionItemOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal session detail error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil {
		t.Fatalf("session detail data is nil (body=%s)", w.Body.String())
	}

	// 与同一事实口径的推导结果对齐（详情出参 == StateOf(facts)）。
	expected := ai.StateOf(ai.InteractionFacts{UserTurns: 1, MaxUserTurns: 7})
	if resp.Data.InteractionState != expected.State {
		t.Errorf("interactionState = %q, want %q", resp.Data.InteractionState, expected.State)
	}
	if resp.Data.InteractionSubstate != expected.Substate {
		t.Errorf("interactionSubstate = %q, want %q", resp.Data.InteractionSubstate, expected.Substate)
	}
	if resp.Data.Locked != expected.Locked {
		t.Errorf("locked = %v, want %v", resp.Data.Locked, expected.Locked)
	}
	if resp.Data.UserTurns != expected.UserTurns {
		t.Errorf("userTurns = %d, want %d", resp.Data.UserTurns, expected.UserTurns)
	}
	if resp.Data.MaxUserTurns != expected.MaxUserTurns {
		t.Errorf("maxUserTurns = %d, want %d", resp.Data.MaxUserTurns, expected.MaxUserTurns)
	}
	// 显式锚定语义：一条提问、上限 7 → 暂停/成功、未锁定。
	if resp.Data.InteractionState != modelsai.AgentInteractionStatePaused ||
		resp.Data.InteractionSubstate != modelsai.AgentInteractionSubstateSuccess ||
		resp.Data.Locked {
		t.Errorf("session detail state = %q/%q locked=%v, want paused/success locked=false",
			resp.Data.InteractionState, resp.Data.InteractionSubstate, resp.Data.Locked)
	}
}

// TestAgentStatusOutMaxUserTurns 校验 AI 状态出参的轮次上限取自参数生效值：把 session.maxUserTurns 设为
// 7 后，/status 出参的 maxUserTurns 应为 7，供前端在尚无会话时展示闸门口径。
func TestAgentStatusOutMaxUserTurns(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "7")
	rt := newEnabledAiRuntime(t, db)
	r := newInteractionOutTestRouter(db, knowledgeTestNamespace, rt)

	w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("AgentStatusGet status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Data *AgentStatusOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal status error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil {
		t.Fatalf("status data is nil (body=%s)", w.Body.String())
	}
	if !resp.Data.Enabled {
		t.Fatalf("status enabled = false, want true (body=%s)", w.Body.String())
	}
	if resp.Data.MaxUserTurns != 7 {
		t.Errorf("status maxUserTurns = %d, want 7", resp.Data.MaxUserTurns)
	}
}

// TestAgentStatusOutLiveRunInteractionOut 校验 /status 在途运行快照的交互状态五项：为会话播种一条
// DB 登记为 running 的在途运行（无内存活动 → 孤儿），请求 /status 时快照回落数据库并触发孤儿收敛，
// 出参 liveRun 应带 running 阶段与 暂停/失败 的交互状态五项，且 DB 登记被置为 failed。
func TestAgentStatusOutLiveRunInteractionOut(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "7")
	rt := newEnabledAiRuntime(t, db)
	r := newInteractionOutTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "在途会话")

	runID := serializer.NewHexUUIDv4()
	lr := &modelsai.AgentLiveRun{
		NamespaceName: knowledgeTestNamespace,
		UserID:        sessionTestUserID,
		RunID:         runID,
		SessionID:     session.ID,
		Phase:         modelsai.AgentRunPhaseRunning,
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	// 省略关联的命名空间实体：NamespaceName 已指向已播种的行，否则零值关联会被 GORM 执行 upsert。
	if err := db.Omit("Namespace").Create(lr).Error; err != nil {
		t.Fatalf("seed in-flight live run error = %v", err)
	}

	w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/status", map[string]any{"sessionID": session.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("AgentStatusGet status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Data *AgentStatusOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal status error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil || resp.Data.LiveRun == nil {
		t.Fatalf("status liveRun is nil (body=%s)", w.Body.String())
	}
	live := resp.Data.LiveRun
	if live.Phase != modelsai.AgentRunPhaseRunning {
		t.Errorf("liveRun.phase = %q, want %q", live.Phase, modelsai.AgentRunPhaseRunning)
	}
	if live.InteractionState != modelsai.AgentInteractionStatePaused || live.InteractionSubstate != modelsai.AgentInteractionSubstateFailed || live.Locked {
		t.Errorf("liveRun state = %q/%q locked=%v, want paused/failed locked=false",
			live.InteractionState, live.InteractionSubstate, live.Locked)
	}
	if live.UserTurns != 1 {
		t.Errorf("liveRun.userTurns = %d, want 1", live.UserTurns)
	}
	if live.MaxUserTurns != 7 {
		t.Errorf("liveRun.maxUserTurns = %d, want 7", live.MaxUserTurns)
	}

	// 孤儿收敛：DB 登记应已被置为 failed（读取即收敛，杜绝「永远转圈」）。
	var row modelsai.AgentLiveRun
	if err := db.Where("run_id = ?", runID).First(&row).Error; err != nil {
		t.Fatalf("reload live run error = %v", err)
	}
	if row.Phase != modelsai.AgentRunPhaseFailed {
		t.Errorf("persisted live run phase = %q, want %q", row.Phase, modelsai.AgentRunPhaseFailed)
	}
}
