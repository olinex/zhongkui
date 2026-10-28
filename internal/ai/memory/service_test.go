package memory

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	sessionstore "zhongkuisec.com/zhongkui/internal/ai/session" // 标准库

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// newScriptedChat 构造按次序返回给定回复的对话替身。
func newScriptedChat(replies ...string) *scriptedChat {
	turns := make([]scriptedTurn, 0, len(replies))
	for _, reply := range replies {
		turns = append(turns, scriptedTurn{content: reply})
	}
	return &scriptedChat{turns: turns}
}

// errEmbedder 是向量化必然失败的 embedder 替身：使 Store.Write 在向量化阶段即失败（不
// 触达 DB），从而在无 pgvector 的 sqlite 下稳定检验「长期记忆抽取失败只进 Warnings」。
type errEmbedder struct{}

// EmbedStrings 固定返回错误，用于模拟向量化失败。
func (errEmbedder) EmbedStrings(_ context.Context, _ []string, _ ...embedding.Option) ([][]float64, error) {
	return nil, errors.New("embedder: forced failure")
}

// newTestMemory 构造基于 sqlite 内存库的 Memory 服务：embed 为向量化替身，chat 为 flash 档位
// 对话替身（为 nil 时解析器缺失，用于检验摘要失败降级），overrides 叠加在测试参数之上。
func newTestMemory(t *testing.T, db *database.DB, chat model.BaseChatModel, embed port.Embedder, overrides map[string]string) *Service {
	t.Helper()
	params := aitest.Params(t, overrides)
	store, err := NewStore(db, embed, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	sessions, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	var resolver port.ChatResolver
	if chat != nil {
		resolver = slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: chat})
	}
	mem, err := NewService(store, sessions, resolver, params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	return mem
}

// TestNewMemory_NilArgs 测试 Memory 构造的空值校验：store/sessions/params 为空返回英文错误，
// resolver 与 logger 允许为空。
func TestNewMemory_NilArgs(t *testing.T) {
	if _, err := NewService(nil, nil, nil, nil, nil); err == nil {
		t.Error("NewService(nil, nil, nil, nil, nil) = nil error, want an error")
	}
	params := aitest.Params(t, nil)
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db, aitest.StubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	sessions, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	if _, err := NewService(store, nil, nil, nil, nil); err == nil {
		t.Error("NewService(store, nil, nil, nil, nil) = nil error, want an error")
	}
	if _, err := NewService(nil, sessions, nil, params, nil); err == nil {
		t.Error("NewService(nil, sessions, nil, params, nil) = nil error, want an error")
	}
	// resolver 与 logger 允许为空
	if _, err := NewService(store, sessions, nil, params, nil); err != nil {
		t.Errorf("NewService(store, sessions, nil, params, nil) error = %v, want nil", err)
	}
}

// TestSummarize_Advances 测试滚动摘要推进：返回模型回复（去首尾空白）与被挤出消息的最后一条
// seq，且请求携带配置的摘要系统提示与「既有摘要 + 被挤出消息」的用户提示。
func TestSummarize_Advances(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("  new summary  ")
	mem := newTestMemory(t, aitest.NewSessionDB(t), chat, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "summary system prompt",
	})
	session := &ai.AgentSession{NamespaceName: "default", Summary: encTestValue("old summary"), SummaryUptoSeq: 3}
	evicted := []*ai.AgentMessage{
		{Seq: 4, Role: ai.MessageRoleUser, Content: encTestValue("u")},
		{Seq: 5, Role: ai.MessageRoleAssistant, Content: encTestValue("a")},
	}
	summary, upto, err := mem.Summarize(ctx, session, evicted)
	if err != nil {
		t.Fatalf("Summarize error = %v", err)
	}
	if summary != "new summary" || upto != 5 {
		t.Errorf("Summarize() = (%q, %d), want (%q, 5)", summary, upto, "new summary")
	}
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	req := chat.gotInputs[0]
	if len(req) != 2 || req[0].Content != "summary system prompt" {
		t.Fatalf("summary request = %+v, want the system prompt plus the user prompt", req)
	}
	for _, want := range []string{"old summary", "user: u", "assistant: a"} {
		if !strings.Contains(req[1].Content, want) {
			t.Errorf("summary user prompt missing %q: %q", want, req[1].Content)
		}
	}
}

