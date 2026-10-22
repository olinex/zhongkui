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
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// stubChat 是测试用对话模型替身：实现 model.BaseChatModel，记录收到的消息，
// 按配置返回完整回复（Generate）或分块流（Stream）。
type stubChat struct {
	reply     string              // Generate 的完整回复
	chunks    []string            // Stream 的分块；为空时退回单块 reply
	reason    []string            // Stream 的思考分块（ReasoningContent），在正文分块之前外发
	usage     *schema.TokenUsage  // 非空时随响应元信息上报的 token 用量（Generate 直接附带；Stream 随末帧附带）
	gotInputs [][]*schema.Message // 记录每次调用收到的消息列表
	gotOpts   [][]model.Option    // 记录每次调用收到的选项
}

// responseMeta 按配置的用量构造响应元信息；未配置用量时返回 nil。
func (s *stubChat) responseMeta() *schema.ResponseMeta {
	if s.usage == nil {
		return nil
	}
	return &schema.ResponseMeta{Usage: s.usage}
}

// Generate 记录输入消息并返回配置的完整回复（思考内容一并聚合返回）。
func (s *stubChat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	s.gotInputs = append(s.gotInputs, input)
	s.gotOpts = append(s.gotOpts, opts)
	return &schema.Message{Role: schema.Assistant, Content: s.reply, ReasoningContent: strings.Join(s.reason, ""), ResponseMeta: s.responseMeta()}, nil
}

// Stream 记录输入消息并返回配置的分块流，未配置分块时退回单块回复；思考分块先于正文分块外发；
// 配置了用量时再补发一帧仅承载用量的空正文帧（用量常落在末帧，供采集用量而正文不受影响）。
func (s *stubChat) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	s.gotInputs = append(s.gotInputs, input)
	s.gotOpts = append(s.gotOpts, opts)
	chunks := s.chunks
	if len(chunks) == 0 {
		chunks = []string{s.reply}
	}
	extra := 0
	if s.usage != nil {
		extra = 1
	}
	sr, sw := schema.Pipe[*schema.Message](len(chunks) + len(s.reason) + extra)
	go func() {
		defer sw.Close()
		for _, r := range s.reason {
			sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: r}, nil)
		}
		for _, c := range chunks {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: c}, nil)
		}
		if meta := s.responseMeta(); meta != nil {
			sw.Send(&schema.Message{Role: schema.Assistant, ResponseMeta: meta}, nil)
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
	return func(slot ai.Slot) (model.BaseChatModel, error) { return chat, nil }
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
	agent, err := NewAgent(context.Background(), stubResolver(chat), retriever, store, params, nil, nil, nil, nil, withTestCapabilities())
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return agent
}

// newKnowledgeToolAgent 构造带真实 knowledge_search 工具的测试 Agent：经内置工具注册表
// 构造工具实例并取其 schema 清单，再复用 newAgentWithTools 装配（检索器替身由调用方提供）。
func newKnowledgeToolAgent(t *testing.T, chat model.BaseChatModel, rtv retriever.Retriever) *Agent {
	t.Helper()
	ctx := context.Background()
	built, err := tools.Build(ctx, "knowledge_search", tools.Deps{})
	if err != nil {
		t.Fatalf("tools.Build(knowledge_search) error = %v", err)
	}
	info, err := built.Info(ctx)
	if err != nil {
		t.Fatalf("knowledge_search Info error = %v", err)
	}
	return newAgentWithTools(t, chat, rtv, []tool.BaseTool{built}, []*schema.ToolInfo{info}, nil)
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
		params   *Params
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
// 草稿，Rego 规则（无）下 Valid 恒真；模型未调用 knowledge_search，故引用为空。
func TestAgent_Generate_Structured(t *testing.T) {
	chat := &stubChat{reply: `{"content": "**hello**", "draft": {"rule": {"name": "http-rule"}}}`}
	agent := newTestAgent(t, chat, &stubRetriever{})

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
// done 项携带聚合全文；模型未调用 knowledge_search，故引用为空。
func TestAgent_Stream_DeltasAndDone(t *testing.T) {
	chat := &stubChat{chunks: []string{"Hello", " world", "!"}}
	agent := newTestAgent(t, chat, &stubRetriever{})

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
	if done.Content != "Hello world!" || len(done.Citations) != 0 {
		t.Errorf("done = %+v, want the aggregated content and no citations", done)
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

// TestBuildAgentMessages 测试消息组装顺序：系统提示 + 历史 + 用户消息（业务现场 + 提问）。
func TestBuildAgentMessages(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("prev q"), schema.AssistantMessage("prev a", nil)}
	data := BusinessData{"src": "192.168.1.1"}
	messages := buildAgentMessages(testParams(t, nil), ai.AgentKindKnowledge, "", nil, history, data, "  current q  ")
	if len(messages) != 4 {
		t.Fatalf("buildAgentMessages() len = %d, want 4 (system + 2 history + user)", len(messages))
	}
	if messages[0].Role != schema.System || !strings.Contains(messages[0].Content, "ZhongKui") {
		t.Errorf("messages[0] = %+v, want the agent system prompt", messages[0])
	}
	if !strings.Contains(messages[0].Content, "format=capability") {
		t.Errorf("messages[0] 缺少能力发现指引: %q", messages[0].Content)
	}
	user := messages[3]
	if user.Role != schema.User {
		t.Errorf("messages[3] role = %v, want user", user.Role)
	}
	for _, want := range []string{"## Current Business Scene", "192.168.1.1", "## User Question", "current q"} {
		if !strings.Contains(user.Content, want) {
			t.Errorf("user content missing %q: %q", want, user.Content)
		}
	}
}

// TestToSchemaMessages 测试持久化消息转 eino 消息：角色映射、跳过空内容与未知角色。
func TestToSchemaMessages(t *testing.T) {
	messages := []*ai.AgentMessage{
		{Role: ai.MessageRoleUser, Content: encTestValue("u")},
		{Role: ai.MessageRoleAssistant, Content: encTestValue("a")},
		{Role: ai.MessageRoleSystem, Content: encTestValue("s")},
		{Role: ai.MessageRoleUser, Content: encTestValue("  ")},
		{Role: "robot", Content: encTestValue("x")},
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
// 携带正文、草稿与引用（引用由模型调用 knowledge_search 触发检索后写入）；
// LatestAssistantMessage 可回读。
func TestStreamRun_PersistsMessages(t *testing.T) {
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "knowledge_search", Arguments: `{"query":"hello"}`}}}},
		{content: `{"content": "answer", "draft": {"rule": "r1"}}`},
	}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "3",
		Content:  "kb",
		MetaData: map[string]any{"documentID": "3", "title": "KB Doc", "chunkIndex": 0, "score": 0.7},
	}}}
	agent := newKnowledgeToolAgent(t, chat, rtv)
	session, err := agent.store.CreateSession(context.Background(), "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	reader, err := agent.StreamRun(context.Background(), AgentInput{
		Query: "hello", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
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
	messages, err := agent.store.ListMessages(context.Background(), session.ID, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + assistant)", len(messages))
	}
	if messages[0].Role != ai.MessageRoleUser || string(messages[0].Content.Value()) != "hello" {
		t.Errorf("messages[0] = %+v, want the persisted user message", messages[0])
	}
	if messages[1].Role != ai.MessageRoleAssistant || string(messages[1].Content.Value()) != "answer" {
		t.Errorf("messages[1] = %+v, want the persisted assistant message", messages[1])
	}
	if draft := messages[1].DraftJSON.Value(); len(draft) == 0 || !strings.Contains(string(draft), "r1") {
		t.Errorf("messages[1].DraftJSON = %q, want the persisted draft", draft)
	}
	if citations := messages[1].CitationsJSON.Value(); len(citations) == 0 || !strings.Contains(string(citations), "KB Doc") {
		t.Errorf("messages[1].CitationsJSON = %q, want the persisted citations", citations)
	}
	latest, err := agent.store.LatestAssistantMessage(context.Background(), session.ID, session.UserID)
	if err != nil || string(latest.Content.Value()) != "answer" {
		t.Errorf("LatestAssistantMessage() = (%+v, %v), want the answer", latest, err)
	}
}

