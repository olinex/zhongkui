package turns

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

type fakeChatModel struct {
	answer string
}

// Generate 返回固定回答。
func (f *fakeChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: f.answer}, nil
}

// Stream 以单分片流返回固定回答。
func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: f.answer}}), nil
}

func TestChatTurnPersistsAndRenders(t *testing.T) {
	store, sess := newTestStore(t)
	params := aitest.Params(t, map[string]string{
		"session.maxContextMessages": "20",
	})
	rt, err := agentkit.New(context.Background(), agentkit.Deps{
		Models: agentkit.ModelSet{
			Chat:  map[modelsai.Slot]model.BaseModel[*schema.Message]{modelsai.SlotStandard: &fakeChatModel{answer: "integration answer"}},
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

	if _, err := manager.Chat(context.Background(), ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "hello",
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(context.Background(), sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil {
			return false
		}
		if state.Running {
			return false
		}
		rows, err := store.ListNativeMessages(context.Background(), sess.ID, sess.NamespaceName, sess.UserID, 0)
		return err == nil && len(rows) >= 2
	})

	rows, err := store.ListNativeMessages(context.Background(), sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("messages = %d, want 2", len(rows))
	}
	assistantMsg, err := msgops.DecodeForModel[*schema.Message](rows[1].Payload.Value(), llm.APIModeChat)
	if err != nil {
		t.Fatalf("decode assistant: %v", err)
	}
	if msgops.AssistantText(assistantMsg) != "integration answer" {
		t.Fatalf("assistant text = %q", msgops.AssistantText(assistantMsg))
	}
	if rows[1].Phase != modelsai.AgentRunPhaseDone {
		t.Fatalf("assistant phase = %q", rows[1].Phase)
	}

	rendered, err := manager.RenderAssistant(context.Background(), sess.ID, sess.UserID, sess.NamespaceName)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	found := false
	for _, item := range rendered.Replay {
		if item.Event == nil || item.Event.Type != assistant.EventTypeSnapshotMessage {
			continue
		}
		snapshot := item.Event.Payload.(assistant.SnapshotMessagePayload).Message
		if snapshot.Role == "assistant" && strings.Contains(snapshot.Content, "integration answer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("render does not contain the answer")
	}
}

// scriptedToolModel 先调用一个危险的后端工具，然后在工具结果出现后
// 给出回答。
type scriptedToolModel struct {
	mu sync.Mutex
}

func (f *scriptedToolModel) next(msgs []*schema.Message) *schema.Message {
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
func (f *scriptedToolModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.next(msgs), nil
}

// Stream 以单分片流返回脚本结果。
func (f *scriptedToolModel) Stream(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{f.next(msgs)}), nil
}

type dangerTool struct{}

// Info 返回工具描述。
func (dangerTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "danger", Desc: "a dangerous backend operation"}, nil
}

// InvokableRun 执行危险工具并返回固定结果。
func (dangerTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return "danger executed", nil
}

func TestApprovalInterruptAndResume(t *testing.T) {
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

	if _, err := manager.Chat(context.Background(), ChatInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Query:         "do the dangerous thing",
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	var pending State
	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(context.Background(), sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil || !state.Interrupted {
			return false
		}
		pending = state
		return true
	})

	interactions, err := store.ListInteractions(context.Background(), sess.ID, sess.UserID)
	if err != nil || len(interactions) != 1 {
		t.Fatalf("interactions = %v, err = %v", interactions, err)
	}
	if interactions[0].Kind != modelsai.InteractionKindApproval || interactions[0].Status != modelsai.InteractionStatusPending {
		t.Fatalf("interaction = %+v", interactions[0])
	}

	if _, err := manager.Action(context.Background(), ActionInput{
		SessionID:     sess.ID,
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Action: contract.ActionRequest{
			Action:      contract.ActionApprovalDecide,
			InterruptID: pending.PendingInterruptIDs[0],
			Decisions:   []contract.ApprovalDecision{{CallID: "call_1", Approved: true}},
		},
	}); err != nil {
		t.Fatalf("action: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		state, err := manager.State(context.Background(), sess.ID, sess.UserID, sess.NamespaceName)
		if err != nil {
			return false
		}
		if state.Running || state.Interrupted {
			return false
		}
		rows, err := store.ListNativeMessages(context.Background(), sess.ID, sess.NamespaceName, sess.UserID, 0)
		if err != nil || len(rows) < 3 {
			return false
		}
		for _, row := range rows {
			if row.Payload == nil {
				continue
			}
			msg, err := msgops.DecodeForModel[*schema.Message](row.Payload.Value(), llm.APIModeChat)
			if err == nil && msgops.AssistantText(msg) == "approved and done" {
				return true
			}
		}
		return false
	})

	interactions, err = store.ListInteractions(context.Background(), sess.ID, sess.UserID)
	if err != nil {
		t.Fatalf("list interactions: %v", err)
	}
	if len(interactions) != 1 || interactions[0].Status != modelsai.InteractionStatusAnswered {
		t.Fatalf("interaction after resume = %+v", interactions)
	}
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %s", timeout)
}
