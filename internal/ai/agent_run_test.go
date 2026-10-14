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
