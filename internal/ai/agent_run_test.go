package ai

import (
	// 标准库

	"context"
	"strings"
	"testing"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// testClientToolName 是本包运行现场用例使用的客户端工具名：生产 client 注册表登记了问卷类工具
// ask_user（其入参契约固定为问卷载荷，不便于承载任意自由入参的通用挂起/回传用例），故用例另行登记
// 一个占位 client 工具，使「命中 client 工具即挂起」的分支可被通用地触发。
const testClientToolName = "test_client_tool"

// ensureTestClientTool 幂等登记测试用 client 工具 schema 并返回之（同名只登记一次，避免注册表
// 的重复登记守卫 panic）。入参为自由 JSON 对象，后端不解释其内容，只负责原样挂起与回传。
func ensureTestClientTool() *schema.ToolInfo {
	if info, ok := clientToolInfo(testClientToolName); ok {
		return info
	}
	info := &schema.ToolInfo{
		Name: testClientToolName,
		Desc: "a client-side tool executed by the frontend under the user's own credentials",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"payload": {
				Type:     schema.String,
				Desc:     "free-form arguments interpreted by the client",
				Required: true,
			},
		}),
	}
	registerClientToolInfo(testClientToolName, info)
	return info
}

// agentRunFixture 是运行现场相关用例的共享装配：含 agent_runs 表的测试库、仅含 client 工具
// test_client_tool（可另含 knowledge_search）的 Agent、一个已建会话，以及记录 tool_request 的通道。
type agentRunFixture struct {
	agent    *Agent           // 被测 Agent
	store    *SessionStore    // 会话存储（回读运行现场与消息用）
	session  *ai.AgentSession // 预置会话
	requests []ToolRequest    // 已上报的 tool_request
}

// reportRequest 记录一条 tool_request（挂起时逐条调用）。
func (f *agentRunFixture) reportRequest(req ToolRequest) error {
	f.requests = append(f.requests, req)
	return nil
}

// input 构造一次携带审批通道的会话运行输入。
func (f *agentRunFixture) input(query string) AgentInput {
	return AgentInput{
		Query:               query,
		SessionID:           f.session.ID,
		NamespaceName:       "default",
		UserID:              f.session.UserID,
		ToolRequestReporter: f.reportRequest,
	}
}

// resumeInput 构造一次携带审批通道的恢复输入。
func (f *agentRunFixture) resumeInput(results ...ToolResult) ResumeInput {
	return ResumeInput{
		SessionID:     f.session.ID,
		NamespaceName: "default",
		UserID:        f.session.UserID,
		Results:       results,
		ToolRequests:  f.reportRequest,
	}
}

// latestRun 回读该会话最近一行运行现场。
func (f *agentRunFixture) latestRun(t *testing.T) ai.AgentRun {
	t.Helper()
	var run ai.AgentRun
	if err := f.store.db.Where("session_id = ?", f.session.ID).Order("id DESC").First(&run).Error; err != nil {
		t.Fatalf("loading the latest agent run error = %v", err)
	}
	return run
}

// newAgentRunFixture 构造上述装配；withKnowledge 为真时额外并入 knowledge_search 工具（server
// 执行，用于在挂起前产生检索引用）。Agent 通过 client schema 注册表并入 test_client_tool 的
// schema，但不在 toolIndex 中构建其执行器，从而走挂起分支。
func newAgentRunFixture(t *testing.T, chat model.BaseChatModel, rtv retriever.Retriever, withKnowledge bool) *agentRunFixture {
	t.Helper()
	ctx := context.Background()
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	var built []tool.BaseTool
	infos := make([]*schema.ToolInfo, 0, 2)
	if withKnowledge {
		b, err := tools.Build(ctx, "knowledge_search", tools.Deps{})
		if err != nil {
			t.Fatalf("tools.Build(knowledge_search) error = %v", err)
		}
		info, err := b.Info(ctx)
		if err != nil {
			t.Fatalf("knowledge_search Info error = %v", err)
		}
		built = append(built, b)
		infos = append(infos, info)
	}
	infos = append(infos, ensureTestClientTool())
	// 挂起前会另行调用 flash 档位对本次全部待执行调用做一次审核重要级评估（见 evaluateAuditLevels）；
	// 该评估走独立替身，避免消费本次运行档位脚本化的轮次序列（其轮次只应对应各次生成）。评估替身
	// 对 fixture 里的 backend 工具 knowledge_search（call-ks）显式判为非 must，使其照旧行内执行；
	// client 工具不在输出中即保守回落 must，但会话阀门 each 已使其一律需人工审核，不影响既有断言。
	resolver := slotChatResolver(map[ai.Slot]model.BaseChatModel{
		ai.SlotFlash:    &stubChat{reply: `{"levels": [{"callID": "call-ks", "level": "none"}]}`},
		ai.SlotStandard: chat,
	})
	agent, err := NewAgent(ctx, resolver, rtv, store,
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

// clientCall 构造一次 test_client_tool 的工具调用（callID + 入参 JSON 文本）。
func clientCall(id, arguments string) schema.ToolCall {
	return schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{
		Name: testClientToolName, Arguments: arguments,
	}}
}

// askUserCall 构造一次问卷类 client 工具 ask_user 的调用（callID + 问卷入参 JSON 文本）。
func askUserCall(id, arguments string) schema.ToolCall {
	return schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{
		Name: askUserToolName, Arguments: arguments,
	}}
}

