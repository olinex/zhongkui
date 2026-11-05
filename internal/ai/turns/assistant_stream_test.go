package turns

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// newEventIterator 用预置事件构建异步迭代器。
func newEventIterator[M adk.MessageType](events ...*adk.TypedAgentEvent[M]) *adk.AsyncIterator[*adk.TypedAgentEvent[M]] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[M]]()
	for _, event := range events {
		generator.Send(event)
	}
	generator.Close()
	return iterator
}

func chatExitEvent() *adk.TypedAgentEvent[*schema.Message] {
	return &adk.TypedAgentEvent[*schema.Message]{Action: &adk.AgentAction{Exit: true}}
}

func chatStreamEvent(chunks ...*schema.Message) *adk.TypedAgentEvent[*schema.Message] {
	return &adk.TypedAgentEvent[*schema.Message]{
		Output: &adk.TypedAgentOutput[*schema.Message]{
			MessageOutput: &adk.TypedMessageVariant[*schema.Message]{
				IsStreaming:   true,
				MessageStream: schema.StreamReaderFromArray(chunks),
				Role:          schema.Assistant,
			},
		},
	}
}

// eventsOf 取出缓冲中的全部事件。
func eventsOf(stream *turnStream) []assistant.Event {
	items := stream.Items(-1)
	out := make([]assistant.Event, 0, len(items))
	for _, item := range items {
		if item.Event != nil {
			out = append(out, *item.Event)
		}
	}
	return out
}

func eventTypes(events []assistant.Event) []assistant.EventType {
	out := make([]assistant.EventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

func eventOf[T any](t *testing.T, events []assistant.Event, eventType assistant.EventType) T {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		payload, ok := event.Payload.(T)
		if !ok {
			t.Fatalf("%s payload 类型 = %T", eventType, event.Payload)
		}
		return payload
	}
	t.Fatalf("缺少事件 %s", eventType)
	var zero T
	return zero
}

// collectStream 读取流中的回放与实时事件直到通道关闭。
func collectStream(t *testing.T, stream *Stream) []assistant.Event {
	t.Helper()
	out := make([]assistant.Event, 0, len(stream.Replay))
	for _, item := range stream.Replay {
		if item.Event != nil {
			out = append(out, *item.Event)
		}
	}
	if stream.Events == nil {
		return out
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case item, ok := <-stream.Events:
			if !ok {
				return out
			}
			if item.Event != nil {
				out = append(out, *item.Event)
			}
		case <-deadline:
			t.Fatalf("等待流关闭超时，已收到 %d 条事件", len(out))
		}
	}
}

func TestActionRejectsChatSend(t *testing.T) {
	store, sess := newTestStore(t)
	manager, err := New(context.Background(), Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	_, err = manager.Action(context.Background(), ActionInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Action:        contract.ActionRequest{Action: contract.ActionChatSend, Text: "next step"},
	})
	if !errors.Is(err, contract.ErrInvalidAction) {
		t.Fatalf("chat.send error = %v", err)
	}
}

