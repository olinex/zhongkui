package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"net/http"
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
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newRerunInputTestRouter 创建「回退到助手待输入回合」端点测试路由引擎：装配与同目录会话 / 交互视图
// 测试同源（i18n、日志、核心库、命名空间、当前用户、AI 运行时），仅注册回退端点。
func newRerunInputTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group.POST("/agent/:sessionID/input/rerun", AgentAssistantInputRerun)
	return r
}

// rerunInputPath 拼接「回退到助手待输入回合」路径。
func rerunInputPath(id uint) string {
	return "/ai/agent/" + strconv.FormatUint(uint64(id), 10) + "/input/rerun"
}

// seedRerunInputFixture 播种一个含「历史助手待输入回合」的会话：一条用户提问后紧跟承载该回合的助手锚点
// （questionnaireJSON 非空时把该轮问卷落为锚点的 questionnaire 块并置 phase=suspended；为空且待执行调用非空
// 时把该轮待审批调用落为锚点的 approval 块），锚点之后另有一条助手消息（回退时应被删除），并播一条承载该
// 回合的运行现场（状态由 runStatus 指定，pendingCallsJSON 为该回合的挂起调用）。返回会话、锚点、锚点之后的
// 更晚消息与运行现场。
func seedRerunInputFixture(t *testing.T, db *database.DB, title, questionnaireJSON, pendingCallsJSON string, runStatus modelsai.AgentRunStatus) (*modelsai.AgentSession, *modelsai.AgentMessage, *modelsai.AgentMessage, *modelsai.AgentRun) {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), knowledgeTestNamespace, title, modelsai.SlotStandard, modelsai.AgentKindKnowledge, sessionTestUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role modelsai.MessageRole, msg *modelsai.AgentMessage) *modelsai.AgentMessage {
		t.Helper()
		msg.SessionID, msg.NamespaceName, msg.UserID, msg.Role = session.ID, knowledgeTestNamespace, sessionTestUserID, role
		saved, err := store.AppendMessage(context.Background(), msg)
		if err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
		return saved
	}
	_ = appendMsg(modelsai.MessageRoleUser, &modelsai.AgentMessage{Content: serializer.NewCrypto([]byte("u1"))})
	anchorAttrs := &modelsai.AgentMessage{
		Content:          serializer.NewCrypto([]byte("旧正文")),
		DraftJSON:        serializer.NewCrypto([]byte(`{"draft":1}`)),
		CitationsJSON:    serializer.NewCrypto([]byte(`[{"title":"旧引用"}]`)),
		PromptTokens:     11,
		CompletionTokens: 22,
		TotalTokens:      33,
		ElapsedMs:        44,
		Phase:            modelsai.AgentRunPhaseSuspended,
	}
	anchor := appendMsg(modelsai.MessageRoleAssistant, anchorAttrs)
	// 问卷回合：把该轮的题目与作答落为锚点的一条 questionnaire 块（block_seq=1），供回退重答据块取回题目
	// 与上次答案预填——旧 questionnaire_json 列已删除，思考 / 问卷一律改由块承载。
	if questionnaireJSON != "" {
		block := &modelsai.AgentMessageBlock{
			NamespaceName: knowledgeTestNamespace,
			UserID:        sessionTestUserID,
			MessageID:     anchor.ID,
			BlockSeq:      1,
			Kind:          modelsai.AgentMessageBlockKindQuestionnaire,
			Payload:       serializer.NewCrypto([]byte(questionnaireJSON)),
		}
		if err := store.AppendBlock(context.Background(), block); err != nil {
			t.Fatalf("AppendBlock error = %v", err)
		}
	}
	// 授权回合：把该轮的待审批调用落为锚点的一条 approval 块（block_seq=1），供回退重授权据块重建审批面板
	// ——授权清单同样改由块承载（与运行期 addSuspensionBlocks 同构：非问卷类调用一律标为需审批）。
	if strings.TrimSpace(questionnaireJSON) == "" && strings.TrimSpace(pendingCallsJSON) != "" && strings.TrimSpace(pendingCallsJSON) != "[]" {
		var records []*ai.ApprovalCallRecord
		if uerr := json.Unmarshal([]byte(pendingCallsJSON), &records); uerr != nil {
			t.Fatalf("unmarshal pending calls error = %v", uerr)
		}
		for _, record := range records {
			record.ApprovalRequired = true
		}
		payload, merr := json.Marshal(ai.ApprovalBlockPayload{Calls: records})
		if merr != nil {
			t.Fatalf("marshaling approval payload error = %v", merr)
		}
		block := &modelsai.AgentMessageBlock{
			NamespaceName: knowledgeTestNamespace,
			UserID:        sessionTestUserID,
			MessageID:     anchor.ID,
			BlockSeq:      1,
			Kind:          modelsai.AgentMessageBlockKindApproval,
			Payload:       serializer.NewCrypto(payload),
		}
		if err := store.AppendBlock(context.Background(), block); err != nil {
			t.Fatalf("AppendBlock error = %v", err)
		}
	}
	later := appendMsg(modelsai.MessageRoleAssistant, &modelsai.AgentMessage{Content: serializer.NewCrypto([]byte("退回后应被删除"))})
	if later.Seq <= anchor.Seq {
		t.Fatalf("test fixture error: the later message seq %d must exceed the anchor seq %d", later.Seq, anchor.Seq)
	}
	run := &modelsai.AgentRun{
		NamespaceName: knowledgeTestNamespace, UserID: sessionTestUserID, SessionID: session.ID,
		Status: runStatus, Slot: modelsai.SlotStandard, AnchorMessageID: anchor.ID,
		MessagesJSON:     serializer.NewCrypto([]byte(`[]`)),
		PendingCallsJSON: serializer.NewCrypto([]byte(pendingCallsJSON)),
		CitationsJSON:    serializer.NewCrypto([]byte(`[]`)),
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}
	if err := db.Omit("Namespace").Create(run).Error; err != nil {
		t.Fatalf("seed agent run error = %v", err)
	}
	return session, anchor, later, run
}

