package ai

import (
	// 标准库
	"context"
	"io"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// scriptedTurn 是 stub 对话模型的一次流式/非流式响应：正文与（可选的）工具调用。
type scriptedTurn struct {
	content   string
	toolCalls []schema.ToolCall
}

// scriptedChat 是测试用对话模型替身：按调用次序返回预置的每一轮响应（超出后重复最后一轮），
// 并记录每次收到的消息列表与工具清单。
type scriptedChat struct {
	turns     []scriptedTurn
	calls     int
	gotInputs [][]*schema.Message
	gotTools  [][]*schema.ToolInfo
}

// Generate 记录输入与工具清单，并按次序返回预置的下一轮非流式响应。
func (c *scriptedChat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	c.gotInputs = append(c.gotInputs, input)
	c.gotTools = append(c.gotTools, model.GetCommonOptions(&model.Options{}, opts...).Tools)
	turn := c.pick()
	return &schema.Message{Role: schema.Assistant, Content: turn.content, ToolCalls: turn.toolCalls}, nil
}

// Stream 记录输入与工具清单，并按次序把预置的下一轮响应拆成流式分片返回。
func (c *scriptedChat) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.gotInputs = append(c.gotInputs, input)
	c.gotTools = append(c.gotTools, model.GetCommonOptions(&model.Options{}, opts...).Tools)
	turn := c.pick()
	sr, sw := schema.Pipe[*schema.Message](4)
	go func() {
		defer sw.Close()
		if turn.content != "" {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: turn.content}, nil)
		}
		if len(turn.toolCalls) > 0 {
			sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: turn.toolCalls}, nil)
		}
	}()
	return sr, nil
}

// pick 取本次调用对应的轮次并推进计数（超出预置轮数后重复最后一轮）。
func (c *scriptedChat) pick() scriptedTurn {
	idx := c.calls
	if idx >= len(c.turns) {
		idx = len(c.turns) - 1
	}
	c.calls++
	return c.turns[idx]
}

// recordingTool 是测试用工具替身：实现 tool.InvokableTool，返回配置的结果并记录收到的参数。
type recordingTool struct {
	name     string
	infoErr  error
	run      func(ctx context.Context, args string) (string, error)
	gotArgs  []string
	runCalls int
}

// Info 返回预设错误，否则返回该工具的名称与描述信息。
func (f *recordingTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return &schema.ToolInfo{Name: f.name, Desc: "a test tool"}, nil
}

// InvokableRun 记录调用参数并转调预设的 run 函数返回结果。
func (f *recordingTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	f.runCalls++
	f.gotArgs = append(f.gotArgs, argumentsInJSON)
	if f.run == nil {
		return "", nil
	}
	return f.run(ctx, argumentsInJSON)
}

// eventRecorder 记录上报的阶段事件；failAt 指定第几个事件（1-based）的上报返回错误，用于
// 模拟 SSE 客户端断开。
type eventRecorder struct {
	events []StageEvent
	failAt int
	seen   int
}

func (r *eventRecorder) report(ev StageEvent) error {
	r.seen++
	r.events = append(r.events, ev)
	if r.failAt != 0 && r.seen == r.failAt {
		return errors.New("client disconnected")
	}
	return nil
}

// findEvent 返回首个匹配阶段/状态/工具名的事件（未找到返回 nil）。
func (r *eventRecorder) findEvent(stage EvalStage, status EvalStageStatus, toolName string) *StageEvent {
	for i := range r.events {
		ev := &r.events[i]
		if ev.Stage == stage && ev.Status == status && ev.Tool == toolName {
			return ev
		}
	}
	return nil
}

// newAgentWithTools 构造带工具清单的测试 Agent（复用 testParams 与内存会话库）。
func newAgentWithTools(t *testing.T, chat model.BaseChatModel, rtv retriever.Retriever, built []tool.BaseTool, infos []*schema.ToolInfo, overrides map[string]string) *Agent {
	t.Helper()
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	agent, err := NewAgent(context.Background(), stubResolver(chat), rtv, store, testParams(t, overrides), nil, nil, built, infos)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return agent
}

