package ai

import (
	// stdlib
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	// third-party
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	// internal
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// stubChat 是测试用对话模型替身：实现 model.BaseChatModel，记录收到的消息，
// 按配置返回完整回复（Generate）或分块流（Stream）。
// stubChat is a test double of the chat model: it implements model.BaseChatModel,
// records the received messages and returns the configured reply (Generate) or
// a chunked stream (Stream).
type stubChat struct {
	reply     string              // Generate 的完整回复
	chunks    []string            // Stream 的分块；为空时退回单块 reply
	gotInputs [][]*schema.Message // 记录每次调用收到的消息列表
}

func (s *stubChat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	s.gotInputs = append(s.gotInputs, input)
	return &schema.Message{Role: schema.Assistant, Content: s.reply}, nil
}

func (s *stubChat) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	s.gotInputs = append(s.gotInputs, input)
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

// stubRetriever 是测试用检索器替身：实现 retriever.Retriever，返回配置的文档
// 并记录查询与选项。
// stubRetriever is a test double of the retriever: it implements
// retriever.Retriever, returns the configured documents and records the query
// and options.
type stubRetriever struct {
	docs  []*schema.Document
	query string
	opts  []retriever.Option
}

func (s *stubRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	s.query = query
	s.opts = opts
	return s.docs, nil
}

// stubResolver 返回始终解析到同一个替身的档位解析器（测试用）。
// stubResolver returns a slot resolver that always resolves to the same double (test use).
func stubResolver(chat model.BaseChatModel) ChatResolver {
	return func(slot llm.Slot) (model.BaseChatModel, error) { return chat, nil }
}

// newTestAgent 构造基于 sqlite 内存库 + 给定替身的 Agent，配置会话上限
// （历史 10 条、业务数据 4096 字节）。
// newTestAgent builds an Agent over an in-memory sqlite DB with the given
// doubles, configuring the session bounds (10 history messages, 4096 business
// bytes).
func newTestAgent(t *testing.T, chat model.BaseChatModel, retriever retriever.Retriever) *Agent {
	t.Helper()
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"session.maxContextMessages": "10",
		"session.maxContextBytes":    "4096",
	})
	agent, err := NewAgent(stubResolver(chat), retriever, store, params)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return agent
}

// TestNewAgent_ValidationErrors 测试依赖为空时返回英文错误。
// TestNewAgent_ValidationErrors verifies nil dependencies yield English errors.
func TestNewAgent_ValidationErrors(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, nil)
	chat := &stubChat{}
	rtv := &stubRetriever{}
	cases := []struct {
		name     string
		resolver ChatResolver
		rtv      retriever.Retriever
		store    *SessionStore
		params   *llm.Params
	}{
		{"nil resolver", nil, rtv, store, params},
		{"nil retriever", stubResolver(chat), nil, store, params},
		{"nil store", stubResolver(chat), rtv, nil, params},
		{"nil params", stubResolver(chat), rtv, store, nil},
	}
	for _, c := range cases {
		if _, err := NewAgent(c.resolver, c.rtv, c.store, c.params); err == nil {
			t.Errorf("%s: NewAgent() = nil error, want an error", c.name)
		}
	}
}

// TestAgent_Generate_Structured 测试非流式执行：结构化 JSON 回复被解析为正文 +
// 草稿，检索引用随 done 项输出，Rego 规则（无）下 Valid 恒真。
// TestAgent_Generate_Structured verifies the non-streaming execution: the
// structured JSON reply is parsed into content + draft, retrieval citations ride
// the done item, and Valid is true without a Rego rule.
func TestAgent_Generate_Structured(t *testing.T) {
	chat := &stubChat{reply: `{"content": "**hello**", "draft": {"rule": {"name": "http-rule"}}}`}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "7",
		Content:  "chunk body",
		MetaData: map[string]any{"documentID": "7", "title": "Doc A", "chunkIndex": 0, "score": 0.9},
	}}}
	agent := newTestAgent(t, chat, rtv)

	out, err := agent.Generate(context.Background(), AgentInput{
		Query: "how to build a rule?", NamespaceName: "default",
	})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if !out.Done || out.Content != "**hello**" || !out.Valid {
		t.Errorf("Generate() = %+v, want done with content and valid", out)
	}
	if string(out.Draft) == "" || !json.Valid(out.Draft) {
		t.Errorf("Generate() draft = %q, want a valid JSON draft", out.Draft)
	}
	if len(out.Citations) != 1 || out.Citations[0].Title != "Doc A" {
		t.Errorf("Generate() citations = %+v, want the retrieved doc", out.Citations)
	}
	// 检索器应收到命名空间限定选项
	if rtv.query != "how to build a rule?" {
		t.Errorf("retriever query = %q, want the agent input query", rtv.query)
	}
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{}, rtv.opts...)
	if o.NamespaceName != "default" {
		t.Errorf("retriever namespace = %q, want %q", o.NamespaceName, "default")
	}
}

