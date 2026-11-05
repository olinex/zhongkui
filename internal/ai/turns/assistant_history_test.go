package turns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// appendNative 追加一条原生消息并返回落库行。
func appendNative(t *testing.T, store *session.Store, row *modelsai.AgentMessage) *modelsai.AgentMessage {
	t.Helper()
	out, err := store.AppendNativeMessage(context.Background(), row)
	if err != nil {
		t.Fatalf("append native message: %v", err)
	}
	return out
}

// nativeRow 构造一条原生消息行。
func nativeRow(sess *modelsai.AgentSession, role modelsai.MessageRole, msg any) *modelsai.AgentMessage {
	payload, _ := json.Marshal(msg)
	return &modelsai.AgentMessage{
		SessionID:     sess.ID,
		NamespaceName: sess.NamespaceName,
		UserID:        sess.UserID,
		Role:          role,
		MessageKind:   llm.APIModeChat,
		Payload:       serializer.NewCrypto(payload),
	}
}

func TestRenderAssistantSnapshots(t *testing.T) {
	store, sess := newTestStore(t)
	ctx := context.Background()

	userRow := nativeRow(sess, modelsai.MessageRoleUser, msgops.NewUser[*schema.Message]("what is zhongkui"))
	skillID := uint(7)
	userRow.SkillID = &skillID
	userRow.SkillName = "periodic_report_cocreate"
	appendNative(t, store, userRow)

	answer := &schema.Message{Role: schema.Assistant, ReasoningContent: "think", Content: "hello answer"}
	answerRow := appendNative(t, store, nativeRow(sess, modelsai.MessageRoleAssistant, answer))
	answerRow.PromptTokens = 10
	answerRow.CompletionTokens = 5
	answerRow.TotalTokens = 15
	answerRow.ElapsedMs = 120
	answerRow.Phase = modelsai.AgentRunPhaseDone
	citationsJSON, _ := json.Marshal([]retrieval.RetrievalResult{{DocumentID: "doc-1", Title: "Doc", NamespaceName: "default"}})
	answerRow.CitationsJSON = serializer.NewCrypto(citationsJSON)
	if err := store.UpdateNativeMessage(ctx, answerRow); err != nil {
		t.Fatalf("update answer usage: %v", err)
	}
	if err := store.UpdateSummaryAndCheckpoint(ctx, sess, "summary", uint(answerRow.Seq), "rolling", contract.CompactionMetrics{
		TokensBefore: 100,
		FoldedCount:  2,
		TokensAfter:  50,
	}); err != nil {
		t.Fatalf("save compaction: %v", err)
	}

	idx := 0
	toolCall := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Index:    &idx,
			Function: schema.FunctionCall{Name: "danger", Arguments: `{}`},
		}},
	}
	callRow := appendNative(t, store, nativeRow(sess, modelsai.MessageRoleAssistant, toolCall))
	resultRow := appendNative(t, store, nativeRow(sess, modelsai.MessageRole("tool"), &schema.Message{
		Role:       schema.Tool,
		ToolCallID: "call_1",
		ToolName:   "danger",
		Content:    "danger executed",
	}))

	approvalPayload, _ := json.Marshal(contract.ApprovalPayload{
		Calls:     []contract.PendingCall{{CallID: "call_1", ToolName: "danger", InterruptID: "root-1", ApprovalRequired: true}},
		Decisions: []contract.ApprovalDecision{{CallID: "call_1", Approved: true, Result: "ok"}},
	})
	if _, err := store.AppendInteraction(ctx, &modelsai.AgentInteraction{
		SessionID: sess.ID, NamespaceName: sess.NamespaceName, UserID: sess.UserID,
		MessageID: callRow.ID, Kind: modelsai.InteractionKindApproval,
		Status: modelsai.InteractionStatusAnswered, Payload: serializer.NewCrypto(approvalPayload),
	}); err != nil {
		t.Fatalf("append approval: %v", err)
	}

	questionnairePayload, _ := json.Marshal(contract.QuestionnairePayload{
		Questions:    []contract.QuestionnaireQuestion{{ID: "q1", Question: "Which one?"}},
		Awaiting:     true,
		InterruptIDs: []string{"root-q"},
	})
	if _, err := store.AppendInteraction(ctx, &modelsai.AgentInteraction{
		SessionID: sess.ID, NamespaceName: sess.NamespaceName, UserID: sess.UserID,
		MessageID: resultRow.ID, Kind: modelsai.InteractionKindQuestionnaire,
		Status: modelsai.InteractionStatusPending, Payload: serializer.NewCrypto(questionnairePayload),
	}); err != nil {
		t.Fatalf("append questionnaire: %v", err)
	}
	pending, _ := json.Marshal([]contract.PendingCall{{
		CallID:      "ask_1",
		ToolName:    contract.AskUserToolName,
		InterruptID: "root-q",
		ExecOwner:   contract.ToolExecOwnerClient,
	}})
	if err := store.SaveAgentRun(ctx, &modelsai.AgentRun{
		NamespaceName: sess.NamespaceName, UserID: sess.UserID, SessionID: sess.ID,
		Status: modelsai.AgentRunStatusPending, Slot: modelsai.SlotStandard,
		PendingCallsJSON: serializer.NewCrypto(pending),
		CheckpointID:     "cp-1",
		AnchorMessageID:  resultRow.ID,
	}); err != nil {
		t.Fatalf("save run: %v", err)
	}

	manager, err := New(ctx, Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	stream, err := manager.RenderAssistant(ctx, sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("render assistant: %v", err)
	}
	events := make([]assistant.Event, 0, len(stream.Replay))
	for _, item := range stream.Replay {
		if item.Event != nil {
			events = append(events, *item.Event)
		}
	}
	if len(events) != 4 {
		t.Fatalf("事件数 = %d, types = %v", len(events), eventTypes(events))
	}
	if events[0].Type != assistant.EventTypeSnapshotBegin || events[len(events)-1].Type != assistant.EventTypeSnapshotEnd {
		t.Fatalf("快照边界 = %v", eventTypes(events))
	}

	var messages []assistant.MessageSnapshot
	for _, event := range events {
		if event.Type != assistant.EventTypeSnapshotMessage {
			continue
		}
		messages = append(messages, event.Payload.(assistant.SnapshotMessagePayload).Message)
	}
	if len(messages) != 2 {
		t.Fatalf("消息数 = %d", len(messages))
	}
	user, answerSnapshot := messages[0], messages[1]
	if user.Role != "user" || user.UserMessageIndex != 1 || user.Content != "what is zhongkui" {
		t.Fatalf("用户消息 = %+v", user)
	}
	if user.SkillID != 7 || user.SkillName != "periodic_report_cocreate" {
		t.Fatalf("用户消息技能 = (%d, %q)", user.SkillID, user.SkillName)
	}
	if answerSnapshot.Role != "assistant" {
		t.Fatalf("助手消息 = %+v", answerSnapshot)
	}
	if answerSnapshot.ID != resultRow.ID {
		t.Fatalf("助手消息 ID = %d, want %d", answerSnapshot.ID, resultRow.ID)
	}
	if answerSnapshot.PromptTokens != 10 || answerSnapshot.CompletionTokens != 5 || answerSnapshot.TotalTokens != 15 {
		t.Fatalf("用量 = %+v", answerSnapshot)
	}
	if answerSnapshot.Phase != modelsai.AgentRunPhaseDone {
		t.Fatalf("phase = %q", answerSnapshot.Phase)
	}
	if answerSnapshot.Content != "hello answer" {
		t.Fatalf("content = %q", answerSnapshot.Content)
	}
	if len(answerSnapshot.Citations) != 1 || answerSnapshot.Citations[0].DocumentID != "doc-1" {
		t.Fatalf("citations = %+v", answerSnapshot.Citations)
	}
	if answerSnapshot.Compress == nil ||
		answerSnapshot.Compress.Phase != assistant.CompressPhaseDone ||
		answerSnapshot.Compress.TokensBefore != 100 ||
		answerSnapshot.Compress.FoldedCount != 2 ||
		answerSnapshot.Compress.TokensAfter != 50 ||
		answerSnapshot.Compress.Summary != "summary" {
		t.Fatalf("compress = %+v", answerSnapshot.Compress)
	}

	kinds := make([]assistant.BlockKind, 0, len(answerSnapshot.Blocks))
	for _, block := range answerSnapshot.Blocks {
		kinds = append(kinds, block.Kind)
		if block.Seq == 0 {
			t.Fatalf("块缺少 seq: %+v", block)
		}
	}
	want := []assistant.BlockKind{
		assistant.BlockKindReasoning,
		assistant.BlockKindContent,
		assistant.BlockKindToolCalls,
		assistant.BlockKindApproval,
		assistant.BlockKindQuestionnaire,
	}
	if len(kinds) != len(want) {
		t.Fatalf("块序列 = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("第 %d 块 = %s, want %s", i, kinds[i], want[i])
		}
	}
	toolBlock := answerSnapshot.Blocks[2].Payload.(assistant.ToolCallsBlockPayload)
	if len(toolBlock.Calls) != 1 || toolBlock.Calls[0].Status != assistant.ToolCallStatusDone || toolBlock.Calls[0].Result != "danger executed" {
		t.Fatalf("tool_calls 块 = %+v", toolBlock)
	}
	approvalBlock := answerSnapshot.Blocks[3].Payload.(assistant.ApprovalBlockPayload)
	if len(approvalBlock.Calls) != 1 || approvalBlock.Calls[0].Decision == nil || !*approvalBlock.Calls[0].Decision {
		t.Fatalf("approval 块 = %+v", approvalBlock)
	}
	questionnaireBlock := answerSnapshot.Blocks[4].Payload.(assistant.QuestionnaireBlockPayload)
	if !questionnaireBlock.Awaiting || len(questionnaireBlock.Questions) != 1 {
		t.Fatalf("questionnaire 块 = %+v", questionnaireBlock)
	}
	if len(answerSnapshot.PendingCalls) != 1 || answerSnapshot.PendingCalls[0].CallID != "ask_1" ||
		answerSnapshot.PendingCalls[0].InterruptID != "root-q" {
		t.Fatalf("pendingCalls = %+v", answerSnapshot.PendingCalls)
	}

	// 快照事件不携带块创建标记（历史块是终态）。
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	if strings.Contains(string(raw), `"created"`) {
		t.Fatalf("快照不应携带 created: %s", raw)
	}
}

