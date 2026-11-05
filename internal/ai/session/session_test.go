package session

import (
	// 标准库
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	// 第三方
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// encTestValue 把明文字符串包装为加密字段值（*serializer.Crypto），供测试构造
// AgentSession.Summary / AgentMessage.Content 等经 crypto 序列化器落库的字段。
func encTestValue(s string) *serializer.Crypto {
	return serializer.NewCrypto([]byte(s))
}

// newTestSessionDB 构造会话测试用的 sqlite 内存库并迁移会话/消息/持久化交互/长期记忆/挂起运行等表
// （会话删除级联用例需要这些表同库；sqlite 不支持 pgvector/tsvector，用例只覆盖空嵌入的路径）。
func newTestSessionDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(
		&ai.AgentSession{}, &ai.AgentMessage{}, &ai.AgentSessionCompaction{}, &ai.AgentMemory{}, &ai.AgentRun{},
		&ai.AgentCheckpoint{}, &ai.AgentInteraction{},
	); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestNewStore_NilDB 测试 db 为空时返回英文错误。
func TestNewStore_NilDB(t *testing.T) {
	if _, err := NewStore(nil); err == nil {
		t.Fatal("NewStore(nil) = nil error, want an error")
	}
}

// TestSessionStore_CreateGet 测试会话创建后字段落库、状态为 active，GetSession 往返一致。
func TestSessionStore_CreateGet(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), "default", "首轮对话", "gpt-4o", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if session.ID == 0 || session.Status != ai.SessionStatusActive {
		t.Fatalf("CreateSession() = %+v, want a non-zero ID and active status", session)
	}
	got, err := store.GetSession(context.Background(), session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.NamespaceName != "default" || got.Title != "首轮对话" || got.TargetModel != "gpt-4o" {
		t.Errorf("GetSession() = %+v, want the created fields", got)
	}
	if _, err := store.GetSession(context.Background(), 99999, "default", testUserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(missing) error = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionStore_ListSessions 测试列表按更新时间倒序、分页生效、命名空间隔离。
func TestSessionStore_ListSessions(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.CreateSession(ctx, "default", "s", "m", "knowledge", testUserID); err != nil {
			t.Fatalf("CreateSession error = %v", err)
		}
	}
	if _, err := store.CreateSession(ctx, "other", "s", "m", "knowledge", testUserID); err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	// 内部运行会话（周期报告）不进入助手会话列表
	if _, err := store.CreateSessionWithOrigin(ctx, "default", "报告计划：s", "m", "knowledge", ai.SessionOriginPeriodicReport, testUserID); err != nil {
		t.Fatalf("CreateSessionWithOrigin error = %v", err)
	}
	all, err := store.ListSessions(ctx, "default", testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListSessions error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListSessions(default) len = %d, want 3", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].UpdatedAt.After(all[i-1].UpdatedAt) {
			t.Errorf("ListSessions() not ordered by updated_at DESC at [%d]", i)
		}
	}
	page, err := store.ListSessions(ctx, "default", testUserID, 2, 1)
	if err != nil {
		t.Fatalf("ListSessions(page) error = %v", err)
	}
	if len(page) != 2 {
		t.Errorf("ListSessions(limit=2, offset=1) len = %d, want 2", len(page))
	}
}

// TestSessionStore_RenameAndStatus 测试重命名与状态更新后返回更新后的会话。
func TestSessionStore_RenameAndStatus(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "old", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	renamed, err := store.RenameSession(ctx, session.ID, "default", session.UserID, "new title")
	if err != nil {
		t.Fatalf("RenameSession error = %v", err)
	}
	if renamed.Title != "new title" {
		t.Errorf("RenameSession() title = %q, want %q", renamed.Title, "new title")
	}
	archived, err := store.SetSessionStatus(ctx, session.ID, "default", session.UserID, ai.SessionStatusArchived)
	if err != nil {
		t.Fatalf("SetSessionStatus error = %v", err)
	}
	if archived.Status != ai.SessionStatusArchived {
		t.Errorf("SetSessionStatus() status = %q, want %q", archived.Status, ai.SessionStatusArchived)
	}
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Title != "new title" || got.Status != ai.SessionStatusArchived {
		t.Errorf("GetSession() = %+v, want the renamed and archived session", got)
	}
}

// TestSessionStore_DeleteCascadesMessages 测试删除会话时消息、压缩检查点、长期记忆与挂起运行现场
// 在同一事务内硬删除，会话行本身亦为真实（硬）删除（GetSession 返回
// ErrSessionNotFound，且 agent_sessions 中不再有该行）。
func TestSessionStore_DeleteCascadesMessages(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("q"),
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	// 该会话的长期记忆与挂起运行现场各一条：删除会话须连同它们一并硬删除。
	if err := db.Exec("INSERT INTO agent_memories (session_id, namespace_name, user_id, agent_kind) VALUES (?, ?, ?, ?)",
		session.ID, "default", session.UserID, ai.AgentKindKnowledge).Error; err != nil {
		t.Fatalf("inserting an agent memory error = %v", err)
	}
	if err := db.Exec("INSERT INTO agent_runs (session_id, namespace_name, user_id, status, slot) VALUES (?, ?, ?, ?, ?)",
		session.ID, "default", session.UserID, ai.AgentRunStatusPending, ai.SlotStandard).Error; err != nil {
		t.Fatalf("inserting an agent run error = %v", err)
	}
	if err := store.DeleteSession(ctx, session.ID, "default", session.UserID); err != nil {
		t.Fatalf("DeleteSession error = %v", err)
	}
	if _, err := store.GetSession(ctx, session.ID, "default", session.UserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(after delete) error = %v, want ErrSessionNotFound", err)
	}
	messages, err := store.ListMessages(ctx, session.ID, session.NamespaceName, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("ListMessages(after delete) len = %d, want 0 (cascade delete)", len(messages))
	}
	// 长期记忆与运行现场须整行消失：按 session_id 计数不过滤 deleted_at，若只做软删除
	// （残留一行 deleted_at 非空）计数即非 0，从而暴露「未硬删除」的回归。
	assertNoRowsForSession(t, db, "agent_memories", session.ID)
	assertNoRowsForSession(t, db, "agent_runs", session.ID)
	// 会话行本身须真实删除（整行消失），而非软删除（deleted_at 置位、行仍在）。
	var sessionRows int64
	if err := db.Raw("SELECT count(*) FROM agent_sessions WHERE id = ?", session.ID).Scan(&sessionRows).Error; err != nil {
		t.Fatalf("counting the deleted session row error = %v", err)
	}
	if sessionRows != 0 {
		t.Errorf("agent_sessions rows = %d, want 0 (the session must be hard deleted, not soft deleted)", sessionRows)
	}
}

// assertNoRowsForSession 断言表中该会话的行已整行消失（不过滤 deleted_at，软删残留亦计入而报错）。
func assertNoRowsForSession(t *testing.T, db *database.DB, table string, sessionID uint) {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT count(*) FROM "+table+" WHERE session_id = ?", sessionID).Scan(&count).Error; err != nil {
		t.Fatalf("counting %s rows error = %v", table, err)
	}
	if count != 0 {
		t.Errorf("%s rows for the deleted session = %d, want 0 (hard delete)", table, count)
	}
}

