package ai

import (
	// 标准库

	"bytes"
	"context"
	"testing"

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

// newTestSessionDB 构造会话测试用的 sqlite 内存库并迁移会话/消息两张表。
func newTestSessionDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(&ai.AgentSession{}, &ai.AgentMessage{}); err != nil {
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
	session, err := store.CreateSession(context.Background(), "default", "首轮对话", "gpt-4o", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if session.ID == 0 || session.Status != ai.SessionStatusActive {
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
	archived, err := store.SetSessionStatus(ctx, session.ID, ai.SessionStatusArchived)
	if err != nil {
		t.Fatalf("SetSessionStatus error = %v", err)
	}
	if archived.Status != ai.SessionStatusArchived {
		t.Errorf("SetSessionStatus() status = %q, want %q", archived.Status, ai.SessionStatusArchived)
	}
	got, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Title != "new title" || got.Status != ai.SessionStatusArchived {
		t.Errorf("GetSession() = %+v, want the renamed and archived session", got)
	}
}

// TestSessionStore_DeleteCascadesMessages 测试删除会话时消息在同一事务内级联删除，
// 会话本身为软删除（GetSession 返回 ErrSessionNotFound）。
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
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", Role: ai.MessageRoleUser, Content: encTestValue("q"),
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
	appendMsg := func(role ai.MessageRole, content string) {
		t.Helper()
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", Role: role, Content: encTestValue(content),
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	appendMsg(ai.MessageRoleUser, "u1")
	appendMsg(ai.MessageRoleAssistant, "a1")
	appendMsg(ai.MessageRoleUser, "u2")

	all, err := store.ListMessages(ctx, session.ID, 0, 0)
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

	history, err := store.HistoryMessages(ctx, session.ID, 2)
	if err != nil {
		t.Fatalf("HistoryMessages error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("HistoryMessages(max=2) len = %d, want 2", len(history))
	}
	if string(history[0].Content.Value()) != "a1" || string(history[1].Content.Value()) != "u2" {
		t.Errorf("HistoryMessages() = [%s, %s], want [a1, u2] (latest N ascending)", history[0].Content.Value(), history[1].Content.Value())
	}

	latest, err := store.LatestAssistantMessage(ctx, session.ID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if string(latest.Content.Value()) != "a1" {
		t.Errorf("LatestAssistantMessage() content = %q, want %q", latest.Content.Value(), "a1")
	}
	if _, err := store.LatestAssistantMessage(ctx, 99999); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("LatestAssistantMessage(missing) error = %v, want ErrMessageNotFound", err)
	}
}

// TestSessionStore_EncryptedAtRest 测试会话与消息的涉敏字段确实以密文落库：直读原始列既不得
// 出现明文，也应长于 AES-GCM 最小密文长度；同时经模型回读仍能解密还原（含 UpdateSummary 的
// 「结构体 + Select」写路径，防止退回 map 更新而绕过序列化器）。
func TestSessionStore_EncryptedAtRest(t *testing.T) {
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		Role:          ai.MessageRoleAssistant,
		Content:       encTestValue("secret-content"),
		DraftJSON:     encTestValue(`{"draft":"secret-draft"}`),
		CitationsJSON: encTestValue(`[{"title":"secret-citation"}]`),
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if err := store.UpdateSummary(ctx, session.ID, "secret-summary", msg.ID); err != nil {
		t.Fatalf("UpdateSummary error = %v", err)
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
	got, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "secret-summary" {
		t.Errorf("GetSession().Summary = %q, want %q", got.Summary.Value(), "secret-summary")
	}
	latest, err := store.LatestAssistantMessage(ctx, session.ID)
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