// TestAgent_Generate_PlainText 测试非结构化回复降级：正文原样返回、无草稿，
// Valid 仍为真（无校验规则）。
// TestAgent_Generate_PlainText verifies the non-structured reply degradation:
// the raw text is the content, no draft, Valid stays true (no validation rule).
func TestAgent_Generate_PlainText(t *testing.T) {
	chat := &stubChat{reply: "just a plain answer"}
	agent := newTestAgent(t, chat, &stubRetriever{})

	out, err := agent.Generate(context.Background(), AgentInput{Query: "hi"})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if out.Content != "just a plain answer" || out.Done != true || out.Valid != true {
		t.Errorf("Generate() = %+v, want the plain text as content", out)
	}
	if len(out.Draft) != 0 {
		t.Errorf("Generate() draft = %q, want no draft", out.Draft)
	}
}

// TestAgent_Generate_EmptyReply 测试模型空回复返回错误。
// TestAgent_Generate_EmptyReply verifies an empty model reply yields an error.
func TestAgent_Generate_EmptyReply(t *testing.T) {
	chat := &stubChat{reply: "   "}
	agent := newTestAgent(t, chat, &stubRetriever{})

	if _, err := agent.Generate(context.Background(), AgentInput{Query: "hi"}); err == nil {
		t.Fatal("Generate() = nil error, want an empty-response error")
	}
}

// TestAgent_Stream_DeltasAndDone 测试流式执行：delta 增量项之后是 done 完成项，
// done 项携带聚合全文与引用。
// TestAgent_Stream_DeltasAndDone verifies the streaming execution: delta items
// come first, then the done item carrying the aggregated text and citations.
func TestAgent_Stream_DeltasAndDone(t *testing.T) {
	chat := &stubChat{chunks: []string{"Hello", " world", "!"}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "1",
		Content:  "kb",
		MetaData: map[string]any{"documentID": "1", "title": "KB", "chunkIndex": 0, "score": 0.8},
	}}}
	agent := newTestAgent(t, chat, rtv)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", NamespaceName: "ns"})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	defer reader.Close()
	var deltas []string
	var done *AgentOutput
	for {
		item, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv error = %v", err)
		}
		if item.Done {
			cp := item
			done = &cp
			break
		}
		deltas = append(deltas, item.Delta)
	}
	if done == nil {
		t.Fatal("Stream() stream ended without a done item")
	}
	if strings.Join(deltas, "") != "Hello world!" {
		t.Errorf("deltas = %q, want %q", strings.Join(deltas, ""), "Hello world!")
	}
	if done.Content != "Hello world!" || len(done.Citations) != 1 {
		t.Errorf("done = %+v, want the aggregated content and citations", done)
	}
}