// TestSessionStore_Messages 测试消息追加/列表/历史（最近 N 条按时间升序返回）与
// 最近助手消息查询。
func TestSessionStore_Messages(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	if _, err := store.AppendMessage(context.Background(), nil); err == nil {
		t.Fatal("AppendMessage(nil) = nil error, want an error")
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string) {
		t.Helper()
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content),
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	appendMsg(ai.MessageRoleUser, "u1")
	appendMsg(ai.MessageRoleAssistant, "a1")
	appendMsg(ai.MessageRoleUser, "u2")

	all, err := store.ListMessages(ctx, session.ID, session.NamespaceName, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListMessages len = %d, want 3", len(all))
	}
	wantSeq := []string{"u1", "a1", "u2"}
	for i, m := range all {
		if got := string(m.Content.Value()); got != wantSeq[i] {
			t.Errorf("ListMessages()[%d] content = %q, want %q (ascending order)", i, got, wantSeq[i])
		}
	}

	history, err := store.HistoryMessages(ctx, session.ID, session.NamespaceName, session.UserID, 2)
	if err != nil {
		t.Fatalf("HistoryMessages error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("HistoryMessages(max=2) len = %d, want 2", len(history))
	}
	if string(history[0].Content.Value()) != "a1" || string(history[1].Content.Value()) != "u2" {
		t.Errorf("HistoryMessages() = [%s, %s], want [a1, u2] (latest N ascending)", history[0].Content.Value(), history[1].Content.Value())
	}

	latest, err := store.LatestAssistantMessage(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if string(latest.Content.Value()) != "a1" {
		t.Errorf("LatestAssistantMessage() content = %q, want %q", latest.Content.Value(), "a1")
	}
	if _, err := store.LatestAssistantMessage(ctx, 99999, "default", testUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(missing) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_ContextUsage_SkipsUnmeasured 测试上下文占用口径：只认「有用量记录」的助手消息
// （prompt_tokens > 0），跳过被取消 / 失败 / 挂起而从未记下用量的助手消息、回落到末次真实测量；全无
// 测量时返回 0（「尚无测量」，不是「占用为 0」），单会话与批量两个口径一致。会话尚无压缩推进时，
// 展示口径（ContextUsage）与压缩基线口径（ContextTokens）取同一值。
func TestSessionStore_ContextUsage_SkipsUnmeasured(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(msgSessionID uint, role ai.MessageRole, promptTokens int) {
		t.Helper()
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: msgSessionID, NamespaceName: "default", UserID: testUserID, Role: role,
			Content: encTestValue("c"), PromptTokens: promptTokens,
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	// 首轮未记下用量（取消落库）→ 次轮真实测量 300 → 末轮又未记下用量（最新一轮回复被取消）
	appendMsg(session.ID, ai.MessageRoleUser, 0)
	appendMsg(session.ID, ai.MessageRoleAssistant, 0)
	appendMsg(session.ID, ai.MessageRoleUser, 0)
	appendMsg(session.ID, ai.MessageRoleAssistant, 300)
	appendMsg(session.ID, ai.MessageRoleUser, 0)
	appendMsg(session.ID, ai.MessageRoleAssistant, 0)

	got, err := store.ContextTokens(ctx, session.ID, session.NamespaceName, testUserID)
	if err != nil {
		t.Fatalf("ContextTokens error = %v", err)
	}
	if got != 300 {
		t.Errorf("ContextTokens() = %d, want the last measured 300 (skipping the unmeasured latest)", got)
	}
	usage, err := store.ContextUsage(ctx, session.ID, session.NamespaceName, testUserID)
	if err != nil {
		t.Fatalf("ContextUsage error = %v", err)
	}
	if usage != 300 {
		t.Errorf("ContextUsage() = %d, want the last measured 300 (no compaction yet)", usage)
	}
	bySession, err := store.ContextUsageBySession(ctx, "default", testUserID, []uint{session.ID})
	if err != nil {
		t.Fatalf("ContextUsageBySession error = %v", err)
	}
	if bySession[session.ID] != 300 {
		t.Errorf("ContextUsageBySession()[%d] = %d, want 300", session.ID, bySession[session.ID])
	}

	// 全无测量的会话：单会话返回 0，批量结果中不出现（调用方缺省为 0）
	unmeasured, err := store.CreateSession(ctx, "default", "t2", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg(unmeasured.ID, ai.MessageRoleAssistant, 0)
	if got, err := store.ContextUsage(ctx, unmeasured.ID, unmeasured.NamespaceName, testUserID); err != nil || got != 0 {
		t.Errorf("ContextUsage(unmeasured) = (%d, %v), want (0, nil)", got, err)
	}
	bySession, err = store.ContextUsageBySession(ctx, "default", testUserID, []uint{unmeasured.ID})
	if err != nil {
		t.Fatalf("ContextUsageBySession error = %v", err)
	}
	if _, ok := bySession[unmeasured.ID]; ok {
		t.Errorf("ContextUsageBySession contains the unmeasured session %d, want it absent", unmeasured.ID)
	}
	if empty, err := store.ContextUsageBySession(ctx, "default", testUserID, nil); err != nil || len(empty) != 0 {
		t.Errorf("ContextUsageBySession(nil) = (%v, %v), want an empty map", empty, err)
	}
}

// TestSessionStore_ContextUsage_PrefersLaterCompactionEstimate 测试展示占用的取舍：压缩晚于末次真实
// 测量时取该次压缩的即时估算（压缩后尚无生成落定，此时真实测量仍是压缩前的旧值）；此后任一生成落定
// （助手消息行更晚落库）即由真实测量接管；不带估算的推进（静默滚动）不参与取舍，仍取末次真实测量。
func TestSessionStore_ContextUsage_PrefersLaterCompactionEstimate(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	anchor, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: testUserID, Role: ai.MessageRoleAssistant,
		Content: encTestValue("a1"), PromptTokens: 6670,
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	// 压缩晚于末次测量：展示占用取压缩后的估算 4100（压缩基线口径仍取真实测量 6670）。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s1", uint(anchor.Seq), "manual",
		contract.CompactionMetrics{TokensBefore: 6670, FoldedCount: 2, TokensAfter: 4100}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}
	if got, err := store.ContextUsage(ctx, session.ID, session.NamespaceName, testUserID); err != nil || got != 4100 {
		t.Errorf("ContextUsage() = (%d, %v), want the post-compaction estimate 4100", got, err)
	}
	if got, err := store.ContextTokens(ctx, session.ID, session.NamespaceName, testUserID); err != nil || got != 6670 {
		t.Errorf("ContextTokens() = (%d, %v), want the untouched measurement baseline 6670", got, err)
	}
	// 批量口径与单会话一致。
	if bySession, err := store.ContextUsageBySession(ctx, "default", testUserID, []uint{session.ID}); err != nil || bySession[session.ID] != 4100 {
		t.Errorf("ContextUsageBySession() = (%d, %v), want 4100", bySession[session.ID], err)
	}

	// 压缩后的一次生成落定：新的助手消息行更晚落库，展示占用由真实测量接管。
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: testUserID, Role: ai.MessageRoleAssistant,
		Content: encTestValue("a2"), PromptTokens: 4300,
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if got, err := store.ContextUsage(ctx, session.ID, session.NamespaceName, testUserID); err != nil || got != 4300 {
		t.Errorf("ContextUsage() = (%d, %v), want the fresh measurement 4300", got, err)
	}
	if bySession, err := store.ContextUsageBySession(ctx, "default", testUserID, []uint{session.ID}); err != nil || bySession[session.ID] != 4300 {
		t.Errorf("ContextUsageBySession() = (%d, %v), want 4300", bySession[session.ID], err)
	}

	// 不带估算的推进（静默滚动）：不参与取舍，展示占用仍取末次真实测量。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s2", uint(anchor.Seq), "rolling",
		contract.CompactionMetrics{TokensBefore: 4300, FoldedCount: 1}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint(rolling) error = %v", err)
	}
	if got, err := store.ContextUsage(ctx, session.ID, session.NamespaceName, testUserID); err != nil || got != 4300 {
		t.Errorf("ContextUsage(rolling advance) = (%d, %v), want the measurement 4300", got, err)
	}
}

// TestSessionStore_AssistantSeqAtMessage 测试按助手消息 ID 定位该回答的会话内 seq：命中助手消息返回其
// seq；消息不存在、不属于该会话与属主、或不是助手消息时均返回 ErrMessageNotFound。
func TestSessionStore_AssistantSeqAtMessage(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string) *ai.AgentMessage {
		t.Helper()
		msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content),
		})
		if err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
		return msg
	}
	// 会话内容：u1(1) a1(2) u2(3) a2(4) u3(5)。
	u1 := appendMsg(ai.MessageRoleUser, "u1")
	a1 := appendMsg(ai.MessageRoleAssistant, "a1")
	appendMsg(ai.MessageRoleUser, "u2")
	a2 := appendMsg(ai.MessageRoleAssistant, "a2")
	appendMsg(ai.MessageRoleUser, "u3")

	if got, err := store.AssistantSeqAtMessage(ctx, session.ID, session.NamespaceName, session.UserID, a1.ID); err != nil || got != a1.Seq {
		t.Errorf("AssistantSeqAtMessage(a1) = (%d, %v), want (%d, nil)", got, err, a1.Seq)
	}
	if got, err := store.AssistantSeqAtMessage(ctx, session.ID, session.NamespaceName, session.UserID, a2.ID); err != nil || got != a2.Seq {
		t.Errorf("AssistantSeqAtMessage(a2) = (%d, %v), want (%d, nil)", got, err, a2.Seq)
	}
	// 消息不存在视为未找到。
	if _, err := store.AssistantSeqAtMessage(ctx, session.ID, session.NamespaceName, session.UserID, 99999); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AssistantSeqAtMessage(missing) error = %v, want ErrMessageNotFound", err)
	}
	// 非助手消息（用户提问）视为未找到。
	if _, err := store.AssistantSeqAtMessage(ctx, session.ID, session.NamespaceName, session.UserID, u1.ID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AssistantSeqAtMessage(user message) error = %v, want ErrMessageNotFound", err)
	}
	// 属主不符视为未找到（跨用户隔离）。
	if _, err := store.AssistantSeqAtMessage(ctx, session.ID, session.NamespaceName, testUserID+1, a1.ID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AssistantSeqAtMessage(other user) error = %v, want ErrMessageNotFound", err)
	}
}