func TestBuildResumeUpdates(t *testing.T) {
	approval := resumeBlock{
		Substate: assistant.InteractionSubstateApproval,
		Seq:      2,
		Kind:     assistant.BlockKindApproval,
		Payload: assistant.ApprovalBlockPayload{
			Calls:   []assistant.ApprovalCallRecord{{CallID: "call_1", ToolName: "danger"}},
			CallIDs: []string{"call_1"},
		},
	}
	questionnaire := resumeBlock{
		Substate: assistant.InteractionSubstateAnswer,
		Seq:      4,
		Kind:     assistant.BlockKindQuestionnaire,
		Payload: assistant.QuestionnaireBlockPayload{
			Questions:    []contract.QuestionnaireQuestion{{ID: "q1", Question: "which?"}},
			Awaiting:     true,
			InterruptIDs: []string{"root-q"},
		},
	}
	updates := buildResumeUpdates([]resumeBlock{approval, questionnaire}, contract.ActionRequest{
		Action:    contract.ActionApprovalDecide,
		Decisions: []contract.ApprovalDecision{{CallID: "call_1", Approved: false, Error: "denied"}},
		Answers:   []contract.QuestionnaireAnswer{{QuestionID: "q1", Answered: true, Answer: "A"}},
	})
	if len(updates) != 2 {
		t.Fatalf("updates = %+v", updates)
	}
	approvalPayload := updates[0].Payload.(assistant.ApprovalBlockPayload)
	if updates[0].Seq != 2 || approvalPayload.Calls[0].Decision == nil || *approvalPayload.Calls[0].Decision {
		t.Fatalf("approval update = %+v", approvalPayload)
	}
	if approvalPayload.Calls[0].Error != "denied" {
		t.Fatalf("approval error = %+v", approvalPayload.Calls[0])
	}
	// 原位更新不得修改挂起轮次保存的块载荷。
	if approval.Payload.(assistant.ApprovalBlockPayload).Calls[0].Decision != nil {
		t.Fatal("挂起块载荷被就地修改")
	}

	questionnairePayload := updates[1].Payload.(assistant.QuestionnaireBlockPayload)
	if updates[1].Seq != 4 || questionnairePayload.Awaiting {
		t.Fatalf("questionnaire update = %+v", questionnairePayload)
	}
	if len(questionnairePayload.Answers) != 1 || questionnairePayload.Answers[0].Answer != "A" {
		t.Fatalf("questionnaire answers = %+v", questionnairePayload.Answers)
	}

	previous := newTurnStream(8, 7, "run-1")
	previous.NoteBlockSeq(4)
	previous.NoteInteraction(assistant.InteractionSubstateApproval, approval)
	previous.NoteInteraction(assistant.InteractionSubstateAnswer, questionnaire)
	next := newTurnStream(8, 7, "run-2")
	next.ResumeFrom(previous, updates, []string{"root-a", "root-q"})
	if next.ProjectionSeed().BlockSeq != 4 {
		t.Fatalf("续跑块序号 = %d", next.ProjectionSeed().BlockSeq)
	}
	// 已消费的挂起块不再保留，避免下一次续跑重复下发。
	if pending := next.PendingBlocks(); len(pending) != 0 {
		t.Fatalf("残留挂起块 = %+v", pending)
	}
	if seed := next.ProjectionSeed(); len(seed.Updates) != 2 || len(seed.Interrupts) != 2 {
		t.Fatalf("续跑种子 = %+v", seed)
	}
}

func TestAssistantProjectorEventOrder(t *testing.T) {
	stream := newTurnStream(64, 7, "run-1")
	events := newEventIterator(
		chatStreamEvent(
			&schema.Message{Role: schema.Assistant, ReasoningContent: "think"},
			&schema.Message{Role: schema.Assistant, Content: "Hel"},
			&schema.Message{Role: schema.Assistant, Content: "lo"},
		),
		chatExitEvent(),
	)

	result, err := streamAssistantEvents[*schema.Message](stream, events, assistant.TurnStartedPayload{RunID: "run-1"})
	if err != nil {
		t.Fatalf("投影失败: %v", err)
	}
	if result.LastContent != "Hello" {
		t.Fatalf("LastContent = %q", result.LastContent)
	}

	collected := eventsOf(stream)
	want := []assistant.EventType{
		assistant.EventTypeTurnStarted,
		assistant.EventTypeMessageStarted,
		assistant.EventTypeBlockCreated,
		assistant.EventTypeBlockUpdated,
		assistant.EventTypeBlockCreated,
		assistant.EventTypeBlockDelta,
		assistant.EventTypeBlockUpdated,
	}
	if got := eventTypes(collected); len(got) != len(want) {
		t.Fatalf("事件 = %v", got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("第 %d 个事件 = %s，want %s", i, got[i], want[i])
			}
		}
	}

	reasoning := collected[2].Payload.(assistant.BlockCreatedPayload)
	if reasoning.BlockSeq != 1 || reasoning.Kind != assistant.BlockKindReasoning {
		t.Fatalf("reasoning created = %+v", reasoning)
	}
	sealed := collected[3].Payload.(assistant.BlockUpdatedPayload)
	if sealed.BlockSeq != 1 || sealed.Payload.(assistant.ReasoningBlockPayload).Text != "think" {
		t.Fatalf("reasoning updated = %+v", sealed)
	}
	content := collected[4].Payload.(assistant.BlockCreatedPayload)
	if content.BlockSeq != 2 || content.Kind != assistant.BlockKindContent {
		t.Fatalf("content created = %+v", content)
	}
	delta := collected[5].Payload.(assistant.BlockDeltaPayload)
	if delta.BlockSeq != 2 || delta.Text != "lo" {
		t.Fatalf("content delta = %+v", delta)
	}
	final := collected[6].Payload.(assistant.BlockUpdatedPayload)
	if final.Payload.(assistant.ContentBlockPayload).Text != "Hello" {
		t.Fatalf("content updated = %+v", final)
	}
}