// seedLaterRerunRun 在同一会话下播种一条更晚的运行现场（id 大于 run），供断言「更晚现场回退后一律置为
// 已消费」。status 由调用方指定（真实场景常为 pending）。
func seedLaterRerunRun(t *testing.T, db *database.DB, sessionID uint, status modelsai.AgentRunStatus) *modelsai.AgentRun {
	t.Helper()
	run := &modelsai.AgentRun{
		NamespaceName: knowledgeTestNamespace, UserID: sessionTestUserID, SessionID: sessionID,
		Status: status, Slot: modelsai.SlotStandard,
		MessagesJSON:  serializer.NewCrypto([]byte(`[]`)),
		CitationsJSON: serializer.NewCrypto([]byte(`[]`)),
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	if err := db.Omit("Namespace").Create(run).Error; err != nil {
		t.Fatalf("seed later agent run error = %v", err)
	}
	return run
}

// reloadRerunRun 重新读取一条运行现场行。
func reloadRerunRun(t *testing.T, db *database.DB, runID uint) *modelsai.AgentRun {
	t.Helper()
	row := &modelsai.AgentRun{}
	if err := db.Where("id = ?", runID).First(row).Error; err != nil {
		t.Fatalf("reload agent run %d error = %v", runID, err)
	}
	return row
}

// reloadRerunMessage 重新读取一条消息行。
func reloadRerunMessage(t *testing.T, db *database.DB, messageID uint) *modelsai.AgentMessage {
	t.Helper()
	row := &modelsai.AgentMessage{}
	if err := db.Where("id = ?", messageID).First(row).Error; err != nil {
		t.Fatalf("reload message %d error = %v", messageID, err)
	}
	return row
}

// countMessagesAfter 返回会话内 seq 大于 afterSeq 的消息条数。
func countMessagesAfter(t *testing.T, db *database.DB, sessionID uint, afterSeq uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&modelsai.AgentMessage{}).
		Where("session_id = ? AND seq > ?", sessionID, afterSeq).Count(&count).Error; err != nil {
		t.Fatalf("counting messages after seq error = %v", err)
	}
	return count
}

// decodeRerunCode 解析响应体中的业务码。
func decodeRerunCode(t *testing.T, body []byte) int {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, string(body))
	}
	return resp.Code
}