// newConcurrentTestSessionDB 构造并发用例专用的 sqlite 库：普通 ":memory:" 下每条新连接都是独立
// 空库（连接池并发取到新连接即 no such table），故改用临时文件库 + busy_timeout(10s)，使多个连接
// 看到同一份库、写冲突时按超时串行重试，从而在真实并发下检验 seq 的原子分配。
func newConcurrentTestSessionDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "session_seq_concurrent.db") + "?_pragma=busy_timeout(10000)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(
		&ai.AgentSession{}, &ai.AgentMessage{}, &ai.AgentSessionCompaction{}, &ai.AgentMemory{}, &ai.AgentRun{},
		&ai.AgentCheckpoint{}, &ai.AgentInteraction{},
	); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestSessionStore_AppendMessage_SeqMonotonic 测试同一会话连续追加 N 条消息时 seq 依次为 1..N。
func TestSessionStore_AppendMessage_SeqMonotonic(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	const n = 5
	for i := 1; i <= n; i++ {
		msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("m"),
		})
		if err != nil {
			t.Fatalf("AppendMessage(%d) error = %v", i, err)
		}
		if msg.Seq != uint64(i) {
			t.Errorf("AppendMessage(%d).Seq = %d, want %d", i, msg.Seq, i)
		}
	}
}

