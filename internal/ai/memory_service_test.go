package ai

import (
	// 标准库
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"

	// 项目内部
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// newScriptedChat 构造按次序返回给定回复的对话替身（复用 agent_tools_test.go 的 scriptedChat）。
func newScriptedChat(replies ...string) *scriptedChat {
	turns := make([]scriptedTurn, 0, len(replies))
	for _, reply := range replies {
		turns = append(turns, scriptedTurn{content: reply})
	}
	return &scriptedChat{turns: turns}
}

// errEmbedder 是向量化必然失败的 embedder 替身：使 MemoryStore.Write 在向量化阶段即失败（不
// 触达 DB），从而在无 pgvector 的 sqlite 下稳定检验「长期记忆抽取失败只进 Warnings」。
type errEmbedder struct{}

// EmbedStrings 固定返回错误，用于模拟向量化失败。
func (errEmbedder) EmbedStrings(_ context.Context, _ []string, _ ...embedding.Option) ([][]float64, error) {
	return nil, errors.New("embedder: forced failure")
}

// newTestMemory 构造基于 sqlite 内存库的 Memory 服务：embed 为向量化替身，chat 为 flash 档位
// 对话替身（为 nil 时解析器缺失，用于检验摘要失败降级），overrides 叠加在测试参数之上。
func newTestMemory(t *testing.T, db *database.DB, chat model.BaseChatModel, embed embedder, overrides map[string]string) *Memory {
	t.Helper()
	params := testParams(t, overrides)
	store, err := NewMemoryStore(db, embed, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	var resolver ChatResolver
	if chat != nil {
		resolver = slotChatResolver(map[llm.Slot]model.BaseChatModel{llm.SlotFlash: chat})
	}
	mem, err := NewMemory(store, resolver, params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	return mem
}

// TestNewMemory_NilArgs 测试 Memory 构造的空值校验：store/params 为空返回英文错误，resolver 与
// logger 允许为空。
func TestNewMemory_NilArgs(t *testing.T) {
	if _, err := NewMemory(nil, nil, nil, nil); err == nil {
		t.Error("NewMemory(nil, nil, nil, nil) = nil error, want an error")
	}
	params := testParams(t, nil)
	store, err := NewMemoryStore(newTestSessionDB(t), stubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	if _, err := NewMemory(store, nil, nil, nil); err == nil {
		t.Error("NewMemory(store, nil, nil, nil) = nil error, want an error")
	}
	// resolver 与 logger 允许为空
	if _, err := NewMemory(store, nil, params, nil); err != nil {
		t.Errorf("NewMemory(store, nil, params, nil) error = %v, want nil", err)
	}
}

// TestSummarize_Advances 测试滚动摘要推进：返回模型回复（去首尾空白）与被挤出消息的最后一条
// ID，且请求携带配置的摘要系统提示与「既有摘要 + 被挤出消息」的用户提示。
func TestSummarize_Advances(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("  new summary  ")
	mem := newTestMemory(t, newTestSessionDB(t), chat, stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "summary system prompt",
	})
	session := &modelsai.AgentSession{NamespaceName: "default", Summary: "old summary", SummaryUptoMessageID: 3}
	evicted := []*modelsai.AgentMessage{
		{BaseModel: database.BaseModel{ID: 4}, Role: llm.MessageRoleUser, Content: "u"},
		{BaseModel: database.BaseModel{ID: 5}, Role: llm.MessageRoleAssistant, Content: "a"},
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
	mem := newTestMemory(t, newTestSessionDB(t), newScriptedChat("x"), stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	if _, _, err := mem.Summarize(ctx, nil, nil); err == nil {
		t.Error("Summarize(nil session) = nil error, want an error")
	}
	session := &modelsai.AgentSession{Summary: "keep", SummaryUptoMessageID: 7}
	summary, upto, err := mem.Summarize(ctx, session, nil)
	if err != nil || summary != "keep" || upto != 7 {
		t.Errorf("Summarize(no evicted) = (%q, %d, %v), want (keep, 7, nil)", summary, upto, err)
	}
	noResolver := newTestMemory(t, newTestSessionDB(t), nil, stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	evicted := []*modelsai.AgentMessage{{Role: llm.MessageRoleUser, Content: "c"}}
	if _, _, err := noResolver.Summarize(ctx, &modelsai.AgentSession{}, evicted); err == nil {
		t.Error("Summarize(nil resolver) = nil error, want an error")
	}
	empty := newTestMemory(t, newTestSessionDB(t), newScriptedChat("   "), stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "p",
	})
	if _, _, err := empty.Summarize(ctx, &modelsai.AgentSession{}, evicted); err == nil {
		t.Error("Summarize(empty reply) = nil error, want an error")
	}
}

// TestGenerateTopic_GeneratesAndTruncates 测试会话主题生成：模型回复经去首尾空白后作为主题返回，
// 超过 sessionTopicByteLimit 时按字节截断并追加省略号；请求携带配置的主题系统提示与用户的首条消息。
func TestGenerateTopic_GeneratesAndTruncates(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("  " + strings.Repeat("a", 100) + "  ")
	mem := newTestMemory(t, newTestSessionDB(t), chat, stubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "topic system prompt",
	})
	got := mem.GenerateTopic(ctx, "  用户的首条问题  ")
	want := strings.Repeat("a", sessionTopicByteLimit) + "..."
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
// 模型空响应一律回退为「首条消息按 sessionTopicByteLimit 截断」，且绝不返回错误。
func TestGenerateTopic_Fallbacks(t *testing.T) {
	ctx := context.Background()
	chat := newScriptedChat("topic")
	mem := newTestMemory(t, newTestSessionDB(t), chat, stubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	if got := mem.GenerateTopic(ctx, "   "); got != "" {
		t.Errorf("GenerateTopic(empty query) = %q, want an empty string", got)
	}
	if len(chat.gotInputs) != 0 {
		t.Errorf("chat calls for an empty query = %d, want 0", len(chat.gotInputs))
	}
	noResolver := newTestMemory(t, newTestSessionDB(t), nil, stubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	empty := newTestMemory(t, newTestSessionDB(t), newScriptedChat("   "), stubEmbedder{}, map[string]string{
		"session.topicSystemPrompt": "p",
	})
	query := strings.Repeat("问", 30)
	want := SessionTopicFallback(query)
	for name, m := range map[string]*Memory{"no resolver": noResolver, "empty reply": empty} {
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
	if !strings.HasSuffix(got, "...") || len(got) > sessionTopicByteLimit+3 {
		t.Errorf("SessionTopicFallback() = %q, want at most %d bytes plus an ellipsis",
			got, sessionTopicByteLimit)
	}
	exact := strings.Repeat("a", sessionTopicByteLimit)
	if got := SessionTopicFallback(exact); got != exact {
		t.Errorf("SessionTopicFallback(exact) = %q, want it unchanged", got)
	}
}

// TestCompact_ExtractionFailureOnlyWarnings 测试抽取落库失败只进 Warnings：摘要成功落库并返回，
// Extracted 为 0、Warnings 非空，且摘要与抽取各调用模型一次。
func TestCompact_ExtractionFailureOnlyWarnings(t *testing.T) {
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleUser, Content: "u1",
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
	got, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Summary != "folded summary" || got.SummaryUptoMessageID == 0 {
		t.Errorf("persisted summary = (%q, %d), want the folded summary with an advanced cursor",
			got.Summary, got.SummaryUptoMessageID)
	}
}

// TestCompact_DisabledSkipsExtraction 测试长期记忆开关关闭时仍刷新摘要但跳过抽取：模型仅被调用
// 一次（摘要），Extracted 为 0 且带一条开关关闭告警。
func TestCompact_DisabledSkipsExtraction(t *testing.T) {
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleUser, Content: "u1",
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	chat := newScriptedChat("folded summary")
	mem := newTestMemory(t, db, chat, stubEmbedder{}, map[string]string{
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
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	session.Summary = "existing"
	chat := newScriptedChat("unused")
	mem := newTestMemory(t, db, chat, stubEmbedder{}, map[string]string{
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
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &modelsai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleUser, Content: "u1",
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	mem := newTestMemory(t, db, nil, stubEmbedder{}, nil)
	result, err := mem.Compact(ctx, session)
	if err == nil || result != nil {
		t.Errorf("Compact() = (%+v, %v), want (nil, error)", result, err)
	}
	got, gerr := store.GetSession(ctx, session.ID)
	if gerr != nil {
		t.Fatalf("GetSession error = %v", gerr)
	}
	if got.Summary != "" || got.SummaryUptoMessageID != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (no advance on failure)",
			got.Summary, got.SummaryUptoMessageID)
	}
}

// TestParseExtractedMemories 测试抽取结果的容错解析：对象形状、代码围栏、裸数组回退、条数上限
// 截断，以及空白与非法内容的英文错误。
func TestParseExtractedMemories(t *testing.T) {
	items, err := parseExtractedMemories(`{"memories":[{"kind":"fact","title":"t","content":"c"}]}`, 0)
	if err != nil || len(items) != 1 || items[0].Content != "c" {
		t.Errorf("parseExtractedMemories(object) = (%+v, %v), want one item", items, err)
	}
	items, err = parseExtractedMemories("```json\n{\"memories\":[{\"content\":\"c1\"},{\"content\":\"c2\"}]}\n```", 0)
	if err != nil || len(items) != 2 {
		t.Errorf("parseExtractedMemories(fence) = (%+v, %v), want two items", items, err)
	}
	items, err = parseExtractedMemories(`[{"content":"a"},{"content":"b"}]`, 0)
	if err != nil || len(items) != 2 || items[1].Content != "b" {
		t.Errorf("parseExtractedMemories(bare array) = (%+v, %v), want two items", items, err)
	}
	items, err = parseExtractedMemories(`{"memories":[{"content":"1"},{"content":"2"},{"content":"3"}]}`, 2)
	if err != nil || len(items) != 2 {
		t.Errorf("parseExtractedMemories(max=2) = (%+v, %v), want two items", items, err)
	}
	if _, err := parseExtractedMemories("   ", 0); err == nil {
		t.Error("parseExtractedMemories(blank) = nil error, want an error")
	}
	if _, err := parseExtractedMemories("not json", 0); err == nil {
		t.Error("parseExtractedMemories(malformed) = nil error, want an error")
	}
}
