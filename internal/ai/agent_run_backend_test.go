package ai

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// testBackendToolName 是本文件用例使用的后端（builtin / MCP 风格）工具名：它注册进 toolIndex，
// 由后端执行，用于验证「backend 工具在 must 级时挂起等审批、批准后由后端自行执行」。
const testBackendToolName = "test_backend_tool"

// backendCall 构造一次 test_backend_tool 的工具调用（callID + 入参 JSON 文本）。
func backendCall(id, arguments string) schema.ToolCall {
	return schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{
		Name: testBackendToolName, Arguments: arguments,
	}}
}

// newBackendRunFixture 构造运行现场用例的装配：内存测试库（含 agent_runs 表）、含给定 backend 工具
// 的 Agent、一个已建会话，以及记录 tool_request 的通道。评估用 flash 档位走独立替身，使用例可对
// 待评估调用注入审核重要级（must / auto 等）。
func newBackendRunFixture(t *testing.T, chat model.BaseChatModel, flash model.BaseChatModel, built []tool.BaseTool, infos []*schema.ToolInfo) *agentRunFixture {
	t.Helper()
	ctx := context.Background()
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	resolver := slotChatResolver(map[ai.Slot]model.BaseChatModel{
		ai.SlotFlash:    flash,
		ai.SlotStandard: chat,
	})
	agent, err := NewAgent(ctx, resolver, &stubRetriever{}, store,
		testParams(t, map[string]string{"agent.maxToolRounds": "5"}), nil, nil, built, infos)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	return &agentRunFixture{agent: agent, store: store, session: session}
}

// TestAgentRun_BackendMustSuspendsThenExecutes 测试 backend 工具 must 级挂起 → 批准 → 后端执行 →
// 续跑：评估为 must 的 backend 工具不再行内执行，而是产生 execOwner=backend 的 tool_request 并挂起；
// 批准恢复时由后端自行执行该工具，结果回灌为 tool message 并续跑产出 done。
func TestAgentRun_BackendMustSuspendsThenExecutes(t *testing.T) {
	ctx := context.Background()
	backend := &recordingTool{name: testBackendToolName, run: func(_ context.Context, _ string) (string, error) {
		return "backend-result", nil
	}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{backendCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	flash := &stubChat{reply: `{"levels":[{"callID":"call-1","level":"must"}]}`}
	f := newBackendRunFixture(t, chat, flash,
		[]tool.BaseTool{backend},
		[]*schema.ToolInfo{{Name: testBackendToolName, Desc: "a backend tool that requires approval"}})

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, done, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	if backend.runCalls != 0 {
		t.Fatalf("backend tool ran %d times before approval, want 0 (must level suspends)", backend.runCalls)
	}
	if len(f.requests) != 1 {
		t.Fatalf("tool requests = %+v, want exactly one", f.requests)
	}
	req := f.requests[0]
	if req.ExecOwner != ToolExecOwnerBackend {
		t.Errorf("tool request execOwner = %q, want backend", req.ExecOwner)
	}
	if !req.ApprovalRequired || req.AuditLevel != AuditLevelMust {
		t.Errorf("tool request = %+v, want must level with approval required", req)
	}

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	_, resumed, err := collectStream(t, resumeReader)
	if err != nil {
		t.Fatalf("resume stream error = %v", err)
	}
	if !resumed.Done || resumed.Content != "final answer" {
		t.Fatalf("resumed output = %+v, want the final answer done item", resumed)
	}
	if backend.runCalls != 1 {
		t.Fatalf("backend tool ran %d times after approval, want exactly 1", backend.runCalls)
	}
	if len(chat.gotInputs) != 2 {
		t.Fatalf("chat calls = %d, want 2 (suspend round + resume round)", len(chat.gotInputs))
	}
	last := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if last.Role != schema.Tool || last.ToolCallID != "call-1" || last.Content != "backend-result" {
		t.Errorf("fed-back tool message = %+v, want the backend execution result", last)
	}
}

// TestAgentRun_BackendNonMustExecutesInline 测试 backend 工具非 must（auto / none）不挂起：评估为
// auto 的 backend 工具照旧行内执行，不产生任何 tool_request，结果回灌后续跑完成。
func TestAgentRun_BackendNonMustExecutesInline(t *testing.T) {
	ctx := context.Background()
	backend := &recordingTool{name: testBackendToolName, run: func(_ context.Context, _ string) (string, error) {
		return "inline-result", nil
	}}
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{backendCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	flash := &stubChat{reply: `{"levels":[{"callID":"call-1","level":"auto"}]}`}
	f := newBackendRunFixture(t, chat, flash,
		[]tool.BaseTool{backend},
		[]*schema.ToolInfo{{Name: testBackendToolName, Desc: "a backend tool that runs inline"}})

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	_, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if !done.Done || done.Content != "final answer" {
		t.Fatalf("output = %+v, want the final answer done item", done)
	}
	if len(f.requests) != 0 {
		t.Fatalf("tool requests = %+v, want none for a non-must backend tool", f.requests)
	}
	if backend.runCalls != 1 {
		t.Fatalf("backend tool ran %d times, want exactly 1 (inline execution)", backend.runCalls)
	}
	last := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if last.Role != schema.Tool || last.Content != "inline-result" {
		t.Errorf("fed-back tool message = %+v, want the inline execution result", last)
	}
}
