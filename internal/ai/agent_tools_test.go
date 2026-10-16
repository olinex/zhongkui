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

// scriptedTurn 是 stub 对话模型的一次流式/非流式响应：思考过程（可选）、正文与（可选的）工具调用。
type scriptedTurn struct {
	reason    string
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
	gotOpts   [][]model.Option
}

// Generate 记录输入与工具清单，并按次序返回预置的下一轮非流式响应。
func (c *scriptedChat) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	c.gotInputs = append(c.gotInputs, input)
	c.gotTools = append(c.gotTools, model.GetCommonOptions(&model.Options{}, opts...).Tools)
	c.gotOpts = append(c.gotOpts, opts)
	turn := c.pick()
	return &schema.Message{Role: schema.Assistant, Content: turn.content, ToolCalls: turn.toolCalls}, nil
}

// Stream 记录输入与工具清单，并按次序把预置的下一轮响应拆成流式分片返回。
func (c *scriptedChat) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.gotInputs = append(c.gotInputs, input)
	c.gotTools = append(c.gotTools, model.GetCommonOptions(&model.Options{}, opts...).Tools)
	c.gotOpts = append(c.gotOpts, opts)
	turn := c.pick()
	sr, sw := schema.Pipe[*schema.Message](4)
	go func() {
		defer sw.Close()
		if turn.reason != "" {
			sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: turn.reason}, nil)
		}
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