// assertRerunSideEffects 断言三类真实副作用：锚点之后的消息被截断、锚点行本轮产出被清零并重置为
// suspended、承载现场被复位为 pending 且存活期限被刷新。
func assertRerunSideEffects(t *testing.T, db *database.DB, session *modelsai.AgentSession, anchor *modelsai.AgentMessage, run *modelsai.AgentRun, beforeExpires time.Time) {
	t.Helper()
	if remain := countMessagesAfter(t, db, session.ID, anchor.Seq); remain != 0 {
		t.Errorf("messages after the anchor = %d, want 0 (the later replies must be deleted)", remain)
	}
	reloaded := reloadRerunMessage(t, db, anchor.ID)
	if body := string(reloaded.Content.Value()); body != "" {
		t.Errorf("anchor.Content = %q, want empty", body)
	}
	if draft := string(reloaded.DraftJSON.Value()); draft != "" {
		t.Errorf("anchor.DraftJSON = %q, want empty", draft)
	}
	if citations := string(reloaded.CitationsJSON.Value()); citations != "" {
		t.Errorf("anchor.CitationsJSON = %q, want empty", citations)
	}
	if reloaded.PromptTokens != 0 || reloaded.CompletionTokens != 0 || reloaded.TotalTokens != 0 || reloaded.ElapsedMs != 0 {
		t.Errorf("anchor usage = %d/%d/%d/%d, want all zero",
			reloaded.PromptTokens, reloaded.CompletionTokens, reloaded.TotalTokens, reloaded.ElapsedMs)
	}
	if reloaded.Phase != modelsai.AgentRunPhaseSuspended {
		t.Errorf("anchor.Phase = %q, want %q", reloaded.Phase, modelsai.AgentRunPhaseSuspended)
	}
	resetRun := reloadRerunRun(t, db, run.ID)
	if resetRun.Status != modelsai.AgentRunStatusPending {
		t.Errorf("agent run status = %q, want %q", resetRun.Status, modelsai.AgentRunStatusPending)
	}
	if !resetRun.ExpiresAt.After(beforeExpires) {
		t.Errorf("agent run expiresAt = %v, want refreshed beyond %v", resetRun.ExpiresAt, beforeExpires)
	}
}

