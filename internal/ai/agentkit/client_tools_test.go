package agentkit

import (
	"context"
	"encoding/json"
	"errors"
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
	arguments := `{"capability":"traffic.list","params":{"limit":10},"complete":true}`
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

// TestApiInvokeRetriesIncompleteArguments 校验被截断的调用不会挂起或执行：工具把「参数不完整」回灌给
// 模型后，模型重新发起完整调用，只有完整调用进入审批；最终只产生一个待审批的 create。
func TestApiInvokeRetriesIncompleteArguments(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "ai.skill.create",
		Method:    "POST",
		Path:      "/api/v1/ai/skill/create",
		Access:    capability.AccessWrite,
		Invokable: true,
	}}
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return capabilities }}
	chat := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_bad",
			Function: schema.FunctionCall{Name: contract.ApiInvokeToolName, Arguments: `{"capability":"ai.skill.create","params":{"name":"monthly_report"}}`},
		}}},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_ok",
			Function: schema.FunctionCall{Name: contract.ApiInvokeToolName, Arguments: `{"capability":"ai.skill.create","params":{"name":"monthly_report","content":"完整正文"},"complete":true}`},
		}}},
		{Role: schema.Assistant, Content: "wire done"},
	}}
	store := newMemCheckpointStore()
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.Message]{
		Agent: newChatAgent(t, chat, toolImpl), EnableStreaming: true, CheckPointStore: store,
	})
	iter := runner.Run(context.Background(), []*schema.Message{schema.UserMessage("create skill")}, adk.WithCheckPointID("api-retry"))
	_, results, interruptID, info := drainChatEventsWithInterrupt(t, iter)
	if len(results) == 0 || !strings.Contains(results[0], "arguments are incomplete") {
		t.Fatalf("tool results = %v, want the incomplete-arguments hint", results)
	}
	if interruptID == "" {
		t.Fatal("expected the complete retry to interrupt for approval")
	}
	request, ok := info.(*contract.ApprovalRequest)
	if !ok || request.CallID != "call_ok" {
		t.Fatalf("interrupt info = %#v, want approval for call_ok", info)
	}
	if !strings.Contains(request.Arguments, "完整正文") {
		t.Fatalf("canonical arguments = %q, want the complete content", request.Arguments)
	}
}

// TestApiInvokeParamsPassThrough 校验 api_invoke 不对列表接口的过滤字段做任何按接口的补齐或改写：
// 过滤字段契约由各接口自身声明，工具层只负责 JSON 对象与字节上限校验。
func TestApiInvokeParamsPassThrough(t *testing.T) {
	raw := json.RawMessage(`{"filterBy":{` +
		`"timestampFrom":{"action":"gte","value":"2026-08-20T00:00:00Z"},` +
		`"timestampTo":{"action":"lte","value":"2026-08-20T23:59:59Z"},` +
		`"namespace":{"action":"eq","value":"default"}}}`)
	got, err := apiInvokeParams(raw)
	if err != nil {
		t.Fatalf("apiInvokeParams() error = %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("params = %s, want the original filterBy unchanged", got)
	}
}

// TestApiInvokeAllowsWriteCapability 校验 api_invoke 的可调用判定只看 Invokable：标记为可调用的
// 写入能力同样放行（由客户端工具挂起、用户审批后执行），未标记可调用的能力一律拒绝。
func TestApiInvokeAllowsWriteCapability(t *testing.T) {
	capabilities := []capability.Capability{
		{
			ID:        "ai.skill.create",
			Method:    "POST",
			Path:      "/api/v1/ai/skill/create",
			Access:    capability.AccessWrite,
			Invokable: true,
		},
		{
			ID:        "review.only",
			Method:    "POST",
			Path:      "/api/v1/review/only",
			Access:    capability.AccessRead,
			Invokable: false,
		},
	}
	canonical, err := prepareApiInvokeCall(`{"capability":"ai.skill.create","params":{"name":"monthly_report"},"complete":true}`, capabilities)
	if err != nil {
		t.Fatalf("prepareApiInvokeCall(write) error = %v", err)
	}
	if !strings.Contains(canonical, `"method":"POST"`) ||
		!strings.Contains(canonical, `"path":"/api/v1/ai/skill/create"`) {
		t.Fatalf("canonical arguments = %q", canonical)
	}

	if _, err := prepareApiInvokeCall(`{"capability":"review.only"}`, capabilities); err == nil ||
		!strings.Contains(err.Error(), "not an invokable capability") {
		t.Fatalf("prepareApiInvokeCall(non-invokable) error = %v", err)
	}
}

// TestApiInvokeRejectsIncompleteArguments 校验缺少 complete 自声明（或显式为 false）的调用不会进入
// 挂起与执行：流式输出被提前截断时参数通常已非法 JSON，能解析但缺标志同样必须拒绝并要求重发。
func TestApiInvokeRejectsIncompleteArguments(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "ai.skill.create",
		Method:    "POST",
		Path:      "/api/v1/ai/skill/create",
		Access:    capability.AccessWrite,
		Invokable: true,
	}}
	for _, arguments := range []string{
		`{"capability":"ai.skill.create","params":{"name":"monthly_report","content":"半截正文"}}`,
		`{"capability":"ai.skill.create","params":{"name":"monthly_report"},"complete":false}`,
	} {
		if _, err := prepareApiInvokeCall(arguments, capabilities); err == nil ||
			!strings.Contains(err.Error(), "arguments are incomplete") {
			t.Fatalf("prepareApiInvokeCall(%s) error = %v, want incomplete", arguments, err)
		}
	}
}