// TestRenderAssistantUsageUsesLastRun 验证挂起 + 续跑的多条助手行只保留续跑终态的用量，
// 避免历史 token 数比实时 turn.completed 翻倍。
func TestRenderAssistantUsageUsesLastRun(t *testing.T) {
	store, sess := newTestStore(t)
	ctx := context.Background()
	appendNative(t, store, nativeRow(sess, modelsai.MessageRoleUser, msgops.NewUser[*schema.Message]("query")))

	suspended := appendNative(t, store, nativeRow(sess, modelsai.MessageRoleAssistant, &schema.Message{
		Role:    schema.Assistant,
		Content: "partial",
	}))
	suspended.PromptTokens = 20
	suspended.CompletionTokens = 5
	suspended.TotalTokens = 25
	suspended.Phase = modelsai.AgentRunPhaseSuspended
	if err := store.UpdateNativeMessage(ctx, suspended); err != nil {
		t.Fatalf("update suspended usage: %v", err)
	}

	done := appendNative(t, store, nativeRow(sess, modelsai.MessageRoleAssistant, &schema.Message{
		Role:    schema.Assistant,
		Content: "final",
	}))
	done.PromptTokens = 10
	done.CompletionTokens = 5
	done.TotalTokens = 15
	done.Phase = modelsai.AgentRunPhaseDone
	if err := store.UpdateNativeMessage(ctx, done); err != nil {
		t.Fatalf("update done usage: %v", err)
	}

	manager, err := New(ctx, Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	stream, err := manager.RenderAssistant(ctx, sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("render assistant: %v", err)
	}
	var assistantSnapshot assistant.MessageSnapshot
	for _, item := range stream.Replay {
		if item.Event == nil || item.Event.Type != assistant.EventTypeSnapshotMessage {
			continue
		}
		message := item.Event.Payload.(assistant.SnapshotMessagePayload).Message
		if message.Role == "assistant" {
			assistantSnapshot = message
		}
	}
	if assistantSnapshot.TotalTokens != 15 || assistantSnapshot.PromptTokens != 10 || assistantSnapshot.CompletionTokens != 5 {
		t.Fatalf("用量 = %+v，want 10/5/15（续跑终态）", assistantSnapshot)
	}
	if assistantSnapshot.ID != done.ID {
		t.Fatalf("消息 ID = %d, want %d", assistantSnapshot.ID, done.ID)
	}
	if assistantSnapshot.Content != "partial\n\nfinal" {
		t.Fatalf("content = %q", assistantSnapshot.Content)
	}
}

// TestBuildAssistantSnapshotsDropsInteractionsWithMissingAnchor 验证锚点消息已删除的交互不再回放：
// 回退截断后会话为空时，残留的「推荐的下一步」交互不得合成一条空助手回答。
func TestBuildAssistantSnapshotsDropsInteractionsWithMissingAnchor(t *testing.T) {
	interactions := []contract.Interaction{{
		MessageID: 404,
		Kind:      contract.InteractionNextSteps,
		Status:    contract.InteractionAnswered,
		Payload:   contract.NextStepsPayload{Steps: []contract.NextStep{{Title: "下一步", Text: "继续查询"}}},
	}}
	snapshots := buildAssistantSnapshots[*schema.Message](nil, interactions, nil, nil)
	if len(snapshots) != 0 {
		t.Fatalf("快照数 = %d, want 0（锚点缺失的交互应跳过）", len(snapshots))
	}
}

func TestRenderAssistantAppendsActiveTurnEvents(t *testing.T) {
	store, sess := newTestStore(t)
	ctx := context.Background()
	appendNative(t, store, nativeRow(sess, modelsai.MessageRoleUser, msgops.NewUser[*schema.Message]("hi")))

	manager, err := New(ctx, Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	live := newTurnStream(8, sess.ID, "run-live")
	live.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.BlockCreated(1, assistant.BlockKindContent, assistant.ContentBlockPayload{Text: "streaming"})
	})
	state := manager.stateFor(sess.ID)
	state.beginTurn(live, modelsai.SlotStandard)

	stream, err := manager.RenderAssistant(ctx, sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("render assistant: %v", err)
	}
	types := make([]assistant.EventType, 0, len(stream.Replay))
	for _, item := range stream.Replay {
		if item.Event != nil {
			types = append(types, item.Event.Type)
		}
	}
	if len(types) < 4 || types[len(types)-1] != assistant.EventTypeBlockCreated {
		t.Fatalf("事件序列 = %v", types)
	}
}