// TestStreamRun_ReasoningStreamedAndPersisted 测试思考过程通道：携带 ReasoningContent 的模型流
// 按到达顺序外发 reasoning 增量项（与 delta 通道并行），done 项携带思考全文；助手消息落库时
// reasoning 列非空且可回读，正文通道与用户消息保持原样（用户消息不写思考）。
func TestStreamRun_ReasoningStreamedAndPersisted(t *testing.T) {
	chat := &stubChat{reason: []string{"think ", "harder"}, chunks: []string{"Hello"}}
	agent := newTestAgent(t, chat, &stubRetriever{})
	ctx := context.Background()
	session, err := agent.store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	reader, err := agent.StreamRun(ctx, AgentInput{Query: "q", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID})
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	defer reader.Close()
	var reasoningDeltas, deltas []string
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
		if item.Reasoning != "" {
			reasoningDeltas = append(reasoningDeltas, item.Reasoning)
		}
		if item.Delta != "" {
			deltas = append(deltas, item.Delta)
		}
	}
	if got := strings.Join(reasoningDeltas, ""); got != "think harder" {
		t.Errorf("reasoning deltas = %q, want %q", got, "think harder")
	}
	if got := strings.Join(deltas, ""); got != "Hello" {
		t.Errorf("content deltas = %q, want %q", got, "Hello")
	}
	if done == nil || done.Reasoning != "think harder" || done.Content != "Hello" {
		t.Fatalf("done = %+v, want reasoning and content full text", done)
	}
	messages, err := agent.store.ListMessages(ctx, session.ID, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + assistant)", len(messages))
	}
	if got := reasoningTextOf(t, agent.store, messages[1]); got != "think harder" {
		t.Errorf("persisted reasoning = %q, want %q", got, "think harder")
	}
	if string(messages[1].Content.Value()) != "Hello" {
		t.Errorf("persisted content = %q, want %q", messages[1].Content.Value(), "Hello")
	}
	if got := reasoningTextOf(t, agent.store, messages[0]); got != "" {
		t.Errorf("user message reasoning = %q, want none", got)
	}
}

// thinkingStubChat 在 stubChat 之上实现 requestOptionsProvider（与 *llm.ChatClient 一致的可选能力）：
// 记录 generate 传入的思考模式，并返回单个选项用于验证其被透传给 Stream。
type thinkingStubChat struct {
	stubChat
	thinkings []string
}

// RequestOptions 记录思考模式并返回单个选项。
func (c *thinkingStubChat) RequestOptions(thinking string) []model.Option {
	c.thinkings = append(c.thinkings, thinking)
	return []model.Option{model.WithTemperature(0.42)}
}

// TestGenerate_ThinkingRequestOptionsApplied 测试思考开关透传（无工具单次流式路径）：对话客户端
// 实现 requestOptionsProvider 时，generate 以本次输入的思考模式调用其 RequestOptions，并把返回的
// 选项透传给 Stream。
func TestGenerate_ThinkingRequestOptionsApplied(t *testing.T) {
	chat := &thinkingStubChat{}
	chat.reply = "hi"
	agent := newTestAgent(t, chat, &stubRetriever{})

	if _, err := agent.Generate(context.Background(), AgentInput{Query: "q", Thinking: "on"}); err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if len(chat.thinkings) != 1 || chat.thinkings[0] != "on" {
		t.Errorf("RequestOptions thinkings = %v, want [on]", chat.thinkings)
	}
	if len(chat.gotOpts) != 1 || len(chat.gotOpts[0]) != 1 {
		t.Errorf("Stream opts = %v, want exactly one option passed from RequestOptions", chat.gotOpts)
	}
}

