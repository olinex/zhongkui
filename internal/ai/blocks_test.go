package ai

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// blockSeqs 返回 writes 中按首次出现顺序排列的各块 BlockSeq（用于校验块序列）。
func blockSeqs(writes []blockWrite) []uint64 {
	seen := make(map[uint64]bool, len(writes))
	var seqs []uint64
	for _, w := range writes {
		if seen[w.BlockSeq] {
			continue
		}
		seen[w.BlockSeq] = true
		seqs = append(seqs, w.BlockSeq)
	}
	return seqs
}

// kindAt 返回 writes 中指定 BlockSeq 的块类型（用于校验块类型）。
func kindAt(writes []blockWrite, seq uint64) ai.AgentMessageBlockKind {
	for _, w := range writes {
		if w.BlockSeq == seq {
			return w.Kind
		}
	}
	return ""
}

// finalPayloadAt 返回 writes 中指定 BlockSeq 的最后一次写入载荷（即该块的终态内容）。
func finalPayloadAt(t *testing.T, writes []blockWrite, seq uint64) []byte {
	t.Helper()
	var payload []byte
	found := false
	for _, w := range writes {
		if w.BlockSeq == seq {
			payload = w.Payload
			found = true
		}
	}
	if !found {
		t.Fatalf("no write for block_seq %d", seq)
	}
	return payload
}

// TestBlockBuilder_ReasoningCoalescesAndSplits 覆盖「思考段切分」：思考 -> 工具调用 -> 思考 落为两个
// 独立 reasoning 块（各位于其后的 tool_calls 块之前），而非一整段思考。
func TestBlockBuilder_ReasoningCoalescesAndSplits(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddReasoning("思考一")
	b.AddReasoning("（续）")
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "search", CallID: "c1", Round: 1}, `{"q":"x"}`, AuditLevelNone, ToolExecOwnerBackend)
	b.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: "search", CallID: "c1", Round: 1, ElapsedMs: 12}, "命中 3 条")
	b.AddReasoning("思考二")
	b.Close()
	writes := b.DrainWrites()

	seqs := blockSeqs(writes)
	if len(seqs) != 3 || seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 {
		t.Fatalf("block seqs = %v, want [1 2 3]", seqs)
	}
	if kindAt(writes, 1) != ai.AgentMessageBlockKindReasoning || kindAt(writes, 3) != ai.AgentMessageBlockKindReasoning {
		t.Errorf("kinds = %s / %s, want reasoning at seq 1 and 3", kindAt(writes, 1), kindAt(writes, 3))
	}
	if kindAt(writes, 2) != ai.AgentMessageBlockKindToolCalls {
		t.Errorf("kind at seq 2 = %s, want tool_calls", kindAt(writes, 2))
	}
	var first, second ReasoningBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, 1), &first); err != nil {
		t.Fatalf("decode reasoning#1 error = %v", err)
	}
	if err := decodeBlockPayload(finalPayloadAt(t, writes, 3), &second); err != nil {
		t.Fatalf("decode reasoning#2 error = %v", err)
	}
	if first.Text != "思考一（续）" || second.Text != "思考二" {
		t.Errorf("reasoning texts = %q / %q, want two independent segments", first.Text, second.Text)
	}
}

// TestBlockBuilder_ContentRule 覆盖正文增量规则：当前开放块不是 content 时新建 content 块，否则追加；
// 与思考交错时各自成块。
func TestBlockBuilder_ContentRule(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddContent("正文一")
	b.AddContent("（续）")
	b.AddReasoning("思考")
	b.AddContent("正文二")
	b.Close()
	writes := b.DrainWrites()

	seqs := blockSeqs(writes)
	if len(seqs) != 3 {
		t.Fatalf("block seqs = %v, want 3 blocks", seqs)
	}
	if kindAt(writes, 1) != ai.AgentMessageBlockKindContent || kindAt(writes, 2) != ai.AgentMessageBlockKindReasoning || kindAt(writes, 3) != ai.AgentMessageBlockKindContent {
		t.Fatalf("kinds = %s/%s/%s, want content/reasoning/content", kindAt(writes, 1), kindAt(writes, 2), kindAt(writes, 3))
	}
	var first ContentBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, 1), &first); err != nil {
		t.Fatalf("decode content#1 error = %v", err)
	}
	if first.Text != "正文一（续）" {
		t.Errorf("content#1 text = %q, want accumulated text", first.Text)
	}
}