// TestSummarize_Errors 测试摘要的错误与短路路径：nil 会话、缺失解析器、模型空回复均返回英文
// 错误；被挤出消息为空时原样返回既有摘要与游标且不调用模型。
func TestSummarize_Errors(t *testing.T) {
	ctx := context.Background()
	mem := newTestMemory(t, aitest.NewSessionDB(t), newScriptedChat("x"), aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	if _, _, err := mem.Summarize(ctx, nil, nil); err == nil {
		t.Error("Summarize(nil session) = nil error, want an error")
	}
	session := &ai.AgentSession{Summary: encTestValue("keep"), SummaryUptoSeq: 7}
	summary, upto, err := mem.Summarize(ctx, session, nil)
	if err != nil || summary != "keep" || upto != 7 {
		t.Errorf("Summarize(no evicted) = (%q, %d, %v), want (keep, 7, nil)", summary, upto, err)
	}
	noResolver := newTestMemory(t, aitest.NewSessionDB(t), nil, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	evicted := []*ai.AgentMessage{{Role: ai.MessageRoleUser, Content: encTestValue("c")}}
	if _, _, err := noResolver.Summarize(ctx, &ai.AgentSession{}, evicted); err == nil {
		t.Error("Summarize(nil resolver) = nil error, want an error")
	}
	empty := newTestMemory(t, aitest.NewSessionDB(t), newScriptedChat("   "), aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	if _, _, err := empty.Summarize(ctx, &ai.AgentSession{}, evicted); err == nil {
		t.Error("Summarize(empty reply) = nil error, want an error")
	}
}

// TestSummarizeStream_DeltasAndCallbackError 测试流式摘要：onDelta 非空时逐块回调的增量拼接
// 等于最终摘要（游标同 Summarize）；回调返回错误时立即中止并透传该错误。
func TestSummarizeStream_DeltasAndCallbackError(t *testing.T) {
	ctx := context.Background()
	session := &ai.AgentSession{NamespaceName: "default", Summary: encTestValue("old summary"), SummaryUptoSeq: 3}
	evicted := []*ai.AgentMessage{
		{Seq: 4, Role: ai.MessageRoleUser, Content: encTestValue("u")},
		{Seq: 5, Role: ai.MessageRoleAssistant, Content: encTestValue("a")},
	}
	chat := &stubChat{chunks: []string{"folded ", "summary"}}
	mem := newTestMemory(t, aitest.NewSessionDB(t), chat, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
	})
	var deltas []string
	summary, upto, _, err := mem.SummarizeStream(ctx, session, evicted, func(d string) error {
		deltas = append(deltas, d)
		return nil
	})
	if err != nil {
		t.Fatalf("SummarizeStream error = %v", err)
	}
	if summary != "folded summary" || upto != 5 {
		t.Errorf("SummarizeStream() = (%q, %d), want (%q, 5)", summary, upto, "folded summary")
	}
	if strings.Join(deltas, "") != summary {
		t.Errorf("deltas = %q, want them to concatenate to %q", strings.Join(deltas, ""), summary)
	}

	wantErr := errors.New("summary sink closed")
	errMem := newTestMemory(t, aitest.NewSessionDB(t), &stubChat{chunks: []string{"a", "b"}}, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
	})
	if _, _, _, err := errMem.SummarizeStream(ctx, session, evicted, func(string) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("SummarizeStream(callback error) = %v, want %v", err, wantErr)
	}
}

// TestGenerateTopic_GeneratesAndTruncates 测试会话主题生成：模型回复经去首尾空白后作为主题返回，
// 超过 SessionTopicByteLimit 时按字节截断并追加省略号；请求携带配置的主题系统提示与用户的首条消息。
func TestGenerateTopic_GeneratesAndTruncates(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("  " + strings.Repeat("a", 100) + "  ")
	mem := newTestMemory(t, aitest.NewSessionDB(t), chat, aitest.StubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "topic system prompt",
	})
	got := mem.GenerateTopic(ctx, "  用户的首条问题  ")
	want := strings.Repeat("a", SessionTopicByteLimit) + "..."
	if got != want {
		t.Errorf("GenerateTopic() = %q, want %q", got, want)
	}
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	req := chat.gotInputs[0]
	if len(req) != 2 || req[0].Content != "topic system prompt" || req[1].Content != "用户的首条问题" {
		t.Fatalf("topic request = %+v, want the topic system prompt plus the trimmed query", req)
	}
}