// TestAgent_Generate_KnowledgeToolCitations 测试模型调用 knowledge_search 后的引用回传：
// 首轮工具调用触发一次检索，命中经采集器写入 done.Citations，命中结果作为工具消息回填次轮。
func TestAgent_Generate_KnowledgeToolCitations(t *testing.T) {
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "knowledge_search", Arguments: `{"query":"how to build a rule"}`}}}},
		{content: `{"content": "answer", "draft": {}}`},
	}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "7",
		Content:  "chunk body",
		MetaData: map[string]any{"documentID": "7", "title": "Doc A", "chunkIndex": 0, "score": 0.9},
	}}}
	agent := newKnowledgeToolAgent(t, chat, rtv)

	out, err := agent.Generate(context.Background(), AgentInput{
		Query: "how to build a rule?", NamespaceName: "default", DocIDs: []uint{7},
	})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if out.Content != "answer" {
		t.Errorf("Generate() content = %q, want %q", out.Content, "answer")
	}
	if len(out.Citations) != 1 || out.Citations[0].Title != "Doc A" {
		t.Errorf("Generate() citations = %+v, want the knowledge-search hit", out.Citations)
	}
	if chat.calls != 2 {
		t.Fatalf("chat calls = %d, want 2 (tool round + answer round)", chat.calls)
	}
	toolMsg := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if toolMsg.Role != schema.Tool || !strings.Contains(toolMsg.Content, "Doc A") {
		t.Errorf("second-round tool message = %+v, want the knowledge hit fed back", toolMsg)
	}
}

// TestAgent_KnowledgeTool_ScopeFromRuntime 测试命名空间/文档限定由运行上下文注入而非模型入参：
// 模型入参只有 query，适配器据本次运行输入把命名空间与文档限定传给检索器。
func TestAgent_KnowledgeTool_ScopeFromRuntime(t *testing.T) {
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "knowledge_search", Arguments: `{"query":"q"}`}}}},
		{content: `{"content": "answer", "draft": {}}`},
	}}
	rtv := &stubRetriever{}
	agent := newKnowledgeToolAgent(t, chat, rtv)

	if _, err := agent.Generate(context.Background(), AgentInput{
		Query: "q2", NamespaceName: "tenant-a", DocIDs: []uint{7, 9},
	}); err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if rtv.query != "q" {
		t.Errorf("retriever query = %q, want the model-provided query %q", rtv.query, "q")
	}
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{}, rtv.opts...)
	if o.NamespaceName != "tenant-a" {
		t.Errorf("retriever namespace = %q, want %q", o.NamespaceName, "tenant-a")
	}
	if len(o.DocIDs) != 2 || o.DocIDs[0] != 7 || o.DocIDs[1] != 9 {
		t.Errorf("retriever docIDs = %v, want [7 9]", o.DocIDs)
	}
}

// TestAgent_KnowledgeTool_NotCalled 测试模型未调用工具时无检索：单轮纯文本回复不触发
// knowledge_search，检索器不被调用，done.Citations 为空。
func TestAgent_KnowledgeTool_NotCalled(t *testing.T) {
	chat := &scriptedChat{turns: []scriptedTurn{{content: `{"content": "answer", "draft": {}}`}}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "7",
		Content:  "chunk body",
		MetaData: map[string]any{"documentID": "7", "title": "Doc A", "chunkIndex": 0, "score": 0.9},
	}}}
	agent := newKnowledgeToolAgent(t, chat, rtv)

	out, err := agent.Generate(context.Background(), AgentInput{Query: "q", NamespaceName: "default"})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if len(out.Citations) != 0 {
		t.Errorf("Generate() citations = %+v, want none when the model does not call the tool", out.Citations)
	}
	if rtv.query != "" {
		t.Errorf("retriever query = %q, want the retriever untouched", rtv.query)
	}
}

// TestStreamRun_InjectsHistory 测试 StreamRun 注入历史：先持久化一轮问答，
// 第二轮调用时替身收到的消息列表包含上一轮的历史。
func TestStreamRun_InjectsHistory(t *testing.T) {
	chat := &stubChat{reply: `{"content": "a2", "draft": {}}`}
	agent := newTestAgent(t, chat, &stubRetriever{})
	ctx := context.Background()
	session, err := agent.store.CreateSession(ctx, "default", "t", "m", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	// 预置一轮历史（user + assistant）
	for _, m := range []*ai.AgentMessage{
		{SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleUser, Content: encTestValue("q1")},
		{SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleAssistant, Content: encTestValue("a1")},
	} {
		if _, err := agent.store.AppendMessage(ctx, m); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}

	reader, err := agent.StreamRun(ctx, AgentInput{Query: "q2", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID})
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
func slotChatResolver(bySlot map[ai.Slot]model.BaseChatModel) ChatResolver {
	return func(slot ai.Slot) (model.BaseChatModel, error) {
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
			out = append(out, &ai.AgentMessage{Content: encTestValue(c)})
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
				if got := string(got[i].Content.Value()); got != tt.want[i] {
					t.Errorf("trimMessagesByBytes()[%d] = %q, want %q", i, got, tt.want[i])
				}
			}
		})
	}
}