// TestSessionStore_AppendMessage_SeqConcurrent 测试同一会话并发追加 N 条消息时序号全局串行：
// 全部成功且 seq 集合恰为 1..N、无重复。
func TestSessionStore_AppendMessage_SeqConcurrent(t *testing.T) {
	store, err := NewStore(newConcurrentTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	const n = 20
	seqs := make([]uint64, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
				SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("m"),
			})
			if err != nil {
				errs[i] = err
				return
			}
			seqs[i] = msg.Seq
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("AppendMessage(goroutine %d) error = %v", i, err)
		}
	}
	seen := make(map[uint64]bool, n)
	for _, seq := range seqs {
		if seq < 1 || seq > n {
			t.Fatalf("seq = %d, want it within 1..%d", seq, n)
		}
		if seen[seq] {
			t.Fatalf("duplicate seq %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != n {
		t.Fatalf("distinct seq count = %d, want %d", len(seen), n)
	}
}

// TestSessionStore_AppendMessage_MissingSession 测试对不存在的会话追加消息返回 ErrSessionNotFound。
func TestSessionStore_AppendMessage_MissingSession(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	_, err = store.AppendMessage(context.Background(), &ai.AgentMessage{
		SessionID: 99999, NamespaceName: "default", UserID: testUserID, Role: ai.MessageRoleUser, Content: encTestValue("orphan"),
	})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("AppendMessage(missing session) error = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionStore_TruncateMessagesFromUserMessage 测试从某一轮用户消息起截断会话消息：该轮提问
// 及其之后的全部消息被删除、之前的消息保留（定位仅按用户消息计数）；序号越界返回
// ErrMessageNotFound，序号小于 1 直接报错。
func TestSessionStore_TruncateMessagesFromUserMessage(t *testing.T) {
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{
		ai.MessageRoleUser, ai.MessageRoleAssistant,
		ai.MessageRoleUser, ai.MessageRoleAssistant,
		ai.MessageRoleUser, ai.MessageRoleAssistant,
	}
	contents := []string{"u1", "a1", "u2", "a2", "u3", "a3"}
	for i, role := range roles {
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(contents[i]),
		}); err != nil {
			t.Fatalf("AppendMessage(%d) error = %v", i, err)
		}
	}
	// 从第 2 轮提问起截断：仅保留 u1 / a1。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 2); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	remain, err := store.ListMessages(ctx, session.ID, session.NamespaceName, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(remain) != 2 {
		t.Fatalf("remaining message count = %d, want 2", len(remain))
	}
	for i, want := range []string{"u1", "a1"} {
		if got := string(remain[i].Content.Value()); got != want {
			t.Errorf("remaining[%d].Content = %q, want %q", i, got, want)
		}
	}
	// 截断后仅剩 1 轮提问，序号 5 越界。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 5); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("TruncateMessagesFromUserMessage(out of range) error = %v, want ErrMessageNotFound", err)
	}
	// 非法序号直接报错。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 0); err == nil {
		t.Error("TruncateMessagesFromUserMessage(0) = nil error, want an error")
	}
}