// TestBlockBuilder_DeltaWritesOnce 覆盖「增量与收敛各写一次」：同一思考块的多段增量只累积、不逐段落库，
// 仅创建与收敛各产出一次写入（避免每 token 一次 UPDATE）。
func TestBlockBuilder_DeltaWritesOnce(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddReasoning("a")
	b.AddReasoning("b")
	b.AddReasoning("c")
	b.Close()
	writes := b.DrainWrites()

	if len(writes) != 2 {
		t.Fatalf("writes len = %d, want 2 (one create, one converge)", len(writes))
	}
	if !writes[0].Created || writes[1].Created {
		t.Errorf("created flags = %t/%t, want true then false", writes[0].Created, writes[1].Created)
	}
	if writes[0].BlockSeq != 1 || writes[1].BlockSeq != 1 {
		t.Errorf("writes seqs = %d/%d, want both 1", writes[0].BlockSeq, writes[1].BlockSeq)
	}
}

// TestBlockBuilder_ToolCallsGroupByRound 覆盖「同轮并行调用」：同 round 的多次调用归入同一 tool_calls
// 块，不同 round 各自成块。
func TestBlockBuilder_ToolCallsGroupByRound(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	for _, id := range []string{"c1", "c2", "c3"} {
		b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "t", CallID: id, Round: 1}, "{}", AuditLevelNone, ToolExecOwnerBackend)
	}
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "t", CallID: "c4", Round: 2}, "{}", AuditLevelNone, ToolExecOwnerBackend)
	b.Close()
	writes := b.DrainWrites()

	seqs := blockSeqs(writes)
	if len(seqs) != 2 {
		t.Fatalf("block seqs = %v, want 2 tool_calls blocks", seqs)
	}
	var round1 ToolCallBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, seqs[0]), &round1); err != nil {
		t.Fatalf("decode round1 error = %v", err)
	}
	if len(round1.Calls) != 3 {
		t.Fatalf("round1 calls = %d, want 3 merged into one block", len(round1.Calls))
	}
	for i, call := range round1.Calls {
		if call.Round != 1 {
			t.Errorf("round1 call[%d].Round = %d, want 1", i, call.Round)
		}
		if call.Status != EvalStatusStart {
			t.Errorf("round1 call[%d].Status = %q, want start", i, call.Status)
		}
	}
	var round2 ToolCallBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, seqs[1]), &round2); err != nil {
		t.Fatalf("decode round2 error = %v", err)
	}
	if len(round2.Calls) != 1 || round2.Calls[0].CallID != "c4" {
		t.Fatalf("round2 calls = %+v, want a single c4", round2.Calls)
	}
}

// TestBlockBuilder_ToolResultBackfill 覆盖 toolResult 归并回填：按 callID 找回记录并回填状态 / 耗时 /
// 错误 / 结果。
func TestBlockBuilder_ToolResultBackfill(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "search", CallID: "c1", Round: 1}, `{"q":"x"}`, AuditLevelMust, ToolExecOwnerClient)
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "write", CallID: "c2", Round: 1}, `{"v":1}`, AuditLevelNone, ToolExecOwnerBackend)
	b.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: "search", CallID: "c1", Round: 1, ElapsedMs: 34}, "结果文本")
	b.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: "write", CallID: "c2", Round: 1, ElapsedMs: 5, Err: "boom"}, "")
	b.Close()
	writes := b.DrainWrites()

	var payload ToolCallBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, 1), &payload); err != nil {
		t.Fatalf("decode tool_calls error = %v", err)
	}
	if len(payload.Calls) != 2 {
		t.Fatalf("calls len = %d, want 2", len(payload.Calls))
	}
	done := payload.Calls[0]
	if done.CallID != "c1" || done.Status != EvalStatusDone || done.ElapsedMs != 34 || done.Result != "结果文本" {
		t.Errorf("call c1 = %+v, want done with result", done)
	}
	if done.Arguments != `{"q":"x"}` || done.AuditLevel != AuditLevelMust || done.ExecOwner != ToolExecOwnerClient {
		t.Errorf("call c1 = %+v, want arguments/audit/owner preserved", done)
	}
	failed := payload.Calls[1]
	if failed.CallID != "c2" || failed.Status != EvalStatusFailed || failed.ElapsedMs != 5 || failed.Error != "boom" {
		t.Errorf("call c2 = %+v, want failed with error", failed)
	}
}

