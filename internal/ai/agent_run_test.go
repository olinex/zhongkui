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
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// testClientToolName 是本包运行现场用例使用的客户端工具名：生产 client 注册表当前为空，故用例
// 自行登记一个占位 client 工具，使「命中 client 工具即挂起」的分支可被触发。
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
	if got := string(messages[1].Reasoning.Value()); got != reasoning {
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

	inSR, inSW := schema.Pipe[AgentOutput](8)
	go func() {
		defer inSW.Close()
		inSW.Send(AgentOutput{Delta: strings.Repeat("p", assistantCheckpointBytes*2)}, nil)
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