// TestGenerateTopic_Fallbacks 测试主题生成的兜底路径：空 query 返回空串且不调用模型；解析器缺失、
// 模型空响应一律回退为「首条消息按 SessionTopicByteLimit 截断」，且绝不返回错误。
func TestGenerateTopic_Fallbacks(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("topic")
	mem := newTestMemory(t, aitest.NewSessionDB(t), chat, aitest.StubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	if got := mem.GenerateTopic(ctx, "   "); got != "" {
		t.Errorf("GenerateTopic(empty query) = %q, want an empty string", got)
	}
	if len(chat.gotInputs) != 0 {
		t.Errorf("chat calls for an empty query = %d, want 0", len(chat.gotInputs))
	}
	noResolver := newTestMemory(t, aitest.NewSessionDB(t), nil, aitest.StubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	empty := newTestMemory(t, aitest.NewSessionDB(t), newScriptedChat("   "), aitest.StubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	query := strings.Repeat("问", 30)
	want := SessionTopicFallback(query)
	for name, m := range map[string]*Service{"no resolver": noResolver, "empty reply": empty} {
		if got := m.GenerateTopic(ctx, query); got != want {
			t.Errorf("GenerateTopic(%s) = %q, want the fallback %q", name, got, want)
		}
	}
}

// TestSessionTopicFallback_UTF8Boundary 测试兜底主题的 UTF-8 边界：超限的多字节消息按 rune 边界
// 截断并追加省略号，不产生残缺字符；恰好等于上限时不截断也不追加省略号。
func TestSessionTopicFallback_UTF8Boundary(t *testing.T) {
	got := SessionTopicFallback(strings.Repeat("中", 30))
	if !utf8.ValidString(got) {
		t.Errorf("SessionTopicFallback() = %q, want a valid UTF-8 string", got)
	}
	if !strings.HasSuffix(got, "...") || len(got) > SessionTopicByteLimit+3 {
		t.Errorf("SessionTopicFallback() = %q, want at most %d bytes plus an ellipsis",
			got, SessionTopicByteLimit)
	}
	exact := strings.Repeat("a", SessionTopicByteLimit)
	if got := SessionTopicFallback(exact); got != exact {
		t.Errorf("SessionTopicFallback(exact) = %q, want it unchanged", got)
	}
}

// failingContextTokensSessions 是读取压缩前占用必然失败的会话存储替身：消息读取正常，但
// ContextTokens 返回错误，用于验证 Compact 不再吞掉度量读取错误。
type failingContextTokensSessions struct{}

// ListMessagesAfter 返回一条待折叠消息，使 Compact 继续到 ContextTokens 读取。
func (failingContextTokensSessions) ListMessagesAfter(_ context.Context, _ uint, _ string, _ uint, _ uint) ([]*ai.AgentMessage, error) {
	return []*ai.AgentMessage{{Seq: 1}}, nil
}

// ContextTokens 固定返回错误。
func (failingContextTokensSessions) ContextTokens(_ context.Context, _ uint, _ string, _ uint) (int, error) {
	return 0, errors.New("context tokens unavailable")
}

// UpdateSummaryAndCheckpoint 不应被调用（ContextTokens 已失败）。
func (failingContextTokensSessions) UpdateSummaryAndCheckpoint(_ context.Context, _ *ai.AgentSession, _ string, _ uint, _ string, _ contract.CompactionMetrics) error {
	return nil
}

// TestCompact_ContextTokensErrorPropagates 测试压缩前占用读取失败时 Compact 返回错误而不是
// 把失败吞成 TokensBefore=0：0 只表示「尚无测量」，不能同时表示「读取失败」。
func TestCompact_ContextTokensErrorPropagates(t *testing.T) {
	mem := &Service{sessions: failingContextTokensSessions{}}
	result, err := mem.Compact(context.Background(), &ai.AgentSession{ID: 1, NamespaceName: "default", UserID: 1})
	if err == nil {
		t.Fatal("Compact() error = nil, want the ContextTokens failure to propagate")
	}
	if result != nil {
		t.Errorf("Compact() result = %+v, want nil", result)
	}
	if !strings.Contains(err.Error(), "context tokens unavailable") {
		t.Errorf("Compact() error = %v, want the ContextTokens failure", err)
	}
}

// TestCompact_ExtractionFailureOnlyWarnings 测试抽取落库失败只进 Warnings：摘要成功落库并返回，
// Extracted 为 0、Warnings 非空，且摘要与抽取各调用模型一次。
func TestCompact_ExtractionFailureOnlyWarnings(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("u1"),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	chat := newScriptedChat(
		"folded summary",
		`{"memories":[{"kind":"fact","title":"t1","content":"c1"}]}`,
	)
	mem := newTestMemory(t, db, chat, errEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
		"memory.extractSystemPrompt": "ep",
		"memory.extractMaxItems":     "5",
		"memory.enabled":             "true",
	})
	result, err := mem.Compact(ctx, session)
	if err != nil {
		t.Fatalf("Compact error = %v", err)
	}
	if result == nil || result.Summary != "folded summary" {
		t.Fatalf("Compact() = %+v, want the folded summary with no error", result)
	}
	if result.Extracted != 0 {
		t.Errorf("Compact() extracted = %d, want 0 (the write failed)", result.Extracted)
	}
	if len(result.Warnings) == 0 {
		t.Error("Compact() warnings empty, want the extraction write failure to be surfaced")
	}
	if len(chat.gotInputs) != 2 {
		t.Errorf("chat calls = %d, want 2 (summary + extraction)", len(chat.gotInputs))
	}
	if got := chat.gotInputs[1][0].Content; got != "ep" {
		t.Errorf("extraction system prompt = %q, want the configured prompt", got)
	}
	got, err := store.GetSession(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "folded summary" || got.SummaryUptoSeq == 0 {
		t.Errorf("persisted summary = (%q, %d), want the folded summary with an advanced cursor",
			got.Summary.Value(), got.SummaryUptoSeq)
	}
}

// TestCompact_DisabledSkipsExtraction 测试长期记忆开关关闭时仍刷新摘要但跳过抽取：模型仅被调用
// 一次（摘要），Extracted 为 0 且带一条开关关闭告警。
func TestCompact_DisabledSkipsExtraction(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("u1"),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	chat := newScriptedChat("folded summary")
	mem := newTestMemory(t, db, chat, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
		"memory.enabled":             "false",
	})
	result, err := mem.Compact(ctx, session)
	if err != nil {
		t.Fatalf("Compact error = %v", err)
	}
	if result == nil || result.Summary != "folded summary" || result.Extracted != 0 {
		t.Fatalf("Compact() = %+v, want the folded summary and no extraction", result)
	}
	if len(chat.gotInputs) != 1 {
		t.Errorf("chat calls = %d, want 1 (the extraction is skipped)", len(chat.gotInputs))
	}
	if len(result.Warnings) == 0 {
		t.Error("Compact() warnings empty, want the disabled-extraction notice")
	}
}

// TestCompact_NoMessages 测试游标之后无消息时不调用模型、原样返回既有摘要。
func TestCompact_NoMessages(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	session.Summary = encTestValue("existing")
	chat := newScriptedChat("unused")
	mem := newTestMemory(t, db, chat, aitest.StubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
	})
	result, err := mem.Compact(ctx, session)
	if err != nil {
		t.Fatalf("Compact error = %v", err)
	}
	if result == nil || result.Summary != "existing" || result.Extracted != 0 {
		t.Fatalf("Compact() = %+v, want the existing summary and no extraction", result)
	}
	if len(chat.gotInputs) != 0 {
		t.Errorf("chat calls = %d, want 0", len(chat.gotInputs))
	}
}

// TestCompact_SummaryFailure 测试摘要失败时返回 (nil, err)：flash 档位不可解析（解析器缺失）
// 即整体失败，不落库、不抽取。
func TestCompact_SummaryFailure(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := sessionstore.NewStore(db)
	if err != nil {
		t.Fatalf("sessionstore.NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("u1"),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	mem := newTestMemory(t, db, nil, aitest.StubEmbedder{}, nil)
	result, err := mem.Compact(ctx, session)
	if err == nil || result != nil {
		t.Errorf("Compact() = (%+v, %v), want (nil, error)", result, err)
	}
	got, gerr := store.GetSession(ctx, session.ID, session.NamespaceName, session.UserID)
	if gerr != nil {
		t.Fatalf("GetSession error = %v", gerr)
	}
	if len(got.Summary.Value()) != 0 || got.SummaryUptoSeq != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (no advance on failure)",
			got.Summary.Value(), got.SummaryUptoSeq)
	}
}

// memoryIDsForUser 按属主用户直读长期记忆 ID（绕过序列化器，仅用于隔离用例定位被测行）。
func memoryIDsForUser(t *testing.T, db *database.DB, userID uint) []uint {
	t.Helper()
	var ids []uint
	if err := db.Raw("SELECT id FROM agent_memories WHERE user_id = ? ORDER BY id ASC", userID).Scan(&ids).Error; err != nil {
		t.Fatalf("listing memory ids for user %d error = %v", userID, err)
	}
	return ids
}

// TestMemoryStore_CrossUserIsolation 测试长期记忆的跨用户隔离：同一命名空间下，用户只可见、
// 可改、可删属于自己的记忆；对他人记忆的详情、启停一律视为不存在（ErrMemoryNotFound），
// 删除他人记忆不生效。
func TestMemoryStore_CrossUserIsolation(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db, aitest.StubEmbedder{}, aitest.Params(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	// 同一命名空间下，用户 1 与用户 2 各有一条长期记忆。
	if err := db.Exec("INSERT INTO agent_memories (session_id, namespace_name, user_id, agent_kind, status) VALUES (?, ?, ?, ?, ?)",
		1, "default", testUserID, ai.AgentKindKnowledge, ai.MemoryStatusActive).Error; err != nil {
		t.Fatalf("inserting the owner memory error = %v", err)
	}
	if err := db.Exec("INSERT INTO agent_memories (session_id, namespace_name, user_id, agent_kind, status) VALUES (?, ?, ?, ?, ?)",
		2, "default", testOtherUserID, ai.AgentKindKnowledge, ai.MemoryStatusActive).Error; err != nil {
		t.Fatalf("inserting the other user memory error = %v", err)
	}
	ownerIDs := memoryIDsForUser(t, db, testUserID)
	if len(ownerIDs) != 1 {
		t.Fatalf("owner memory count = %d, want 1", len(ownerIDs))
	}
	ownerID := ownerIDs[0]

	// 列表按属主过滤：各自只看到自己的那一条
	ownerList, total, err := store.List(ctx, MemoryQuery{NamespaceName: "default", UserID: testUserID})
	if err != nil {
		t.Fatalf("List(owner) error = %v", err)
	}
	if total != 1 || len(ownerList) != 1 || ownerList[0].ID != ownerID {
		t.Errorf("List(owner) = (%d items, total %d), want only the owner's memory", len(ownerList), total)
	}
	// 详情：他人读取视为不存在
	if _, err := store.Get(ctx, "default", testOtherUserID, ownerID); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("Get(cross-user) error = %v, want ErrMemoryNotFound", err)
	}
	// 启停：他人操作视为不存在
	if err := store.SetStatus(ctx, "default", testOtherUserID, ownerID, ai.MemoryStatusArchived); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("SetStatus(cross-user) error = %v, want ErrMemoryNotFound", err)
	}
	// 删除：他人删除不生效，属主仍可读到
	if err := store.Delete(ctx, "default", testOtherUserID, []uint{ownerID}); err != nil {
		t.Fatalf("Delete(cross-user) error = %v, want nil (no rows affected)", err)
	}
	if _, err := store.Get(ctx, "default", testUserID, ownerID); err != nil {
		t.Errorf("Get(owner after cross-user delete) error = %v, want the memory to survive", err)
	}
	// 属主自身删除生效
	if err := store.Delete(ctx, "default", testUserID, []uint{ownerID}); err != nil {
		t.Fatalf("Delete(owner) error = %v", err)
	}
	if _, err := store.Get(ctx, "default", testUserID, ownerID); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("Get(owner after delete) error = %v, want ErrMemoryNotFound", err)
	}
}