// TestBlockBuilder_ToolResultTruncated 覆盖工具结果按回灌字节上限截断后再落库。
func TestBlockBuilder_ToolResultTruncated(t *testing.T) {
	b := newBlockBuilder(7, 4)
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "t", CallID: "c1", Round: 1}, "{}", AuditLevelNone, ToolExecOwnerBackend)
	b.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, CallID: "c1", Round: 1}, "abcdefgh")
	b.Close()
	writes := b.DrainWrites()

	var payload ToolCallBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, 1), &payload); err != nil {
		t.Fatalf("decode tool_calls error = %v", err)
	}
	if len(payload.Calls) != 1 || payload.Calls[0].Result != "abcd..." {
		t.Fatalf("result = %+v, want truncated \"abcd...\"", payload.Calls)
	}
}

// TestBlockBuilder_ApprovalAndQuestionnaire 覆盖授权与问卷建块：tool_request 创建 approval 块（承载
// PendingCall 形状清单且初始无决策），ask_user 挂起创建 questionnaire 块（待回答）；多轮提问各成一块。
func TestBlockBuilder_ApprovalAndQuestionnaire(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddApproval([]PendingCall{
		{CallID: "c1", ToolName: "delete_doc", Arguments: "{}", AuditLevel: AuditLevelMust, ApprovalRequired: true, ExecOwner: ToolExecOwnerClient},
	})
	b.AddQuestionnaire([]QuestionnaireQuestion{{ID: "q1", Question: "选哪个?", Options: []QuestionnaireOption{{Text: "A"}, {Text: "B"}}}})
	b.AddQuestionnaire([]QuestionnaireQuestion{{ID: "q2", Question: "还有呢?", Options: []QuestionnaireOption{{Text: "C"}}}})
	b.Close()
	writes := b.DrainWrites()

	seqs := blockSeqs(writes)
	if len(seqs) != 3 {
		t.Fatalf("block seqs = %v, want 3 blocks", seqs)
	}
	if kindAt(writes, seqs[0]) != ai.AgentMessageBlockKindApproval {
		t.Errorf("kind at seq %d = %s, want approval", seqs[0], kindAt(writes, seqs[0]))
	}
	var approval ApprovalBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, seqs[0]), &approval); err != nil {
		t.Fatalf("decode approval error = %v", err)
	}
	if len(approval.Calls) != 1 || approval.Calls[0].CallID != "c1" || approval.Calls[0].Decision != nil {
		t.Fatalf("approval calls = %+v, want one undecided call", approval.Calls)
	}
	if approval.Calls[0].AuditLevel != AuditLevelMust || approval.Calls[0].ToolName != "delete_doc" {
		t.Errorf("approval call = %+v, want pending call shape preserved", approval.Calls[0])
	}
	var last QuestionnaireBlockPayload
	if err := decodeBlockPayload(finalPayloadAt(t, writes, seqs[2]), &last); err != nil {
		t.Fatalf("decode questionnaire error = %v", err)
	}
	if kindAt(writes, seqs[2]) != ai.AgentMessageBlockKindQuestionnaire || !last.Awaiting || last.Questions[0].ID != "q2" {
		t.Fatalf("questionnaire#2 = %+v, want awaiting round with q2", last)
	}
}

// TestBlockBuilder_ClosedStopsNewBlocks 覆盖收敛后不再新增块。
func TestBlockBuilder_ClosedStopsNewBlocks(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddReasoning("a")
	b.Close()
	if got := b.DrainWrites(); len(got) != 2 {
		t.Fatalf("writes before reopen len = %d, want 2", len(got))
	}
	b.AddReasoning("b")
	b.AddContent("c")
	b.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: "t", CallID: "c1", Round: 1}, "{}", AuditLevelNone, ToolExecOwnerBackend)
	b.AddApproval([]PendingCall{{CallID: "c2"}})
	b.AddQuestionnaire([]QuestionnaireQuestion{{ID: "q1"}})
	if got := b.DrainWrites(); len(got) != 0 {
		t.Fatalf("writes after close len = %d, want 0 (no new blocks after converge)", len(got))
	}
	if b.OpenSeq() != 0 {
		t.Errorf("OpenSeq after close = %d, want 0", b.OpenSeq())
	}
}

// TestBlockBuilder_OpenAccessors 覆盖「当前开放块」访问器随规则变化：思考开放时指向 reasoning 块，
// 切到正文时指向新的 content 块。
func TestBlockBuilder_OpenAccessors(t *testing.T) {
	b := newBlockBuilder(7, 32768)
	b.AddReasoning("a")
	if b.OpenSeq() != 1 || b.OpenKind() != ai.AgentMessageBlockKindReasoning {
		t.Fatalf("open = %d/%s, want 1/reasoning", b.OpenSeq(), b.OpenKind())
	}
	b.AddContent("b")
	if b.OpenSeq() != 2 || b.OpenKind() != ai.AgentMessageBlockKindContent {
		t.Fatalf("open = %d/%s, want 2/content", b.OpenSeq(), b.OpenKind())
	}
}