// TestSessionStore_TruncateRemovesInteractionsOfDeletedTurns 测试截断时一并删除锚点落在被截断区间内的
// 持久化交互及其 redo checkpoint：交互卡片依附于助手消息，残留会让历史回放复现已回退的内容。
func TestSessionStore_TruncateRemovesInteractionsOfDeletedTurns(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMessage := func(role ai.MessageRole, content string) *ai.AgentMessage {
		t.Helper()
		row, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content),
		})
		if err != nil {
			t.Fatalf("AppendMessage(%s) error = %v", content, err)
		}
		return row
	}
	appendMessage(ai.MessageRoleUser, "u1")
	keptAnchor := appendMessage(ai.MessageRoleAssistant, "a1")
	appendMessage(ai.MessageRoleUser, "u2")
	removedAnchor := appendMessage(ai.MessageRoleAssistant, "a2")

	payload, err := json.Marshal(contract.NextStepsPayload{Steps: []contract.NextStep{{Title: "t", Text: "next"}}})
	if err != nil {
		t.Fatalf("marshal next steps payload error = %v", err)
	}
	appendInteraction := func(anchor *ai.AgentMessage) *ai.AgentInteraction {
		t.Helper()
		item, err := store.AppendInteraction(ctx, &ai.AgentInteraction{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
			MessageID: anchor.ID, Kind: ai.InteractionKindNextSteps, Status: ai.InteractionStatusAnswered,
			Payload: serializer.NewCrypto(payload),
		})
		if err != nil {
			t.Fatalf("AppendInteraction error = %v", err)
		}
		return item
	}
	kept := appendInteraction(keptAnchor)
	removed := appendInteraction(removedAnchor)
	for _, interaction := range []*ai.AgentInteraction{kept, removed} {
		if err := db.Create(&ai.AgentCheckpoint{
			CheckpointID: RedoCheckpointID(session.ID, interaction.ID),
			Payload:      encTestValue("redo"),
		}).Error; err != nil {
			t.Fatalf("create redo checkpoint error = %v", err)
		}
	}

	// 从第 2 轮提问起截断：仅保留 u1 / a1 与其交互。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 2); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	items, err := store.ListInteractions(ctx, session.ID, session.UserID)
	if err != nil {
		t.Fatalf("ListInteractions error = %v", err)
	}
	if len(items) != 1 || items[0].ID != kept.ID {
		t.Fatalf("remaining interactions = %+v, want only %d", items, kept.ID)
	}
	for _, tc := range []struct {
		interaction *ai.AgentInteraction
		want        int64
	}{
		{kept, 1},
		{removed, 0},
	} {
		var count int64
		if err := db.Model(&ai.AgentCheckpoint{}).
			Where("checkpoint_id = ?", RedoCheckpointID(session.ID, tc.interaction.ID)).
			Count(&count).Error; err != nil {
			t.Fatalf("count redo checkpoint error = %v", err)
		}
		if count != tc.want {
			t.Errorf("redo checkpoint count for interaction %d = %d, want %d", tc.interaction.ID, count, tc.want)
		}
	}
}

