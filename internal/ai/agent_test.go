package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkui.com/zhongkui/internal/ai/tools"
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// stubChat 是测试用对话模型替身：实现 model.BaseChatModel，记录收到的消息，
// 按配置返回完整回复（Generate）或分块流（Stream）。
type stubChat struct {
	reply     string              // Generate 的完整回复
	chunks    []string            // Stream 的分块；为空时退回单块 reply
	gotInputs [][]*schema.Message // 记录每次调用收到的消息列表
}

// Generate 记录输入消息并返回配置的完整回复。
func (s *stubChat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	s.gotInputs = append(s.gotInputs, input)
	return &schema.Message{Role: schema.Assistant, Content: s.reply}, nil
}

// Stream 记录输入消息并返回配置的分块流，未配置分块时退回单块回复。
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
type stubRetriever struct {
	docs  []*schema.Document
	query string
	opts  []retriever.Option
}

// Retrieve 记录查询与选项并返回配置的文档。
func (s *stubRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	s.query = query
	s.opts = opts
	return s.docs, nil
}

// stubResolver 返回始终解析到同一个替身的档位解析器（测试用）。
func stubResolver(chat model.BaseChatModel) ChatResolver {
	return func(slot llm.Slot) (model.BaseChatModel, error) { return chat, nil }
}

// newTestAgent 构造基于 sqlite 内存库 + 给定替身的 Agent，配置会话上限
// （历史 10 条、业务数据 4096 字节）。
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
	agent, err := NewAgent(context.Background(), stubResolver(chat), retriever, store, params, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return agent
}

// TestNewAgent_ValidationErrors 测试依赖为空时返回英文错误。
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
		if _, err := NewAgent(context.Background(), c.resolver, c.rtv, c.store, c.params, nil, nil, nil, nil); err == nil {
			t.Errorf("%s: NewAgent() = nil error, want an error", c.name)
		}
	}
}

// TestAgent_Generate_Structured 测试非流式执行：结构化 JSON 回复被解析为正文 +
// 草稿，检索引用随 done 项输出，Rego 规则（无）下 Valid 恒真。
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
func TestAgent_Generate_EmptyReply(t *testing.T) {
	chat := &stubChat{reply: "   "}
	agent := newTestAgent(t, chat, &stubRetriever{})

	if _, err := agent.Generate(context.Background(), AgentInput{Query: "hi"}); err == nil {
		t.Fatal("Generate() = nil error, want an empty-response error")
	}
}