// TestParseStructuredReply 测试结构化 JSON 解析：常规、代码围栏包裹、空 content
// 报错、draft 为 null 时返回 nil。
// TestParseStructuredReply verifies the structured JSON parsing: plain, code
// fence wrapped, empty content error, and null draft yielding nil.
func TestParseStructuredReply(t *testing.T) {
	content, draft, err := parseStructuredReply(`{"content": "c1", "draft": {"a": 1}}`)
	if err != nil || content != "c1" || string(draft) != `{"a": 1}` {
		t.Errorf("parseStructuredReply() = (%q, %q, %v), want (c1, {\"a\": 1}, nil)", content, draft, err)
	}
	content, draft, err = parseStructuredReply("```json\n{\"content\": \"c2\", \"draft\": {}}\n```")
	if err != nil || content != "c2" || len(draft) == 0 || string(draft) != "{}" {
		t.Errorf("parseStructuredReply(fence) = (%q, %q, %v), want (c2, {}, nil)", content, draft, err)
	}
	content, draft, err = parseStructuredReply(`{"content": "c3", "draft": null}`)
	if err != nil || content != "c3" || draft != nil {
		t.Errorf("parseStructuredReply(null draft) = (%q, %v, %v), want (c3, nil, nil)", content, draft, err)
	}
	if _, _, err := parseStructuredReply(`{"draft": {}}`); err == nil {
		t.Error("parseStructuredReply(empty content) = nil error, want an error")
	}
	if _, _, err := parseStructuredReply("not json"); err == nil {
		t.Error("parseStructuredReply(bad json) = nil error, want an error")
	}
}

// TestValidateDraftWithRego 测试 Rego 校验规则：真值通过、假值/非布尔/非法规则
// 以 detail 形式返回失败（不返回 error）。
// TestValidateDraftWithRego verifies the Rego validation rule: true passes,
// false / non-boolean / broken rules fail via the detail (no error).
func TestValidateDraftWithRego(t *testing.T) {
	ctx := context.Background()
	valid, detail, err := validateDraftWithRego(ctx, "package agent\nvalidate := input.count > 0", json.RawMessage(`{"count": 1}`))
	if err != nil || !valid || detail != "" {
		t.Errorf("validateDraftWithRego(valid) = (%v, %q, %v), want (true, \"\", nil)", valid, detail, err)
	}
	valid, detail, err = validateDraftWithRego(ctx, "package agent\nvalidate := input.count > 0", json.RawMessage(`{"count": 0}`))
	if err != nil || valid {
		t.Errorf("validateDraftWithRego(false) = (%v, %q, %v), want (false, detail, nil)", valid, detail, err)
	}
	if detail == "" {
		t.Error("validateDraftWithRego(false) detail is empty, want a failure explanation")
	}
	valid, detail, err = validateDraftWithRego(ctx, "package agent\nvalidate := 42", json.RawMessage(`{}`))
	if err != nil || valid || !strings.Contains(detail, "boolean") {
		t.Errorf("validateDraftWithRego(non-bool) = (%v, %q, %v), want non-boolean detail", valid, detail, err)
	}
	valid, detail, err = validateDraftWithRego(ctx, "package agent\nvalidate :=", json.RawMessage(`{}`))
	if err != nil || valid || !strings.Contains(detail, "failed to evaluate") {
		t.Errorf("validateDraftWithRego(bad rule) = (%v, %q, %v), want evaluation-failure detail", valid, detail, err)
	}
	valid, detail, err = validateDraftWithRego(ctx, "package agent\nvalidate := input.count > 0", nil)
	if err != nil || valid {
		t.Errorf("validateDraftWithRego(nil draft) = (%v, %q, %v), want (false, detail, nil)", valid, detail, err)
	}
}

// TestBuildAgentMessages 测试消息组装顺序：系统提示 + 历史 + 用户消息（检索知识 +
// 业务现场 + 提问）。
// TestBuildAgentMessages verifies the message assembly order: system prompt +
// history + user message (retrieved knowledge + business scene + question).
func TestBuildAgentMessages(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("prev q"), schema.AssistantMessage("prev a", nil)}
	docs := []*schema.Document{{ID: "1", Content: "kb body", MetaData: map[string]any{"title": "KB"}}}
	data := llm.BusinessData{"src": "192.168.1.1"}
	messages := buildAgentMessages(testParams(t, nil), history, docs, data, "  current q  ")
	if len(messages) != 4 {
		t.Fatalf("buildAgentMessages() len = %d, want 4", len(messages))
	}
	if messages[0].Role != schema.System || !strings.Contains(messages[0].Content, "ZhongKui") {
		t.Errorf("messages[0] = %+v, want the agent system prompt", messages[0])
	}
	user := messages[3]
	if user.Role != schema.User {
		t.Errorf("messages[3] role = %v, want user", user.Role)
	}
	for _, want := range []string{"## Retrieved Knowledge", "KB: kb body", "## Current Business Scene", "192.168.1.1", "## User Question", "current q"} {
		if !strings.Contains(user.Content, want) {
			t.Errorf("user content missing %q: %q", want, user.Content)
		}
	}
}