// findEventByCallID 返回首个匹配阶段/状态/调用标识的事件（未找到返回 nil）。
func (r *eventRecorder) findEventByCallID(stage EvalStage, status EvalStageStatus, callID string) *StageEvent {
	for i := range r.events {
		ev := &r.events[i]
		if ev.Stage == stage && ev.Status == status && ev.CallID == callID {
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

// thinkingScriptedChat 在 scriptedChat 之上实现 requestOptionsProvider，记录 generate 传入的思考
// 模式并返回单个选项，用于验证工具循环每轮 Stream 都带上思考选项。
type thinkingScriptedChat struct {
	scriptedChat
	thinkings []string
}

// RequestOptions 记录思考模式并返回单个选项。
func (c *thinkingScriptedChat) RequestOptions(thinking string) []model.Option {
	c.thinkings = append(c.thinkings, thinking)
	return []model.Option{model.WithTemperature(0.42)}
}

// TestAgent_Tools_ThinkingRequestOptionsEachRound 测试含工具时工具循环每轮 Stream 都携带
// requestOptionsProvider 返回的思考选项（与工具选项一并传入）。
func TestAgent_Tools_ThinkingRequestOptionsEachRound(t *testing.T) {
	built := &recordingTool{name: "weather", run: func(ctx context.Context, args string) (string, error) {
		return "sunny", nil
	}}
	infos := []*schema.ToolInfo{{Name: "weather", Desc: "weather tool"}}
	chat := &thinkingScriptedChat{scriptedChat: scriptedChat{turns: []scriptedTurn{
		{
			content:   "checking",
			toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{"city":"sf"}`}}},
		},
		{content: "final answer"},
	}}}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	if _, err := agent.Generate(context.Background(), AgentInput{Query: "q", Thinking: "off"}); err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if len(chat.thinkings) != 1 || chat.thinkings[0] != "off" {
		t.Errorf("RequestOptions thinkings = %v, want [off]", chat.thinkings)
	}
	if len(chat.gotOpts) != 2 {
		t.Fatalf("Stream calls = %d, want 2 (tool round + answer round)", len(chat.gotOpts))
	}
	for i, opts := range chat.gotOpts {
		// 每轮应同时携带工具选项与来自 RequestOptions 的思考选项。
		if len(opts) != 2 {
			t.Errorf("round %d opts = %d, want tools + thinking option", i, len(opts))
		}
	}
}

// TestAgent_Tools_ExecutesToolThenAnswers 测试含工具时首轮返回 ToolCalls：工具被执行、发生
// 二次生成，两轮正文都随流外发（工具轮的前导说明文本不隐藏），done 正文与实时增量一致，
// 上报 toolCall(start) 与 toolResult(done)。
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
	if deltas != "let me check the weatherfinal answer" || done.Content != deltas {
		t.Errorf("output = (%q, %q), want every round's text streamed and done matching deltas", deltas, done.Content)
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

// TestAgent_Tools_MultipleSameToolCallsCarryCallID 测试一次回答内模型对同一工具的多轮调用：每次
// 调用的 toolCall(start) 与 toolResult(done) 各自携带该次调用的 callID，供前端把同一次调用的
// 「发起」与「结果」归并到同一行，并按调用顺序自上而下成行展示。
func TestAgent_Tools_MultipleSameToolCallsCarryCallID(t *testing.T) {
	built := &recordingTool{name: "search", run: func(ctx context.Context, args string) (string, error) {
		return "hit:" + args, nil
	}}
	infos := []*schema.ToolInfo{{Name: "search", Desc: "search tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"a"}`}}}},
		{toolCalls: []schema.ToolCall{{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"b"}`}}}},
		{content: "final answer"},
	}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if built.runCalls != 2 {
		t.Fatalf("tool run calls = %d, want 2 (two rounds calling the same tool)", built.runCalls)
	}
	for _, id := range []string{"call-1", "call-2"} {
		if ev := rec.findEventByCallID(EvalStageToolCall, EvalStatusStart, id); ev == nil {
			t.Errorf("events = %+v, want a toolCall start event carrying callID %q", rec.events, id)
		}
		if ev := rec.findEventByCallID(EvalStageToolResult, EvalStatusDone, id); ev == nil {
			t.Errorf("events = %+v, want a toolResult done event carrying callID %q", rec.events, id)
		}
	}
}

// TestAgent_Tools_ParallelCallsShareRound 测试同轮内并发发起多次调用：这些调用的 toolCall(start) 与
// toolResult(done) 阶段事件携带同一轮次（Round 相同），供前端把「一次思考 → N 次并行调用」聚为一组
// 展示；跨轮则轮次递增。
func TestAgent_Tools_ParallelCallsShareRound(t *testing.T) {
	built := &recordingTool{name: "search", run: func(ctx context.Context, args string) (string, error) {
		return "hit:" + args, nil
	}}
	infos := []*schema.ToolInfo{{Name: "search", Desc: "search tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{
			{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"a"}`}},
			{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"b"}`}},
		}},
		{toolCalls: []schema.ToolCall{{ID: "call-3", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"c"}`}}}},
		{content: "final answer"},
	}}
	rec := &eventRecorder{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q", ToolReporter: rec.report})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	// 第一轮并发发起 call-1 / call-2（同为 Round 1），第二轮发起 call-3（Round 2）。
	wantRound := map[string]int{"call-1": 1, "call-2": 1, "call-3": 2}
	for id, want := range wantRound {
		start := rec.findEventByCallID(EvalStageToolCall, EvalStatusStart, id)
		if start == nil {
			t.Fatalf("events = %+v, want a toolCall start event for %s", rec.events, id)
		}
		if start.Round != want {
			t.Errorf("toolCall start %s round = %d, want %d", id, start.Round, want)
		}
		done := rec.findEventByCallID(EvalStageToolResult, EvalStatusDone, id)
		if done == nil {
			t.Fatalf("events = %+v, want a toolResult done event for %s", rec.events, id)
		}
		if done.Round != want {
			t.Errorf("toolResult done %s round = %d, want %d", id, done.Round, want)
		}
	}
}