// fakeEventChatModel 是一次性回答的对话模型。
type fakeEventChatModel struct {
	answer string
}

// Generate 返回固定回答。
func (f *fakeEventChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: f.answer}, nil
}

// Stream 以单分片流返回固定回答。
func (f *fakeEventChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: f.answer}}), nil
}

func newEventManager(t *testing.T, chat model.BaseModel[*schema.Message], tools []tool.BaseTool) (*Manager, *modelsai.AgentSession) {
	t.Helper()
	store, sess := newTestStore(t)
	params := aitest.Params(t, map[string]string{"session.maxContextMessages": "20"})
	infos := []*schema.ToolInfo(nil)
	if len(tools) > 0 {
		infos = []*schema.ToolInfo{{Name: "danger", Desc: "a dangerous backend operation"}}
	}
	rt, err := agentkit.New(context.Background(), agentkit.Deps{
		Models: agentkit.ModelSet{
			Chat:  map[modelsai.Slot]model.BaseModel[*schema.Message]{modelsai.SlotStandard: chat},
			Kinds: map[modelsai.Slot]llm.APIMode{modelsai.SlotStandard: llm.APIModeChat},
		},
		Params:           params,
		Tools:            tools,
		ToolInfos:        infos,
		Checkpoints:      agentkit.NewMemoryCheckPointStore(),
		SlotContextLimit: func(modelsai.Slot) int { return 100000 },
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	manager, err := New(context.Background(), Deps{
		Agents:      func() *agentkit.Runtime { return rt },
		Sessions:    store,
		Params:      params,
		Checkpoints: rt.Checkpoints(),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	return manager, sess
}

func TestChatTurnEvents(t *testing.T) {
	manager, sess := newEventManager(t, &fakeEventChatModel{answer: "event answer"}, nil)
	stream, err := manager.Chat(context.Background(), ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "hello",
		SkillID:       9,
		SkillName:     "periodic_report_cocreate",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	collected := collectStream(t, stream)
	types := eventTypes(collected)
	for _, want := range []assistant.EventType{
		assistant.EventTypeTurnStarted,
		assistant.EventTypeMessageStarted,
		assistant.EventTypeBlockCreated,
		assistant.EventTypeBlockUpdated,
		assistant.EventTypeMessagePersisted,
		assistant.EventTypeTurnCompleted,
	} {
		found := false
		for _, got := range types {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺少事件 %s：%v", want, types)
		}
	}

	started := eventOf[assistant.TurnStartedPayload](t, collected, assistant.EventTypeTurnStarted)
	if started.Resumed || started.UserMessage == nil {
		t.Fatalf("turn.started = %+v", started)
	}
	if started.UserMessage.MessageID == 0 || started.UserMessage.UserIndex != 1 || started.UserMessage.Content != "hello" {
		t.Fatalf("userMessage = %+v", started.UserMessage)
	}
	if started.UserMessage.SkillID != 9 || started.UserMessage.SkillName != "periodic_report_cocreate" {
		t.Fatalf("userMessage 技能 = (%d, %q)", started.UserMessage.SkillID, started.UserMessage.SkillName)
	}
	completed := eventOf[assistant.TurnCompletedPayload](t, collected, assistant.EventTypeTurnCompleted)
	if completed.Phase != modelsai.AgentRunPhaseDone || completed.MessageID == 0 || completed.Usage == nil {
		t.Fatalf("turn.completed = %+v", completed)
	}
	if completed.MessageID != eventOf[assistant.MessagePersistedPayload](t, collected, assistant.EventTypeMessagePersisted).MessageID {
		t.Fatalf("turn.completed messageID 与 message.persisted 不一致")
	}
	// 技能随用户消息落库：刷新后历史快照据该列回放本轮技能。
	rows, err := manager.deps.Sessions.ListNativeMessages(context.Background(), sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list native messages: %v", err)
	}
	var userRow *modelsai.AgentMessage
	for _, row := range rows {
		if row.Role == modelsai.MessageRoleUser {
			userRow = row
			break
		}
	}
	if userRow == nil || modelsai.SkillIDFromColumn(userRow.SkillID) != 9 || userRow.SkillName != "periodic_report_cocreate" {
		t.Fatalf("用户消息落库技能 = %+v", userRow)
	}
}

func TestEventsResumeCursor(t *testing.T) {
	store, sess := newTestStore(t)
	manager, err := New(context.Background(), Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	stream := newTurnStream(8, sess.ID, "run-1")
	for i := 0; i < 3; i++ {
		stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.BlockDelta(1, assistant.BlockKindContent, "x")
		})
	}
	state := manager.stateFor(sess.ID)
	state.beginTurn(stream, modelsai.SlotStandard)

	ctx := context.Background()
	resumed, err := manager.Events(ctx, sess.ID, sess.UserID, sess.NamespaceName, 1)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if resumed == nil || resumed.Resume == nil || resumed.Resume.Reset != "" {
		t.Fatalf("resume = %+v", resumed)
	}
	if len(resumed.Replay) != 2 {
		t.Fatalf("replay = %d, want 2", len(resumed.Replay))
	}
	if resumed.Replay[0].Event == nil || resumed.Replay[0].Event.Seq != 2 {
		t.Fatalf("第一个重放事件 = %+v", resumed.Replay[0])
	}

	all, err := manager.Events(ctx, sess.ID, sess.UserID, sess.NamespaceName, 0)
	if err != nil || all == nil || len(all.Replay) != 3 {
		t.Fatalf("after=0 replay = %+v, err = %v", all, err)
	}

	gap, err := manager.Events(ctx, sess.ID, sess.UserID, sess.NamespaceName, 9)
	if err != nil {
		t.Fatalf("gap events: %v", err)
	}
	if gap == nil || gap.Resume == nil || gap.Resume.Reset != "gap" {
		t.Fatalf("越界游标 = %+v", gap)
	}

	stream.Close()
	if closed, err := manager.Events(ctx, sess.ID, sess.UserID, sess.NamespaceName, 0); err != nil || closed != nil {
		t.Fatalf("收敛后事件 = %+v, err = %v", closed, err)
	}
}

func TestClosePreemptedAndNotice(t *testing.T) {
	t.Run("preempted", func(t *testing.T) {
		stream := newTurnStream(8, 7, "run-1")
		stream.ClosePreempted("run-2")
		collected := eventsOf(stream)
		if len(collected) != 1 || collected[0].Type != assistant.EventTypeTurnPreempted {
			t.Fatalf("events = %v", eventTypes(collected))
		}
		payload := collected[0].Payload.(assistant.TurnPreemptedPayload)
		if payload.ByRunID != "run-2" {
			t.Fatalf("preempted = %+v", payload)
		}
		if !stream.Closed() {
			t.Fatal("被抢占的流应关闭")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		stream := newTurnStream(8, 7, "run-1")
		stream.CloseNotice("cancelled", "the turn was aborted")
		collected := eventsOf(stream)
		if len(collected) != 1 || collected[0].Type != assistant.EventTypeTurnCompleted {
			t.Fatalf("events = %v", eventTypes(collected))
		}
		payload := collected[0].Payload.(assistant.TurnCompletedPayload)
		if payload.Phase != modelsai.AgentRunPhaseCancelled {
			t.Fatalf("completed = %+v", payload)
		}
	})

	t.Run("error", func(t *testing.T) {
		stream := newTurnStream(8, 7, "run-1")
		stream.CloseNotice("error", "boom")
		collected := eventsOf(stream)
		if len(collected) != 2 ||
			collected[0].Type != assistant.EventTypeTurnError ||
			collected[1].Type != assistant.EventTypeTurnCompleted {
			t.Fatalf("events = %v", eventTypes(collected))
		}
		if collected[1].Payload.(assistant.TurnCompletedPayload).Phase != modelsai.AgentRunPhaseFailed {
			t.Fatalf("completed = %+v", collected[1].Payload)
		}
	})
}

// scriptedApprovalToolModel 先调用一个危险的后端工具，然后在工具结果出现后给出回答。
type scriptedApprovalToolModel struct {
	mu sync.Mutex
}

func (f *scriptedApprovalToolModel) next(msgs []*schema.Message) *schema.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(msgs) > 0 && msgs[len(msgs)-1].Role == schema.Tool {
		return &schema.Message{Role: schema.Assistant, Content: "approved and done"}
	}
	idx := 0
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Index:    &idx,
			Function: schema.FunctionCall{Name: "danger", Arguments: `{}`},
		}},
	}
}

// Generate 按脚本返回工具调用或最终回答。
func (f *scriptedApprovalToolModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.next(msgs), nil
}

// Stream 以单分片流返回脚本结果。
func (f *scriptedApprovalToolModel) Stream(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{f.next(msgs)}), nil
}

