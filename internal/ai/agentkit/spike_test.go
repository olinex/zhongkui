package agentkit

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// fakeChat 重放固定的 *schema.Message 响应脚本。它有意保持最小化：
// 忽略模型选项（包括 WithTools），以便在没有真实服务商的情况下走通
// ADK agent 的 ToolsNode。
type fakeChat struct {
	mu     sync.Mutex
	script []*schema.Message
	idx    int
}

func (f *fakeChat) next() *schema.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.script) {
		return &schema.Message{Role: schema.Assistant, Content: "script exhausted"}
	}
	msg := f.script[f.idx]
	f.idx++
	return msg
}

// Generate 是 fakeChat 的测试桩，按脚本返回下一条消息。
func (f *fakeChat) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.next(), nil
}

// Stream 是 fakeChat 的测试桩，把下一条脚本消息作为单元素流返回。
func (f *fakeChat) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg := f.next()
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

// fakeAgentic 重放固定的 *schema.AgenticMessage 响应脚本。
type fakeAgentic struct {
	mu     sync.Mutex
	script []*schema.AgenticMessage
	idx    int
}

func (f *fakeAgentic) next() *schema.AgenticMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.script) {
		return &schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: "script exhausted"})},
		}
	}
	msg := f.script[f.idx]
	f.idx++
	return msg
}

// Generate 是 fakeAgentic 的测试桩，按脚本返回下一条消息。
func (f *fakeAgentic) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	return f.next(), nil
}

// Stream 是 fakeAgentic 的测试桩，把下一条脚本消息作为单元素流返回。
func (f *fakeAgentic) Stream(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg := f.next()
	return schema.StreamReaderFromArray([]*schema.AgenticMessage{msg}), nil
}

// memCheckpointStore 是一个最小的 adk.CheckPointStore。
type memCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemCheckpointStore() *memCheckpointStore {
	return &memCheckpointStore{data: map[string][]byte{}}
}

// Get 读取指定检查点的数据副本。
func (s *memCheckpointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[id]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), v...), true, nil
}

// Set 保存指定检查点的数据副本。
func (s *memCheckpointStore) Set(_ context.Context, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = append([]byte(nil), data...)
	return nil
}

// Delete 删除指定检查点。
func (s *memCheckpointStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
	return nil
}

type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addTool struct{}

// Info 返回 add 工具的 schema。
func (addTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "add",
		Desc: "add two integers",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"a": {Type: schema.Integer, Desc: "first", Required: true},
			"b": {Type: schema.Integer, Desc: "second", Required: true},
		}),
	}, nil
}

// InvokableRun 解析参数并返回两数之和。
func (addTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var in addArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]int{"sum": in.A + in.B})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type approvalTool struct{}

// Info 返回 approval 工具的 schema。
func (approvalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "approval", Desc: "requires approval"}, nil
}

// InvokableRun 首次调用触发中断，恢复后返回审批数据。
func (approvalTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if !wasInterrupted {
		return "", tool.StatefulInterrupt(ctx, "approval needed", argumentsInJSON)
	}
	isTarget, hasData, data := tool.GetResumeContext[string](ctx)
	if isTarget && hasData {
		return data, nil
	}
	return "", tool.StatefulInterrupt(ctx, "approval needed", argumentsInJSON)
}

func drainChatEvents(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]) (string, []string, string) {
	t.Helper()
	var text strings.Builder
	var toolResults []string
	var interruptID string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("agent event error: %v", ev.Err)
		}
		if ev.Action != nil && ev.Action.Interrupted != nil {
			for _, ic := range ev.Action.Interrupted.InterruptContexts {
				if ic.IsRootCause {
					interruptID = ic.ID
					break
				}
			}
			continue
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput
		if mv.Role == schema.Tool {
			msg, err := mv.GetMessage()
			if err != nil {
				t.Fatalf("read tool result: %v", err)
			}
			toolResults = append(toolResults, msg.Content)
			continue
		}
		if mv.IsStreaming && mv.MessageStream != nil {
			for {
				chunk, err := mv.MessageStream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("read stream: %v", err)
				}
				text.WriteString(chunk.Content)
			}
			continue
		}
		if mv.Message != nil {
			text.WriteString(mv.Message.Content)
		}
	}
	return text.String(), toolResults, interruptID
}

func drainAgenticEvents(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) (string, []string, string) {
	t.Helper()
	var text strings.Builder
	var toolResults []string
	var interruptID string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("agent event error: %v", ev.Err)
		}
		if ev.Action != nil && ev.Action.Interrupted != nil {
			for _, ic := range ev.Action.Interrupted.InterruptContexts {
				if ic.IsRootCause {
					interruptID = ic.ID
					break
				}
			}
			continue
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput
		if mv.IsStreaming && mv.MessageStream != nil {
			for {
				chunk, err := mv.MessageStream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("read stream: %v", err)
				}
				collectAgentic(t, &text, &toolResults, chunk)
			}
			continue
		}
		if mv.Message != nil {
			collectAgentic(t, &text, &toolResults, mv.Message)
		}
	}
	return text.String(), toolResults, interruptID
}

func collectAgentic(t *testing.T, text *strings.Builder, toolResults *[]string, msg *schema.AgenticMessage) {
	t.Helper()
	if msg == nil {
		return
	}
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.AssistantGenText != nil:
			text.WriteString(block.AssistantGenText.Text)
		case block.FunctionToolResult != nil:
			for _, part := range block.FunctionToolResult.Content {
				if part != nil && part.Text != nil {
					*toolResults = append(*toolResults, part.Text.Text)
				}
			}
		}
	}
}