// TestApiInvokeAcceptsCompleteAnywhere 校验完整性只由 JSON 解析结果与 complete 标志决定：
// JSON 对象键序无语义，complete 位于参数对象任意位置都应视为完整，不再依赖「必须放在末尾」。
func TestApiInvokeAcceptsCompleteAnywhere(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "ai.skill.create",
		Method:    "POST",
		Path:      "/api/v1/ai/skill/create",
		Access:    capability.AccessWrite,
		Invokable: true,
	}}
	for _, arguments := range []string{
		`{"capability":"ai.skill.create","complete":true,"params":{"name":"monthly_report","content":"完整正文"}}`,
		`{"complete":true,"params":{"name":"monthly_report","content":"完整正文"},"capability":"ai.skill.create"}`,
	} {
		canonical, err := prepareApiInvokeCall(arguments, capabilities)
		if err != nil {
			t.Fatalf("prepareApiInvokeCall(%s) error = %v, want accepted", arguments, err)
		}
		if !strings.Contains(canonical, `"capability":"ai.skill.create"`) ||
			!strings.Contains(canonical, `"path":"/api/v1/ai/skill/create"`) {
			t.Fatalf("canonical arguments = %q, want capability and resolved path", canonical)
		}
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

// TestAskUserUnattendedReturnsNotice 校验无人值守时 ask_user 不校验参数、不挂起，直接返回固定降级提示。
func TestAskUserUnattendedReturnsNotice(t *testing.T) {
	text, err := (&askUserTool{}).InvokableRun(
		WithAutomation(context.Background(), &Automation{}), `{"questions":[]}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v, want nil (no interrupt)", err)
	}
	if text != AskUserUnattendedNotice {
		t.Fatalf("text = %q, want the unattended notice", text)
	}
}

// TestApiInvokeUnattendedInvokesCallback 校验无人值守时 api_invoke 按规范入参调用服务端回调并把结果
// 回灌给模型，不产生挂起。
func TestApiInvokeUnattendedInvokesCallback(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "traffic.list",
		Method:    "GET",
		Path:      "/api/v1/traffic",
		Access:    capability.AccessRead,
		Invokable: true,
	}}
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return capabilities }}
	var (
		gotMethod string
		gotPath   string
		gotParams json.RawMessage
	)
	automation := &Automation{InvokeAPI: func(_ context.Context, method, path string, params json.RawMessage) (string, error) {
		gotMethod, gotPath, gotParams = method, path, params
		return `{"items":[]}`, nil
	}}
	text, err := toolImpl.InvokableRun(
		WithAutomation(context.Background(), automation),
		`{"capability":"traffic.list","params":{"limit":10},"complete":true}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v, want nil (no interrupt)", err)
	}
	if text != `{"items":[]}` {
		t.Fatalf("text = %q, want the callback result", text)
	}
	if gotMethod != "GET" || gotPath != "/api/v1/traffic" {
		t.Fatalf("callback got method=%q path=%q", gotMethod, gotPath)
	}
	if !strings.Contains(string(gotParams), `"limit":10`) {
		t.Fatalf("callback params = %s, want the canonical params", gotParams)
	}
}

// TestApiInvokeUnattendedCallbackError 校验回调失败时只把错误文本回灌给模型，返回 nil error、不中断。
func TestApiInvokeUnattendedCallbackError(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "traffic.list",
		Method:    "GET",
		Path:      "/api/v1/traffic",
		Access:    capability.AccessRead,
		Invokable: true,
	}}
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return capabilities }}
	automation := &Automation{InvokeAPI: func(_ context.Context, _, _ string, _ json.RawMessage) (string, error) {
		return "", errors.New("boom")
	}}
	text, err := toolImpl.InvokableRun(
		WithAutomation(context.Background(), automation),
		`{"capability":"traffic.list","complete":true}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v, want nil (no interrupt)", err)
	}
	if text != ApiInvokeFailedPrefix+"boom" {
		t.Fatalf("text = %q, want the failure text", text)
	}
}

// TestApiInvokeUnattendedWithoutInvoker 校验未配置服务端调用器时回灌固定的「不可用」文本。
func TestApiInvokeUnattendedWithoutInvoker(t *testing.T) {
	capabilities := []capability.Capability{{
		ID:        "traffic.list",
		Method:    "GET",
		Path:      "/api/v1/traffic",
		Access:    capability.AccessRead,
		Invokable: true,
	}}
	toolImpl := &apiInvokeTool{capabilities: func() []capability.Capability { return capabilities }}
	text, err := toolImpl.InvokableRun(
		WithAutomation(context.Background(), &Automation{}),
		`{"capability":"traffic.list","complete":true}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v, want nil (no interrupt)", err)
	}
	if text != ApiInvokeUnavailableNotice {
		t.Fatalf("text = %q, want the unavailable notice", text)
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
