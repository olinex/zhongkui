package turns

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// TestReopenApprovalRestoresSuspension 验证「重新审批」回退到审批挂起状态：删掉上次决策之后的
// 续跑产物、把审批重置为待处理、并据挂起时留底的图现场重建现场，使用户可重新选择批准 / 拒绝，
// 而无需重跑整轮回答。首次批准 → 回退 → 再次拒绝，断言两端都能续跑到完成。
func TestReopenApprovalRestoresSuspension(t *testing.T) {
	store, sess := newTestStore(t)
	params := aitest.Params(t, map[string]string{
		"session.maxContextMessages": "20",
	})
	rt, err := agentkit.New(context.Background(), agentkit.Deps{
		Models: agentkit.ModelSet{
			Chat:  map[modelsai.Slot]model.BaseModel[*schema.Message]{modelsai.SlotStandard: &scriptedToolModel{}},
			Kinds: map[modelsai.Slot]llm.APIMode{modelsai.SlotStandard: llm.APIModeChat},
		},
		Params:           params,
		Tools:            []tool.BaseTool{dangerTool{}},
		ToolInfos:        []*schema.ToolInfo{{Name: "danger", Desc: "a dangerous backend operation"}},
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

	ctx := context.Background()
	if _, err := manager.Chat(ctx, ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "do the dangerous thing",
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	var pending State
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil || !state.Interrupted {
			return false
		}
		pending = state
		return true
	})

	interactions, err := store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil || len(interactions) != 1 {
		t.Fatalf("interactions = %v, err = %v", interactions, err)
	}
	var payload contract.ApprovalPayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &payload); err != nil || len(payload.Calls) == 0 {
		t.Fatalf("decode approval payload: %v, calls = %+v", err, payload.Calls)
	}
	callID := payload.Calls[0].CallID

	// 首次批准并等待续跑完成：此时锚点之后已产生工具结果与最终回答。
	decide := func(approved bool, interruptIDs []string) {
		t.Helper()
		if _, err := manager.Action(ctx, ActionInput{
			SessionID:     sess.ID,
			UserID:        sess.UserID,
			NamespaceName: sess.NamespaceName,
			Action: contract.ActionRequest{
				Action:      contract.ActionApprovalDecide,
				InterruptID: interruptIDs[0],
				Decisions:   []contract.ApprovalDecision{{CallID: callID, Approved: approved}},
			},
		}); err != nil {
			t.Fatalf("action approved=%v: %v", approved, err)
		}
	}
	decide(true, pending.PendingInterruptIDs)
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		return err == nil && !state.Running && !state.Interrupted
	})
	answered, err := store.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(answered) < 3 {
		t.Fatalf("messages after resume = %d, want >= 3", len(answered))
	}

	// 重新审批：回退到该次审批挂起的状态。
	if err := manager.ReopenInteraction(ctx, ReopenInteractionInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		CallIDs:       []string{callID},
	}); err != nil {
		t.Fatalf("reopen approval: %v", err)
	}
	rewound, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("state after reopen: %v", err)
	}
	if !rewound.Interrupted || rewound.Running {
		t.Fatalf("state after reopen = %+v, want waiting for approval", rewound)
	}
	rows, err := store.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list messages after reopen: %v", err)
	}
	if len(rows) >= len(answered) {
		t.Fatalf("messages after reopen = %d, want fewer than %d", len(rows), len(answered))
	}
	interactions, err = store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil || len(interactions) != 1 || interactions[0].Status != modelsai.InteractionStatusPending {
		t.Fatalf("interactions after reopen = %+v, err = %v", interactions, err)
	}
	// 回退后审批载荷应清空决策，面板按待处理重新渲染。
	var reopened contract.ApprovalPayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &reopened); err != nil {
		t.Fatalf("decode reopened payload: %v", err)
	}
	if len(reopened.Decisions) != 0 || len(reopened.Calls) == 0 {
		t.Fatalf("reopened payload = %+v, want calls without decisions", reopened)
	}

	// 再次决策（拒绝）应能沿用挂起时的图现场续跑到完成。
	decide(false, rewound.PendingInterruptIDs)
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil || state.Running || state.Interrupted {
			return false
		}
		items, err := store.ListInteractions(ctx, sess.ID, sess.UserID)
		return err == nil && len(items) == 1 && items[0].Status == modelsai.InteractionStatusAnswered
	})
	interactions, err = store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		t.Fatalf("list interactions: %v", err)
	}
	var decided contract.ApprovalPayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &decided); err != nil {
		t.Fatalf("decode decided payload: %v", err)
	}
	if len(decided.Decisions) != 1 || decided.Decisions[0].Approved {
		t.Fatalf("decisions after reopen = %+v, want one rejected decision", decided.Decisions)
	}
}

// scriptedAskUserModel 先发起一轮 ask_user 问卷，拿到作答后续跑并给出回答。
type scriptedAskUserModel struct {
	mu sync.Mutex
}