// TestAgentAssistantInputRerun_ReanswerQuestionnaire 端到端覆盖「回退到历史问卷待输入回合」：会话处于
// 暂停态（无在途活动、无 pending 现场），锚点是一条 phase=suspended 且带问卷载荷的助手消息、其后另有
// 一条更晚回复；回退端点应先经 CanRerunUserInput 放行，再截断更晚消息、清零并重置锚点、把承载现场复位
// 为 pending（刷新存活期限）、把更晚现场置为已消费，出参以 Questionnaire 返回题目（awaiting=true）与
// 待执行调用。
func TestAgentAssistantInputRerun_ReanswerQuestionnaire(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "20")
	rt := newEnabledAiRuntime(t, db)
	r := newRerunInputTestRouter(db, knowledgeTestNamespace, rt)

	payload, err := json.Marshal(ai.QuestionnairePayload{Questions: []ai.QuestionnaireQuestion{
		{ID: "q1", Question: "要查什么？", Options: []ai.QuestionnaireOption{{Text: "全量"}}},
	}})
	if err != nil {
		t.Fatalf("marshaling questionnaire payload error = %v", err)
	}
	const pendingCalls = `[{"callID":"c1","toolName":"ask_user","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"要查什么？\",\"options\":[{\"text\":\"全量\"}]}]}","execOwner":"client","auditLevel":"none"}]`
	session, anchor, _, run := seedRerunInputFixture(t, db, "重新回答问卷", string(payload), pendingCalls, modelsai.AgentRunStatusConsumed)
	laterRun := seedLaterRerunRun(t, db, session.ID, modelsai.AgentRunStatusPending)
	beforeExpires := run.ExpiresAt

	w := doKnowledgeJSONRequest(r, http.MethodPost, rerunInputPath(session.ID), map[string]any{"messageID": anchor.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("rerun status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Code int                          `json:"code"`
		Data *AgentAssistantInputRerunOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil {
		t.Fatalf("rerun data is nil (body=%s)", w.Body.String())
	}
	if resp.Data.Kind != string(modelsai.AgentInteractionSubstateAnswer) {
		t.Errorf("kind = %q, want %q", resp.Data.Kind, modelsai.AgentInteractionSubstateAnswer)
	}
	if len(resp.Data.Questionnaires) != 1 {
		t.Fatalf("questionnaires = %+v, want exactly one round", resp.Data.Questionnaires)
	}
	q := resp.Data.Questionnaires[0]
	if !q.Awaiting {
		t.Error("questionnaire.awaiting = false, want true")
	}
	if len(q.Questions) != 1 || q.Questions[0].ID != "q1" {
		t.Errorf("questionnaire.questions = %+v, want the anchor's questions", q.Questions)
	}
	if len(q.PendingCalls) != 1 || q.PendingCalls[0].CallID != "c1" || q.PendingCalls[0].ApprovalRequired {
		t.Errorf("questionnaire.pendingCalls = %+v, want the pending ask_user call exempt from approval", q.PendingCalls)
	}
	expected := ai.StateOf(ai.InteractionFacts{UserTurns: 1, MaxUserTurns: 20})
	if resp.Data.InteractionState != string(expected.State) {
		t.Errorf("interactionState = %q, want %q", resp.Data.InteractionState, expected.State)
	}
	assertRerunSideEffects(t, db, session, anchor, run, beforeExpires)
	// 锚点的 questionnaire 块须保留（供前端预填重开），不随锚点本轮产物清零。
	var blocks []modelsai.AgentMessageBlock
	if err := db.Where("message_id = ?", anchor.ID).Order("block_seq ASC").Find(&blocks).Error; err != nil {
		t.Fatalf("reload anchor blocks error = %v", err)
	}
	if len(blocks) != 1 || blocks[0].Kind != modelsai.AgentMessageBlockKindQuestionnaire ||
		len(blocks[0].Payload.Value()) == 0 {
		t.Errorf("anchor blocks = %+v, want the questionnaire block kept for the frontend to prefill", blocks)
	}
	if later := reloadRerunRun(t, db, laterRun.ID); later.Status != modelsai.AgentRunStatusConsumed {
		t.Errorf("later agent run status = %q, want %q", later.Status, modelsai.AgentRunStatusConsumed)
	}
}

// TestAgentAssistantInputRerun_ReanswerAnsweredQuestionnaire 覆盖「已作答问卷」的回退重答（回归用例）：
// 锚点载荷携带用户上一次的作答（真实场景——用户答完才发现要改），此时 FullyAnswered 为真、phase 亦为
// suspended。回退端点仍须把末轮置 awaiting=true 并下发待执行调用，否则出参按「末轮已作答」判定为 false、
// 前端会把面板渲染成已答卡片而不重开可交互问卷（用户报告的现象）。同时断言上次答案被保留以供预填。
func TestAgentAssistantInputRerun_ReanswerAnsweredQuestionnaire(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "20")
	rt := newEnabledAiRuntime(t, db)
	r := newRerunInputTestRouter(db, knowledgeTestNamespace, rt)

	payload, err := json.Marshal(ai.QuestionnairePayload{
		Questions: []ai.QuestionnaireQuestion{
			{ID: "q1", Question: "要查什么？", Options: []ai.QuestionnaireOption{{Text: "全量"}}},
		},
		Answers: []ai.QuestionnaireAnswer{{QuestionID: "q1", Answered: true, Answer: "全量"}},
	})
	if err != nil {
		t.Fatalf("marshaling answered questionnaire payload error = %v", err)
	}
	const pendingCalls = `[{"callID":"c1","toolName":"ask_user","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"要查什么？\",\"options\":[{\"text\":\"全量\"}]}]}","execOwner":"client","auditLevel":"none"}]`
	session, anchor, _, run := seedRerunInputFixture(t, db, "重答已作答问卷", string(payload), pendingCalls, modelsai.AgentRunStatusConsumed)
	beforeExpires := run.ExpiresAt

	w := doKnowledgeJSONRequest(r, http.MethodPost, rerunInputPath(session.ID), map[string]any{"messageID": anchor.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("rerun status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Code int                          `json:"code"`
		Data *AgentAssistantInputRerunOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil || len(resp.Data.Questionnaires) != 1 {
		t.Fatalf("questionnaires = %+v, want exactly one round", resp.Data)
	}
	q := resp.Data.Questionnaires[0]
	if !q.Awaiting {
		t.Error("questionnaire.awaiting = false, want true (the answered round must be reopened as awaiting)")
	}
	if len(q.Answers) != 1 || !q.Answers[0].Answered || q.Answers[0].Answer != "全量" {
		t.Errorf("questionnaire.answers = %+v, want the previous answers kept for prefill", q.Answers)
	}
	if len(q.PendingCalls) != 1 || q.PendingCalls[0].CallID != "c1" {
		t.Errorf("questionnaire.pendingCalls = %+v, want the pending ask_user call", q.PendingCalls)
	}
	assertRerunSideEffects(t, db, session, anchor, run, beforeExpires)
}

// TestAgentAssistantInputRerun_ReauthorizeTool 端到端覆盖「回退到历史工具授权回合」：锚点为无问卷载荷的
// 助手消息、现场调用为需审批的后端工具，回退端点同样截断与重置，出参以 PendingCalls 返回待审批调用清单
// （kind=approval，Questionnaire 为空）。
func TestAgentAssistantInputRerun_ReauthorizeTool(t *testing.T) {
	db := createKnowledgeTestDB(t)
	seedMaxUserTurns(t, db, "20")
	rt := newEnabledAiRuntime(t, db)
	r := newRerunInputTestRouter(db, knowledgeTestNamespace, rt)

	const pendingCalls = `[{"callID":"c2","toolName":"search_docs","arguments":"{\"query\":\"x\"}","execOwner":"backend","auditLevel":"must"}]`
	session, anchor, _, run := seedRerunInputFixture(t, db, "重新授权工具", "", pendingCalls, modelsai.AgentRunStatusConsumed)
	laterRun := seedLaterRerunRun(t, db, session.ID, modelsai.AgentRunStatusPending)
	beforeExpires := run.ExpiresAt

	w := doKnowledgeJSONRequest(r, http.MethodPost, rerunInputPath(session.ID), map[string]any{"messageID": anchor.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("rerun status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Code int                          `json:"code"`
		Data *AgentAssistantInputRerunOut `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v (body=%s)", err, w.Body.String())
	}
	if resp.Data == nil {
		t.Fatalf("rerun data is nil (body=%s)", w.Body.String())
	}
	if resp.Data.Kind != string(modelsai.AgentInteractionSubstateApproval) {
		t.Errorf("kind = %q, want %q", resp.Data.Kind, modelsai.AgentInteractionSubstateApproval)
	}
	if len(resp.Data.Questionnaires) != 0 {
		t.Errorf("questionnaires = %+v, want none for an approval turn", resp.Data.Questionnaires)
	}
	if len(resp.Data.PendingCalls) != 1 {
		t.Fatalf("pendingCalls = %+v, want exactly one pending call", resp.Data.PendingCalls)
	}
	call := resp.Data.PendingCalls[0]
	if call.CallID != "c2" || call.ToolName != "search_docs" || !call.ApprovalRequired {
		t.Errorf("pendingCall = %+v, want the search_docs call requiring approval", call)
	}
	expected := ai.StateOf(ai.InteractionFacts{UserTurns: 1, MaxUserTurns: 20})
	if resp.Data.InteractionState != string(expected.State) {
		t.Errorf("interactionState = %q, want %q", resp.Data.InteractionState, expected.State)
	}
	assertRerunSideEffects(t, db, session, anchor, run, beforeExpires)
	if later := reloadRerunRun(t, db, laterRun.ID); later.Status != modelsai.AgentRunStatusConsumed {
		t.Errorf("later agent run status = %q, want %q", later.Status, modelsai.AgentRunStatusConsumed)
	}
}

// TestAgentAssistantInputRerun_RejectsRunning 端到端覆盖「运行中 → 回退被拒 1125」：为该会话起一次阻塞的
// 在途活动（内存登记为 running），回退端点应先经 CanRerunUserInput 以 409 + 业务码 1125 拒绝，且未产生
// 任何改动（消息未截断、锚点未重置、现场未复位）。用例走 PostgreSQL 测试库（起活动会登记 agent_live_runs
// 含 TIMESTAMPTZ 列）。
func TestAgentAssistantInputRerun_RejectsRunning(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newRerunInputTestRouter(db, knowledgeTestNamespace, rt)
	const pendingCalls = `[{"callID":"c1","toolName":"ask_user","arguments":"{}","execOwner":"client","auditLevel":"none"}]`
	session, anchor, later, run := seedRerunInputFixture(t, db, "运行中回退", "", pendingCalls, modelsai.AgentRunStatusConsumed)

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
	// 起活动的瞬间 RunHub 会发布初始 state 事件并由后台落库 goroutine 写库；等这一条写库完成后落库
	// goroutine 即进入空闲，避免处理器的读库与后台写库在同一连接（单连接事务）上并发。
	time.Sleep(100 * time.Millisecond)
	defer func() {
		_ = rt.RunHub().Cancel(context.Background(), session.ID, sessionTestUserID, knowledgeTestNamespace)
	}()

	w := doKnowledgeJSONRequest(r, http.MethodPost, rerunInputPath(session.ID), map[string]any{"messageID": anchor.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("rerun(running) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeRerunCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("rerun(running) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if remain := countMessagesAfter(t, db, session.ID, anchor.Seq); remain != 1 {
		t.Errorf("messages after the anchor = %d, want 1 (nothing may be truncated on rejection)", remain)
	}
	if reloaded := reloadRerunMessage(t, db, anchor.ID); string(reloaded.Content.Value()) != "旧正文" {
		t.Errorf("anchor.Content = %q, want %q (the anchor must stay untouched)", string(reloaded.Content.Value()), "旧正文")
	}
	if got := reloadRerunRun(t, db, run.ID); got.Status != modelsai.AgentRunStatusConsumed {
		t.Errorf("agent run status = %q, want %q (the run must stay untouched)", got.Status, modelsai.AgentRunStatusConsumed)
	}
	if got := reloadRerunMessage(t, db, later.ID); string(got.Content.Value()) != "退回后应被删除" {
		t.Errorf("later message content = %q, want it kept on rejection", string(got.Content.Value()))
	}
}

// TestAgentAssistantInputRerun_RejectsWaiting 端到端覆盖「等待中 → 回退被拒 1125」：为该会话播一次真实
// 挂起现场（agent_runs 待恢复登记 + agent_live_runs 挂起登记、无内存活动），回退端点应先经
// CanRerunUserInput 以 409 + 业务码 1125 拒绝，且未产生任何改动。
func TestAgentAssistantInputRerun_RejectsWaiting(t *testing.T) {
	db := createKnowledgeTestDB(t)
	rt := newEnabledAiRuntime(t, db)
	r := newRerunInputTestRouter(db, knowledgeTestNamespace, rt)
	const pendingCalls = `[{"callID":"c1","toolName":"ask_user","arguments":"{}","execOwner":"client","auditLevel":"none"}]`
	session, anchor, later, run := seedRerunInputFixture(t, db, "等待中回退", "", pendingCalls, modelsai.AgentRunStatusPending)
	beforeExpires := run.ExpiresAt
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

	w := doKnowledgeJSONRequest(r, http.MethodPost, rerunInputPath(session.ID), map[string]any{"messageID": anchor.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("rerun(waiting) status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if code := decodeRerunCode(t, w.Body.Bytes()); code != response.CodeAgentInteractionBusy {
		t.Errorf("rerun(waiting) code = %d, want %d", code, response.CodeAgentInteractionBusy)
	}
	if remain := countMessagesAfter(t, db, session.ID, anchor.Seq); remain != 1 {
		t.Errorf("messages after the anchor = %d, want 1 (nothing may be truncated on rejection)", remain)
	}
	if reloaded := reloadRerunMessage(t, db, anchor.ID); string(reloaded.Content.Value()) != "旧正文" {
		t.Errorf("anchor.Content = %q, want %q (the anchor must stay untouched)", string(reloaded.Content.Value()), "旧正文")
	}
	resetRun := reloadRerunRun(t, db, run.ID)
	if resetRun.Status != modelsai.AgentRunStatusPending || !resetRun.ExpiresAt.Equal(beforeExpires) {
		t.Errorf("agent run = %q/%v, want unchanged %q/%v", resetRun.Status, resetRun.ExpiresAt, modelsai.AgentRunStatusPending, beforeExpires)
	}
	if got := reloadRerunMessage(t, db, later.ID); string(got.Content.Value()) != "退回后应被删除" {
		t.Errorf("later message content = %q, want it kept on rejection", string(got.Content.Value()))
	}
}