// TestBlockPayload_EncodeDecode 覆盖统一编解码往返与非法载荷报错。
func TestBlockPayload_EncodeDecode(t *testing.T) {
	raw, err := encodeBlockPayload(&ReasoningBlockPayload{Text: "思考"})
	if err != nil {
		t.Fatalf("encodeBlockPayload error = %v", err)
	}
	if string(raw) != `{"text":"思考"}` {
		t.Errorf("encoded = %s, want {\"text\":\"思考\"}", raw)
	}
	var out ReasoningBlockPayload
	if err := decodeBlockPayload(raw, &out); err != nil {
		t.Fatalf("decodeBlockPayload error = %v", err)
	}
	if out.Text != "思考" {
		t.Errorf("decoded text = %q, want 思考", out.Text)
	}
	if err := decodeBlockPayload([]byte("{"), &out); err == nil {
		t.Error("decodeBlockPayload(invalid) = nil error, want an error")
	}
}

// TestSessionStore_BlockRoundTrip 覆盖块读写的往返：新建 -> 按 block_seq 升序取回 -> 更新载荷回读 ->
// 批量取回 -> 租户归属隔离（越权读写被拒）。
func TestSessionStore_BlockRoundTrip(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID: session.ID, NamespaceName: "default", UserID: testUserID,
		Role: ai.MessageRoleAssistant, Content: encTestValue(""),
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	// 顺序写入两块：先 reasoning，后 content。
	if err := store.AppendBlock(ctx, &ai.AgentMessageBlock{
		NamespaceName: "default", UserID: testUserID, MessageID: msg.ID, BlockSeq: 1,
		Kind: ai.AgentMessageBlockKindReasoning, Payload: encTestValue(`{"text":"思考"}`),
	}); err != nil {
		t.Fatalf("AppendBlock(reasoning) error = %v", err)
	}
	if err := store.AppendBlock(ctx, &ai.AgentMessageBlock{
		NamespaceName: "default", UserID: testUserID, MessageID: msg.ID, BlockSeq: 2,
		Kind: ai.AgentMessageBlockKindContent, Payload: encTestValue(`{"text":"正文"}`),
	}); err != nil {
		t.Fatalf("AppendBlock(content) error = %v", err)
	}

	blocks, err := store.ListBlocksByMessage(ctx, msg.ID, "default", testUserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessage error = %v", err)
	}
	if len(blocks) != 2 || blocks[0].BlockSeq != 1 || blocks[1].BlockSeq != 2 {
		t.Fatalf("blocks = %+v, want ordered [1 2]", blocks)
	}
	if blocks[0].Kind != ai.AgentMessageBlockKindReasoning {
		t.Errorf("first block kind = %s, want reasoning", blocks[0].Kind)
	}

	grouped, err := store.ListBlocksByMessages(ctx, []uint{msg.ID}, "default", testUserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessages error = %v", err)
	}
	if len(grouped[msg.ID]) != 2 {
		t.Fatalf("grouped len = %d, want 2", len(grouped[msg.ID]))
	}

	// 更新同一块（收敛写回）后回读新载荷。
	if err := store.UpdateBlock(ctx, &ai.AgentMessageBlock{
		NamespaceName: "default", UserID: testUserID, MessageID: msg.ID, BlockSeq: 2,
		Payload: encTestValue(`{"text":"正文（终态）"}`),
	}); err != nil {
		t.Fatalf("UpdateBlock error = %v", err)
	}
	blocks, err = store.ListBlocksByMessage(ctx, msg.ID, "default", testUserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessage(after update) error = %v", err)
	}
	var content ContentBlockPayload
	if err := decodeBlockPayload(blocks[1].Payload.Value(), &content); err != nil {
		t.Fatalf("decode updated payload error = %v", err)
	}
	if content.Text != "正文（终态）" {
		t.Errorf("updated content = %q, want 正文（终态）", content.Text)
	}

	// 租户归属隔离：另一用户读不到、改不动。
	if other, err := store.ListBlocksByMessage(ctx, msg.ID, "default", uint(999)); err != nil || len(other) != 0 {
		t.Errorf("ListBlocksByMessage(other user) = %v / %v, want empty", other, err)
	}
	if err := store.UpdateBlock(ctx, &ai.AgentMessageBlock{
		NamespaceName: "default", UserID: uint(999), MessageID: msg.ID, BlockSeq: 2,
		Payload: encTestValue(`{"text":"越权"}`),
	}); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("UpdateBlock(other user) error = %v, want ErrMessageNotFound", err)
	}
}