func newChatAgent(t *testing.T, m model.BaseModel[*schema.Message], tools ...tool.BaseTool) *adk.TypedChatModelAgent[*schema.Message] {
	t.Helper()
	agent, err := adk.NewTypedChatModelAgent(context.Background(), &adk.TypedChatModelAgentConfig[*schema.Message]{
		Name:  "spike-chat",
		Model: m,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
		MaxIterations: 5,
	})
	if err != nil {
		t.Fatalf("build chat agent: %v", err)
	}
	return agent
}

func newAgenticAgent(t *testing.T, m model.BaseModel[*schema.AgenticMessage], tools ...tool.BaseTool) *adk.TypedChatModelAgent[*schema.AgenticMessage] {
	t.Helper()
	agent, err := adk.NewTypedChatModelAgent(context.Background(), &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:  "spike-agentic",
		Model: m,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
		MaxIterations: 5,
	})
	if err != nil {
		t.Fatalf("build agentic agent: %v", err)
	}
	return agent
}

func TestSpikeChatReAct(t *testing.T) {
	m := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.FunctionCall{Name: "add", Arguments: `{"a":1,"b":2}`}}}},
		{Role: schema.Assistant, Content: "the sum is 3"},
	}}
	agent := newChatAgent(t, m, addTool{})
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{Agent: agent, EnableStreaming: true})

	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("1+2?")})
	text, results, _ := drainChatEvents(t, iter)
	if text != "the sum is 3" {
		t.Fatalf("final text = %q", text)
	}
	if len(results) != 1 || !strings.Contains(results[0], `"sum":3`) {
		t.Fatalf("tool results = %#v", results)
	}
}

func TestSpikeChatInterruptResume(t *testing.T) {
	m := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.FunctionCall{Name: "approval", Arguments: `{}`}}}},
		{Role: schema.Assistant, Content: "done after approval"},
	}}
	store := newMemCheckpointStore()
	agent := newChatAgent(t, m, approvalTool{})
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{Agent: agent, EnableStreaming: true, CheckPointStore: store})

	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("run approval")}, adk.WithCheckPointID("spike-cp"))
	_, _, interruptID := drainChatEvents(t, iter)
	if interruptID == "" {
		t.Fatal("expected an interrupt id")
	}

	agent2 := newChatAgent(t, m, approvalTool{})
	runner2 := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{Agent: agent2, EnableStreaming: true, CheckPointStore: store})
	resumeIter, err := runner2.ResumeWithParams(context.Background(), "spike-cp", &adk.ResumeParams{
		Targets: map[string]any{interruptID: "approved result"},
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	text, results, _ := drainChatEvents(t, resumeIter)
	if text != "done after approval" {
		t.Fatalf("final text = %q", text)
	}
	if len(results) != 1 || results[0] != "approved result" {
		t.Fatalf("tool results = %#v", results)
	}
}

func TestSpikeAgenticReAct(t *testing.T) {
	m := &fakeAgentic{script: []*schema.AgenticMessage{
		{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolCall{CallID: "call_1", Name: "add", Arguments: `{"a":2,"b":3}`}),
			},
		},
		{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.AssistantGenText{Text: "the sum is 5"}),
			},
		},
	}}
	agent := newAgenticAgent(t, m, addTool{})
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: true})

	iter := runner.Run(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("2+3?")})
	text, results, _ := drainAgenticEvents(t, iter)
	if text != "the sum is 5" {
		t.Fatalf("final text = %q", text)
	}
	if len(results) != 1 || !strings.Contains(results[0], `"sum":5`) {
		t.Fatalf("tool results = %#v", results)
	}
}

type loopItem struct {
	Query string
}

func TestSpikeTurnLoopBasic(t *testing.T) {
	m := &fakeChat{script: []*schema.Message{{Role: schema.Assistant, Content: "loop answer"}}}
	agent := newChatAgent(t, m)

	done := make(chan string, 1)
	loop := adk.NewTurnLoop(adk.TurnLoopConfig[*loopItem, *schema.Message]{
		GenInput: func(_ context.Context, _ *adk.TurnLoop[*loopItem, *schema.Message], items []*loopItem) (*adk.GenInputResult[*loopItem, *schema.Message], error) {
			return &adk.GenInputResult[*loopItem, *schema.Message]{
				Input: &adk.TypedAgentInput[*schema.Message]{
					Messages:        []*schema.Message{schema.UserMessage(items[0].Query)},
					EnableStreaming: true,
				},
				Consumed: items,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *adk.TurnLoop[*loopItem, *schema.Message], _ []*loopItem) (adk.TypedAgent[*schema.Message], error) {
			return agent, nil
		},
		OnAgentEvents: func(_ context.Context, _ *adk.TurnContext[*loopItem, *schema.Message], events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]) error {
			text, _, _ := drainChatEvents(t, events)
			done <- text
			return nil
		},
		Store:        newMemCheckpointStore(),
		CheckpointID: "spike-loop",
	})
	loop.Push(&loopItem{Query: "hello"})
	loop.Run(context.Background())

	select {
	case text := <-done:
		if text != "loop answer" {
			t.Fatalf("loop text = %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for loop answer")
	}
	loop.Stop(adk.WithStopCause("done"))
	_ = loop.Wait()
}