// TestParseExtractedMemories 测试抽取结果的容错解析：对象形状、代码围栏、裸数组回退、条数上限
// 截断，以及空白与非法内容的英文错误。
func TestParseExtractedMemories(t *testing.T) {
	items, err := ParseExtractedMemories(`{"memories":[{"kind":"fact","title":"t","content":"c"}]}`, 0)
	if err != nil || len(items) != 1 || items[0].Content != "c" {
		t.Errorf("ParseExtractedMemories(object) = (%+v, %v), want one item", items, err)
	}
	items, err = ParseExtractedMemories("```json\n{\"memories\":[{\"content\":\"c1\"},{\"content\":\"c2\"}]}\n```", 0)
	if err != nil || len(items) != 2 {
		t.Errorf("ParseExtractedMemories(fence) = (%+v, %v), want two items", items, err)
	}
	items, err = ParseExtractedMemories(`[{"content":"a"},{"content":"b"}]`, 0)
	if err != nil || len(items) != 2 || items[1].Content != "b" {
		t.Errorf("ParseExtractedMemories(bare array) = (%+v, %v), want two items", items, err)
	}
	items, err = ParseExtractedMemories(`{"memories":[{"content":"1"},{"content":"2"},{"content":"3"}]}`, 2)
	if err != nil || len(items) != 2 {
		t.Errorf("ParseExtractedMemories(max=2) = (%+v, %v), want two items", items, err)
	}
	if _, err := ParseExtractedMemories("   ", 0); err == nil {
		t.Error("ParseExtractedMemories(blank) = nil error, want an error")
	}
	if _, err := ParseExtractedMemories("not json", 0); err == nil {
		t.Error("ParseExtractedMemories(malformed) = nil error, want an error")
	}
}