// TestAgentRun_SuspendThenApproveResume 测试挂起 → 批准续跑：命中 client 工具时挂起且不发 done，
// tool_request 携带工具名与原样入参；批准后结果按原序回灌为 tool message，从生成节点续跑并产出
// 带持久化引用的 done。
func TestAgentRun_SuspendThenApproveResume(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{
			{ID: "call-ks", Type: "function", Function: schema.FunctionCall{Name: "knowledge_search", Arguments: `{"query":"x"}`}},
			clientCall("call-1", `{"payload":"page-1"}`),
		}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "7",
		Content:  "chunk body",
		MetaData: map[string]any{"documentID": "7", "title": "Doc A", "chunkIndex": 0, "score": 0.9},
	}}}
	f := newAgentRunFixture(t, chat, rtv, true)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	_, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	}
	if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	if len(f.requests) != 1 {
		t.Fatalf("tool requests = %+v, want exactly one", f.requests)
	}
	req := f.requests[0]
	if req.CallID != "call-1" || req.ToolName != testClientToolName || !req.ApprovalRequired {
		t.Errorf("tool request = %+v, want call-1 / %s / approval required", req, testClientToolName)
	}
	if req.Arguments != `{"payload":"page-1"}` {
		t.Errorf("arguments = %q, want the raw model arguments", req.Arguments)
	}
	// 挂起一律落锚点行：本回合的助手消息行以 phase=suspended 保留，其 ID 回写运行现场并随 tool_request
	// 载荷下发（本回合无任何已外发正文，故为一条空锚点行），使前端实时会话能把挂起定位到该行。
	messages, err := f.store.ListMessages(ctx, f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + the suspension anchor)", len(messages))
	}
	anchor := messages[1]
	if anchor.Role != ai.MessageRoleAssistant || anchor.Phase != ai.AgentRunPhaseSuspended {
		t.Errorf("anchor = %+v, want an assistant row with phase suspended", anchor)
	}
	if req.MessageID != anchor.ID {
		t.Errorf("tool request message id = %d, want the anchor row %d", req.MessageID, anchor.ID)
	}
	if run := f.latestRun(t); run.AnchorMessageID != anchor.ID {
		t.Errorf("run anchor message id = %d, want the anchor row %d", run.AnchorMessageID, anchor.ID)
	}

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Result: `{"ok":true}`}))
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
	if len(resumed.Citations) != 1 || resumed.Citations[0].Title != "Doc A" {
		t.Errorf("resumed citations = %+v, want the persisted citation", resumed.Citations)
	}
	// 恢复后的第二轮模型输入应包含按序回灌的 tool 结果。
	if len(chat.gotInputs) != 2 {
		t.Fatalf("chat calls = %d, want 2 (suspend round + resume round)", len(chat.gotInputs))
	}
	last := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if last.Role != schema.Tool || last.ToolCallID != "call-1" || last.Content != `{"ok":true}` {
		t.Errorf("fed-back tool message = %+v, want the approved result", last)
	}
	// 续跑原地终态化锚点行：同一回合仍只有一条助手消息（不新增一行造成「残篇 + 完整回复」），锚点行 phase=done。
	final, err := f.store.ListMessages(ctx, f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(final) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (the anchor row is finalized in place, not duplicated)", len(final))
	}
	if final[1].ID != anchor.ID || final[1].Phase != ai.AgentRunPhaseDone {
		t.Errorf("final assistant = %+v, want the anchor row %d finalized to done", final[1], anchor.ID)
	}
}

// TestAgentRun_SuspendThenRejectResume 测试挂起 → 拒绝续跑：批准为 false 时以「用户拒绝」文案回灌，
// 生成继续而非中断。
func TestAgentRun_SuspendThenRejectResume(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: false}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	_, resumed, err := collectStream(t, resumeReader)
	if err != nil {
		t.Fatalf("resume stream error = %v", err)
	}
	if !resumed.Done || resumed.Content != "final answer" {
		t.Fatalf("resumed output = %+v, want the generation to continue after a rejection", resumed)
	}
	last := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if last.Role != schema.Tool || !strings.Contains(last.Content, "declined") {
		t.Errorf("fed-back tool message = %+v, want the decline notice", last)
	}
}