// TestBuildAgentMessages_SummaryInjected 测试摘要段的注入位置：系统提示 + 摘要段 + 历史消息 +
// 用户消息，摘要段为独立 system 消息且以英文标题常量开头。
func TestBuildAgentMessages_SummaryInjected(t *testing.T) {
	history := []*schema.Message{schema.UserMessage("prev q"), schema.AssistantMessage("prev a", nil)}
	messages := buildAgentMessages(testParams(t, nil), ai.AgentKindKnowledge, "  folded  ", nil, history, nil, "q")
	if len(messages) != 5 {
		t.Fatalf("buildAgentMessages() len = %d, want 5 (system + summary + 2 history + user)", len(messages))
	}
	if messages[0].Role != schema.System {
		t.Errorf("messages[0] role = %v, want system", messages[0].Role)
	}
	if messages[1].Role != schema.System || !strings.HasPrefix(messages[1].Content, agentSummarySectionTitle) || !strings.Contains(messages[1].Content, "folded") {
		t.Errorf("messages[1] = %+v, want the summary section after the system prompt", messages[1])
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
// 窗口取最近 1 条，故前 3 条被挤出，其最后一条 seq 即摘要游标上界。
type rollingSummaryFixture struct {
	agent          *Agent           // 被测 Agent（已注入 Memory）
	store          *SessionStore    // 会话存储（回读摘要用）
	session        *ai.AgentSession // 预置会话
	memoryChat     *stubChat        // flash 档位替身：记忆摘要
	agentChat      *stubChat        // standard 档位替身：Agent 生成
	evictedLastSeq uint             // 被挤出消息的最后一条 seq
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
		slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: memoryChat}), params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	agent, err := NewAgent(ctx,
		slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotStandard: agentChat}),
		&stubRetriever{}, store, params, memory, nil, nil, nil, withTestCapabilities())
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string) *ai.AgentMessage {
		t.Helper()
		msg := &ai.AgentMessage{SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content)}
		if _, err := store.AppendMessage(ctx, msg); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
		return msg
	}
	appendMsg(ai.MessageRoleUser, "u1")
	appendMsg(ai.MessageRoleAssistant, "a1")
	evictedLast := appendMsg(ai.MessageRoleUser, "u2")
	appendMsg(ai.MessageRoleAssistant, "a2")
	return &rollingSummaryFixture{
		agent:          agent,
		store:          store,
		session:        session,
		memoryChat:     memoryChat,
		agentChat:      agentChat,
		evictedLastSeq: uint(evictedLast.Seq),
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
	drainStreamRun(t, f.agent, ctx, AgentInput{Query: "q3", SessionID: f.session.ID, NamespaceName: "default", UserID: f.session.UserID})

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
	got, err := f.store.GetSession(ctx, f.session.ID, f.session.NamespaceName, f.session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "folded summary" || got.SummaryUptoSeq != f.evictedLastSeq {
		t.Errorf("persisted summary = (%q, %d), want (%q, %d)",
			got.Summary.Value(), got.SummaryUptoSeq, "folded summary", f.evictedLastSeq)
	}
}

// TestAgent_CompressSession 测试统一流式压缩例程：report 依次收到 start（含占用/容量/阈值）、
// 若干 delta、done（含折叠条数、摘要游标与压缩后占用估算）；evicted 为空时只有 start +
// done(FoldedCount=0)；摘要模型失败时仅告警、返回 nil，且以 foldedCount=0 的 done 收尾。
func TestAgent_CompressSession(t *testing.T) {
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"memory.summarySystemPrompt":    "sp",
		"memory.enabled":                "false",
		"session.compactThresholdRatio": "0.8",
	})
	// 摘要模型上报用量：压缩前占用 500 − prompt 120 + completion 20 = 估算 400。
	memory := newTestMemory(t, db, &stubChat{
		chunks: []string{"folded ", "summary"},
		usage:  &schema.TokenUsage{PromptTokens: 120, CompletionTokens: 20},
	}, stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
		"memory.enabled":             "false",
	})
	agentChat := &stubChat{reply: "unused"}
	agent, err := NewAgent(ctx, stubResolver(agentChat), &stubRetriever{}, store, params, memory, nil, nil, nil, WithSlotContextLimit(func(slot ai.Slot) int {
		switch slot {
		case ai.SlotStandard:
			return 128000
		case ai.SlotFlash:
			return 32000
		default:
			return 0
		}
	}))
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string, promptTokens int) {
		t.Helper()
		if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: role, Content: encTestValue(content), PromptTokens: promptTokens,
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	appendMsg(ai.MessageRoleUser, "u1", 0)
	appendMsg(ai.MessageRoleAssistant, "a1", 0)
	appendMsg(ai.MessageRoleUser, "u2", 0)
	// 最新助手消息的 prompt tokens 即压缩前占用（500），故估算压缩后占用为 500-120+20=400。
	appendMsg(ai.MessageRoleAssistant, "a2", 500)
	evicted, err := store.ListMessagesAfter(ctx, session.ID, session.UserID, 0)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}

	var events []CompressEvent
	report := func(e CompressEvent) error {
		events = append(events, e)
		return nil
	}
	result := agent.CompressSession(ctx, session, evicted, "manual", "", report)
	if result == nil || result.Summary != "folded summary" {
		t.Fatalf("CompressSession() = %+v, want the folded summary", result)
	}
	if len(events) != 4 {
		t.Fatalf("events = %+v, want start + 2 deltas + done", events)
	}
	start := events[0]
	if start.Type != ai.AgentRunEventCompressStart || start.Reason != "manual" || start.Limit != 128000 || start.Threshold != 0.8 {
		t.Errorf("start event = %+v, want the manual start with the standard slot limit", start)
	}
	if events[1].Type != ai.AgentRunEventCompressDelta || events[1].Delta != "folded " ||
		events[2].Type != ai.AgentRunEventCompressDelta || events[2].Delta != "summary" {
		t.Errorf("delta events = %+v, want the streamed summary chunks", events[1:3])
	}
	done := events[3]
	if done.Type != ai.AgentRunEventCompressDone || done.FoldedCount != 4 || done.SummaryUptoSeq != 4 || done.Reason != "manual" || done.TokensBefore != start.Tokens {
		t.Errorf("done event = %+v, want 4 folded messages and cursor 4", done)
	}
	if done.TokensAfter != 400 {
		t.Errorf("done event TokensAfter = %d, want the estimated 400", done.TokensAfter)
	}
	if session.SummaryUptoSeq != 4 || string(session.Summary.Value()) != "folded summary" {
		t.Errorf("session after compression = (%q, %d), want the folded summary in place", session.Summary.Value(), session.SummaryUptoSeq)
	}
	// 压缩后占用的即时估算随检查点落库，供刷新 / 重进会话后回显前后对比与展示占用下调。
	var checkpoint ai.AgentSessionCompaction
	if err := db.Order("id DESC").First(&checkpoint).Error; err != nil {
		t.Fatalf("loading the compaction checkpoint error = %v", err)
	}
	if checkpoint.TokensBefore != start.Tokens || checkpoint.FoldedCount != 4 || checkpoint.TokensAfter != 400 {
		t.Errorf("checkpoint metrics = (tokens_before=%d, folded_count=%d, tokens_after=%d), want (%d, 4, 400)",
			checkpoint.TokensBefore, checkpoint.FoldedCount, checkpoint.TokensAfter, start.Tokens)
	}

	// evicted 为空：只有 start 与 foldedCount=0 的 done；显式 slot 覆盖会话档位（flash 容量 32000）。
	events = nil
	if got := agent.CompressSession(ctx, session, nil, "auto", ai.SlotFlash, report); got == nil {
		t.Fatal("CompressSession(no evicted) returned nil, want the existing summary")
	}
	if len(events) != 2 || events[0].Type != ai.AgentRunEventCompressStart || events[0].Limit != 32000 ||
		events[1].Type != ai.AgentRunEventCompressDone || events[1].FoldedCount != 0 {
		t.Errorf("events = %+v, want just start(flash limit 32000) + done(foldedCount=0)", events)
	}

	// 摘要模型失败：resolver 缺失 -> 仅告警、返回 nil，以 foldedCount=0 的 done 收尾。
	failing := newTestMemory(t, newTestSessionDB(t), nil, stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
	})
	failingAgent, err := NewAgent(ctx, stubResolver(agentChat), &stubRetriever{}, store, params, failing, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent(failing memory) error = %v", err)
	}
	events = nil
	if got := failingAgent.CompressSession(ctx, session, evicted, "auto", "", report); got != nil {
		t.Errorf("CompressSession(summary failure) = %+v, want nil", got)
	}
	if len(events) != 2 || events[1].Type != ai.AgentRunEventCompressDone || events[1].FoldedCount != 0 {
		t.Errorf("failure events = %+v, want start + done(foldedCount=0)", events)
	}
}