// collectStream 抽干 AgentOutput 流，返回拼接后的增量文本、done 项与首个流错误（若有）。
func collectStream(t *testing.T, reader *schema.StreamReader[AgentOutput]) (string, AgentOutput, error) {
	t.Helper()
	defer reader.Close()
	var deltas []string
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return strings.Join(deltas, ""), AgentOutput{}, nil
			}
			return strings.Join(deltas, ""), AgentOutput{}, err
		}
		if item.Done {
			return strings.Join(deltas, ""), item, nil
		}
		deltas = append(deltas, item.Delta)
	}
}

// TestAgent_Tools_NoToolsUnchanged 测试无启用工具时节点 4 退化为单次流式生成：delta 与
// done 与引入工具前一致，不产生任何工具阶段事件，也不传工具清单。
func TestAgent_Tools_NoToolsUnchanged(t *testing.T) {
	chat := &scriptedChat{turns: []scriptedTurn{{content: "plain answer"}}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, nil, nil, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	deltas, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if deltas != "plain answer" || done.Content != "plain answer" {
		t.Errorf("no-tools output = (%q, %q), want (plain answer, plain answer)", deltas, done.Content)
	}
	if chat.calls != 1 {
		t.Errorf("chat calls = %d, want 1 (single generation)", chat.calls)
	}
	if len(chat.gotTools) != 1 || len(chat.gotTools[0]) != 0 {
		t.Errorf("chat tools = %+v, want no tool list in no-tool mode", chat.gotTools)
	}
	if len(rec.events) != 0 {
		t.Errorf("recorded events = %+v, want none in no-tool mode", rec.events)
	}
}

// TestAgent_Tools_ExecutesToolThenAnswers 测试含工具时首轮返回 ToolCalls：工具被执行、发生
// 二次生成，最终外发文本只含答案轮文本，上报 toolCall(start) 与 toolResult(done)。
func TestAgent_Tools_ExecutesToolThenAnswers(t *testing.T) {
	built := &recordingTool{name: "weather", run: func(ctx context.Context, args string) (string, error) {
		return "sunny", nil
	}}
	infos := []*schema.ToolInfo{{Name: "weather", Desc: "weather tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{
			content:   "let me check the weather",
			toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{"city":"sf"}`}}},
		},
		{content: "final answer"},
	}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	deltas, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if deltas != "final answer" || done.Content != "final answer" {
		t.Errorf("output = (%q, %q), want only the answer round text", deltas, done.Content)
	}
	if strings.Contains(deltas, "let me check") {
		t.Errorf("deltas = %q, want no intermediate-round text", deltas)
	}
	if chat.calls != 2 {
		t.Fatalf("chat calls = %d, want 2 (tool round + answer round)", chat.calls)
	}
	for i, tools := range chat.gotTools {
		if len(tools) != 1 || tools[0].Name != "weather" {
			t.Errorf("call %d tools = %+v, want the weather schema", i, tools)
		}
	}
	if built.runCalls != 1 || len(built.gotArgs) != 1 || built.gotArgs[0] != `{"city":"sf"}` {
		t.Errorf("tool run = (%d, %q), want one call with the model arguments", built.runCalls, built.gotArgs)
	}
	// 第二次生成的消息列表应包含 assistant(toolCalls) 与 tool(result)
	second := chat.gotInputs[1]
	if len(second) < 2 {
		t.Fatalf("second call input len = %d, want the assistant tool-call and tool-result messages appended", len(second))
	}
	assistant := second[len(second)-2]
	toolMsg := second[len(second)-1]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call-1" {
		t.Errorf("appended assistant message = %+v, want the tool call", assistant)
	}
	if toolMsg.Role != schema.Tool || toolMsg.ToolCallID != "call-1" || toolMsg.Content != "sunny" {
		t.Errorf("appended tool message = %+v, want the tool result fed back", toolMsg)
	}
	if ev := rec.findEvent(EvalStageToolCall, EvalStatusStart, "weather"); ev == nil {
		t.Errorf("events = %+v, want a toolCall start event for weather", rec.events)
	}
	if ev := rec.findEvent(EvalStageToolResult, EvalStatusDone, "weather"); ev == nil {
		t.Errorf("events = %+v, want a toolResult done event for weather", rec.events)
	}
}

// TestAgent_Tools_FailureFeedsErrorBack 测试工具执行失败：错误文本作为工具结果回填，本轮仍
// 正常 done、无流错误，上报 toolResult(failed)。
func TestAgent_Tools_FailureFeedsErrorBack(t *testing.T) {
	built := &recordingTool{name: "boom", run: func(ctx context.Context, args string) (string, error) {
		return "", errors.New("upstream down")
	}}
	infos := []*schema.ToolInfo{{Name: "boom", Desc: "failing tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{
			toolCalls: []schema.ToolCall{{ID: "call-9", Type: "function", Function: schema.FunctionCall{Name: "boom", Arguments: `{}`}}},
		},
		{content: "recovered answer"},
	}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	_, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v, want the turn to survive a tool failure", err)
	}
	if done.Content != "recovered answer" {
		t.Errorf("done content = %q, want the recovered answer", done.Content)
	}
	if ev := rec.findEvent(EvalStageToolResult, EvalStatusFailed, "boom"); ev == nil {
		t.Fatalf("events = %+v, want a toolResult failed event for boom", rec.events)
	} else if ev.Err != "upstream down" {
		t.Errorf("toolResult failed err = %q, want the tool error", ev.Err)
	}
	second := chat.gotInputs[1]
	toolMsg := second[len(second)-1]
	if toolMsg.Role != schema.Tool || !strings.HasPrefix(toolMsg.Content, "tool execution failed: ") || !strings.Contains(toolMsg.Content, "upstream down") {
		t.Errorf("fed-back tool message = %+v, want the failure text", toolMsg)
	}
}

// TestAgent_Tools_RoundBoundReached 测试达到轮数上限：agent.maxToolRounds=1 且模型始终返回
// ToolCalls 时只跑 1 轮、外发该轮文本、上报 toolCall(failed)、不执行工具、不返回错误。
func TestAgent_Tools_RoundBoundReached(t *testing.T) {
	built := &recordingTool{name: "loop"}
	infos := []*schema.ToolInfo{{Name: "loop", Desc: "looping tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{
			content:   "partial answer",
			toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "loop", Arguments: `{}`}}},
		},
	}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, map[string]string{"agent.maxToolRounds": "1"})

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	deltas, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v, want no error when the round bound is reached", err)
	}
	if deltas != "partial answer" || done.Content != "partial answer" {
		t.Errorf("output = (%q, %q), want the bounded round's text as the answer", deltas, done.Content)
	}
	if chat.calls != 1 {
		t.Errorf("chat calls = %d, want 1 (the round bound)", chat.calls)
	}
	if built.runCalls != 0 {
		t.Errorf("tool run calls = %d, want 0 (the pending call is not executed)", built.runCalls)
	}
	if ev := rec.findEvent(EvalStageToolCall, EvalStatusFailed, ""); ev == nil || ev.Err == "" {
		t.Errorf("events = %+v, want a toolCall failed event explaining the round bound", rec.events)
	}
}

// TestAgent_Tools_ReporterErrorAbortsLoop 测试上报失败（如 SSE 客户端断开）中止工具循环：
// 流收到错误、工具不被执行、不再发起后续模型调用。
func TestAgent_Tools_ReporterErrorAbortsLoop(t *testing.T) {
	built := &recordingTool{name: "weather"}
	infos := []*schema.ToolInfo{{Name: "weather", Desc: "weather tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{}`}}}},
		{content: "answer"},
	}}
	rec := &eventRecorder{failAt: 1} // 首个事件（toolCall start）上报即失败
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err == nil {
		t.Fatal("stream error = nil, want the report error to abort the loop")
	}
	if built.runCalls != 0 {
		t.Errorf("tool run calls = %d, want 0 (aborted before execution)", built.runCalls)
	}
	if chat.calls != 1 {
		t.Errorf("chat calls = %d, want 1 (no further model call)", chat.calls)
	}
}