// scriptedTurn 是一轮预置响应：正文、思考与工具调用。
type scriptedTurn struct {
	content   string
	reason    string
	toolCalls []schema.ToolCall
}

// scriptedChat 按调用次序返回预置的每一轮响应（超出后重复最后一轮），并记录收到的消息。
type scriptedChat struct {
	turns     []scriptedTurn
	calls     int
	gotInputs [][]*schema.Message
}

// pick 取下一轮响应。
func (c *scriptedChat) pick() scriptedTurn {
	if len(c.turns) == 0 {
		return scriptedTurn{}
	}
	i := c.calls
	if i >= len(c.turns) {
		i = len(c.turns) - 1
	}
	c.calls++
	return c.turns[i]
}

// Generate 记录输入并返回下一轮非流式响应。
func (c *scriptedChat) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	c.gotInputs = append(c.gotInputs, input)
	turn := c.pick()
	return &schema.Message{Role: schema.Assistant, Content: turn.content, ReasoningContent: turn.reason, ToolCalls: turn.toolCalls}, nil
}

// Stream 记录输入并返回下一轮流式响应。
func (c *scriptedChat) Stream(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.gotInputs = append(c.gotInputs, input)
	turn := c.pick()
	chunks := make([]*schema.Message, 0, 3)
	if turn.reason != "" {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, ReasoningContent: turn.reason})
	}
	if turn.content != "" {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: turn.content})
	}
	if len(turn.toolCalls) > 0 {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, ToolCalls: turn.toolCalls})
	}
	return schema.StreamReaderFromArray(chunks), nil
}