// TestEstimatedTokensAfter_Guards 测试压缩后占用估算的守卫：仅在输入可信且结果合理时给出量值，
// 否则一律返回 0（调用方据此不携带 TokensAfter、回落为等待真实测量回填）。重点覆盖「压缩前占用
// 为 0（无基线）」这一来自真实缺陷的入力——公式会因「摘要 completion 大于摘要 prompt」而得出比
// 压缩前更大的无意义正值。
func TestEstimatedTokensAfter_Guards(t *testing.T) {
	cases := []struct {
		name   string
		before int
		usage  *schema.TokenUsage
		want   int
	}{
		{"metrics and a plausible fold", 500, &schema.TokenUsage{PromptTokens: 120, CompletionTokens: 20}, 400},
		{"no usage from the provider", 500, nil, 0},
		{"no baseline, per-message content-like usage", 0, &schema.TokenUsage{PromptTokens: 0, CompletionTokens: 122}, 0},
		{"no baseline with a plain usage", 0, &schema.TokenUsage{PromptTokens: 120, CompletionTokens: 20}, 0},
		{"fold would not shrink the context", 500, &schema.TokenUsage{PromptTokens: 0, CompletionTokens: 30}, 0},
		{"fold would empty the context", 500, &schema.TokenUsage{PromptTokens: 500, CompletionTokens: 0}, 0},
	}
	for _, tc := range cases {
		if got := estimatedTokensAfter(tc.before, tc.usage); got != tc.want {
			t.Errorf("estimatedTokensAfter(%d, %+v) = %d, want %d", tc.before, tc.usage, got, tc.want)
		}
	}
}

