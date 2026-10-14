package ai

import (
	// 标准库

	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	// 第三方
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部
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

// newTestSessionDB 构造会话测试用的 sqlite 内存库并迁移会话/消息/长期记忆/挂起运行四张表
// （会话删除级联用例需要后两者同库；sqlite 不支持 pgvector/tsvector，用例只覆盖空嵌入的路径）。
func newTestSessionDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(
		&ai.AgentSession{}, &ai.AgentMessage{}, &ai.AgentSessionCompaction{}, &ai.AgentMemory{}, &ai.AgentRun{},
		&ai.AgentLiveRun{}, &ai.AgentRunEvent{},
	); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestNewSessionStore_NilDB 测试 db 为空时返回英文错误。
func TestNewSessionStore_NilDB(t *testing.T) {
	if _, err := NewSessionStore(nil); err == nil {
		t.Fatal("NewSessionStore(nil) = nil error, want an error")
	}
}

// TestSessionStore_CreateGet 测试会话创建后字段落库、状态为 active，GetSession 往返一致。
func TestSessionStore_CreateGet(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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

// TestSessionStore_DeleteCascadesMessages 测试删除会话时消息、压缩检查点、长期记忆、挂起运行现场
// 与驻留运行登记及事件在同一事务内硬删除，会话行本身亦为真实（硬）删除（GetSession 返回
// ErrSessionNotFound，且 agent_sessions 中不再有该行）。
func TestSessionStore_DeleteCascadesMessages(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	if err := db.Exec("INSERT INTO agent_runs (session_id, namespace_name, user_id, status, slot, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		session.ID, "default", session.UserID, ai.AgentRunStatusPending, ai.SlotStandard, time.Now()).Error; err != nil {
		t.Fatalf("inserting an agent run error = %v", err)
	}
	// 该会话的驻留运行登记与事件日志各一条：删除会话须连同它们一并硬删除。
	if err := db.Exec("INSERT INTO agent_live_runs (session_id, namespace_name, user_id, run_id, status, last_seq, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		session.ID, "default", session.UserID, "11111111-1111-1111-1111-111111111111", ai.AgentRunPhaseRunning, 0, time.Now()).Error; err != nil {
		t.Fatalf("inserting an agent live run error = %v", err)
	}
	if err := db.Exec("INSERT INTO agent_run_events (session_id, namespace_name, user_id, run_id, seq, type) VALUES (?, ?, ?, ?, ?, ?)",
		session.ID, "default", session.UserID, "11111111-1111-1111-1111-111111111111", 1, ai.AgentRunEventDelta).Error; err != nil {
		t.Fatalf("inserting an agent run event error = %v", err)
	}
	if err := store.DeleteSession(ctx, session.ID, "default", session.UserID); err != nil {
		t.Fatalf("DeleteSession error = %v", err)
	}
	if _, err := store.GetSession(ctx, session.ID, "default", session.UserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(after delete) error = %v, want ErrSessionNotFound", err)
	}
	messages, err := store.ListMessages(ctx, session.ID, session.UserID, 0, 0)
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
	assertNoRowsForSession(t, db, "agent_live_runs", session.ID)
	assertNoRowsForSession(t, db, "agent_run_events", session.ID)
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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

	all, err := store.ListMessages(ctx, session.ID, session.UserID, 0, 0)
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

	history, err := store.HistoryMessages(ctx, session.ID, session.UserID, 2)
	if err != nil {
		t.Fatalf("HistoryMessages error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("HistoryMessages(max=2) len = %d, want 2", len(history))
	}
	if string(history[0].Content.Value()) != "a1" || string(history[1].Content.Value()) != "u2" {
		t.Errorf("HistoryMessages() = [%s, %s], want [a1, u2] (latest N ascending)", history[0].Content.Value(), history[1].Content.Value())
	}

	latest, err := store.LatestAssistantMessage(ctx, session.ID, session.UserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if string(latest.Content.Value()) != "a1" {
		t.Errorf("LatestAssistantMessage() content = %q, want %q", latest.Content.Value(), "a1")
	}
	if _, err := store.LatestAssistantMessage(ctx, 99999, testUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(missing) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_AssistantSeqAtUserMessage 测试按「第 N 条用户消息」定位该轮助手回复的 seq：
// 正常命中返回其后首条助手消息 seq；序号越界或其后尚无助手回复均返回 ErrMessageNotFound。
func TestSessionStore_AssistantSeqAtUserMessage(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string) uint64 {
		t.Helper()
		msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content),
		})
		if err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
		return msg.Seq
	}
	// 会话内容：u1(1) a1(2) u2(3) a2(4) u3(5) —— u3 之后无助手回复。
	appendMsg(ai.MessageRoleUser, "u1")
	a1 := appendMsg(ai.MessageRoleAssistant, "a1")
	appendMsg(ai.MessageRoleUser, "u2")
	a2 := appendMsg(ai.MessageRoleAssistant, "a2")
	appendMsg(ai.MessageRoleUser, "u3")

	if got, err := store.AssistantSeqAtUserMessage(ctx, session.ID, session.UserID, 1); err != nil || got != a1 {
		t.Errorf("AssistantSeqAtUserMessage(1) = (%d, %v), want (%d, nil)", got, err, a1)
	}
	if got, err := store.AssistantSeqAtUserMessage(ctx, session.ID, session.UserID, 2); err != nil || got != a2 {
		t.Errorf("AssistantSeqAtUserMessage(2) = (%d, %v), want (%d, nil)", got, err, a2)
	}
	// 越界与「无助手回复」均视为未找到。
	if _, err := store.AssistantSeqAtUserMessage(ctx, session.ID, session.UserID, 99); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AssistantSeqAtUserMessage(out of range) error = %v, want ErrMessageNotFound", err)
	}
	if _, err := store.AssistantSeqAtUserMessage(ctx, session.ID, session.UserID, 3); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AssistantSeqAtUserMessage(no assistant reply) error = %v, want ErrMessageNotFound", err)
	}
	if _, err := store.AssistantSeqAtUserMessage(ctx, session.ID, session.UserID, 0); err == nil {
		t.Error("AssistantSeqAtUserMessage(index < 1) = nil error, want an error")
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
		&ai.AgentLiveRun{}, &ai.AgentRunEvent{},
	); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestSessionStore_AppendMessage_SeqMonotonic 测试同一会话连续追加 N 条消息时 seq 依次为 1..N。
func TestSessionStore_AppendMessage_SeqMonotonic(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	store, err := NewSessionStore(newConcurrentTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	remain, err := store.ListMessages(ctx, session.ID, session.UserID, 0, 0)
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

// appendSessionMessages 按给定角色/正文序列为会话追加消息（seq 依次递增），供压缩检查点与回滚用例复用。
func appendSessionMessages(t *testing.T, store *SessionStore, session *ai.AgentSession, roles []ai.MessageRole, contents []string) {
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
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "summary-1", uint(msg.Seq), "rolling"); err != nil {
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
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2", "u3", "a3"})
	// 两次推进：seq=2 记为 s2（第一轮后）、seq=4 记为 s4（第二轮后）。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s2", 2, "rolling"); err != nil {
		t.Fatalf("UpdateSummaryAndCheckpoint(2) error = %v", err)
	}
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto"); err != nil {
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
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto"); err != nil {
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
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	// 游标仅推进到 seq=2（覆盖第一轮），回滚点在第 2 轮（seq=3）未被覆盖。
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s2", 2, "rolling"); err != nil {
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
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	roles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}
	appendSessionMessages(t, store, session, roles, []string{"u1", "a1", "u2", "a2"})
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "s4", 4, "auto"); err != nil {
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
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	leaked, err := store.ListMessages(ctx, owner.ID, testOtherUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages(cross-user) error = %v", err)
	}
	if len(leaked) != 0 {
		t.Errorf("ListMessages(cross-user) len = %d, want 0", len(leaked))
	}
	if _, err := store.LatestAssistantMessage(ctx, owner.ID, testOtherUserID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(cross-user) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_EncryptedAtRest 测试会话与消息的涉敏字段确实以密文落库：直读原始列既不得
// 出现明文，也应长于 AES-GCM 最小密文长度；同时经模型回读仍能解密还原（含 UpdateSummaryAndCheckpoint
// 的「结构体 + Select」写路径，防止退回 map 更新而绕过序列化器）。
func TestSessionStore_EncryptedAtRest(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
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
	if err := store.UpdateSummaryAndCheckpoint(ctx, session, "secret-summary", uint(msg.Seq), "rolling"); err != nil {
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
	latest, err := store.LatestAssistantMessage(ctx, session.ID, session.UserID)
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