// slotChatResolver 返回按档位查表的解析器（测试用）。
func slotChatResolver(chats map[ai.Slot]model.BaseChatModel) port.ChatResolver {
	return func(slot ai.Slot) (model.BaseChatModel, error) {
		if chat, ok := chats[slot]; ok {
			return chat, nil
		}
		return nil, errors.Newf("slot %q is not bound", slot)
	}
}

// encTestValue 把明文字符串包装为加密字段值（*serializer.Crypto）。
func encTestValue(s string) *serializer.Crypto {
	return serializer.NewCrypto([]byte(s))
}

// stubChat 是测试用对话模型替身：按配置分块外发正文（未配置分块时返回 reply）。
type stubChat struct {
	reply  string
	chunks []string
}

// Generate 返回完整回复。
func (s *stubChat) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: s.reply}, nil
}

// Stream 返回配置的分块流。
func (s *stubChat) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	chunks := s.chunks
	if len(chunks) == 0 {
		chunks = []string{s.reply}
	}
	sr, sw := schema.Pipe[*schema.Message](len(chunks))
	go func() {
		defer sw.Close()
		for _, c := range chunks {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: c}, nil)
		}
	}()
	return sr, nil
}

// testUserID 是测试使用的属主用户 ID。
const testUserID uint = 1

// testOtherUserID 是跨用户隔离用例的另一个属主用户 ID。
const testOtherUserID uint = 2
