package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newCompactChatServer 启动一个 OpenAI 兼容的假对话端点：stream=true 时按 SSE 返回两段摘要
// 增量后以 [DONE] 收尾（供滚动摘要流式生成），否则返回一段非 JSON 正文（供记忆抽取：解析失败仅
// 记 warning、不触发向量化落库）。端点不发起任何真实模型调用。
func newCompactChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, piece := range []string{"压缩", "摘要"} {
				_, _ = io.WriteString(w, `data: {"id":"1","object":"chat.completion.chunk","created":0,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"`+piece+`"},"finish_reason":null}]}`+"\n\n")
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","created":0,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"no-op"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
}

// newCompactingAiRuntime 构造一个可真正完成压缩的 AI 运行时：flash 与 standard 档位绑定到
// baseURL 指向的假对话端点，embedding 档位绑定到不可达地址（装配阶段不联网）。flash 可用使
// 滚动摘要走真实流式路径，便于断言语标推进。
func newCompactingAiRuntime(t *testing.T, db *database.DB, baseURL string) *ai.Runtime {
	t.Helper()
	chat := &modelsai.LlmModel{
		Name: "chat-compact", Kind: llm.ModelKindChat,
		BaseURL: baseURL + "/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "gpt-test", Enabled: true,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("seed compacting chat model: %v", err)
	}
	embed := &modelsai.LlmModel{
		Name: "embed-compact", Kind: llm.ModelKindEmbedding,
		BaseURL: "http://127.0.0.1:1/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "emb-test", Enabled: true,
	}
	if err := db.Create(embed).Error; err != nil {
		t.Fatalf("seed compacting embedding model: %v", err)
	}
	for _, binding := range []modelsai.LlmBinding{
		{Slot: modelsai.SlotFlash, ModelID: chat.ID},
		{Slot: modelsai.SlotStandard, ModelID: chat.ID},
		{Slot: modelsai.SlotEmbedding, ModelID: embed.ID},
	} {
		if err := db.Omit("Model").Create(&binding).Error; err != nil {
			t.Fatalf("seed slot binding %q: %v", binding.Slot, err)
		}
	}
	rt, err := ai.NewRuntime(context.Background(), db, nil, newKnowledgeTestLogger())
	if err != nil {
		t.Fatalf("build compacting ai runtime: %v", err)
	}
	return rt
}

// newSessionCompactTestRouter 创建仅注册会话压缩路由的测试引擎（装配范式同 newSessionTestRouter）。
func newSessionCompactTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group.POST("/:id/compact", AgentSessionCompact)
	return r
}

// compactPath 拼接会话压缩路径。
func compactPath(id uint) string {
	return "/ai/session/" + strconv.FormatUint(uint64(id), 10) + "/compact"
}

// loadSessionMessages 读取一个会话的全部消息（按 seq 升序），供测试定位折叠边界。
func loadSessionMessages(t *testing.T, db *database.DB, sessionID uint) []*modelsai.AgentMessage {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	messages, err := store.ListMessagesAfter(context.Background(), sessionID, sessionTestUserID, 0)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}
	return messages
}

// loadMessagesAfter 读取 seq > afterSeq 的消息（按 seq 升序），供折叠下界断言。
func loadMessagesAfter(t *testing.T, db *database.DB, sessionID uint, afterSeq uint) []*modelsai.AgentMessage {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	messages, err := store.ListMessagesAfter(context.Background(), sessionID, sessionTestUserID, afterSeq)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}
	return messages
}

// reloadCompactSession 重新读取一个会话行，供断言游标与摘要落库。
func reloadCompactSession(t *testing.T, db *database.DB, sessionID uint) *modelsai.AgentSession {
	t.Helper()
	row := &modelsai.AgentSession{}
	if err := db.Where("id = ?", sessionID).First(row).Error; err != nil {
		t.Fatalf("reload session %d: %v", sessionID, err)
	}
	return row
}

// decodeCompactCode 解析压缩响应体中的业务码，供被拒用例断言 1125。
func decodeCompactCode(t *testing.T, body []byte) int {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, string(body))
	}
	return resp.Code
}