// TestAgentRun_ResumeKeepsSuspendedRoundText 测试挂起轮的前导说明文本在续跑时被并回最终 done
// 正文：该文本已随上次运行的流逐块外发给用户，而续跑的流只含新生成的正文；前端在 done 时以
// done 正文覆盖已累积的增量，若不并回，用户已看到的文本会凭空消失。
func TestAgentRun_ResumeKeepsSuspendedRoundText(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{
			content:   "let me check the page ",
			toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"page-1"}`)},
		},
		{content: "final answer"},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	deltas, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	}
	if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	if deltas != "let me check the page " {
		t.Fatalf("suspend deltas = %q, want the round text streamed before suspension", deltas)
	}
	// 挂起锚点行保留本回合已外发的前导说明文本（phase=suspended）：旧实现回收该行，刷新后这段文本会消失。
	messages, err := f.store.ListMessages(ctx, f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + the suspension anchor)", len(messages))
	}
	if messages[1].Phase != ai.AgentRunPhaseSuspended || string(messages[1].Content.Value()) != "let me check the page " {
		t.Errorf("anchor = %+v, want the streamed round text kept with phase suspended", messages[1])
	}

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Result: "ok"}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	resumedDeltas, resumed, err := collectStream(t, resumeReader)
	if err != nil {
		t.Fatalf("resume stream error = %v", err)
	}
	if resumedDeltas != "final answer" {
		t.Errorf("resume deltas = %q, want only the newly generated text", resumedDeltas)
	}
	if want := "let me check the page final answer"; !resumed.Done || resumed.Content != want {
		t.Fatalf("resumed output = %+v, want content %q (streamed text kept)", resumed, want)
	}
}

// TestAgentRun_SuspendThenExecErrorResume 测试挂起 → 前端回传执行错误：错误文本作为工具结果回灌，
// 绝不作为流错误上抛，生成继续。
func TestAgentRun_SuspendThenExecErrorResume(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Error: "upstream returned 500"}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	_, resumed, err := collectStream(t, resumeReader)
	if err != nil {
		t.Fatalf("resume stream error = %v, want the execution error fed back rather than raised", err)
	}
	if !resumed.Done {
		t.Fatalf("resumed output = %+v, want a done item", resumed)
	}
	last := chat.gotInputs[1][len(chat.gotInputs[1])-1]
	if last.Role != schema.Tool || last.Content != "upstream returned 500" {
		t.Errorf("fed-back tool message = %+v, want the execution error text", last)
	}
}

// TestAgentRun_EncryptedAtRest 测试运行现场的三个 JSON 字段确实以密文落库：直读原始列既不得
// 出现明文（用户原文、待执行调用、检索引用），也长于 AES-GCM 最小密文长度；恢复路径仍能解密还原。
func TestAgentRun_EncryptedAtRest(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{
			{ID: "call-ks", Type: "function", Function: schema.FunctionCall{Name: "knowledge_search", Arguments: `{"query":"x"}`}},
			clientCall("call-1", `{"payload":"page-1"}`),
		}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	rtv := &stubRetriever{docs: []*schema.Document{{
		ID:       "7",
		Content:  "chunk body",
		MetaData: map[string]any{"documentID": "7", "title": "Doc A", "chunkIndex": 0, "score": 0.9},
	}}}
	f := newAgentRunFixture(t, chat, rtv, true)

	reader, err := f.agent.StreamRun(ctx, f.input("secret-question"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}

	var row struct {
		Messages     []byte
		PendingCalls []byte
		Citations    []byte
	}
	if err := f.store.db.
		Raw("SELECT messages, pending_calls, citations FROM agent_runs WHERE session_id = ? ORDER BY id DESC LIMIT 1", f.session.ID).
		Scan(&row).Error; err != nil {
		t.Fatalf("reading the raw agent run columns error = %v", err)
	}
	assertEncryptedColumn(t, "agent_runs.messages", row.Messages, "secret-question")
	assertEncryptedColumn(t, "agent_runs.pending_calls", row.PendingCalls, "call-1")
	assertEncryptedColumn(t, "agent_runs.citations", row.Citations, "chunk body")

	// 恢复路径经 ClaimAgentRun 读库解密，运行现场可原样还原。
	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Result: `{"ok":true}`}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("resume stream error = %v", err)
	} else if !resumed.Done || len(resumed.Citations) != 1 {
		t.Fatalf("resumed output = %+v, want the final answer with the restored citation", resumed)
	}
}

// TestAgentRun_Expired 测试过期拒绝：运行现场超过存活期限时恢复返回 ErrAgentRunExpired，且该行被
// 置为 expired。
func TestAgentRun_Expired(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}
	if err := f.store.db.Model(&ai.AgentRun{}).Where("session_id = ?", f.session.ID).
		Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expiring the run error = %v", err)
	}

	_, err = f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true}))
	if !errors.Is(err, ErrAgentRunExpired) {
		t.Fatalf("ResumeRun(expired) error = %v, want ErrAgentRunExpired", err)
	}
	if run := f.latestRun(t); run.Status != ai.AgentRunStatusExpired {
		t.Errorf("run status = %q, want expired", run.Status)
	}
}

// TestAgentRun_ConsumedTwice 测试重复消费拒绝：同一运行现场成功恢复一次后，再次恢复返回
// ErrAgentRunConsumed。
func TestAgentRun_ConsumedTwice(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}

	first, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Result: "r"}))
	if err != nil {
		t.Fatalf("first ResumeRun error = %v", err)
	}
	if _, _, err := collectStream(t, first); err != nil {
		t.Fatalf("first resume stream error = %v", err)
	}
	if _, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true})); !errors.Is(err, ErrAgentRunConsumed) {
		t.Fatalf("second ResumeRun error = %v, want ErrAgentRunConsumed", err)
	}
}

// TestAgentRun_ResuspendOnResume 测试多轮再次挂起：恢复后再次命中 client 工具时再次产生 tool_request，
// 并写入一条新的 pending 运行现场；再次批准后可正常收尾。
func TestAgentRun_ResuspendOnResume(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{toolCalls: []schema.ToolCall{clientCall("call-2", `{"payload":"y"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}
	firstRun := f.latestRun(t)

	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-1", Approved: true, Result: "r1"}))
	if err != nil {
		t.Fatalf("first ResumeRun error = %v", err)
	}
	if _, done, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("first resume stream error = %v", err)
	} else if done.Done {
		t.Fatal("the resumed stream produced a done item, want a re-suspension")
	}
	if len(f.requests) != 2 || f.requests[1].CallID != "call-2" {
		t.Fatalf("tool requests = %+v, want a second request for call-2", f.requests)
	}
	secondRun := f.latestRun(t)
	if secondRun.Status != ai.AgentRunStatusPending || secondRun.ID <= firstRun.ID {
		t.Errorf("second run = %+v, want a new pending run after the first (%d)", secondRun, firstRun.ID)
	}

	finalReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-2", Approved: true, Result: "r2"}))
	if err != nil {
		t.Fatalf("second ResumeRun error = %v", err)
	}
	_, done, err := collectStream(t, finalReader)
	if err != nil {
		t.Fatalf("second resume stream error = %v", err)
	}
	if !done.Done || done.Content != "final answer" {
		t.Fatalf("final output = %+v, want the final answer done item", done)
	}
}

// TestAgentRun_ForeignNamespaceNotFound 测试归属隔离：命名空间与运行现场不一致时，运行现场在
// 查询阶段即被过滤，恢复返回 ErrAgentRunNotFound（不泄露其存在性）。
func TestAgentRun_ForeignNamespaceNotFound(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{clientCall("call-1", `{"payload":"x"}`)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v", err)
	}

	_, err = f.agent.ResumeRun(ctx, ResumeInput{
		SessionID:     f.session.ID,
		NamespaceName: "another-namespace",
		UserID:        f.session.UserID,
		Results:       []ToolResult{{CallID: "call-1", Approved: true}},
		ToolRequests:  f.reportRequest,
	})
	if !errors.Is(err, ErrAgentRunNotFound) {
		t.Fatalf("ResumeRun(foreign namespace) error = %v, want ErrAgentRunNotFound", err)
	}
}