// TestAgent_Tools_ReasoningNotFedBack 测试工具循环不把思考内容回灌给模型：本轮思考全程随流外发
// （done 的 Reasoning 汇总各轮思考），但追加进下一轮上下文的 assistant（含 ToolCalls）消息其
// ReasoningContent 已被清空，故模型侧上下文不含思考内容。
func TestAgent_Tools_ReasoningNotFedBack(t *testing.T) {
	built := &recordingTool{name: "weather", run: func(ctx context.Context, args string) (string, error) {
		return "sunny", nil
	}}
	infos := []*schema.ToolInfo{{Name: "weather", Desc: "weather tool"}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{
			reason:    "let me check the weather",
			content:   "checking",
			toolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{"city":"sf"}`}}},
		},
		{reason: "now I can answer", content: "final answer"},
	}}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	out, err := agent.Generate(context.Background(), AgentInput{Query: "q"})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	// 思考过程照常随流外发：done 的 Reasoning 汇总两轮思考。
	if want := "let me check the weathernow I can answer"; out.Reasoning != want {
		t.Errorf("done reasoning = %q, want %q streamed to the user", out.Reasoning, want)
	}
	if len(chat.gotInputs) != 2 {
		t.Fatalf("chat calls = %d, want 2 (tool round + answer round)", len(chat.gotInputs))
	}
	// 第二轮上下文里带 ToolCalls 的 assistant 消息不得携带思考内容。
	var toolCallMsg *schema.Message
	for _, m := range chat.gotInputs[1] {
		if len(m.ToolCalls) > 0 {
			toolCallMsg = m
			break
		}
	}
	if toolCallMsg == nil {
		t.Fatalf("second call input = %+v, want the assistant tool-call message", chat.gotInputs[1])
	}
	if toolCallMsg.ReasoningContent != "" {
		t.Errorf("fed-back assistant reasoning = %q, want empty (thinking must not be fed back)", toolCallMsg.ReasoningContent)
	}
	// 送入模型的每一条消息都不应携带思考内容。
	for i, input := range chat.gotInputs {
		for _, m := range input {
			if m.ReasoningContent != "" {
				t.Errorf("call %d message = %+v, want no reasoning in the model context", i, m)
			}
		}
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

// chunkedPreambleChat 是「前导文本被切成一串分块后才给出工具调用」的模型替身：首轮逐块下发
// 前导文本再下发 ToolCalls，次轮逐块下发答案，用于确认工具轮的前导文本被逐块外发且与 done 一致。
type chunkedPreambleChat struct{ calls int }

// Generate 实现 model.BaseChatModel，供需要非流式响应的路径使用。
func (c *chunkedPreambleChat) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "g"}, nil
}

// Stream 按轮次逐块下发：首轮为多块前导文本 + 工具调用，其余轮为多块答案。
func (c *chunkedPreambleChat) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.calls++
	round := c.calls
	sr, sw := schema.Pipe[*schema.Message](4)
	go func() {
		defer sw.Close()
		send := func(content string) {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: content}, nil)
		}
		if round == 1 {
			send("let ")
			send("me ")
			send("check ")
			sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
				ID:       "call-1",
				Type:     "function",
				Function: schema.FunctionCall{Name: "weather", Arguments: `{}`},
			}}}, nil)
			return
		}
		send("final ")
		send("answer")
	}()
	return sr, nil
}

// TestAgent_Tools_ChunkedPreambleStreamsToo 测试前导文本被切成一串分块、工具调用在其后才出现
// 时：前导文本逐块随流外发（不再被抑制），答案轮同样逐块外发，且 done 正文与实时增量完全一致
// ——前端在 done 时以正文覆盖增量，两者一致才不会出现文本跳变或丢失。
func TestAgent_Tools_ChunkedPreambleStreamsToo(t *testing.T) {
	built := &recordingTool{name: "weather", run: func(ctx context.Context, args string) (string, error) {
		return "sunny", nil
	}}
	infos := []*schema.ToolInfo{{Name: "weather", Desc: "weather tool"}}
	chat := &chunkedPreambleChat{}
	agent := newAgentWithTools(t, chat, &stubRetriever{}, []tool.BaseTool{built}, infos, nil)

	reader, err := agent.Stream(context.Background(), AgentInput{Query: "q"})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	deltas, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v", err)
	}
	want := "let me check final answer"
	if deltas != want {
		t.Errorf("deltas = %q, want every round's text forwarded chunk by chunk", deltas)
	}
	if done.Content != deltas {
		t.Errorf("done content = %q, want it to match the streamed deltas %q", done.Content, deltas)
	}
	if chat.calls != 2 {
		t.Errorf("chat calls = %d, want 2 (tool round + answer round)", chat.calls)
	}
}