// TestAgentCompressSessionFoldScope 覆盖 Task 8 的折叠区间口径（8.1 / 8.2）：入参以选中的助手回答为
// 折叠上界、以会话当前游标 summary_upto_seq 为下界，更晚的消息保持原文。以同步方式直接调用
// Agent.CompressSession（与端点共用的折叠集合选择函数 compactFoldSet），不经 RunHub 在途活动——
// 测试库为单连接事务（testdb.Core），在途活动的后台落库 goroutine 与该活动的读库并发会触发
// conn busy，无法稳定断言；在途活动的生命周期语义另由 internal/ai 的 RunHub 用例覆盖。
func TestAgentCompressSessionFoldScope(t *testing.T) {
	db := createKnowledgeTestDB(t)
	srv := newCompactChatServer(t)
	defer srv.Close()
	rt := newCompactingAiRuntime(t, db, srv.URL)
	agent := rt.Current().Agent
	if agent == nil {
		t.Fatal("compacting runtime has no agent assembled")
	}
	session := seedBranchSourceSession(t, db, "源会话")
	messages := loadSessionMessages(t, db, session.ID)
	if len(messages) != 4 {
		t.Fatalf("seeded messages = %d, want 4", len(messages))
	}
	a1 := messages[1] // u1(1) a1(2) u2(3) a2(4)
	a2 := messages[3]
	ctx := context.Background()

	var reports []ai.CompressEvent
	report := func(ev ai.CompressEvent) error {
		reports = append(reports, ev)
		return nil
	}

	// 第一次：上界 = a1.seq，下界 = 会话当前游标（尚未摘要，为 0）→ 折叠 u1 / a1 两条。
	evicted := compactFoldSet(messages, a1.Seq)
	if len(evicted) != 2 {
		t.Fatalf("first fold set = %d messages, want 2", len(evicted))
	}
	agent.CompressSession(ctx, session, evicted, "manual", session.TargetModel, report)
	if len(reports) == 0 {
		t.Fatal("first compression reported no events")
	}
	first := reports[len(reports)-1]
	if first.Type != modelsai.AgentRunEventCompressDone {
		t.Fatalf("first compression last event = %q, want compress_done", first.Type)
	}
	if first.FoldedCount != 2 {
		t.Errorf("first compression foldedCount = %d, want 2", first.FoldedCount)
	}
	if first.SummaryUptoSeq != uint(a1.Seq) {
		t.Errorf("first compression summaryUptoSeq = %d, want %d", first.SummaryUptoSeq, a1.Seq)
	}
	if row := reloadCompactSession(t, db, session.ID); row.SummaryUptoSeq != uint(a1.Seq) {
		t.Fatalf("cursor after first compression = %d, want %d", row.SummaryUptoSeq, a1.Seq)
	}

	// 第二次：下界 = 上次压缩结尾（a1.seq），上界 = a2.seq → 只折叠其间的 u2 / a2 两条。
	later := loadMessagesAfter(t, db, session.ID, session.SummaryUptoSeq)
	evicted2 := compactFoldSet(later, a2.Seq)
	if len(evicted2) != 2 {
		t.Fatalf("second fold set = %d messages, want 2", len(evicted2))
	}
	reports = nil
	agent.CompressSession(ctx, session, evicted2, "manual", session.TargetModel, report)
	if len(reports) == 0 {
		t.Fatal("second compression reported no events")
	}
	second := reports[len(reports)-1]
	if second.FoldedCount != 2 {
		t.Errorf("second compression foldedCount = %d, want 2", second.FoldedCount)
	}
	if second.SummaryUptoSeq != uint(a2.Seq) {
		t.Errorf("second compression summaryUptoSeq = %d, want %d", second.SummaryUptoSeq, a2.Seq)
	}
	if row := reloadCompactSession(t, db, session.ID); row.SummaryUptoSeq != uint(a2.Seq) {
		t.Fatalf("cursor after second compression = %d, want %d", row.SummaryUptoSeq, a2.Seq)
	}
	// 更晚的消息保持原文：折叠不删除任何消息，会话消息总数仍为 4。
	if got := len(loadSessionMessages(t, db, session.ID)); got != 4 {
		t.Errorf("message total after fold = %d, want 4 (later messages must stay verbatim)", got)
	}
}