// appendSessionMessages 按给定角色/正文序列为会话追加消息（seq 依次递增），供压缩检查点与回滚用例复用。
func appendSessionMessages(t *testing.T, store *Store, session *ai.AgentSession, roles []ai.MessageRole, contents []string) {
	t.Helper()
	for i, role := range roles {
		if _, err := store.AppendMessage(context.Background(), &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: session.NamespaceName, UserID: session.UserID, Role: role, Content: encTestValue(contents[i]),
		}); err != nil {
			t.Fatalf("AppendMessage(%d) error = %v", i, err)
		}
	}
}

// compactionUptoSeqs 按 upto_seq 升序直读会话的压缩检查点游标，供回滚用例断言检查点的增删。
func compactionUptoSeqs(t *testing.T, db *database.DB, sessionID uint) []uint64 {
	t.Helper()
	var seqs []uint64
	if err := db.Raw("SELECT upto_seq FROM agent_session_compactions WHERE session_id = ? ORDER BY upto_seq ASC", sessionID).
		Scan(&seqs).Error; err != nil {
		t.Fatalf("listing compaction checkpoints error = %v", err)
	}
	return seqs
}

// TestSessionStore_UpdateSummaryAndCheckpoint 测试推进摘要时在同一事务内更新会话摘要/游标并追加一条
// 压缩检查点：检查点记录推进后的游标与摘要全文（密文落库，回读解密可还原），reason 一并记录。
func TestSessionStore_UpdateSummaryAndCheckpoint(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("u1"),
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "summary-1", uint(msg.Seq), "rolling", contract.CompactionMetrics{TokensBefore: 4096, FoldedCount: 3, TokensAfter: 2048}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}
	var checkpoints []ai.AgentSessionCompaction
	if err := db.Where("session_id = ?", session.ID).Order("upto_seq ASC").Find(&checkpoints).Error; err != nil {
		t.Fatalf("loading compaction checkpoints error = %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint count = %d, want 1", len(checkpoints))
	}
	if checkpoints[0].UptoSeq != msg.Seq || checkpoints[0].Reason != "rolling" || string(checkpoints[0].Summary.Value()) != "summary-1" {
		t.Errorf("checkpoint = %+v, want (upto_seq=%d, rolling, summary-1)", checkpoints[0], msg.Seq)
	}
	// 展示度量（含压缩后占用的即时估算）随检查点落库，供刷新后回显历史压缩记录与前后对比。
	if checkpoints[0].TokensBefore != 4096 || checkpoints[0].FoldedCount != 3 || checkpoints[0].TokensAfter != 2048 {
		t.Errorf("checkpoint metrics = (tokens_before=%d, folded_count=%d, tokens_after=%d), want (4096, 3, 2048)",
			checkpoints[0].TokensBefore, checkpoints[0].FoldedCount, checkpoints[0].TokensAfter)
	}
	// 检查点摘要列为密文：直读原始列不得出现明文。
	var row struct {
		Summary []byte
	}
	if err := db.Raw("SELECT summary FROM agent_session_compactions WHERE id = ?", checkpoints[0].ID).Scan(&row).Error; err != nil {
		t.Fatalf("reading the raw checkpoint summary error = %v", err)
	}
	assertEncryptedColumn(t, "agent_session_compactions.summary", row.Summary, "summary-1")
	// 会话摘要与游标同事务更新。
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "summary-1" || got.SummaryUptoSeq != uint(msg.Seq) {
		t.Errorf("persisted session = (%q, %d), want the advanced summary", got.Summary.Value(), got.SummaryUptoSeq)
	}
}

// TestSessionStore_TruncateRollsBackToEarlierCheckpoint 测试回滚到某次压缩之前时，起点回退到
// 「upto_seq < 回滚点」的最近一个检查点的摘要与游标，并作废 upto_seq ≥ 回滚点的检查点。
func TestSessionStore_TruncateRollsBackToEarlierCheckpoint(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2", "u3", "a3"})
	// 两次推进：seq=2 记为 s2（第一轮后）、seq=4 记为 s4（第二轮后）。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s2", 2, "rolling", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint(2) error = %v", err)
	}
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint(4) error = %v", err)
	}
	// 回滚到第 2 轮提问（u2，seq=3）：当前游标 4 ≥ 3，回退到 upto_seq < 3 的最近检查点（seq=2）。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 2); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "s2" || got.SummaryUptoSeq != 2 {
		t.Errorf("session after rollback = (%q, %d), want the earlier checkpoint (s2, 2)", got.Summary.Value(), got.SummaryUptoSeq)
	}
	if seqs := compactionUptoSeqs(t, db, session.ID); len(seqs) != 1 || seqs[0] != 2 {
		t.Errorf("remaining checkpoints = %v, want just [2] (the >= rollback point ones are discarded)", seqs)
	}
}

// TestSessionStore_TruncateRollsBackToStart 测试回滚到第一轮提问时无更早检查点，摘要被清空、
// summary_upto_seq 置 0，且全部检查点被作废。
func TestSessionStore_TruncateRollsBackToStart(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 1); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if len(got.Summary.Value()) != 0 || got.SummaryUptoSeq != 0 {
		t.Errorf("session after rollback to start = (%q, %d), want an empty summary and cursor 0", got.Summary.Value(), got.SummaryUptoSeq)
	}
	if seqs := compactionUptoSeqs(t, db, session.ID); len(seqs) != 0 {
		t.Errorf("remaining checkpoints = %v, want none", seqs)
	}
}

