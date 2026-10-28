package agentkit

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
)

func TestAskUserInterruptAndResume(t *testing.T) {
	arguments := `{"questions":[{"id":"q1","question":"Which plan?","options":[{"text":"A"},{"text":"B"}]}]}`
	chat := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_q",
			Function: schema.FunctionCall{Name: contract.AskUserToolName, Arguments: arguments},
		}}},
		{Role: schema.Assistant, Content: "answered"},
	}}
	store := newMemCheckpointStore()
	agent := newChatAgent(t, chat, &askUserTool{})
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: agent, EnableStreaming: true, CheckPointStore: store,
	})

	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("ask me")}, adk.WithCheckPointID("ask-user"))
	_, _, interruptID, info := drainChatEventsWithInterrupt(t, iter)
	if interruptID == "" {
		t.Fatal("expected an interrupt id")
	}
	request, ok := info.(*contract.QuestionnaireRequest)
	if !ok {
		t.Fatalf("interrupt info = %T, want *contract.QuestionnaireRequest", info)
	}
	if request.CallID != "call_q" || len(request.Questions) != 1 || request.Questions[0].ID != "q1" {
		t.Fatalf("questionnaire request = %+v", request)
	}

	resumeAgent := newChatAgent(t, chat, &askUserTool{})
	resumeRunner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: resumeAgent, EnableStreaming: true, CheckPointStore: store,
	})
	answer := `{"answers":[{"questionID":"q1","answered":true,"answer":"A"}]}`
	resumeIter, err := resumeRunner.ResumeWithParams(context.Background(), "ask-user", &adk.ResumeParams{
		Targets: map[string]any{interruptID: &contract.ToolResumeResult{CallID: "call_q", Approved: true, Result: answer}},
	})
	if err != nil {
		t.Fatalf("resume error = %v", err)
	}
	text, results, _ := drainChatEvents(t, resumeIter)
	if text != "answered" {
		t.Fatalf("final text = %q", text)
	}
	if len(results) != 1 || results[0] != answer {
		t.Fatalf("tool results = %v", results)
	}
}

func TestAskUserDeclinedResume(t *testing.T) {
	arguments := `{"questions":[{"id":"q1","question":"Which plan?","options":[{"text":"A"}]}]}`
	chat := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_q",
			Function: schema.FunctionCall{Name: contract.AskUserToolName, Arguments: arguments},
		}}},
		{Role: schema.Assistant, Content: "done"},
	}}
	store := newMemCheckpointStore()
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: newChatAgent(t, chat, &askUserTool{}), EnableStreaming: true, CheckPointStore: store,
	})
	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("ask")}, adk.WithCheckPointID("ask-user-declined"))
	_, _, interruptID, _ := drainChatEventsWithInterrupt(t, iter)

	resumeRunner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: newChatAgent(t, chat, &askUserTool{}), EnableStreaming: true, CheckPointStore: store,
	})
	resumeIter, err := resumeRunner.ResumeWithParams(context.Background(), "ask-user-declined", &adk.ResumeParams{
		Targets: map[string]any{interruptID: &contract.ToolResumeResult{CallID: "call_q", Approved: false}},
	})
	if err != nil {
		t.Fatalf("resume error = %v", err)
	}
	_, results, _ := drainChatEvents(t, resumeIter)
	if len(results) != 1 || results[0] != declinedToolCallText {
		t.Fatalf("tool results = %v, want declined text", results)
	}
}