// TestAgentSessionCompactBoundaryNotFound 覆盖压缩边界的本地化 404（8.4）：助手消息 ID 不存在、或指向
// 非助手消息时，端点以 JSON 404（非 SSE）拒绝，且尚未启动任何在途活动。
func TestAgentSessionCompactBoundaryNotFound(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
	session := seedBranchSourceSession(t, db, "源会话")
	messages := loadSessionMessages(t, db, session.ID)
	if len(messages) != 4 {
		t.Fatalf("seeded messages = %d, want 4", len(messages))
	}

	cases := []struct {
		name string
		id   uint
	}{
		{"不存在的助手消息", 99999},
		{"非助手消息（用户提问）", messages[0].ID},
	}
	for _, tc := range cases {
		w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(session.ID), map[string]any{
			"assistantMessageID": tc.id,
		})
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want %d (body=%s)", tc.name, w.Code, http.StatusNotFound, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s: content-type = %q, want application/json", tc.name, ct)
		}
		if body := w.Body.String(); !strings.Contains(body, "not found") {
			t.Errorf("%s: body = %q, want localized not found", tc.name, body)
		}
	}
}

// TestAgentSessionCompact_RejectsRunning 端到端覆盖「运行中 → 压缩被拒 1125」：为该会话起一次阻塞的在途
// 活动（内存登记为 running），压缩端点应先经 CanCompact 守卫以 409 + 业务码 1125 拒绝，且压缩未发生
// （消息数不变、游标仍为 0）。用例走 PostgreSQL 测试库（起活动会登记 agent_live_runs 含 TIMESTAMPTZ 列）。
func TestAgentSessionCompact_RejectsRunning(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "运行中压缩")
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	before := truncateMessageCount(t, store, session.ID)

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
	// 起活动的瞬间 RunHub 会发布初始 state 事件并由后台落库 goroutine 写库；该活动自身阻塞于 ctx.Done
	// 且不再上报任何事件，故等这一条写库完成后落库 goroutine 即进入空闲。测试库为单连接事务
	// （testdb.Core），若不等待，处理器的读库会与该后台写库在同一连接上并发（pgx 报 conn busy）。
	time.Sleep(100 * time.Millisecond)
	defer func() {
		_ = rt.RunHub().Cancel(context.Background(), session.ID, sessionTestUserID, knowledgeTestNamespace)
	}()

	w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(session.ID), map[string]any{})
	if w.Code != http.StatusConflict {
		t.Fatalf("compact(running) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeCompactCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("compact(running) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if got := truncateMessageCount(t, store, session.ID); got != before {
		t.Errorf("message count after rejected compact = %d, want unchanged %d", got, before)
	}
	if row := reloadCompactSession(t, db, session.ID); row.SummaryUptoSeq != 0 || row.Summary != nil {
		t.Errorf("compact(running) mutated the session: summaryUptoSeq=%d summarySet=%v", row.SummaryUptoSeq, row.Summary != nil)
	}
}

// TestAgentSessionCompact_RejectsWaiting 端到端覆盖「等待中 → 压缩被拒 1125」：为该会话播一次真实挂起
// 现场（agent_runs 待恢复登记 + agent_live_runs 挂起登记、无内存活动），压缩端点应先经 CanCompact 守卫
// 以 409 + 业务码 1125 拒绝，且压缩未发生。
func TestAgentSessionCompact_RejectsWaiting(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
	session := seedSessionWithOneTurn(t, db, "等待中压缩")
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	before := truncateMessageCount(t, store, session.ID)

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

	w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(session.ID), map[string]any{})
	if w.Code != http.StatusConflict {
		t.Fatalf("compact(waiting) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeCompactCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("compact(waiting) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if got := truncateMessageCount(t, store, session.ID); got != before {
		t.Errorf("message count after rejected compact = %d, want unchanged %d", got, before)
	}
}