func TestApprovalInterruptAndResumeEvents(t *testing.T) {
	manager, sess := newEventManager(t, &scriptedApprovalToolModel{}, []tool.BaseTool{dangerTool{}})

	stream, err := manager.Chat(context.Background(), ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "do the dangerous thing",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	suspended := collectStream(t, stream)

	waiting := eventOf[assistant.InteractionWaitingPayload](t, suspended, assistant.EventTypeInteractionWaiting)
	if waiting.Substate != assistant.InteractionSubstateApproval || len(waiting.InterruptIDs) == 0 {
		t.Fatalf("interaction.waiting = %+v", waiting)
	}
	approval := eventOf[assistant.BlockCreatedPayload](t, suspended, assistant.EventTypeBlockCreated)
	if approval.Kind != assistant.BlockKindToolCalls {
		t.Fatalf("首个块 = %+v", approval)
	}
	var approvalPayload assistant.ApprovalBlockPayload
	approvalSeq := 0
	for _, event := range suspended {
		if event.Type != assistant.EventTypeBlockCreated {
			continue
		}
		payload := event.Payload.(assistant.BlockCreatedPayload)
		if payload.Kind != assistant.BlockKindApproval {
			continue
		}
		approvalPayload = payload.Payload.(assistant.ApprovalBlockPayload)
		approvalSeq = payload.BlockSeq
	}
	if approvalSeq == 0 || len(approvalPayload.Calls) != 1 || approvalPayload.Calls[0].CallID != "call_1" {
		t.Fatalf("approval block = %+v", approvalPayload)
	}
	if waiting.BlockSeq != approvalSeq {
		t.Fatalf("waiting.blockSeq = %d, want %d", waiting.BlockSeq, approvalSeq)
	}
	for _, event := range suspended {
		if event.Type == assistant.EventTypeTurnCompleted {
			t.Fatal("挂起回合不应下发 turn.completed")
		}
	}

	state, err := manager.State(context.Background(), sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil || !state.Interrupted {
		t.Fatalf("state = %+v, err = %v", state, err)
	}

	resumedStream, err := manager.Action(context.Background(), ActionInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Action: contract.ActionRequest{
			Action:      contract.ActionApprovalDecide,
			InterruptID: waiting.InterruptIDs[0],
			Decisions:   []contract.ApprovalDecision{{CallID: "call_1", Approved: true}},
		},
	})
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	resumed := collectStream(t, resumedStream)

	started := eventOf[assistant.TurnStartedPayload](t, resumed, assistant.EventTypeTurnStarted)
	if !started.Resumed || started.AssistantMessageID == 0 {
		t.Fatalf("turn.started = %+v", started)
	}
	resumedIDs := eventOf[assistant.InteractionResumedPayload](t, resumed, assistant.EventTypeInteractionResumed)
	if len(resumedIDs.InterruptIDs) != 1 || resumedIDs.InterruptIDs[0] != waiting.InterruptIDs[0] {
		t.Fatalf("interaction.resumed = %+v", resumedIDs)
	}

	updated := eventOf[assistant.BlockUpdatedPayload](t, resumed, assistant.EventTypeBlockUpdated)
	if updated.BlockSeq != approvalSeq || updated.Kind != assistant.BlockKindApproval {
		t.Fatalf("approval updated = %+v", updated)
	}
	decided := updated.Payload.(assistant.ApprovalBlockPayload)
	if len(decided.Calls) != 1 || decided.Calls[0].Decision == nil || !*decided.Calls[0].Decision {
		t.Fatalf("approval decision = %+v", decided.Calls)
	}

	toolUpdated := eventOf[assistant.ToolCallUpdatedPayload](t, resumed, assistant.EventTypeToolCallUpdated)
	if toolUpdated.Call.Status != assistant.ToolCallStatusDone || toolUpdated.BlockSeq == 0 {
		t.Fatalf("tool.call.updated = %+v", toolUpdated)
	}

	completed := eventOf[assistant.TurnCompletedPayload](t, resumed, assistant.EventTypeTurnCompleted)
	if completed.Phase != modelsai.AgentRunPhaseDone {
		t.Fatalf("turn.completed = %+v", completed)
	}
}