// TestSessionStore_CreateLiveRun_PartialUnique 校验 CreateLiveRun 以部分唯一索引
// uq_agent_live_runs_running 实现「单会话单在途」：同 (session_id, user_id) 的第二个 running
// 登记被 ON CONFLICT DO NOTHING 拒绝并返回 ErrAgentLiveRunAlreadyRunning；首个收敛为终态后新
// running 可再次登记（终态行不占用该部分唯一索引）。
func TestSessionStore_CreateLiveRun_PartialUnique(t *testing.T) {
	ctx := context.Background()
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	newLiveRun := func(phase ai.AgentRunPhase) *ai.AgentLiveRun {
		return &ai.AgentLiveRun{
			NamespaceName: "default",
			UserID:        session.UserID,
			RunID:         newAgentRunID(),
			SessionID:     session.ID,
			Phase:         phase,
			ExpiresAt:     time.Now().Add(time.Hour),
		}
	}

	first := newLiveRun(ai.AgentRunPhaseRunning)
	if err := store.CreateLiveRun(ctx, first); err != nil {
		t.Fatalf("CreateLiveRun(first running) error = %v", err)
	}
	if err := store.CreateLiveRun(ctx, newLiveRun(ai.AgentRunPhaseRunning)); !errors.Is(err, ErrAgentLiveRunAlreadyRunning) {
		t.Fatalf("CreateLiveRun(second running) error = %v, want ErrAgentLiveRunAlreadyRunning", err)
	}

	// 首个收敛为终态后不再占用部分唯一索引，新 running 可重新登记。
	if err := store.FinishLiveRun(ctx, first.RunID, session.UserID, ai.AgentRunPhaseDone, 0); err != nil {
		t.Fatalf("FinishLiveRun error = %v", err)
	}
	if err := store.CreateLiveRun(ctx, newLiveRun(ai.AgentRunPhaseRunning)); err != nil {
		t.Fatalf("CreateLiveRun(after the previous converged) error = %v, want success", err)
	}
}

// deadlineChat 是「先下发正文与思考，再等上下文过期并以过期错误中断」的模型替身，忠实复现长耗时
// 运行被运行上界（deadline）中断的场景：用于确认中断时已产出的内容被落库、正文保持纯净（提示文案由
// 前端据 phase 渲染而非混入正文）、助手消息落库为 failed 阶段，且落库不被已过期的运行上下文拖垮
// （落库走 WithoutCancel 的 persistCtx）。
type deadlineChat struct {
	reason  string
	content string
}

// Generate 实现 model.BaseChatModel；本替身只服务于流式路径，非流式调用一律报错而非返回 nil。
func (c *deadlineChat) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, errors.New("deadlineChat: only the streaming path is supported")
}

// Stream 先下发思考与正文分片，随后阻塞到 ctx 过期，再以该错误结束消息流。
func (c *deadlineChat) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](4)
	go func() {
		defer sw.Close()
		if c.reason != "" {
			sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: c.reason}, nil)
		}
		if c.content != "" {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: c.content}, nil)
		}
		<-ctx.Done()
		sw.Send(nil, errors.WithStack(ctx.Err()))
	}()
	return sr, nil
}

// TestStreamRun_RunDeadlinePersistsPartialAssistantMessage 测试运行被 deadline 中断时阶段性落库
// 兜住中间产物：已产出的思考与正文落库为助手消息，正文保持纯净（中断提示由前端据 phase 渲染，绝不
// 混入正文以免喂回模型），且助手消息落库为 failed 阶段；落库不受已过期的运行上下文影响（否则
// 「超时被中断」这一最需要落库的场景恰好写不进库）。
func TestStreamRun_RunDeadlinePersistsPartialAssistantMessage(t *testing.T) {
	content := strings.Repeat("c", assistantCheckpointBytes*2)
	reasoning := strings.Repeat("r", assistantCheckpointBytes)
	f := newAgentRunFixture(t, &deadlineChat{reason: reasoning, content: content}, &stubRetriever{}, false)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, _, err := collectStream(t, reader); err == nil {
		t.Fatal("stream error = nil, want the run deadline error")
	}

	messages, err := f.store.ListMessages(context.Background(), f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + the interrupted assistant reply)", len(messages))
	}
	if got, want := string(messages[1].Content.Value()), content; got != want {
		t.Errorf("persisted assistant content = %q, want the partial content only (the notice must not be mixed into the content)", got)
	}
	if got := reasoningTextOf(t, f.store, messages[1]); got != reasoning {
		t.Errorf("persisted assistant reasoning = %q, want the partial reasoning", got)
	}
	if got := messages[1].Phase; got != ai.AgentRunPhaseFailed {
		t.Errorf("persisted assistant phase = %q, want failed", got)
	}
}

// TestWrapPersist_CancelledRunPersistsCancelledPhase 测试运行被取消时助手消息落库为 cancelled 阶段
// （而非 failed）：取消既可能体现为承载上下文被取消，也可能体现为流返回包装过的 context.Canceled
// 错误，本用例直接驱动 wrapPersist 以「已取消的上下文 + 包装 context.Canceled 的流错误」复现，
// 确认中断阶段归类为 cancelled（分类见 interruptedPhase）。
func TestWrapPersist_CancelledRunPersistsCancelledPhase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newAgentRunFixture(t, &scriptedChat{turns: []scriptedTurn{{content: "unused"}}}, &stubRetriever{}, false)

	partial := strings.Repeat("p", assistantCheckpointBytes*2)
	inSR, inSW := schema.Pipe[AgentOutput](8)
	go func() {
		defer inSW.Close()
		inSW.Send(AgentOutput{Delta: partial}, nil)
		cancel()
		inSW.Send(AgentOutput{}, errors.WithStack(context.Canceled))
	}()
	if _, _, err := collectStream(t, f.agent.wrapPersist(ctx, inSR, f.input("q"), newRunMetrics())); err == nil {
		t.Fatal("stream error = nil, want the cancellation error")
	}

	messages, err := f.store.ListMessages(context.Background(), f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("ListMessages len = %d, want 1 (the cancelled assistant reply)", len(messages))
	}
	if got := messages[0].Phase; got != ai.AgentRunPhaseCancelled {
		t.Errorf("persisted assistant phase = %q, want cancelled", got)
	}
	// 中止后保留已生成的部分正文（不向正文拼接任何提示文案：提示由前端据 cancelled 阶段渲染）。
	if got := string(messages[0].Content.Value()); got != partial {
		t.Errorf("persisted assistant content len = %d, want the partial content preserved (len %d)", len(got), len(partial))
	}
}

