package ai

import (
	// stdlib
	"context"
	"testing"

	// third-party
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// newTestSessionDB 构造会话测试用的 sqlite 内存库并迁移会话/消息两张表。
// newTestSessionDB builds an in-memory sqlite DB for session tests and migrates
// the session/message tables.
func newTestSessionDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(&modelsai.AgentSession{}, &modelsai.AgentMessage{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// TestNewSessionStore_NilDB 测试 db 为空时返回英文错误。
// TestNewSessionStore_NilDB verifies a nil db yields an English error.
func TestNewSessionStore_NilDB(t *testing.T) {
	if _, err := NewSessionStore(nil); err == nil {
		t.Fatal("NewSessionStore(nil) = nil error, want an error")
	}
}

// TestSessionStore_CreateGet 测试会话创建后字段落库、状态为 active，GetSession 往返一致。
// TestSessionStore_CreateGet verifies the created session fields persist with the
// active status and GetSession round-trips consistently.
func TestSessionStore_CreateGet(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), "default", "首轮对话", "gpt-4o", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if session.ID == 0 || session.Status != llm.SessionStatusActive {
		t.Fatalf("CreateSession() = %+v, want non-zero ID and active status", session)
	}
	got, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.NamespaceName != "default" || got.Title != "首轮对话" || got.TargetModel != "gpt-4o" {
		t.Errorf("GetSession() = %+v, want the created fields", got)
	}
	if _, err := store.GetSession(context.Background(), 99999); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(missing) error = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionStore_ListSessions 测试列表按更新时间倒序、分页生效、命名空间隔离。
// TestSessionStore_ListSessions verifies the list orders by update time
// descending, applies paging, and isolates namespaces.
func TestSessionStore_ListSessions(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.CreateSession(ctx, "default", "s", "m", "knowledge"); err != nil {
			t.Fatalf("CreateSession error = %v", err)
		}
	}
	if _, err := store.CreateSession(ctx, "other", "s", "m", "knowledge"); err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	all, err := store.ListSessions(ctx, "default", 0, 0)
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
	page, err := store.ListSessions(ctx, "default", 2, 1)
	if err != nil {
		t.Fatalf("ListSessions(page) error = %v", err)
	}
	if len(page) != 2 {
		t.Errorf("ListSessions(limit=2, offset=1) len = %d, want 2", len(page))
	}
}

// TestSessionStore_RenameAndStatus 测试重命名与状态更新后返回更新后的会话。
// TestSessionStore_RenameAndStatus verifies rename and status updates return the
// updated session.
func TestSessionStore_RenameAndStatus(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "old", "m", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	renamed, err := store.RenameSession(ctx, session.ID, "new title")
	if err != nil {
		t.Fatalf("RenameSession error = %v", err)
	}
	if renamed.Title != "new title" {
		t.Errorf("RenameSession() title = %q, want %q", renamed.Title, "new title")
	}
	archived, err := store.SetSessionStatus(ctx, session.ID, llm.SessionStatusArchived)
	if err != nil {
		t.Fatalf("SetSessionStatus error = %v", err)
	}
	if archived.Status != llm.SessionStatusArchived {
		t.Errorf("SetSessionStatus() status = %q, want %q", archived.Status, llm.SessionStatusArchived)
	}
	got, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Title != "new title" || got.Status != llm.SessionStatusArchived {
		t.Errorf("GetSession() = %+v, want the renamed and archived session", got)
	}
}

// TestSessionStore_DeleteCascadesMessages 测试删除会话时消息在同一事务内级联删除，
// 会话本身为软删除（GetSession 返回 ErrSessionNotFound）。
// TestSessionStore_DeleteCascadesMessages verifies deleting a session cascades
// its messages within one transaction and the session itself is soft-deleted
// (GetSession yields ErrSessionNotFound).
func TestSessionStore_DeleteCascadesMessages(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleUser, Content: "q",
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	if err := store.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession error = %v", err)
	}
	if _, err := store.GetSession(ctx, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetSession(after delete) error = %v, want ErrSessionNotFound", err)
	}
	messages, err := store.ListMessages(ctx, session.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("ListMessages(after delete) len = %d, want 0 (cascade delete)", len(messages))
	}
}

// TestSessionStore_Messages 测试消息追加/列表/历史（最近 N 条按时间升序返回）与
// 最近助手消息查询。
// TestSessionStore_Messages verifies message append/list, history (latest N in
// ascending order) and the latest-assistant-message lookup.
func TestSessionStore_Messages(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	if _, err := store.AppendMessage(context.Background(), nil); err == nil {
		t.Fatal("AppendMessage(nil) = nil error, want an error")
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "m", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role llm.MessageRole, content string) {
		t.Helper()
		if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", Role: role, Content: content,
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	appendMsg(llm.MessageRoleUser, "u1")
	appendMsg(llm.MessageRoleAssistant, "a1")
	appendMsg(llm.MessageRoleUser, "u2")

	all, err := store.ListMessages(ctx, session.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListMessages len = %d, want 3", len(all))
	}
	wantSeq := []string{"u1", "a1", "u2"}
	for i, m := range all {
		if m.Content != wantSeq[i] {
			t.Errorf("ListMessages()[%d] content = %q, want %q (ascending order)", i, m.Content, wantSeq[i])
		}
	}

	history, err := store.HistoryMessages(ctx, session.ID, 2)
	if err != nil {
		t.Fatalf("HistoryMessages error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("HistoryMessages(max=2) len = %d, want 2", len(history))
	}
	if history[0].Content != "a1" || history[1].Content != "u2" {
		t.Errorf("HistoryMessages() = [%s, %s], want [a1, u2] (latest N ascending)", history[0].Content, history[1].Content)
	}

	latest, err := store.LatestAssistantMessage(ctx, session.ID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if latest.Content != "a1" {
		t.Errorf("LatestAssistantMessage() content = %q, want %q", latest.Content, "a1")
	}
	if _, err := store.LatestAssistantMessage(ctx, 99999); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(missing) error = %v, want ErrMessageNotFound", err)
	}
}