// TestEstimatedCompactedTokens 测试压缩后占用的即时估算口径：优先取摘要模型用量算出的估值；用量
// 缺失或该估值不可用（不构成缩减）时回落为按字节折算的估值，使压缩完成即刻总有可展示的占用值；压缩
// 前占用为 0（无基线）或按字节折算也不构成缩减时返回 0，回落为等待真实测量回填。
func TestEstimatedCompactedTokens(t *testing.T) {
	// "user" + ": " + "hello world" + "\n" = 18 字节 → 折算 18/3 = 6 个 token。
	folded := []*ai.AgentMessage{{Role: ai.MessageRoleUser, Content: encTestValue("hello world")}}
	cases := []struct {
		name    string
		before  int
		usage   *schema.TokenUsage
		evicted []*ai.AgentMessage
		summary string
		want    int
	}{
		{"usage from the provider wins", 500, &schema.TokenUsage{PromptTokens: 120, CompletionTokens: 20}, folded, "s", 400},
		{"no usage falls back to the byte estimate", 500, nil, folded, "s", 495},
		{"unusable usage also falls back", 500, &schema.TokenUsage{PromptTokens: 0, CompletionTokens: 30}, folded, "s", 495},
		{"no baseline gives no estimate", 0, nil, folded, "s", 0},
		{"fold that does not shrink gives no estimate", 500, nil, folded, strings.Repeat("s", 60), 0},
	}
	for _, tc := range cases {
		if got := estimatedCompactedTokens(tc.before, tc.usage, tc.evicted, tc.summary, 0); got != tc.want {
			t.Errorf("estimatedCompactedTokens(%s) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestAgent_CompressSession_KeepsMessagesAndBlocks 测试压缩不改动会话消息与内容块：压缩只把被挤出的
// 历史折叠进滚动摘要并前移摘要游标（summary_upto_seq），既不删除消息也不删除其块（块不参与摘要折叠）。
// 压缩前后消息条数与块条数须完全一致，且每块的载荷保持不变。
func TestAgent_CompressSession_KeepsMessagesAndBlocks(t *testing.T) {
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"memory.summarySystemPrompt": "sp",
		"memory.enabled":             "false",
	})
	memory := newTestMemory(t, db, &stubChat{chunks: []string{"folded ", "summary"}}, stubEmbedder{}, map[string]string{
		"memory.summarySystemPrompt": "sp",
		"memory.enabled":             "false",
	})
	agent, err := NewAgent(ctx, stubResolver(&stubChat{reply: "unused"}), &stubRetriever{}, store, params, memory, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendAssistantWithBlock := func(content string) {
		t.Helper()
		saved, aerr := store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID, Role: ai.MessageRoleAssistant, Content: encTestValue(content),
		})
		if aerr != nil {
			t.Fatalf("AppendMessage error = %v", aerr)
		}
		if aerr := store.AppendBlock(ctx, &ai.AgentMessageBlock{
			NamespaceName: "default", UserID: session.UserID, MessageID: saved.ID, BlockSeq: 1,
			Kind: ai.AgentMessageBlockKindContent, Payload: encTestValue(`{"text":"` + content + `"}`),
		}); aerr != nil {
			t.Fatalf("AppendBlock error = %v", aerr)
		}
	}
	appendAssistantWithBlock("a1")
	appendAssistantWithBlock("a2")
	evicted, err := store.ListMessagesAfter(ctx, session.ID, session.UserID, 0)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}
	if len(evicted) != 2 {
		t.Fatalf("evicted len = %d, want 2", len(evicted))
	}
	countBlocks := func() int64 {
		t.Helper()
		var n int64
		if cerr := db.Raw("SELECT count(*) FROM agent_message_blocks WHERE user_id = ?", session.UserID).Scan(&n).Error; cerr != nil {
			t.Fatalf("counting blocks error = %v", cerr)
		}
		return n
	}
	messagesBefore := len(evicted)
	blocksBefore := countBlocks()

	report := func(CompressEvent) error { return nil }
	if got := agent.CompressSession(ctx, session, evicted, "manual", "", report); got == nil || got.Summary != "folded summary" {
		t.Fatalf("CompressSession() = %+v, want the folded summary", got)
	}
	if session.SummaryUptoSeq != 2 {
		t.Errorf("summary_upto_seq = %d, want 2 (the cursor advances to the last folded seq)", session.SummaryUptoSeq)
	}
	after, err := store.ListMessages(ctx, session.ID, session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(after) != messagesBefore {
		t.Errorf("messages after compression = %d, want %d (compression must not delete messages)", len(after), messagesBefore)
	}
	if blocks := countBlocks(); blocks != blocksBefore {
		t.Errorf("blocks after compression = %d, want %d (compression must not delete blocks)", blocks, blocksBefore)
	}
}

// TestBuildAgentMessages_MemoryInjected 测试长期记忆分节的注入位置与格式：系统提示 + 摘要段 +
// 记忆段 + 窗口历史 + 用户消息；记忆段为独立 system 消息，条目形如 "- [kind] title: content"，
// 正文为空（去空白后）的命中被跳过。
func TestBuildAgentMessages_MemoryInjected(t *testing.T) {
	params := testParams(t, map[string]string{"memory.sectionTitle": "## Long-term Memory"})
	history := []*schema.Message{schema.UserMessage("prev q")}
	memories := []MemoryHit{
		{ID: 1, Kind: ai.MemoryKindDecision, Title: "chosen rule", Content: "use deny-by-default"},
		{ID: 2, Kind: ai.MemoryKindFact, Content: "the team prefers rego v1"},
		{ID: 3, Kind: ai.MemoryKindFact, Title: "blank", Content: "   "},
	}
	messages := buildAgentMessages(params, ai.AgentKindKnowledge, "folded", memories, history, nil, "q")
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
	for _, memories := range [][]MemoryHit{nil, {{ID: 1, Kind: ai.MemoryKindFact, Content: "  "}}} {
		messages := buildAgentMessages(testParams(t, nil), ai.AgentKindKnowledge, "", memories, nil, nil, "q")
		if len(messages) != 2 {
			t.Fatalf("buildAgentMessages(%+v) len = %d, want 2 (system + user)", memories, len(messages))
		}
		if messages[0].Role != schema.System || !strings.Contains(messages[0].Content, "format=capability") {
			t.Errorf("messages[0] = %+v, want the system prompt with the capability hint", messages[0])
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
	badKind.AgentKind = ai.AgentKind("bogus")
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
		Query: "q", NamespaceName: "default", AgentKind: ai.AgentKindRego,
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
	_, err := f.agent.runTool(f.agent.memoryToolContext(ctx, AgentInput{AgentKind: ai.AgentKindRego, SessionID: 7}), tc)
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
	// 无会话（SessionID 为空）时拒绝写入：记忆按会话隔离，无会话运行不生成记忆
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
	drainStreamRun(t, f.agent, ctx, AgentInput{Query: "q3", SessionID: f.session.ID, NamespaceName: "default", UserID: f.session.UserID})

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
	got, err := f.store.GetSession(ctx, f.session.ID, f.session.NamespaceName, f.session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if len(got.Summary.Value()) != 0 || got.SummaryUptoSeq != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (not advanced)", got.Summary.Value(), got.SummaryUptoSeq)
	}
}

// autoCompressFixture 是自动压缩接入用例的共享装配：sqlite 内存库（会话与记忆同源）、按档位分流的
// 对话替身（agentFlashChat / agentStandardChat 供 Agent 生成，memoryChat 供摘要），参数含小容量档位
// 与低阈值，便于在少量 token 占用下跨越阈值；seedSession 预置带占用量的会话。
type autoCompressFixture struct {
	agent             *Agent        // 被测 Agent（已注入 Memory）
	store             *SessionStore // 会话存储（回读摘要用）
	memoryChat        *stubChat     // flash 档位替身：记忆摘要
	agentStandardChat *stubChat     // standard 档位替身：Agent 生成
	agentFlashChat    *stubChat     // flash 档位替身：Agent 生成
}

// newAutoCompressFixture 构造上述装配（摘要解析器用内置 flash 替身）；overrides 覆盖参数。
func newAutoCompressFixture(t *testing.T, overrides map[string]string) *autoCompressFixture {
	t.Helper()
	return newAutoCompressFixtureWith(t, overrides, nil)
}

// newAutoCompressFixtureWith 同上，memoryResolver 非空时用作记忆摘要解析器（传 nil 用内置 flash
// 替身），便于构造摘要失败的降级用例。
func newAutoCompressFixtureWith(t *testing.T, overrides map[string]string, memoryResolver ChatResolver) *autoCompressFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	values := map[string]string{
		"session.maxContextMessages":       "1",
		"session.compactThresholdRatio":    "0.5",
		"memory.summarySystemPrompt":       "test summary system prompt",
		"memory.summaryMinEvictedMessages": "99",
	}
	for key, value := range overrides {
		values[key] = value
	}
	params := testParams(t, values)
	memoryChat := &stubChat{chunks: []string{"folded ", "summary"}}
	memoryStore, err := NewMemoryStore(db, stubEmbedder{}, params, nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	if memoryResolver == nil {
		memoryResolver = slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: memoryChat})
	}
	memory, err := NewMemory(memoryStore, memoryResolver, params, nil)
	if err != nil {
		t.Fatalf("NewMemory error = %v", err)
	}
	agentFlashChat := &stubChat{reply: `{"content": "ans-flash", "draft": {}}`}
	agentStandardChat := &stubChat{reply: `{"content": "ans", "draft": {}}`}
	agent, err := NewAgent(ctx,
		slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: agentFlashChat, ai.SlotStandard: agentStandardChat}),
		&stubRetriever{}, store, params, memory, nil, nil, nil,
		WithSlotContextLimit(func(slot ai.Slot) int {
			switch slot {
			case ai.SlotFlash:
				return 500
			case ai.SlotStandard:
				return 1000
			default:
				return 0
			}
		}))
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return &autoCompressFixture{
		agent:             agent,
		store:             store,
		memoryChat:        memoryChat,
		agentStandardChat: agentStandardChat,
		agentFlashChat:    agentFlashChat,
	}
}