// TestWrapPersist_SuspensionDiscardsCheckpointedRow 测试「无 done 的 EOF」（挂起等待工具审批）时
// 回收阶段性落库的助手行：挂起不是终局，续跑会把本回合已外发的正文并回最终 done，留着残篇会在续跑
// 结束后留下同一回合的两条助手消息。直接驱动 wrapPersist（不经图循环），使「落库残篇 → 挂起时回收」
// 在无并发写库下确定复现：会话的 message_seq 已自增到 1（证明残篇确实落库过）而消息行为 0（证明它被
// 回收）；若残篇未落库则 message_seq 仍为 0，若未被回收则消息行为 1。
func TestWrapPersist_SuspensionDiscardsCheckpointedRow(t *testing.T) {
	ctx := context.Background()
	f := newAgentRunFixture(t, &scriptedChat{turns: []scriptedTurn{{content: "unused"}}}, &stubRetriever{}, false)

	inSR, inSW := schema.Pipe[AgentOutput](8)
	go func() {
		defer inSW.Close()
		inSW.Send(AgentOutput{Delta: strings.Repeat("p", assistantCheckpointBytes*2)}, nil)
	}()
	if _, _, err := collectStream(t, f.agent.wrapPersist(ctx, inSR, f.input("q"), newRunMetrics())); err != nil {
		t.Fatalf("stream error = %v, want a clean end (the stream ended with EOF, as on suspension)", err)
	}

	messages, err := f.store.ListMessages(ctx, f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("ListMessages len = %d, want 0 (the checkpointed row must be discarded on suspension)", len(messages))
	}
	session, err := f.store.GetSession(ctx, f.session.ID, "default", f.session.UserID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if session.MessageSeq != 1 {
		t.Errorf("session message_seq = %d, want 1 (a checkpoint row was allocated before being discarded)", session.MessageSeq)
	}
}

// TestAgentRun_QuestionnaireAnchor 测试问卷挂起与续跑的锚点消息不变式：ask_user 调用豁免审核
// （tool_request 恒 none / 不需人工审核），挂起时保留本回合的助手消息行作为锚点（phase=suspended、
// 带问卷题目，并把其 ID 回写运行现场）；续跑回传答案后原地终态化该行（phase=done、保留问卷题目
// 并并入答案），同一回合始终只有一条用户消息与一条助手消息。
func TestAgentRun_QuestionnaireAnchor(t *testing.T) {
	ctx := context.Background()
	args := `{"questions":[{"id":"q1","question":"选哪个方案？","options":[{"text":"方案A"},{"text":"方案B","detail":"更稳妥"}]}]}`
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{askUserCall("call-q", args)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, done, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	if len(f.requests) != 1 {
		t.Fatalf("tool requests = %+v, want exactly one", f.requests)
	}
	req := f.requests[0]
	if req.CallID != "call-q" || req.ToolName != askUserToolName {
		t.Errorf("tool request = %+v, want call-q / %s", req, askUserToolName)
	}
	if req.ApprovalRequired || req.AuditLevel != AuditLevelNone {
		t.Errorf("tool request = %+v, want the questionnaire exempt from the review valve (none / not required)", req)
	}

	// 挂起后：会话内恰有一条用户消息与一条助手锚点消息（phase=suspended、带问卷题目）。
	anchor := questionnaireAnchorRow(t, f)
	if anchor.Phase != ai.AgentRunPhaseSuspended {
		t.Errorf("anchor phase = %q, want suspended", anchor.Phase)
	}
	rounds := decodeAnchorRounds(t, f, anchor)
	if len(rounds) != 1 {
		t.Fatalf("anchor rounds = %d, want exactly one round", len(rounds))
	}
	if qs := rounds[0].Questions; len(qs) != 1 || qs[0].ID != "q1" || qs[0].Question != "选哪个方案？" {
		t.Fatalf("anchor questions = %+v, want the single q1 question", qs)
	}
	if len(rounds[0].Answers) != 0 {
		t.Errorf("anchor answers = %+v, want none before the user answered", rounds[0].Answers)
	}
	if run := f.latestRun(t); run.AnchorMessageID != anchor.ID {
		t.Errorf("run anchor message id = %d, want the anchor row %d", run.AnchorMessageID, anchor.ID)
	}
	// 载荷带锚点消息 ID：tool_request 在锚点行落库后才下发，故其 messageID 应为锚点行 ID，
	// 供前端在实时会话内即刻拿到「重新回答」所需的截断目标（实时气泡本地不带 id）。
	if req.MessageID != anchor.ID {
		t.Errorf("tool request message id = %d, want the anchor row %d", req.MessageID, anchor.ID)
	}

	// 续跑：回传答案后原地终态化锚点行，不新增助手消息。
	answers := `{"answers":[{"questionID":"q1","answered":true,"answer":"方案A","custom":false}]}`
	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(ToolResult{CallID: "call-q", Approved: true, Result: answers}))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("resume stream error = %v", err)
	} else if !resumed.Done || resumed.Content != "final answer" {
		t.Fatalf("resumed output = %+v, want the final answer done item", resumed)
	}

	final := questionnaireAnchorRow(t, f)
	if final.ID != anchor.ID {
		t.Errorf("final assistant id = %d, want the anchor row %d (a single assistant message per turn)", final.ID, anchor.ID)
	}
	if final.Phase != ai.AgentRunPhaseDone {
		t.Errorf("final phase = %q, want done", final.Phase)
	}
	finalRounds := decodeAnchorRounds(t, f, final)
	if len(finalRounds) != 1 {
		t.Fatalf("final rounds = %d, want exactly one round", len(finalRounds))
	}
	if as := finalRounds[0].Answers; len(as) != 1 || !as[0].Answered || as[0].Answer != "方案A" || as[0].Custom {
		t.Errorf("final answers = %+v, want the q1 answer as the selected option", as)
	}
}

