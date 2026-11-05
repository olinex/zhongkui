package turns

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

func TestMain(m *testing.M) {
	aitest.InitKeys()
	os.Exit(m.Run())
}

// eventItem 构造一条带游标的事件条目。
func eventItem(seq int) StreamItem {
	event := assistant.Event{
		V:         assistant.Version,
		Seq:       seq,
		Type:      assistant.EventTypeBlockDelta,
		SessionID: 1,
		RunID:     "run-test",
		Payload:   assistant.BlockDeltaPayload{BlockSeq: 1, Kind: assistant.BlockKindContent, Text: strings.Repeat("x", 1)},
	}
	return StreamItem{Event: &event}
}

func hasSeq(items []StreamItem, want int) bool {
	for _, item := range items {
		if item.Event != nil && item.Event.Seq == want {
			return true
		}
	}
	return false
}

func TestTurnStreamReplaySubscribeAndClose(t *testing.T) {
	stream := newTurnStream(2, 1, "run-test")
	stream.Append(eventItem(1))
	stream.Append(eventItem(2))
	stream.Append(eventItem(3))

	all := stream.Items(-1)
	if len(all) != 2 || !hasSeq(all, 2) || !hasSeq(all, 3) {
		t.Fatalf("ring items = %#v", all)
	}

	replay, live, cancel := stream.Subscribe(2)
	if len(replay) != 1 || !hasSeq(replay, 3) {
		t.Fatalf("replay after 2 = %#v", replay)
	}
	defer cancel()

	stream.Append(eventItem(4))
	select {
	case item := <-live:
		if item.Event == nil || item.Event.Seq != 4 {
			t.Fatalf("live item = %#v", item)
		}
	default:
		t.Fatal("live subscriber received nothing")
	}

	stream.Append(eventItem(5))
	stream.Close()
	select {
	case item := <-live:
		if item.Event == nil || item.Event.Seq != 5 {
			t.Fatalf("close item = %#v", item)
		}
	default:
		t.Fatal("close item was not delivered")
	}
	select {
	case _, ok := <-live:
		if ok {
			t.Fatal("subscriber channel should be closed")
		}
	default:
		t.Fatal("subscriber channel should be closed")
	}
}

func TestMapActionTargetsApproval(t *testing.T) {
	pending := []contract.PendingCall{
		{CallID: "call_1", ToolName: "danger", InterruptID: "root-1"},
		{CallID: "call_2", ToolName: "danger", InterruptID: "root-2"},
	}
	targets, err := mapActionTargets(contract.ActionRequest{
		Action: contract.ActionApprovalDecide,
		Decisions: []contract.ApprovalDecision{
			{CallID: "call_1", Approved: true},
			{CallID: "call_2", Approved: false, Reason: "no"},
		},
	}, pending)
	if err != nil {
		t.Fatalf("map approval: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %#v", targets)
	}
	if got := targets["root-1"]; !got.Approved || got.CallID != "call_1" {
		t.Fatalf("root-1 target = %+v", got)
	}
	if got := targets["root-2"]; got.Approved || got.CallID != "call_2" || got.Reason != "no" {
		t.Fatalf("root-2 target = %+v", got)
	}

	byInterrupt, err := mapActionTargets(contract.ActionRequest{
		Action: contract.ActionApprovalDecide,
		Decisions: []contract.ApprovalDecision{
			{InterruptID: "root-2", Approved: true, Result: "ok"},
		},
	}, pending)
	if err != nil {
		t.Fatalf("map approval by interrupt: %v", err)
	}
	if got := byInterrupt["root-2"]; !got.Approved || got.Result != "ok" {
		t.Fatalf("root-2 decided target = %+v", got)
	}

	if _, err := mapActionTargets(contract.ActionRequest{
		Action:    contract.ActionApprovalDecide,
		Decisions: []contract.ApprovalDecision{{CallID: "unknown", Approved: true}},
	}, pending); !errors.Is(err, ErrNoResumeTargets) {
		t.Fatalf("unmapped approval error = %v", err)
	}
}

func TestMapActionTargetsQuestionnaire(t *testing.T) {
	pending := []contract.PendingCall{
		{
			CallID:      "call_q",
			ToolName:    contract.AskUserToolName,
			InterruptID: "root-q",
			Arguments:   `{"questions":[{"id":"q1","question":"one"},{"id":"q2","question":"two"}]}`,
		},
		{CallID: "call_b", ToolName: "danger", InterruptID: "root-b"},
	}
	targets, err := mapActionTargets(contract.ActionRequest{
		Action: contract.ActionQuestionnaireSubmit,
		Answers: []contract.QuestionnaireAnswer{
			{QuestionID: "q1", Answered: true, Answer: "A"},
			{QuestionID: "q2", Answered: false},
		},
	}, pending)
	if err != nil {
		t.Fatalf("map questionnaire: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	target := targets["root-q"]
	if !target.Approved || target.CallID != "call_q" {
		t.Fatalf("questionnaire target = %+v", target)
	}
	var result struct {
		Answers []contract.QuestionnaireAnswer `json:"answers"`
	}
	if err := json.Unmarshal([]byte(target.Result), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Answers) != 1 || result.Answers[0].QuestionID != "q1" || result.Answers[0].Answer != "A" {
		t.Fatalf("answers = %#v", result.Answers)
	}

	empty, err := mapActionTargets(contract.ActionRequest{
		Action:  contract.ActionQuestionnaireSubmit,
		Answers: []contract.QuestionnaireAnswer{{QuestionID: "q1", Answered: false}},
	}, pending)
	if err != nil {
		t.Fatalf("map empty questionnaire: %v", err)
	}
	if got := empty["root-q"]; got.Approved {
		t.Fatalf("empty questionnaire target = %+v", got)
	}
}

func newTestStore(t *testing.T) (*session.Store, *modelsai.AgentSession) {
	t.Helper()
	db := aitest.NewSessionDB(t)
	// sqlite 的 ":memory:" 数据库是每连接独立的；把连接池固定为单个连接，
	// 使后台轮次与测试共享同一 schema。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store, err := session.NewStore(db)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	sess, err := store.CreateSession(context.Background(), "default", "title", modelsai.SlotStandard, modelsai.AgentKindGeneral, 7)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return store, sess
}

func TestManagerChatValidations(t *testing.T) {
	store, sess := newTestStore(t)
	manager, err := New(context.Background(), Deps{Sessions: store, Params: aitest.Params(t, nil)})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	if _, err := manager.Chat(context.Background(), ChatInput{
		SessionID: sess.ID, UserID: sess.UserID, NamespaceName: sess.NamespaceName, Query: "  ",
	}); !errors.Is(err, ErrNoQuery) {
		t.Fatalf("empty query error = %v", err)
	}

	if _, err := manager.Chat(context.Background(), ChatInput{
		SessionID: sess.ID, UserID: sess.UserID + 1, NamespaceName: sess.NamespaceName, Query: "hi",
	}); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("wrong owner error = %v", err)
	}

	_, err = manager.Chat(context.Background(), ChatInput{
		SessionID: sess.ID, UserID: sess.UserID, NamespaceName: sess.NamespaceName, Query: "hi",
	})
	if err == nil || !strings.Contains(err.Error(), "runtime is not configured") {
		t.Fatalf("missing runtime error = %v", err)
	}
}