// TestToSchemaMessages 测试持久化消息转 eino 消息：角色映射、跳过空内容与未知角色。
// TestToSchemaMessages verifies persisted-message conversion: role mapping,
// empty contents and unknown roles are skipped.
func TestToSchemaMessages(t *testing.T) {
	messages := []*ai.AgentMessage{
		{Role: MessageRoleUser, Content: "u"},
		{Role: MessageRoleAssistant, Content: "a"},
		{Role: MessageRoleSystem, Content: "s"},
		{Role: MessageRoleUser, Content: "  "},
		{Role: "robot", Content: "x"},
		{},
	}
	out := toSchemaMessages(messages)
	if len(out) != 3 {
		t.Fatalf("toSchemaMessages() len = %d, want 3", len(out))
	}
	if out[0].Role != schema.User || out[1].Role != schema.Assistant || out[2].Role != schema.System {
		t.Errorf("toSchemaMessages() roles = [%v, %v, %v], want [user, assistant, system]",
			out[0].Role, out[1].Role, out[2].Role)
	}
}

// TestStreamRun_PersistsMessages 测试 StreamRun 持久化用户与助手消息：助手消息
// 携带正文、草稿与引用；LatestAssistantMessage 可回读。
// TestStreamRun_PersistsMessages verifies StreamRun persists the user and the
// assistant messages: the assistant message carries the content, draft and
// citations; LatestAssistantMessage reads it back.
func TestStreamRun_PersistsMessages(t *testing.T) {
	chat := &stubChat{reply: `{"content": "answer", "draft": {"rule": "r1"}}`}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "3",
		Content:  "kb",
		MetaData: map[string]any{"documentID": "3", "title": "KB Doc", "chunkIndex": 0, "score": 0.7},
	}}}
	agent := newTestAgent(t, chat, rtv)
	session, err := agent.store.CreateSession(context.Background(), "default", "t", "m")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	reader, err := agent.StreamRun(context.Background(), AgentInput{
		Query: "hello", SessionID: session.ID, NamespaceName: "default",
	})
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	defer reader.Close()
	var done *AgentOutput
	for {
		item, err := reader.Recv()
		if err != nil {
			t.Fatalf("Recv error = %v", err)
		}
		if item.Done {
			cp := item
			done = &cp
			break
		}
	}
	if done == nil || done.Content != "answer" {
		t.Fatalf("StreamRun done = %+v, want the assistant content", done)
	}
	messages, err := agent.store.ListMessages(context.Background(), session.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + assistant)", len(messages))
	}
	if messages[0].Role != MessageRoleUser || messages[0].Content != "hello" {
		t.Errorf("messages[0] = %+v, want the persisted user message", messages[0])
	}
	if messages[1].Role != MessageRoleAssistant || messages[1].Content != "answer" {
		t.Errorf("messages[1] = %+v, want the persisted assistant message", messages[1])
	}
	if len(messages[1].DraftJSON) == 0 || !strings.Contains(string(messages[1].DraftJSON), "r1") {
		t.Errorf("messages[1].DraftJSON = %q, want the persisted draft", messages[1].DraftJSON)
	}
	if len(messages[1].CitationsJSON) == 0 || !strings.Contains(string(messages[1].CitationsJSON), "KB Doc") {
		t.Errorf("messages[1].CitationsJSON = %q, want the persisted citations", messages[1].CitationsJSON)
	}
	latest, err := agent.store.LatestAssistantMessage(context.Background(), session.ID)
	if err != nil || latest.Content != "answer" {
		t.Errorf("LatestAssistantMessage() = (%+v, %v), want the answer", latest, err)
	}
}