// TestSessionStore_TruncateKeepsSummaryWhenNotCovered 测试回滚点未被摘要覆盖（summary_upto_seq
// < 起始 seq）时摘要、游标与检查点均保持不变。
func TestSessionStore_TruncateKeepsSummaryWhenNotCovered(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	// 游标仅推进到 seq=2（覆盖第一轮），回滚点在第 2 轮（seq=3）未被覆盖。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s2", 2, "rolling", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 2); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "s2" || got.SummaryUptoSeq != 2 {
		t.Errorf("session after uncovered rollback = (%q, %d), want the summary unchanged (s2, 2)", got.Summary.Value(), got.SummaryUptoSeq)
	}
	if seqs := compactionUptoSeqs(t, db, session.ID); len(seqs) != 1 || seqs[0] != 2 {
		t.Errorf("remaining checkpoints = %v, want [2] unchanged", seqs)
	}
}

// TestSessionStore_TruncateKeepsMemories 测试回滚（含起点回退）不删除压缩期间抽取的长期记忆。
func TestSessionStore_TruncateKeepsMemories(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}
	if err := db.Exec("INSERT INTO agent_memories (session_id, namespace_name, user_id, agent_kind) VALUES (?, ?, ?, ?)",
		session.ID, "default", session.UserID, ai.AgentKindKnowledge).Error; err != nil {
		t.Fatalf("inserting an agent memory error = %v", err)
	}
	// 回滚到第一轮：无更早检查点，清空摘要；长期记忆须保留。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, session.UserID, 1); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM agent_memories WHERE session_id = ?", session.ID).Scan(&count).Error; err != nil {
		t.Fatalf("counting agent memories error = %v", err)
	}
	if count != 1 {
		t.Errorf("agent memories after rollback = %d, want 1 (rollback must not delete long-term memory)", count)
	}
}