// seedSession 预置一个会话与其历史：u1 / a1 / u2 / a2 四条，末条助手消息携带 occupancy 的输入
// token（ContextTokens 取最近一条助手消息的 PromptTokens），供自动压缩按占用比例判定。
func (f *autoCompressFixture) seedSession(t *testing.T, slot ai.Slot, occupancy int) *ai.AgentSession {
	t.Helper()
	ctx := context.Background()
	session, err := f.store.CreateSession(ctx, "default", "t", slot, "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	appendMsg := func(role ai.MessageRole, content string, promptTokens int) {
		t.Helper()
		if _, err := f.store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
			Role: role, Content: encTestValue(content), PromptTokens: promptTokens,
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	appendMsg(ai.MessageRoleUser, "u1", 0)
	appendMsg(ai.MessageRoleAssistant, "a1", 0)
	appendMsg(ai.MessageRoleUser, "u2", 0)
	appendMsg(ai.MessageRoleAssistant, "a2", occupancy)
	return session
}

// runAutoCompress 执行一次 StreamRun（注入捕获压缩事件的报告器，不经 RunHub），返回事件序列与
// done 项；流正常结束返回 nil 错误时 done 非空。
func runAutoCompress(t *testing.T, agent *Agent, ctx context.Context, input AgentInput) ([]CompressEvent, *AgentOutput) {
	t.Helper()
	var events []CompressEvent
	input.CompressReporter = func(e CompressEvent) error {
		events = append(events, e)
		return nil
	}
	reader, err := agent.StreamRun(ctx, input)
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	defer reader.Close()
	var done *AgentOutput
	for {
		item, err := reader.Recv()
		if err != nil {
			break
		}
		if item.Done {
			cp := item
			done = &cp
			break
		}
	}
	return events, done
}

// TestStreamRun_AutoCompressTriggers 测试占用比例达到阈值时自动流式压缩：生成前发出
// start → delta* → done 事件，折叠被挤出消息并推进游标、摘要落库，且压缩后摘要注入本轮生成，
// 本轮仍正常产出 done。
func TestStreamRun_AutoCompressTriggers(t *testing.T) {
	f := newAutoCompressFixture(t, nil)
	ctx := context.Background()
	session := f.seedSession(t, ai.SlotStandard, 800) // 800 / 1000 = 0.8 >= 0.5
	events, done := runAutoCompress(t, f.agent, ctx, AgentInput{
		Query: "q3", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
		DeclaredSlot: session.TargetModel,
	})

	if done == nil || done.Content != "ans" {
		t.Fatalf("done = %+v, want the generated answer", done)
	}
	if len(events) < 3 || events[0].Type != ai.AgentRunEventCompressStart {
		t.Fatalf("events = %+v, want start → delta* → done", events)
	}
	start := events[0]
	if start.Reason != "auto" || start.Tokens != 800 || start.Limit != 1000 || start.Threshold != 0.5 {
		t.Errorf("start event = %+v, want auto with occupancy 800 / limit 1000 / threshold 0.5", start)
	}
	for _, e := range events[1 : len(events)-1] {
		if e.Type != ai.AgentRunEventCompressDelta {
			t.Errorf("middle event = %+v, want compress_delta", e)
		}
	}
	last := events[len(events)-1]
	if last.Type != ai.AgentRunEventCompressDone || last.FoldedCount != 3 || last.SummaryUptoSeq != 3 || last.Reason != "auto" {
		t.Errorf("done event = %+v, want 3 folded messages, cursor 3, reason auto", last)
	}

	got, err := f.store.GetSession(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if string(got.Summary.Value()) != "folded summary" || got.SummaryUptoSeq != 3 {
		t.Errorf("persisted summary = (%q, %d), want (%q, 3)", got.Summary.Value(), got.SummaryUptoSeq, "folded summary")
	}

	// 压缩后摘要随本轮生成注入（系统提示之后、窗口历史之前的独立 system 段）。
	if len(f.agentStandardChat.gotInputs) != 1 {
		t.Fatalf("agent chat calls = %d, want 1", len(f.agentStandardChat.gotInputs))
	}
	injected := false
	for _, msg := range f.agentStandardChat.gotInputs[0] {
		if msg.Role == schema.System && strings.HasPrefix(msg.Content, agentSummarySectionTitle) && strings.Contains(msg.Content, "folded summary") {
			injected = true
		}
	}
	if !injected {
		t.Errorf("agent input = %+v, want the compressed summary section injected", f.agentStandardChat.gotInputs[0])
	}
}

// TestStreamRun_AutoCompressBelowThreshold 测试占用比例低于阈值时不压缩：无任何压缩事件、
// 不调用摘要模型、不推进游标，生成照常完成。
func TestStreamRun_AutoCompressBelowThreshold(t *testing.T) {
	f := newAutoCompressFixture(t, nil)
	ctx := context.Background()
	session := f.seedSession(t, ai.SlotStandard, 100) // 100 / 1000 = 0.1 < 0.5
	events, done := runAutoCompress(t, f.agent, ctx, AgentInput{
		Query: "q3", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
		DeclaredSlot: session.TargetModel,
	})

	if done == nil || done.Content != "ans" {
		t.Fatalf("done = %+v, want the generated answer", done)
	}
	if len(events) != 0 {
		t.Errorf("events = %+v, want none below the threshold", events)
	}
	if len(f.memoryChat.gotInputs) != 0 {
		t.Errorf("memory chat calls = %d, want 0 (no summarization)", len(f.memoryChat.gotInputs))
	}
	got, err := f.store.GetSession(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if len(got.Summary.Value()) != 0 || got.SummaryUptoSeq != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (not advanced)", got.Summary.Value(), got.SummaryUptoSeq)
	}
}

// TestStreamRun_AutoCompressBySlot 测试触发随有效档位变化：同一 token 占用下，flash 容量更小
// （300 / 500 >= 0.5）触发压缩，standard 容量更大（300 / 1000 < 0.5）不触发。
func TestStreamRun_AutoCompressBySlot(t *testing.T) {
	f := newAutoCompressFixture(t, nil)
	ctx := context.Background()
	const occupancy = 300

	flashSession := f.seedSession(t, ai.SlotFlash, occupancy)
	flashEvents, flashDone := runAutoCompress(t, f.agent, ctx, AgentInput{
		Query: "q3", SessionID: flashSession.ID, NamespaceName: "default", UserID: flashSession.UserID, Slot: ai.SlotFlash,
	})
	if flashDone == nil || flashDone.Content != "ans-flash" {
		t.Fatalf("flash done = %+v, want the generated answer", flashDone)
	}
	if len(flashEvents) == 0 || flashEvents[0].Limit != 500 {
		t.Errorf("flash events = %+v, want compression with the flash limit 500", flashEvents)
	}

	stdSession := f.seedSession(t, ai.SlotStandard, occupancy)
	stdEvents, stdDone := runAutoCompress(t, f.agent, ctx, AgentInput{
		Query: "q3", SessionID: stdSession.ID, NamespaceName: "default", UserID: stdSession.UserID, Slot: ai.SlotStandard,
	})
	if stdDone == nil || stdDone.Content != "ans" {
		t.Fatalf("standard done = %+v, want the generated answer", stdDone)
	}
	if len(stdEvents) != 0 {
		t.Errorf("standard events = %+v, want none at the larger standard limit", stdEvents)
	}
}

// TestStreamRun_AutoCompressFailureKeepsGeneration 测试压缩失败降级：摘要模型不可用时自动压缩
// 仅告警、以 foldedCount=0 的 done 收尾且不推进游标，本轮生成仍正常完成（错误不外传）。
func TestStreamRun_AutoCompressFailureKeepsGeneration(t *testing.T) {
	f := newAutoCompressFixtureWith(t, nil, slotChatResolver(map[ai.Slot]model.BaseChatModel{}))
	ctx := context.Background()
	session := f.seedSession(t, ai.SlotStandard, 800)
	events, done := runAutoCompress(t, f.agent, ctx, AgentInput{
		Query: "q3", SessionID: session.ID, NamespaceName: "default", UserID: session.UserID,
		DeclaredSlot: session.TargetModel,
	})

	if done == nil || done.Content != "ans" {
		t.Fatalf("done = %+v, want the turn to survive a compression failure", done)
	}
	if len(events) != 2 || events[0].Type != ai.AgentRunEventCompressStart ||
		events[1].Type != ai.AgentRunEventCompressDone || events[1].FoldedCount != 0 {
		t.Errorf("events = %+v, want start + done(foldedCount=0)", events)
	}
	got, err := f.store.GetSession(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if len(got.Summary.Value()) != 0 || got.SummaryUptoSeq != 0 {
		t.Errorf("persisted summary = (%q, %d), want empty (not advanced on failure)", got.Summary.Value(), got.SummaryUptoSeq)
	}
}