// TestStreamRun_InjectsHistory 测试 StreamRun 注入历史：先持久化一轮问答，
// 第二轮调用时替身收到的消息列表包含上一轮的历史。
// TestStreamRun_InjectsHistory verifies StreamRun injects the history: after one
// persisted turn, the second call's received message list includes the previous
// turn.
func TestStreamRun_InjectsHistory(t *testing.T) {
	chat := &stubChat{reply: `{"content": "a2", "draft": {}}`}
	agent := newTestAgent(t, chat, &stubRetriever{})
	ctx := context.Background()
	session, err := agent.store.CreateSession(ctx, "default", "t", "m")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	// 预置一轮历史（user + assistant）
	for _, m := range []*ai.AgentMessage{
		{SessionID: session.ID, NamespaceName: "default", Role: MessageRoleUser, Content: "q1"},
		{SessionID: session.ID, NamespaceName: "default", Role: MessageRoleAssistant, Content: "a1"},
	} {
		if _, err := agent.store.AppendMessage(ctx, m); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}

	reader, err := agent.StreamRun(ctx, AgentInput{Query: "q2", SessionID: session.ID, NamespaceName: "default"})
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	for {
		item, err := reader.Recv()
		if err != nil {
			t.Fatalf("Recv error = %v", err)
		}
		if item.Done {
			break
		}
	}
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	input := chat.gotInputs[0]
	// 系统提示 + 2 条历史 + 本轮用户消息
	if len(input) != 4 {
		t.Fatalf("chat input len = %d, want 4 (system + 2 history + user)", len(input))
	}
	if input[1].Content != "q1" || input[2].Content != "a1" {
		t.Errorf("history injection = [%q, %q], want [q1, a1]", input[1].Content, input[2].Content)
	}
	if input[3].Role != schema.User || !strings.Contains(input[3].Content, "q2") {
		t.Errorf("current user message = %+v, want q2", input[3])
	}
}

// TestStreamRun_MissingSession 测试会话不存在时返回 ErrSessionNotFound 且不落消息。
// TestStreamRun_MissingSession verifies a missing session yields
// ErrSessionNotFound and persists nothing.
func TestStreamRun_MissingSession(t *testing.T) {
	agent := newTestAgent(t, &stubChat{reply: "x"}, &stubRetriever{})
	if _, err := agent.StreamRun(context.Background(), AgentInput{Query: "q", SessionID: 99999}); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("StreamRun(missing session) error = %v, want ErrSessionNotFound", err)
	}
}

// TestStreamRun_EmptyQuery 测试空提问返回英文错误。
// TestStreamRun_EmptyQuery verifies an empty query yields an English error.
func TestStreamRun_EmptyQuery(t *testing.T) {
	agent := newTestAgent(t, &stubChat{reply: "x"}, &stubRetriever{})
	if _, err := agent.StreamRun(context.Background(), AgentInput{Query: "   "}); err == nil {
		t.Fatal("StreamRun(empty query) = nil error, want an error")
	}
}

// TestAgent_Generate_RegoValidation 测试 Rego 校验规则接入：规则校验通过时
// Valid 为真，未通过时 Valid 为假并带说明。
// TestAgent_Generate_RegoValidation verifies the Rego validation wiring: a
// passing rule yields Valid=true and a failing rule yields Valid=false with a
// detail.
func TestAgent_Generate_RegoValidation(t *testing.T) {
	rule := "package agent\nvalidate := input.hits > 0"
	chat := &stubChat{reply: `{"content": "ok", "draft": {"hits": 1}}`}
	agent := newTestAgent(t, chat, &stubRetriever{})

	pass, err := agent.Generate(context.Background(), AgentInput{Query: "q", RegoRule: rule})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if !pass.Valid || pass.ValidationDetail != "" {
		t.Errorf("Generate(passing rule) = valid=%v detail=%q, want valid with no detail", pass.Valid, pass.ValidationDetail)
	}

	chat.reply = `{"content": "ok", "draft": {"hits": 0}}`
	fail, err := agent.Generate(context.Background(), AgentInput{Query: "q", RegoRule: rule})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if fail.Valid || fail.ValidationDetail == "" {
		t.Errorf("Generate(failing rule) = valid=%v detail=%q, want invalid with a detail", fail.Valid, fail.ValidationDetail)
	}
}