// TestSessionStore_CrossUserIsolation 测试会话与消息的跨用户隔离：同一命名空间下，用户只能
// 列举、读取、重命名属于自己的会话；对他人会话的读取/重命名视为不存在（ErrSessionNotFound），
// 删除他人会话不生效，按他人会话 ID 读取消息为空。
func TestSessionStore_CrossUserIsolation(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	owner, err := store.CreateSession(ctx, "default", "owner", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession(owner) error = %v", err)
	}
	if _, err := store.CreateSession(ctx, "default", "intruder", "m", "knowledge", testOtherUserID); err != nil {
		t.Fatalf("CreateSession(other) error = %v", err)
	}

	// 同一命名空间下，各自的列表只含自己的会话
	mine, err := store.ListSessions(ctx, "default", testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListSessions(owner) error = %v", err)
	}
	if len(mine) != 1 || mine[0].ID != owner.ID {
		t.Errorf("ListSessions(owner) = %+v, want only the owner's session", mine)
	}

	// 同一命名空间下直读他人会话视为不存在
	if _, err := store.GetSession(ctx, owner.ID, "default", testOtherUserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(cross-user) error = %v, want ErrSessionNotFound", err)
	}
	// 重命名他人会话视为不存在，原会话标题不变
	if _, err := store.RenameSession(ctx, owner.ID, "default", testOtherUserID, "hacked"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("RenameSession(cross-user) error = %v, want ErrSessionNotFound", err)
	}
	// 删除他人会话不生效：会话仍在且标题未被改动
	if err := store.DeleteSession(ctx, owner.ID, "default", testOtherUserID); err != nil {
		t.Fatalf("DeleteSession(cross-user) error = %v, want nil (no rows affected)", err)
	}
	got, err := store.GetSession(ctx, owner.ID, "default", testUserID)
	if err != nil {
		t.Fatalf("GetSession(owner) error = %v, want the session to survive the cross-user delete", err)
	}
	if got.Title != "owner" {
		t.Errorf("GetSession(owner).Title = %q, want the original title", got.Title)
	}

	// 会话消息同样按属主隔离：他人按同一会话 ID 读取不到任何消息
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: owner.ID, NamespaceName: "default", UserID: owner.UserID, Role: ai.MessageRoleUser, Content: encTestValue("secret"),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	leaked, err := store.ListMessages(ctx, owner.ID, owner.NamespaceName, testOtherUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages(cross-user) error = %v", err)
	}
	if len(leaked) != 0 {
		t.Errorf("ListMessages(cross-user) len = %d, want 0", len(leaked))
	}
	if _, err := store.LatestAssistantMessage(ctx, owner.ID, owner.NamespaceName, testOtherUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(cross-user) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_NamespaceIsolationOnMessages 测试消息读写的命名空间纵深校验：即使会话 ID 与属主
// 正确，命名空间不符时读取为空 / 视为不存在，追加与更新直接拒绝。
func TestSessionStore_NamespaceIsolationOnMessages(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "ns", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: session.NamespaceName, UserID: testUserID,
		Role: ai.MessageRoleUser, Content: encTestValue("question"),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	native, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: session.NamespaceName, UserID: testUserID,
		Role: ai.MessageRoleAssistant, Payload: serializer.NewCrypto([]byte(`{"role":"assistant","content":"answer"}`)),
	})
	if err != nil {
		t.Fatalf("AppendNativeMessage error = %v", err)
	}

	// 错误命名空间下：消息列表为空，单条读取 / 最近助手 / 轮次 / 用量均视为不存在或 0。
	leaked, err := store.ListMessages(ctx, session.ID, "other", testUserID, 0, 0)
	if err != nil || len(leaked) != 0 {
		t.Errorf("ListMessages(other namespace) = (%d, %v), want (0, nil)", len(leaked), err)
	}
	if _, err := store.GetMessage(ctx, native.ID, "other", testUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("GetMessage(other namespace) error = %v, want ErrMessageNotFound", err)
	}
	if _, err := store.LatestAssistantMessage(ctx, session.ID, "other", testUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(other namespace) error = %v, want ErrMessageNotFound", err)
	}
	if turns, err := store.CountUserTurns(ctx, session.ID, "other", testUserID); err != nil || turns != 0 {
		t.Errorf("CountUserTurns(other namespace) = (%d, %v), want (0, nil)", turns, err)
	}
	if tokens, err := store.ContextTokens(ctx, session.ID, "other", testUserID); err != nil || tokens != 0 {
		t.Errorf("ContextTokens(other namespace) = (%d, %v), want (0, nil)", tokens, err)
	}

	// 写入路径同样校验：追加时命名空间与会话不符按会话不存在拒绝，更新时按消息不存在拒绝。
	if _, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "other", UserID: testUserID,
		Role: ai.MessageRoleAssistant, Payload: serializer.NewCrypto([]byte(`{"role":"assistant"}`)),
	}); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("AppendNativeMessage(other namespace) error = %v, want ErrSessionNotFound", err)
	}
	native.NamespaceName = "other"
	if err := store.UpdateNativeMessage(ctx, native); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("UpdateNativeMessage(other namespace) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_EncryptedAtRest 测试会话与消息的涉敏字段确实以密文落库：直读原始列既不得
// 出现明文，也应长于 AES-GCM 最小密文长度；同时经模型回读仍能解密还原（含 UpdateSummaryAndCheckpoint
// 的「结构体 + Select」写路径，防止退回 map 更新而绕过序列化器）。
func TestSessionStore_EncryptedAtRest(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        session.UserID,
		Role:          ai.MessageRoleAssistant,
		Content:       encTestValue("secret-content"),
		DraftJSON:     encTestValue(`{"draft":"secret-draft"}`),
		CitationsJSON: encTestValue(`[{"title":"secret-citation"}]`),
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "secret-summary", uint(msg.Seq), "rolling", contract.CompactionMetrics{}); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint error = %v", err)
	}

	var sessionRow struct {
		Summary []byte
	}
	if err := db.Raw("SELECT summary FROM agent_sessions WHERE id = ?", session.ID).Scan(&sessionRow).Error; err != nil {
		t.Fatalf("reading the raw summary column error = %v", err)
	}
	assertEncryptedColumn(t, "agent_sessions.summary", sessionRow.Summary, "secret-summary")

	var messageRow struct {
		Content       []byte
		DraftJSON     []byte
		CitationsJSON []byte
	}
	if err := db.Raw("SELECT content, draft_json, citations_json FROM agent_messages WHERE id = ?", msg.ID).
		Scan(&messageRow).Error; err != nil {
		t.Fatalf("reading the raw message columns error = %v", err)
	}
	assertEncryptedColumn(t, "agent_messages.content", messageRow.Content, "secret-content")
	assertEncryptedColumn(t, "agent_messages.draft_json", messageRow.DraftJSON, "secret-draft")
	assertEncryptedColumn(t, "agent_messages.citations_json", messageRow.CitationsJSON, "secret-citation")

	// 回读经序列化器解密，明文可原样还原（含 UpdateSummary 写入的摘要）。
	got, err := store.GetSession(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "secret-summary" {
		t.Errorf("GetSession().Summary = %q, want %q", got.Summary.Value(), "secret-summary")
	}
	latest, err := store.LatestAssistantMessage(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if string(latest.Content.Value()) != "secret-content" {
		t.Errorf("LatestAssistantMessage().Content = %q, want %q", latest.Content.Value(), "secret-content")
	}
}

// assertEncryptedColumn 断言原始列既不包含明文，也长于 AES-GCM 最小密文长度（nonce 12 + tag 16）。
func assertEncryptedColumn(t *testing.T, column string, raw []byte, plaintext string) {
	t.Helper()
	if len(raw) < 28 {
		t.Errorf("%s raw length = %d, want an encrypted value (>= 28 bytes)", column, len(raw))
	}
	if bytes.Contains(raw, []byte(plaintext)) {
		t.Errorf("%s raw value leaks the plaintext %q", column, plaintext)
	}
}