// TestAgentRun_QuestionnaireMerge 测试一轮内多条 ask_user 调用的合并：题目按调用顺序合并为同一张问卷
// （锚点行只落一条合并载荷），每条调用各自挂起一条 tool_request（均豁免审核）；续跑逐条回传各自答案后
// 合并写入锚点行——同一回合仍只有一条助手消息。
func TestAgentRun_QuestionnaireMerge(t *testing.T) {
	ctx := context.Background()
	firstArgs := `{"questions":[{"id":"q1","question":"选哪个方案？","options":[{"text":"方案A"},{"text":"方案B"}]}]}`
	secondArgs := `{"questions":[{"id":"q2","question":"何时上线？","options":[{"text":"本周"},{"text":"下周"}]}]}`
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{askUserCall("call-q1", firstArgs), askUserCall("call-q2", secondArgs)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, done, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	if len(f.requests) != 2 {
		t.Fatalf("tool requests = %+v, want two (one per ask_user call)", f.requests)
	}
	for _, req := range f.requests {
		if req.ToolName != askUserToolName || req.ApprovalRequired || req.AuditLevel != AuditLevelNone {
			t.Errorf("tool request = %+v, want an ask_user call exempt from the review valve", req)
		}
	}

	// 挂起后：题目按调用顺序合并进同一条锚点行。
	anchor := questionnaireAnchorRow(t, f)
	if anchor.Phase != ai.AgentRunPhaseSuspended {
		t.Errorf("anchor phase = %q, want suspended", anchor.Phase)
	}
	payload := decodeAnchorRounds(t, f, anchor)
	if len(payload) != 1 {
		t.Fatalf("anchor rounds = %d, want a single merged round", len(payload))
	}
	if qs := payload[0].Questions; len(qs) != 2 || qs[0].ID != "q1" || qs[1].ID != "q2" {
		t.Fatalf("anchor questions = %+v, want the merged q1 / q2 questions", qs)
	}
	// 每条挂起调用都带同一锚点消息 ID（合并问卷只落一条锚点行，两条调用据它共同定位）。
	for _, req := range f.requests {
		if req.MessageID != anchor.ID {
			t.Errorf("tool request = %+v, message id = %d, want the shared anchor row %d", req, req.MessageID, anchor.ID)
		}
	}

	// 续跑：逐条回传各自的答案，合并写入锚点行，仍不新增助手消息。
	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(
		ToolResult{CallID: "call-q1", Approved: true, Result: `{"answers":[{"questionID":"q1","answered":true,"answer":"方案A"}]}`},
		ToolResult{CallID: "call-q2", Approved: true, Result: `{"answers":[{"questionID":"q2","answered":true,"answer":"本周"}]}`},
	))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("resume stream error = %v", err)
	} else if !resumed.Done || resumed.Content != "final answer" {
		t.Fatalf("resumed output = %+v, want the final answer done item", resumed)
	}

	final := questionnaireAnchorRow(t, f)
	if final.ID != anchor.ID {
		t.Errorf("final assistant id = %d, want the anchor row %d (a single assistant message per turn)", final.ID, anchor.ID)
	}
	finalPayload := decodeAnchorRounds(t, f, final)
	if len(finalPayload) != 1 {
		t.Fatalf("final rounds = %d, want a single merged round", len(finalPayload))
	}
	if as := finalPayload[0].Answers; len(as) != 2 {
		t.Fatalf("final answers = %+v, want the merged q1 / q2 answers", as)
	} else if as[0].QuestionID != "q1" || as[0].Answer != "方案A" ||
		as[1].QuestionID != "q2" || as[1].Answer != "本周" {
		t.Errorf("final answers = %+v, want q1 = 方案A and q2 = 本周", as)
	}
}