func TestApiInvokeInterruptAndResume(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "traffic.list",
		Method:    "GET",
		Path:      "/api/v1/traffic",
		Access:    capability.AccessRead,
		Invokable: true,
	}}
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return capabilities }}
	arguments := `{"capability":"traffic.list","params":{"limit":10}}`
	chat := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_api",
			Function: schema.FunctionCall{Name: contract.ApiInvokeToolName, Arguments: arguments},
		}}},
		{Role: schema.Assistant, Content: "wire done"},
	}}
	store := newMemCheckpointStore()
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: newChatAgent(t, chat, toolImpl), EnableStreaming: true, CheckPointStore: store,
	})
	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("list traffic")}, adk.WithCheckPointID("api-invoke"))
	_, _, interruptID, info := drainChatEventsWithInterrupt(t, iter)
	if interruptID == "" {
		t.Fatal("expected an interrupt id")
	}
	request, ok := info.(*contract.ApprovalRequest)
	if !ok {
		t.Fatalf("interrupt info = %T, want *contract.ApprovalRequest", info)
	}
	if request.CallID != "call_api" || request.ToolName != contract.ApiInvokeToolName ||
		request.ExecOwner != contract.ToolExecOwnerClient {
		t.Fatalf("approval request = %+v", request)
	}
	if !strings.Contains(request.Arguments, `"method":"GET"`) ||
		!strings.Contains(request.Arguments, `"path":"/api/v1/traffic"`) {
		t.Fatalf("canonical arguments = %q", request.Arguments)
	}

	resumeRunner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: newChatAgent(t, chat, toolImpl), EnableStreaming: true, CheckPointStore: store,
	})
	resumeIter, err := resumeRunner.ResumeWithParams(context.Background(), "api-invoke", &adk.ResumeParams{
		Targets: map[string]any{interruptID: &contract.ToolResumeResult{CallID: "call_api", Approved: true, Result: `{"items":[]}`}},
	})
	if err != nil {
		t.Fatalf("resume error = %v", err)
	}
	text, results, _ := drainChatEvents(t, resumeIter)
	if text != "wire done" {
		t.Fatalf("final text = %q", text)
	}
	if len(results) != 1 || results[0] != `{"items":[]}` {
		t.Fatalf("tool results = %v", results)
	}
}

// TestApiInvokeParamsPassThrough 校验 api_invoke 不对列表接口的过滤字段做任何按接口的补齐或改写：
// 过滤字段契约由各接口自身声明，工具层只负责 JSON 对象与字节上限校验。
func TestApiInvokeParamsPassThrough(t *testing.T) {
	raw := json.RawMessage(`{"filterBy":{` +
		`"timestampFrom":{"action":"gte","value":"2026-08-20T00:00:00Z"},` +
		`"timestampTo":{"action":"lte","value":"2026-08-20T23:59:59Z"},` +
		`"namespace":{"action":"eq","value":"default"}}}`)
	got, errText := apiInvokeParams(raw)
	if errText != "" {
		t.Fatalf("apiInvokeParams() error = %q", errText)
	}
	if string(got) != string(raw) {
		t.Fatalf("params = %s, want the original filterBy unchanged", got)
	}
}

func TestApiInvokeRejectsUnknownCapability(t *testing.T) {
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return nil }}
	text, err := toolImpl.InvokableRun(context.Background(), `{"capability":"nope"}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	if !strings.Contains(text, "is not registered") {
		t.Fatalf("error text = %q", text)
	}
}

// drainChatEventsWithInterrupt 解析 chat 事件流，额外返回根因中断的 Info 载荷。
func drainChatEventsWithInterrupt(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]) (string, []string, string, any) {
	t.Helper()
	var text strings.Builder
	var toolResults []string
	var interruptID string
	var info any
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("agent event error: %v", event.Err)
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			for _, context := range event.Action.Interrupted.InterruptContexts {
				if context.IsRootCause {
					interruptID = context.ID
					info = context.Info
					break
				}
			}
			continue
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		if variant.Role == schema.Tool {
			message, err := variant.GetMessage()
			if err != nil {
				t.Fatalf("read tool result: %v", err)
			}
			toolResults = append(toolResults, message.Content)
			continue
		}
		if variant.IsStreaming && variant.MessageStream != nil {
			for {
				chunk, err := variant.MessageStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("read stream: %v", err)
				}
				text.WriteString(chunk.Content)
			}
			continue
		}
		if variant.Message != nil {
			text.WriteString(variant.Message.Content)
		}
	}
	return text.String(), toolResults, interruptID, info
}