func (f *scriptedAskUserModel) next(msgs []*schema.Message) *schema.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(msgs) > 0 && msgs[len(msgs)-1].Role == schema.Tool {
		return &schema.Message{Role: schema.Assistant, Content: "answered and done"}
	}
	idx := 0
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "ask_1",
			Index:    &idx,
			Function: schema.FunctionCall{Name: contract.AskUserToolName, Arguments: `{"questions":[{"id":"q1","question":"Which one?","options":[{"text":"A"},{"text":"B"}]}]}`},
		}},
	}
}

// Generate 按脚本返回 ask_user 调用或最终回答。
func (f *scriptedAskUserModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.next(msgs), nil
}

// Stream 以单分片流返回脚本结果。
func (f *scriptedAskUserModel) Stream(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{f.next(msgs)}), nil
}

// TestReopenQuestionnaireRestoresUnanswered 验证「重新回答」同样回退到问卷挂起状态：删除上次作答
// 之后的回答与更晚交互、把问卷重置为未作答、并据挂起时的图现场重建现场，用户可直接重新作答而
// 无需重跑整轮回答。
func TestReopenQuestionnaireRestoresUnanswered(t *testing.T) {
	store, sess := newTestStore(t)
	params := aitest.Params(t, map[string]string{
		"session.maxContextMessages": "20",
	})
	rt, err := agentkit.New(context.Background(), agentkit.Deps{
		Models: agentkit.ModelSet{
			Chat:  map[modelsai.Slot]model.BaseModel[*schema.Message]{modelsai.SlotStandard: &scriptedAskUserModel{}},
			Kinds: map[modelsai.Slot]llm.APIMode{modelsai.SlotStandard: llm.APIModeChat},
		},
		Params:           params,
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

	ctx := context.Background()
	if _, err := manager.Chat(ctx, ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "ask me something",
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	var pending State
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil || !state.Interrupted {
			return false
		}
		pending = state
		return true
	})
	interruptIDs := append([]string(nil), pending.PendingInterruptIDs...)
	interactions, err := store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil || len(interactions) != 1 {
		t.Fatalf("interactions = %v, err = %v", interactions, err)
	}
	if interactions[0].Kind != modelsai.InteractionKindQuestionnaire {
		t.Fatalf("interaction kind = %q, want questionnaire", interactions[0].Kind)
	}
	var payload contract.QuestionnairePayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &payload); err != nil || len(payload.InterruptIDs) == 0 {
		t.Fatalf("decode questionnaire payload: %v, interruptIDs = %v", err, payload.InterruptIDs)
	}

	answer := func(text string) {
		t.Helper()
		if _, err := manager.Action(ctx, ActionInput{
			SessionID:     sess.ID,
			UserID:        sess.UserID,
			NamespaceName: sess.NamespaceName,
			Action: contract.ActionRequest{
				Action:       contract.ActionQuestionnaireSubmit,
				InterruptID:  interruptIDs[0],
				InterruptIDs: interruptIDs,
				Answers:      []contract.QuestionnaireAnswer{{QuestionID: "q1", Answered: true, Answer: text}},
			},
		}); err != nil {
			t.Fatalf("submit answer %q: %v", text, err)
		}
	}
	answer("A")
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		return err == nil && !state.Running && !state.Interrupted
	})
	answeredRows, err := store.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}

	// 回退到未作答之前。
	if err := manager.ReopenInteraction(ctx, ReopenInteractionInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		InterruptIDs:  interruptIDs,
	}); err != nil {
		t.Fatalf("reopen questionnaire: %v", err)
	}
	rewound, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("state after reopen: %v", err)
	}
	if !rewound.Interrupted || rewound.Running {
		t.Fatalf("state after reopen = %+v, want waiting for answers", rewound)
	}
	rows, err := store.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list messages after reopen: %v", err)
	}
	if len(rows) >= len(answeredRows) {
		t.Fatalf("messages after reopen = %d, want fewer than %d", len(rows), len(answeredRows))
	}
	interactions, err = store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil || len(interactions) != 1 || interactions[0].Status != modelsai.InteractionStatusPending {
		t.Fatalf("interactions after reopen = %+v, err = %v", interactions, err)
	}
	var reopened contract.QuestionnairePayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &reopened); err != nil {
		t.Fatalf("decode reopened payload: %v", err)
	}
	if len(reopened.Answers) != 0 || !reopened.Awaiting {
		t.Fatalf("reopened payload = %+v, want unanswered questions", reopened)
	}

	// 重新作答应沿用挂起时的图现场续跑到完成，并回写新答案。
	answer("B")
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(ctx, sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil || state.Running || state.Interrupted {
			return false
		}
		items, err := store.ListInteractions(ctx, sess.ID, sess.UserID)
		return err == nil && len(items) == 1 && items[0].Status == modelsai.InteractionStatusAnswered
	})
	interactions, err = store.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		t.Fatalf("list interactions: %v", err)
	}
	var decided contract.QuestionnairePayload
	if err := json.Unmarshal(interactions[0].Payload.Value(), &decided); err != nil {
		t.Fatalf("decode decided payload: %v", err)
	}
	if len(decided.Answers) != 1 || decided.Answers[0].Answer != "B" {
		t.Fatalf("answers after reopen = %+v, want B", decided.Answers)
	}
}