// TestAgentRun_QuestionnaireSecondRound 测试同一回合内的多轮提问：用户作答第一轮后模型再次调用 ask_user
// 提问第二轮，锚点行的问卷载荷须**按轮次并列**（第二轮挂起时第一轮的题目与答案各自留在自己那一轮里，
// 终态化后两轮题目与答案俱在）。前端据此把各轮渲染成各自独立的问答框，而不是把第二轮题目塞进第一轮框内。
func TestAgentRun_QuestionnaireSecondRound(t *testing.T) {
	ctx := context.Background()
	firstArgs := `{"questions":[{"id":"q1","question":"选哪个方案？","options":[{"text":"方案A"},{"text":"方案B"}]}]}`
	secondArgs := `{"questions":[{"id":"q2","question":"何时上线？","options":[{"text":"本周"},{"text":"下周"}]}]}`
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{askUserCall("call-q1", firstArgs)}},
		{toolCalls: []schema.ToolCall{askUserCall("call-q2", secondArgs)}},
		{content: `{"content": "final answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	// 第一轮挂起：锚点行只含 q1，尚无答案。
	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, done, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	anchor := questionnaireAnchorRow(t, f)
	first := decodeAnchorRounds(t, f, anchor)
	if len(first) != 1 {
		t.Fatalf("first suspension rounds = %d, want exactly one round", len(first))
	}
	if qs := first[0].Questions; len(qs) != 1 || qs[0].ID != "q1" || len(first[0].Answers) != 0 {
		t.Fatalf("first suspension payload = %+v, want the q1 question without answers", first[0])
	}

	// 作答第一轮续跑：模型改提第二轮问题（q2），锚点行须把第一轮作为独立一轮保留、第二轮另起一轮追加。
	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(
		ToolResult{CallID: "call-q1", Approved: true, Result: `{"answers":[{"questionID":"q1","answered":true,"answer":"方案A"}]}`},
	))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	if _, done, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("second suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("the second suspension produced a done item, want tool_request as the terminal state")
	}
	if len(f.requests) != 2 {
		t.Fatalf("tool requests = %+v, want two (one per round)", f.requests)
	}
	if second := f.requests[1]; second.CallID != "call-q2" || second.MessageID != anchor.ID {
		t.Errorf("second tool request = %+v, want call-q2 anchored to the row %d", second, anchor.ID)
	}
	secondRow := questionnaireAnchorRow(t, f)
	if secondRow.ID != anchor.ID || secondRow.Phase != ai.AgentRunPhaseSuspended {
		t.Errorf("second suspension row = (%d, %s), want the same anchor row %d still suspended", secondRow.ID, secondRow.Phase, anchor.ID)
	}
	secondRounds := decodeAnchorRounds(t, f, secondRow)
	if len(secondRounds) != 2 {
		t.Fatalf("second suspension rounds = %d, want two independent rounds", len(secondRounds))
	}
	if qs := secondRounds[0].Questions; len(qs) != 1 || qs[0].ID != "q1" {
		t.Fatalf("first round questions = %+v, want the q1 question", qs)
	}
	if as := secondRounds[0].Answers; len(as) != 1 || as[0].QuestionID != "q1" || as[0].Answer != "方案A" {
		t.Fatalf("first round answers = %+v, want the q1 answer kept in its own round", as)
	}
	if qs := secondRounds[1].Questions; len(qs) != 1 || qs[0].ID != "q2" || len(secondRounds[1].Answers) != 0 {
		t.Fatalf("second round = %+v, want the new q2 question without answers", secondRounds[1])
	}

	// 作答第二轮续跑至终态：两轮题目与答案俱在，同一回合仍只有一条助手消息。
	resumeReader, err = f.agent.ResumeRun(ctx, f.resumeInput(
		ToolResult{CallID: "call-q2", Approved: true, Result: `{"answers":[{"questionID":"q2","answered":true,"answer":"本周"}]}`},
	))
	if err != nil {
		t.Fatalf("ResumeRun error = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("resume stream error = %v", err)
	} else if !resumed.Done || resumed.Content != "final answer" {
		t.Fatalf("resumed output = %+v, want the final answer done item", resumed)
	}
	final := questionnaireAnchorRow(t, f)
	if final.ID != anchor.ID || final.Phase != ai.AgentRunPhaseDone {
		t.Errorf("final row = (%d, %s), want the anchor row %d in done", final.ID, final.Phase, anchor.ID)
	}
	finalRounds := decodeAnchorRounds(t, f, final)
	if len(finalRounds) != 2 {
		t.Fatalf("final rounds = %d, want two independent rounds", len(finalRounds))
	}
	if len(finalRounds[0].Questions) != 1 || len(finalRounds[0].Answers) != 1 ||
		len(finalRounds[1].Questions) != 1 || len(finalRounds[1].Answers) != 1 {
		t.Fatalf("final rounds = %+v, want each round carrying its own question and answer", finalRounds)
	}
	if a0 := finalRounds[0].Answers[0]; a0.QuestionID != "q1" || a0.Answer != "方案A" {
		t.Errorf("first round answer = %+v, want q1 = 方案A", a0)
	}
	if a1 := finalRounds[1].Answers[0]; a1.QuestionID != "q2" || a1.Answer != "本周" {
		t.Errorf("second round answer = %+v, want q2 = 本周", a1)
	}
}

// TestAgentRun_QuestionnaireCancelAfterRerun 测试「重新回答后再取消回答」：回退重开（RerunAssistantInput）
// 会在锚点行上保留上次作答的答案供预填，此后用户取消回答时末轮的旧答案须被清空——否则终态化写回的仍是
// 旧答案，界面 / 刷新后会把上次作答当成本次作答展示（本次取消应当全部题目未作答）。
func TestAgentRun_QuestionnaireCancelAfterRerun(t *testing.T) {
	ctx := context.Background()
	args := `{"questions":[{"id":"q1","question":"选哪个方案？","options":[{"text":"方案A"},{"text":"方案B"}]}]}`
	chat := &scriptedChat{turns: []scriptedTurn{
		{toolCalls: []schema.ToolCall{askUserCall("call-q1", args)}},
		{content: `{"content": "first answer", "draft": {}}`},
		{content: `{"content": "second answer", "draft": {}}`},
	}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	// 第一轮挂起后作答续跑至终态：锚点行的 q1 带上答案（方案A）。
	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	if _, done, err := collectStream(t, reader); err != nil {
		t.Fatalf("suspend stream error = %v, want a clean end", err)
	} else if done.Done {
		t.Fatal("suspend stream produced a done item, want tool_request as the terminal state")
	}
	anchor := questionnaireAnchorRow(t, f)
	resumeReader, err := f.agent.ResumeRun(ctx, f.resumeInput(
		ToolResult{CallID: "call-q1", Approved: true, Result: `{"answers":[{"questionID":"q1","answered":true,"answer":"方案A"}]}`},
	))
	if err != nil {
		t.Fatalf("ResumeRun(error) = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("resume stream error = %v", err)
	} else if !resumed.Done {
		t.Fatalf("resumed output = %+v, want the answered turn to finish", resumed)
	}
	answered := decodeAnchorRounds(t, f, questionnaireAnchorRow(t, f))
	if as := answered[0].Answers; len(as) != 1 || as[0].QuestionID != "q1" || as[0].Answer != "方案A" {
		t.Fatalf("round after answering = %+v, want q1 = 方案A", answered[0])
	}

	// 回退重开：现场复位为 pending、锚点行仍保留上次答案（供前端预填重开）。
	if _, err := f.store.RerunAssistantInput(ctx, f.session.ID, f.session.UserID, anchor.ID); err != nil {
		t.Fatalf("RerunAssistantInput error = %v", err)
	}
	kept := decodeAnchorRounds(t, f, questionnaireAnchorRow(t, f))
	if as := kept[0].Answers; len(as) != 1 || as[0].Answer != "方案A" {
		t.Fatalf("round after rerun = %+v, want the previous answer kept for prefill", kept[0])
	}

	// 取消回答续跑至终态：末轮的旧答案须被清空，回到「全部题目未作答」。
	resumeReader, err = f.agent.ResumeRun(ctx, f.resumeInput(
		ToolResult{CallID: "call-q1", Approved: false},
	))
	if err != nil {
		t.Fatalf("ResumeRun(cancel) error = %v", err)
	}
	if _, resumed, err := collectStream(t, resumeReader); err != nil {
		t.Fatalf("cancel resume stream error = %v", err)
	} else if !resumed.Done {
		t.Fatalf("cancel resumed output = %+v, want the turn to finish", resumed)
	}
	final := decodeAnchorRounds(t, f, questionnaireAnchorRow(t, f))
	if len(final) != 1 || len(final[0].Answers) != 0 {
		t.Fatalf("round after cancel = %+v, want the previous answer cleared (nothing answered)", final)
	}
}

// questionnaireAnchorRow 回读会话内承载问卷锚点的助手消息行，并断言本回合恰有一条用户消息与一条
// 助手消息（维持「同一回合只有一条助手消息」不变式）。
func questionnaireAnchorRow(t *testing.T, f *agentRunFixture) *ai.AgentMessage {
	t.Helper()
	messages, err := f.store.ListMessages(context.Background(), f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + the questionnaire anchor)", len(messages))
	}
	return messages[1]
}

// decodeAnchorRounds 回读一条助手消息上的 questionnaire 块，按 block_seq 先后还原为按轮次并列的问卷
// 载荷；无问卷块或无法解码即失败。
func decodeAnchorRounds(t *testing.T, f *agentRunFixture, msg *ai.AgentMessage) []QuestionnairePayload {
	t.Helper()
	blocks, err := f.store.ListBlocksByMessage(context.Background(), msg.ID, msg.NamespaceName, msg.UserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessage error = %v", err)
	}
	rounds := make([]QuestionnairePayload, 0, len(blocks))
	for _, block := range blocks {
		if block.Kind != ai.AgentMessageBlockKindQuestionnaire || block.Payload == nil {
			continue
		}
		var payload QuestionnaireBlockPayload
		if derr := decodeBlockPayload(block.Payload.Value(), &payload); derr != nil {
			t.Fatalf("decoding questionnaire block payload failed: %v", derr)
		}
		if len(payload.Questions) == 0 {
			continue
		}
		rounds = append(rounds, QuestionnairePayload{Questions: payload.Questions, Answers: payload.Answers})
	}
	if len(rounds) == 0 {
		t.Fatal("the anchor message carries no questionnaire block")
	}
	return rounds
}

// blocksOfMsg 按 block_seq 升序回读一条消息的全部内容块。
func blocksOfMsg(t *testing.T, store *SessionStore, msg *ai.AgentMessage) []*ai.AgentMessageBlock {
	t.Helper()
	blocks, err := store.ListBlocksByMessage(context.Background(), msg.ID, msg.NamespaceName, msg.UserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessage error = %v", err)
	}
	return blocks
}

// reasoningTextOf 归并一条消息的全部 reasoning 块文本（思考段按时序各成一块，此处只关心合计文本）。
func reasoningTextOf(t *testing.T, store *SessionStore, msg *ai.AgentMessage) string {
	t.Helper()
	var sb strings.Builder
	for _, block := range blocksOfMsg(t, store, msg) {
		if block.Kind != ai.AgentMessageBlockKindReasoning || block.Payload == nil {
			continue
		}
		var payload ReasoningBlockPayload
		if derr := decodeBlockPayload(block.Payload.Value(), &payload); derr != nil {
			t.Fatalf("decoding reasoning block payload failed: %v", derr)
		}
		sb.WriteString(payload.Text)
	}
	return sb.String()
}

// TestClaimAgentRun_PrefersPendingOverNewerConsumed 测试认领顺序：回退（reanswer）会把较早的问卷
// 现场重置为 pending 并把更晚的现场置为已消费，故 ClaimAgentRun 必须优先取最新 pending 现场，而非
// 单纯取 id 最大的一行——否则会认领到已消费的现场、回退后的重答无法续跑。
func TestClaimAgentRun_PrefersPendingOverNewerConsumed(t *testing.T) {
	ctx := context.Background()
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "t", "standard", "knowledge", testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	saveRun := func(status ai.AgentRunStatus) ai.AgentRun {
		t.Helper()
		run := &ai.AgentRun{
			NamespaceName:    "default",
			UserID:           session.UserID,
			SessionID:        session.ID,
			Status:           status,
			MessagesJSON:     serializer.NewCrypto([]byte("[]")),
			PendingCallsJSON: serializer.NewCrypto([]byte("[]")),
			CitationsJSON:    serializer.NewCrypto([]byte("[]")),
			Slot:             "standard",
			ExpiresAt:        time.Now().Add(time.Hour),
		}
		if err := store.SaveAgentRun(ctx, run); err != nil {
			t.Fatalf("SaveAgentRun(%s) error = %v", status, err)
		}
		return *run
	}
	older := saveRun(ai.AgentRunStatusPending)
	newer := saveRun(ai.AgentRunStatusConsumed)
	if newer.ID <= older.ID {
		t.Fatalf("run ids = (%d, %d), want the consumed run saved after the pending one", older.ID, newer.ID)
	}

	claimed, err := store.ClaimAgentRun(ctx, session.ID, "default", session.UserID)
	if err != nil {
		t.Fatalf("ClaimAgentRun error = %v", err)
	}
	if claimed.ID != older.ID {
		t.Errorf("claimed run id = %d, want the older pending run %d (not the newer consumed one %d)", claimed.ID, older.ID, newer.ID)
	}
}

// TestAgentRun_DoneCarriesMessageID 测试 done 项携带本回合助手消息的落库行 ID：该 ID 只有落库后才
// 可知，故由持久化收尾处回填；前端实时会话据此把本地气泡绑定到已落库的那一行（如「压缩到此」需按
// 消息 ID 定位折叠边界），无需先回读历史即可发起。
func TestAgentRun_DoneCarriesMessageID(t *testing.T) {
	ctx := context.Background()
	chat := &scriptedChat{turns: []scriptedTurn{{content: `{"content": "done answer", "draft": {}}`}}}
	f := newAgentRunFixture(t, chat, &stubRetriever{}, false)

	reader, err := f.agent.StreamRun(ctx, f.input("q"))
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	_, done, err := collectStream(t, reader)
	if err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if !done.Done || done.Content != "done answer" {
		t.Fatalf("done item = %+v, want the finalized answer", done)
	}
	if done.MessageID == 0 {
		t.Fatal("done item carries no message id, want the persisted assistant row id")
	}
	messages, err := f.store.ListMessages(ctx, f.session.ID, f.session.UserID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ListMessages len = %d, want 2 (user + assistant)", len(messages))
	}
	if messages[1].ID != done.MessageID {
		t.Errorf("done messageID = %d, want the persisted assistant row %d", done.MessageID, messages[1].ID)
	}
}