// TestAgent_Stream_DeltasAndDone 测试流式执行：delta 增量项之后是 done 完成项，
// done 项携带聚合全文与引用。
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
func TestBuildAgentMessages(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("prev q"), schema.AssistantMessage("prev a", nil)}
	docs := []*schema.Document{{ID: "1", Content: "kb body", MetaData: map[string]any{"title": "KB"}}}
	data := llm.BusinessData{"src": "192.168.1.1"}
	messages := buildAgentMessages(testParams(t, nil), "", nil, history, docs, data, "  current q  ")
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
func TestToSchemaMessages(t *testing.T) {
	messages := []*ai.AgentMessage{
		{Role: llm.MessageRoleUser, Content: "u"},
		{Role: llm.MessageRoleAssistant, Content: "a"},
		{Role: llm.MessageRoleSystem, Content: "s"},
		{Role: llm.MessageRoleUser, Content: "  "},
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
func TestStreamRun_PersistsMessages(t *testing.T) {
	chat := &stubChat{reply: `{"content": "answer", "draft": {"rule": "r1"}}`}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "3",
		Content:  "kb",
		MetaData: map[string]any{"documentID": "3", "title": "KB Doc", "chunkIndex": 0, "score": 0.7},
	}}}
	agent := newTestAgent(t, chat, rtv)
	session, err := agent.store.CreateSession(context.Background(), "default", "t", "m", "knowledge")
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
	if messages[0].Role != llm.MessageRoleUser || messages[0].Content != "hello" {
		t.Errorf("messages[0] = %+v, want the persisted user message", messages[0])
	}
	if messages[1].Role != llm.MessageRoleAssistant || messages[1].Content != "answer" {
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
func TestStreamRun_InjectsHistory(t *testing.T) {
	chat := &stubChat{reply: `{"content": "a2", "draft": {}}`}
	agent := newTestAgent(t, chat, &stubRetriever{})
	ctx := context.Background()
	session, err := agent.store.CreateSession(ctx, "default", "t", "m", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	// 预置一轮历史（user + assistant）
	for _, m := range []*ai.AgentMessage{
		{SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleUser, Content: "q1"},
		{SessionID: session.ID, NamespaceName: "default", Role: llm.MessageRoleAssistant, Content: "a1"},
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
func TestStreamRun_MissingSession(t *testing.T) {
	agent := newTestAgent(t, &stubChat{reply: "x"}, &stubRetriever{})
	if _, err := agent.StreamRun(context.Background(), AgentInput{Query: "q", SessionID: 99999}); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("StreamRun(missing session) error = %v, want ErrSessionNotFound", err)
	}
}

// TestStreamRun_EmptyQuery 测试空提问返回英文错误。
func TestStreamRun_EmptyQuery(t *testing.T) {
	agent := newTestAgent(t, &stubChat{reply: "x"}, &stubRetriever{})
	if _, err := agent.StreamRun(context.Background(), AgentInput{Query: "   "}); err == nil {
		t.Fatal("StreamRun(empty query) = nil error, want an error")
	}
}

// TestAgent_Generate_RegoValidation 测试 Rego 校验规则接入：规则校验通过时
// Valid 为真，未通过时 Valid 为假并带说明。
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

// slotChatResolver 返回按档位分发的对话解析器：命中 bySlot 时返回对应替身，其余档位返回英文错误。
func slotChatResolver(bySlot map[llm.Slot]model.BaseChatModel) ChatResolver {
	return func(slot llm.Slot) (model.BaseChatModel, error) {
		if chat, ok := bySlot[slot]; ok {
			return chat, nil
		}
		return nil, errors.Newf("no chat model for slot %q", slot)
	}
}

// TestTrimMessagesByBytes 测试字节预算裁剪：总字节不超预算时原样返回，超预算时从最旧丢整条、
// 至少保留最新一条（故不切断多字节字符），预算非正数时不裁剪。
func TestTrimMessagesByBytes(t *testing.T) {
	// 裁剪只依据 Content 字节，故构造时不设主键（ID 内嵌于 BaseModel，无法用字面量键赋值）。
	newMessages := func(contents ...string) []*ai.AgentMessage {
		out := make([]*ai.AgentMessage, 0, len(contents))
		for _, c := range contents {
			out = append(out, &ai.AgentMessage{Content: c})
		}
		return out
	}
	cases := []struct {
		name     string
		contents []string
		maxBytes int
		want     []string
	}{
		{name: "未超预算原样返回", contents: []string{"aaaa", "bbbb", "cccc"}, maxBytes: 12, want: []string{"aaaa", "bbbb", "cccc"}},
		{name: "从最旧丢起", contents: []string{"aaaa", "bbbb", "cccc"}, maxBytes: 11, want: []string{"bbbb", "cccc"}},
		{name: "至少保留最新一条", contents: []string{"aaaa", "bbbb", "cccc"}, maxBytes: 3, want: []string{"cccc"}},
		{name: "预算为零不裁剪", contents: []string{"aaaa", "bbbb"}, maxBytes: 0, want: []string{"aaaa", "bbbb"}},
		{name: "预算为负不裁剪", contents: []string{"aaaa", "bbbb"}, maxBytes: -1, want: []string{"aaaa", "bbbb"}},
		{name: "多字节字符整条保留", contents: []string{"你好", "世界"}, maxBytes: 6, want: []string{"世界"}},
		{name: "空列表", contents: nil, maxBytes: 10, want: nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := trimMessagesByBytes(newMessages(tt.contents...), tt.maxBytes)
			if len(got) != len(tt.want) {
				t.Fatalf("trimMessagesByBytes() len = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i].Content != tt.want[i] {
					t.Errorf("trimMessagesByBytes()[%d] = %q, want %q", i, got[i].Content, tt.want[i])
				}
			}
		})
	}
}

// TestBuildAgentMessages_SummaryInjected 测试摘要段的注入位置：系统提示 + 摘要段 + 历史消息 +
// 用户消息，摘要段为独立 system 消息且以英文标题常量开头。
func TestBuildAgentMessages_SummaryInjected(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("prev q"), schema.AssistantMessage("prev a", nil)}
	messages := buildAgentMessages(testParams(t, nil), "  folded  ", nil, history, nil, nil, "q")
	if len(messages) != 5 {
		t.Fatalf("buildAgentMessages() len = %d, want 5 (system + summary + 2 history + user)", len(messages))
	}
	if messages[0].Role != schema.System {
		t.Errorf("messages[0] role = %v, want system", messages[0].Role)
	}
	if messages[1].Role != schema.System || !strings.HasPrefix(messages[1].Content, agentSummarySectionTitle) || !strings.Contains(messages[1].Content, "folded") {
		t.Errorf("messages[1] = %+v, want the summary section right after the system prompt", messages[1])
	}
	if messages[2].Content != "prev q" || messages[3].Content != "prev a" {
		t.Errorf("history order = [%q, %q], want [prev q, prev a]", messages[2].Content, messages[3].Content)
	}
	if messages[4].Role != schema.User {
		t.Errorf("messages[4] role = %v, want user", messages[4].Role)
	}
}

// rollingSummaryFixture 是滚动摘要相关用例的共享装配：sqlite 内存库（会话与记忆同源）、按档位
// 分流的对话替身（flash 供记忆摘要、standard 供 Agent 生成），以及一个已预置 4 条历史的会话；
// 窗口取最近 1 条，故前 3 条被挤出，其最后一条 ID 即摘要游标上界。
type rollingSummaryFixture struct {
	agent         *Agent           // 被测 Agent（已注入 Memory）
	store         *SessionStore    // 会话存储（回读摘要用）
	session       *ai.AgentSession // 预置会话
	memoryChat    *stubChat        // flash 档位替身：记忆摘要
	agentChat     *stubChat        // standard 档位替身：Agent 生成
	evictedLastID uint             // 被挤出消息的最后一条 ID
}

// newRollingSummaryFixture 构造上述装配；minEvicted 为触发滚动摘要的最少被挤出消息条数。
func newRollingSummaryFixture(t *testing.T, minEvicted int) *rollingSummaryFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"session.maxContextMessages":       "1",
		"memory.summarySystemPrompt":       "test summary system prompt",
		"memory.summaryMinEvictedMessages": strconv.Itoa(minEvicted),
	})
	memoryChat := &stubChat{reply: "folded summary"}
	agentChat := &stubChat{reply: `{"content": "a3", "draft": {}}`}
	memoryStore, err := NewMemoryStore(db, stubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	memory, err := NewMemory(memoryStore,
		slotChatResolver(map[llm.Slot]model.BaseChatModel{llm.SlotFlash: memoryChat}), params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	agent, err := NewAgent(ctx,
		slotChatResolver(map[llm.Slot]model.BaseChatModel{llm.SlotStandard: agentChat}),
		&stubRetriever{}, store, params, memory, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge")
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role llm.MessageRole, content string) *ai.AgentMessage {
		t.Helper()
		msg := &ai.AgentMessage{SessionID: session.ID, NamespaceName: "default", Role: role, Content: content}
		if _, err := store.AppendMessage(ctx, msg); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
		return msg
	}
	appendMsg(llm.MessageRoleUser, "u1")
	appendMsg(llm.MessageRoleAssistant, "a1")
	evictedLast := appendMsg(llm.MessageRoleUser, "u2")
	appendMsg(llm.MessageRoleAssistant, "a2")
	return &rollingSummaryFixture{
		agent:         agent,
		store:         store,
		session:       session,
		memoryChat:    memoryChat,
		agentChat:     agentChat,
		evictedLastID: evictedLast.ID,
	}
}

// drainStreamRun 执行一次 StreamRun 并把输出流读到结束（done 项或流错误）。
func drainStreamRun(t *testing.T, agent *Agent, ctx context.Context, input AgentInput) {
	t.Helper()
	reader, err := agent.StreamRun(ctx, input)
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	defer reader.Close()
	for {
		item, err := reader.Recv()
		if err != nil {
			return
		}
		if item.Done {
			return
		}
	}
}

// TestStreamRun_RollingSummaryInjected 测试达到阈值时的滚动摘要：被挤出消息条数达标即调用
// flash 重写摘要、落库并把摘要段注入本次生成（位于系统提示之后、窗口历史之前）。
func TestStreamRun_RollingSummaryInjected(t *testing.T) {
	f := newRollingSummaryFixture(t, 2)
	ctx := context.Background()
	drainStreamRun(t, f.agent, ctx, AgentInput{Query: "q3", SessionID: f.session.ID, NamespaceName: "default"})

	if len(f.memoryChat.gotInputs) != 1 {
		t.Fatalf("memory chat calls = %d, want 1 (one summary request)", len(f.memoryChat.gotInputs))
	}
	if got := f.memoryChat.gotInputs[0][0].Content; got != "test summary system prompt" {
		t.Errorf("summary system prompt = %q, want the configured prompt", got)
	}
	if len(f.agentChat.gotInputs) != 1 {
		t.Fatalf("agent chat calls = %d, want 1", len(f.agentChat.gotInputs))
	}
	in := f.agentChat.gotInputs[0]
	if len(in) != 4 {
		t.Fatalf("agent input len = %d, want 4 (system + summary + window + user)", len(in))
	}
	if in[1].Role != schema.System || !strings.HasPrefix(in[1].Content, agentSummarySectionTitle) || !strings.Contains(in[1].Content, "folded summary") {
		t.Errorf("agent input[1] = %+v, want the injected summary section", in[1])
	}
	if in[2].Content != "a2" {
		t.Errorf("agent input[2] = %q, want the window message a2", in[2].Content)
	}
	got, err := f.store.GetSession(ctx, f.session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Summary != "folded summary" || got.SummaryUptoMessageID != f.evictedLastID {
		t.Errorf("persisted summary = (%q, %d), want (%q, %d)",
			got.Summary, got.SummaryUptoMessageID, "folded summary", f.evictedLastID)
	}
}

// TestBuildAgentMessages_MemoryInjected 测试长期记忆分节的注入位置与格式：系统提示 + 摘要段 +
// 记忆段 + 窗口历史 + 用户消息；记忆段为独立 system 消息，条目形如 "- [kind] title: content"，
// 正文为空（去空白后）的命中被跳过。
func TestBuildAgentMessages_MemoryInjected(t *testing.T) {
	params := testParams(t, map[string]string{"memory.sectionTitle": "## Long-term Memory"})
	history := []*schema.Message{schema.UserMessage("prev q")}
	memories := []MemoryHit{
		{ID: 1, Kind: llm.MemoryKindDecision, Title: "chosen rule", Content: "use deny-by-default"},
		{ID: 2, Kind: llm.MemoryKindFact, Content: "the team prefers rego v1"},
		{ID: 3, Kind: llm.MemoryKindFact, Title: "blank", Content: "   "},
	}
	messages := buildAgentMessages(params, "folded", memories, history, nil, nil, "q")
	if len(messages) != 5 {
		t.Fatalf("buildAgentMessages() len = %d, want 5 (system + summary + memory + history + user)", len(messages))
	}
	if messages[2].Role != schema.System || !strings.HasPrefix(messages[2].Content, "## Long-term Memory") {
		t.Errorf("messages[2] = %+v, want the memory section after the summary", messages[2])
	}
	for _, want := range []string{"- [decision] chosen rule: use deny-by-default", "- [fact] the team prefers rego v1"} {
		if !strings.Contains(messages[2].Content, want) {
			t.Errorf("memory section missing %q: %q", want, messages[2].Content)
		}
	}
	if strings.Contains(messages[2].Content, "blank") {
		t.Errorf("memory section = %q, want the blank-bodied hit skipped", messages[2].Content)
	}
	if messages[3].Content != "prev q" || messages[4].Role != schema.User {
		t.Errorf("memory section must sit before the window history: %+v", messages)
	}
}

// TestBuildAgentMessages_MemorySectionSkipped 测试无记忆可用时不注入记忆分节：命中为空或全部
// 正文为空时消息列表与引入长期记忆前完全一致。
func TestBuildAgentMessages_MemorySectionSkipped(t *testing.T) {
	for _, memories := range [][]MemoryHit{nil, {{ID: 1, Kind: llm.MemoryKindFact, Content: "  "}}} {
		messages := buildAgentMessages(testParams(t, nil), "", memories, nil, nil, nil, "q")
		if len(messages) != 2 {
			t.Fatalf("buildAgentMessages(%+v) len = %d, want 2 (system + user)", memories, len(messages))
		}
	}
}

// memoryAgentFixture 是记忆相关用例的共享装配：sqlite 内存库（会话与记忆同源）、注入了记忆服务
// 与 memory_write 工具的 Agent，以及该工具实例。
type memoryAgentFixture struct {
	agent *Agent
	tool  tool.BaseTool
}

// newMemoryAgentFixture 构造上述装配；chat 同时用于 Agent 生成与记忆摘要，overrides 覆盖参数。
func newMemoryAgentFixture(t *testing.T, chat model.BaseChatModel, overrides map[string]string) *memoryAgentFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, overrides)
	memoryStore, err := NewMemoryStore(db, stubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	memory, err := NewMemory(memoryStore, stubResolver(chat), params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	built, err := tools.Build(ctx, "memory_write", tools.Deps{})
	if err != nil {
		t.Fatalf("tools.Build(memory_write) error = %v", err)
	}
	info, err := built.Info(ctx)
	if err != nil {
		t.Fatalf("memory_write Info error = %v", err)
	}
	agent, err := NewAgent(ctx, stubResolver(chat), &stubRetriever{}, store, params, memory, nil,
		[]tool.BaseTool{built}, []*schema.ToolInfo{info})
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return &memoryAgentFixture{agent: agent, tool: built}
}

// TestRecallMemories_DegradesToNoSection 测试长期记忆检索的四条降级路径：记忆服务缺失、
// memory.enabled=false、检索失败（sqlite 无 pgvector/tsvector）与 Agent 类型非法时一律返回空
// 命中（不注入记忆分节），且检索失败绝不作为错误上抛。
func TestRecallMemories_DegradesToNoSection(t *testing.T) {
	ctx := context.Background()
	query := AgentInput{Query: "q", NamespaceName: "default"}

	// 记忆服务缺失：Agent 未注入 Memory。
	noMemory := newAgentWithTools(t, &stubChat{reply: "x"}, &stubRetriever{}, nil, nil, map[string]string{"memory.enabled": "true"})
	if hits := noMemory.recallMemories(ctx, query); hits != nil {
		t.Errorf("recallMemories(no memory service) = %+v, want nil", hits)
	}

	// memory.enabled=false：开关关闭即不检索（不触碰存储）。
	disabled := newMemoryAgentFixture(t, &stubChat{reply: "x"}, map[string]string{"memory.enabled": "false"})
	if hits := disabled.agent.recallMemories(ctx, query); hits != nil {
		t.Errorf("recallMemories(memory.enabled=false) = %+v, want nil", hits)
	}

	// 检索失败：开关打开但底层库不支持向量/全文检索，降级为空且不返回错误。
	failing := newMemoryAgentFixture(t, &stubChat{reply: "x"}, map[string]string{"memory.enabled": "true"})
	if hits := failing.agent.recallMemories(ctx, query); hits != nil {
		t.Errorf("recallMemories(failed retrieval) = %+v, want nil", hits)
	}

	// Agent 类型非法：解析失败即跳过记忆分节。
	badKind := query
	badKind.AgentKind = llm.AgentKind("bogus")
	if hits := failing.agent.recallMemories(ctx, badKind); hits != nil {
		t.Errorf("recallMemories(invalid agent kind) = %+v, want nil", hits)
	}
}

// TestStream_MemoryFailureKeepsGeneration 测试长期记忆检索失败不影响生成：开关打开且底层库
// 不支持检索时，生成仍正常完成，且消息列表中不含记忆分节。
func TestStream_MemoryFailureKeepsGeneration(t *testing.T) {
	chat := &stubChat{chunks: []string{"ok"}}
	f := newMemoryAgentFixture(t, chat, map[string]string{
		"memory.enabled":      "true",
		"memory.sectionTitle": "## Long-term Memory",
	})
	reader, err := f.agent.Stream(context.Background(), AgentInput{
		Query: "q", NamespaceName: "default", AgentKind: llm.AgentKindRego,
	})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	_, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v, want the turn to survive a memory failure", err)
	}
	if done.Content != "ok" {
		t.Errorf("done content = %q, want the model answer", done.Content)
	}
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	for _, msg := range chat.gotInputs[0] {
		if strings.Contains(msg.Content, "## Long-term Memory") {
			t.Errorf("message unexpectedly carries a memory section: %q", msg.Content)
		}
	}
}

// TestMemoryToolContext_InjectsRuntime 测试节点 4 派发工具前的记忆运行期注入：注入记忆服务后
// memory_write 能取回写入器与归属（内容抵达存储层后由存储层按空命名空间拒绝），未注入记忆服务
// 时工具因取不到运行期信息而返回英文错误。
func TestMemoryToolContext_InjectsRuntime(t *testing.T) {
	ctx := context.Background()
	tc := schema.ToolCall{ID: "c1", Type: "function", Function: schema.FunctionCall{Name: "memory_write", Arguments: `{"content":"remember this"}`}}

	f := newMemoryAgentFixture(t, &stubChat{reply: "x"}, nil)
	_, err := f.agent.runTool(f.agent.memoryToolContext(ctx, AgentInput{AgentKind: llm.AgentKindRego, SessionID: 7}), tc)
	if err == nil || !strings.Contains(err.Error(), "memory: empty namespace name") {
		t.Errorf("runTool(with memory runtime) error = %v, want the store to reject the empty namespace", err)
	}

	plain := newAgentWithTools(t, &stubChat{reply: "x"}, &stubRetriever{}, []tool.BaseTool{f.tool}, nil, nil)
	_, err = plain.runTool(plain.memoryToolContext(ctx, AgentInput{NamespaceName: "default"}), tc)
	if err == nil || !strings.Contains(err.Error(), "no memory runtime in context") {
		t.Errorf("runTool(without memory runtime) error = %v, want the absent-runtime error", err)
	}
}

// TestMemoryStoreWriter_Validation 测试写入适配器的入参校验：store 缺失、命名空间为空与正文为
// 空均在触碰数据库前返回英文错误（故无需真实向量库即可覆盖）。
func TestMemoryStoreWriter_Validation(t *testing.T) {
	ctx := context.Background()
	if _, err := (memoryStoreWriter{}).WriteMemory(ctx, tools.MemoryWriteInput{NamespaceName: "ns", Content: "c"}); err == nil {
		t.Error("WriteMemory(nil store) = nil error, want an error")
	}
	params := testParams(t, nil)
	store, err := NewMemoryStore(newTestSessionDB(t), stubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	writer := memoryStoreWriter{store: store}
	if _, err := writer.WriteMemory(ctx, tools.MemoryWriteInput{Content: "c"}); err == nil {
		t.Error("WriteMemory(empty namespace) = nil error, want an error")
	}
	if _, err := writer.WriteMemory(ctx, tools.MemoryWriteInput{NamespaceName: "ns", SessionID: 1, Content: "  "}); err == nil {
		t.Error("WriteMemory(empty content) = nil error, want an error")
	}
	// 无会话（SessionID 为 0）时拒绝写入：记忆按会话隔离，无会话运行不生成记忆
	if _, err := writer.WriteMemory(ctx, tools.MemoryWriteInput{NamespaceName: "ns", Content: "c"}); err == nil {
		t.Error("WriteMemory(empty session) = nil error, want an error")
	}
	// 导入环守卫：该适配器满足 internal/ai/tools 的写入接口。
	var _ tools.MemoryWriter = writer
}

// TestStreamRun_RollingSummaryBelowThreshold 测试未达阈值时不触发滚动摘要：被挤出消息条数不足
// 时不调用 flash、不注入摘要段、不推进游标。
func TestStreamRun_RollingSummaryBelowThreshold(t *testing.T) {
	f := newRollingSummaryFixture(t, 5)
	ctx := context.Background()
	drainStreamRun(t, f.agent, ctx, AgentInput{Query: "q3", SessionID: f.session.ID, NamespaceName: "default"})

	if len(f.memoryChat.gotInputs) != 0 {
		t.Fatalf("memory chat calls = %d, want 0 (below the threshold)", len(f.memoryChat.gotInputs))
	}
	if len(f.agentChat.gotInputs) != 1 {
		t.Fatalf("agent chat calls = %d, want 1", len(f.agentChat.gotInputs))
	}
	for _, msg := range f.agentChat.gotInputs[0] {
		if strings.Contains(msg.Content, agentSummarySectionTitle) {
			t.Errorf("agent input unexpectedly carries a summary section: %q", msg.Content)
		}
	}
	got, err := f.store.GetSession(ctx, f.session.ID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if got.Summary != "" || got.SummaryUptoMessageID != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (not advanced)", got.Summary, got.SummaryUptoMessageID)
	}
}
