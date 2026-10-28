package session

import (
	// 标准库
	"bytes"
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// TestNativeStore_AppendAndListMessages 测试原生消息追加与读取：seq 按会话计数原子分配、MessageKind
// 缺省 chat、payload 密文落库且回读可解密还原、最近 maxCount 条按 seq 升序返回、跨用户隔离。
func TestNativeStore_AppendAndListMessages(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "native", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	userPayload := []byte(`{"role":"user","content_blocks":[{"user_input":{"text":"hello-native"}}]}`)
	first, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleUser,
		MessageKind:   llm.APIModeResponses,
		Payload:       serializer.NewCrypto(userPayload),
	})
	if err != nil {
		t.Fatalf("AppendNativeMessage(user) error = %v", err)
	}
	if first.Seq != 1 || first.MessageKind != llm.APIModeResponses {
		t.Errorf("AppendNativeMessage(user) = (seq=%d, kind=%q), want (1, %q)", first.Seq, first.MessageKind, llm.APIModeResponses)
	}

	assistantPayload := []byte(`{"role":"assistant","content":"native-answer"}`)
	second, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleAssistant,
		Payload:       serializer.NewCrypto(assistantPayload),
		Content:       encTestValue("plain-content"),
	})
	if err != nil {
		t.Fatalf("AppendNativeMessage(assistant) error = %v", err)
	}
	if second.Seq != 2 || second.MessageKind != llm.APIModeChat {
		t.Errorf("AppendNativeMessage(assistant) = (seq=%d, kind=%q), want (2, %q)", second.Seq, second.MessageKind, llm.APIModeChat)
	}

	var raw struct {
		Payload []byte
	}
	if err := db.Raw("SELECT payload FROM agent_messages WHERE id = ?", first.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("reading the raw payload column error = %v", err)
	}
	assertEncryptedColumn(t, "agent_messages.payload", raw.Payload, "hello-native")

	listed, err := store.ListNativeMessages(ctx, session.ID, session.NamespaceName, testUserID, 0)
	if err != nil {
		t.Fatalf("ListNativeMessages error = %v", err)
	}
	if len(listed) != 2 || listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Fatalf("ListNativeMessages() = [(id=%d, seq=%d), (id=%d, seq=%d)], want [(%d), (%d)]",
			listed[0].ID, listed[0].Seq, listed[1].ID, listed[1].Seq, first.ID, second.ID)
	}
	if !bytes.Equal(listed[0].Payload.Value(), userPayload) {
		t.Errorf("ListNativeMessages()[0].Payload = %q, want %q", listed[0].Payload.Value(), userPayload)
	}
	if string(listed[1].Content.Value()) != "plain-content" {
		t.Errorf("ListNativeMessages()[1].Content = %q, want the content text", listed[1].Content.Value())
	}

	latest, err := store.ListNativeMessages(ctx, session.ID, session.NamespaceName, testUserID, 1)
	if err != nil {
		t.Fatalf("ListNativeMessages(maxCount=1) error = %v", err)
	}
	if len(latest) != 1 || latest[0].ID != second.ID {
		t.Errorf("ListNativeMessages(maxCount=1) = %+v, want only the latest message", latest)
	}

	other, err := store.ListNativeMessages(ctx, session.ID, session.NamespaceName, testOtherUserID, 0)
	if err != nil {
		t.Fatalf("ListNativeMessages(other user) error = %v", err)
	}
	if len(other) != 0 {
		t.Errorf("ListNativeMessages(other user) len = %d, want 0", len(other))
	}
}

// TestNativeStore_UpdateNativeMessage 测试原生消息原地更新：载荷 / 消息表示 / 引用 / 用量 / 阶段一并
// 写回且密文落库，跨用户更新视为消息不存在。
func TestNativeStore_UpdateNativeMessage(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "native", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleAssistant,
		Payload:       serializer.NewCrypto([]byte(`{"role":"assistant"}`)),
	})
	if err != nil {
		t.Fatalf("AppendNativeMessage error = %v", err)
	}

	updatedPayload := []byte(`{"role":"assistant","content":"final-answer"}`)
	if err := store.UpdateNativeMessage(ctx, &ai.AgentMessage{
		ID:               msg.ID,
		SessionID:        session.ID,
		NamespaceName:    session.NamespaceName,
		UserID:           testUserID,
		MessageKind:      llm.APIModeResponses,
		Payload:          serializer.NewCrypto(updatedPayload),
		CitationsJSON:    encTestValue(`[{"title":"native-citation"}]`),
		PromptTokens:     11,
		CompletionTokens: 7,
		TotalTokens:      18,
		ElapsedMs:        42,
		Phase:            ai.AgentRunPhaseDone,
	}); err != nil {
		t.Fatalf("UpdateNativeMessage error = %v", err)
	}

	got, err := store.ListNativeMessages(ctx, session.ID, session.NamespaceName, testUserID, 1)
	if err != nil {
		t.Fatalf("ListNativeMessages error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListNativeMessages() len = %d, want 1", len(got))
	}
	if got[0].MessageKind != llm.APIModeResponses || !bytes.Equal(got[0].Payload.Value(), updatedPayload) {
		t.Errorf("updated message = (kind=%q, payload=%q), want (%q, %q)", got[0].MessageKind, got[0].Payload.Value(), llm.APIModeResponses, updatedPayload)
	}
	if got[0].PromptTokens != 11 || got[0].TotalTokens != 18 || got[0].ElapsedMs != 42 || got[0].Phase != ai.AgentRunPhaseDone {
		t.Errorf("updated usage = %+v, want the values written by UpdateNativeMessage", got[0])
	}

	var raw struct {
		Payload       []byte
		CitationsJSON []byte
	}
	if err := db.Raw("SELECT payload, citations_json FROM agent_messages WHERE id = ?", msg.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("reading the raw columns error = %v", err)
	}
	assertEncryptedColumn(t, "agent_messages.payload", raw.Payload, "final-answer")
	assertEncryptedColumn(t, "agent_messages.citations_json", raw.CitationsJSON, "native-citation")

	err = store.UpdateNativeMessage(ctx, &ai.AgentMessage{
		ID:            msg.ID,
		SessionID:     session.ID,
		NamespaceName: session.NamespaceName,
		UserID:        testOtherUserID,
		Payload:       serializer.NewCrypto([]byte(`{"role":"assistant"}`)),
	})
	if !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("UpdateNativeMessage(other user) error = %v, want ErrMessageNotFound", err)
	}
}

// TestNativeStore_PendingInterrupt 测试会话挂起 interrupt 的读写：初始为空，写入后可读回，清空可用
// 空串，跨用户与不存在的会话返回 ErrSessionNotFound。
func TestNativeStore_PendingInterrupt(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "native", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	interruptID, err := store.GetPendingInterrupt(ctx, session.ID, testUserID)
	if err != nil || interruptID != "" {
		t.Errorf("GetPendingInterrupt(initial) = (%q, %v), want (\"\", nil)", interruptID, err)
	}

	if err := store.SetPendingInterrupt(ctx, session.ID, testUserID, "interrupt-1"); err != nil {
		t.Fatalf("SetPendingInterrupt error = %v", err)
	}
	interruptID, err = store.GetPendingInterrupt(ctx, session.ID, testUserID)
	if err != nil || interruptID != "interrupt-1" {
		t.Errorf("GetPendingInterrupt() = (%q, %v), want (\"interrupt-1\", nil)", interruptID, err)
	}

	if err := store.SetPendingInterrupt(ctx, session.ID, testUserID, ""); err != nil {
		t.Fatalf("SetPendingInterrupt(clear) error = %v", err)
	}
	if interruptID, err = store.GetPendingInterrupt(ctx, session.ID, testUserID); err != nil || interruptID != "" {
		t.Errorf("GetPendingInterrupt(after clear) = (%q, %v), want (\"\", nil)", interruptID, err)
	}

	if _, err := store.GetPendingInterrupt(ctx, session.ID, testOtherUserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetPendingInterrupt(other user) error = %v, want ErrSessionNotFound", err)
	}
	if err := store.SetPendingInterrupt(ctx, session.ID, testOtherUserID, "x"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("SetPendingInterrupt(other user) error = %v, want ErrSessionNotFound", err)
	}
	if _, err := store.GetPendingInterrupt(ctx, session.ID+9999, testUserID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("GetPendingInterrupt(missing session) error = %v, want ErrSessionNotFound", err)
	}
}

// TestNativeStore_Interactions 测试持久化交互的 CRUD：自动分配会话内 seq、按 seq 排序读取、状态与
// 载荷更新（nil 载荷时保留原载荷）、载荷密文落库、跨用户隔离与缺失会话拒绝。
func TestNativeStore_Interactions(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "native", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendNativeMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleAssistant,
		Payload:       serializer.NewCrypto([]byte(`{"role":"assistant"}`)),
	})
	if err != nil {
		t.Fatalf("AppendNativeMessage error = %v", err)
	}

	approvalPayload := []byte(`{"calls":[{"callID":"c1","toolName":"api_invoke"}]}`)
	approval, err := store.AppendInteraction(ctx, &ai.AgentInteraction{
		NamespaceName: "default",
		UserID:        testUserID,
		SessionID:     session.ID,
		MessageID:     msg.ID,
		Kind:          ai.InteractionKindApproval,
		Payload:       serializer.NewCrypto(approvalPayload),
	})
	if err != nil {
		t.Fatalf("AppendInteraction(approval) error = %v", err)
	}
	if approval.Seq != 1 || approval.Status != ai.InteractionStatusPending {
		t.Errorf("AppendInteraction(approval) = (seq=%d, status=%q), want (1, %q)", approval.Seq, approval.Status, ai.InteractionStatusPending)
	}

	questionnairePayload := []byte(`{"questions":[{"question":"q1"}],"answers":[{"answer":"a1"}],"awaiting":false}`)
	questionnaire, err := store.AppendInteraction(ctx, &ai.AgentInteraction{
		NamespaceName: "default",
		UserID:        testUserID,
		SessionID:     session.ID,
		MessageID:     msg.ID,
		Kind:          ai.InteractionKindQuestionnaire,
		Status:        ai.InteractionStatusAnswered,
		Payload:       serializer.NewCrypto(questionnairePayload),
	})
	if err != nil {
		t.Fatalf("AppendInteraction(questionnaire) error = %v", err)
	}
	if questionnaire.Seq != 2 || questionnaire.Status != ai.InteractionStatusAnswered {
		t.Errorf("AppendInteraction(questionnaire) = (seq=%d, status=%q), want (2, %q)", questionnaire.Seq, questionnaire.Status, ai.InteractionStatusAnswered)
	}

	nextSteps, err := store.AppendInteraction(ctx, &ai.AgentInteraction{
		NamespaceName: "default",
		UserID:        testUserID,
		SessionID:     session.ID,
		MessageID:     msg.ID,
		Kind:          ai.InteractionKindNextSteps,
		Payload:       serializer.NewCrypto([]byte(`{"steps":[{"title":"s1"}]}`)),
	})
	if err != nil {
		t.Fatalf("AppendInteraction(next_steps) error = %v", err)
	}
	if nextSteps.Seq != 3 {
		t.Errorf("AppendInteraction(next_steps).Seq = %d, want 3", nextSteps.Seq)
	}

	var raw struct {
		Payload []byte
	}
	if err := db.Raw("SELECT payload FROM agent_interactions WHERE id = ?", approval.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("reading the raw interaction payload error = %v", err)
	}
	assertEncryptedColumn(t, "agent_interactions.payload", raw.Payload, "api_invoke")

	items, err := store.ListInteractions(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("ListInteractions error = %v", err)
	}
	if len(items) != 3 || items[0].ID != approval.ID || items[1].ID != questionnaire.ID || items[2].ID != nextSteps.ID {
		t.Fatalf("ListInteractions() = %+v, want the three interactions in seq order", items)
	}
	if !bytes.Equal(items[0].Payload.Value(), approvalPayload) {
		t.Errorf("ListInteractions()[0].Payload = %q, want %q", items[0].Payload.Value(), approvalPayload)
	}

	answeredPayload := []byte(`{"calls":[{"callID":"c1"}],"decisions":[{"callID":"c1","approved":true}]}`)
	if err := store.UpdateInteractionStatus(ctx, approval.ID, testUserID, ai.InteractionStatusAnswered, answeredPayload); err != nil {
		t.Fatalf("UpdateInteractionStatus(approval) error = %v", err)
	}
	items, err = store.ListInteractions(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("ListInteractions(after update) error = %v", err)
	}
	if items[0].Status != ai.InteractionStatusAnswered || !bytes.Equal(items[0].Payload.Value(), answeredPayload) {
		t.Errorf("updated approval = (status=%q, payload=%q), want (%q, %q)", items[0].Status, items[0].Payload.Value(), ai.InteractionStatusAnswered, answeredPayload)
	}

	// nil 载荷只更新状态，保留原业务 JSON。
	if err := store.UpdateInteractionStatus(ctx, questionnaire.ID, testUserID, ai.InteractionStatusCancelled, nil); err != nil {
		t.Fatalf("UpdateInteractionStatus(questionnaire) error = %v", err)
	}
	items, err = store.ListInteractions(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("ListInteractions(after cancel) error = %v", err)
	}
	if items[1].Status != ai.InteractionStatusCancelled || !bytes.Equal(items[1].Payload.Value(), questionnairePayload) {
		t.Errorf("cancelled questionnaire = (status=%q, payload=%q), want (%q, %q)", items[1].Status, items[1].Payload.Value(), ai.InteractionStatusCancelled, questionnairePayload)
	}

	if err := store.UpdateInteractionStatus(ctx, approval.ID, testOtherUserID, ai.InteractionStatusAnswered, nil); !errors.Is(err, ErrInteractionNotFound) {
		t.Errorf("UpdateInteractionStatus(other user) error = %v, want ErrInteractionNotFound", err)
	}
	otherItems, err := store.ListInteractions(ctx, session.ID, testOtherUserID)
	if err != nil {
		t.Fatalf("ListInteractions(other user) error = %v", err)
	}
	if len(otherItems) != 0 {
		t.Errorf("ListInteractions(other user) len = %d, want 0", len(otherItems))
	}
	if _, err := store.AppendInteraction(ctx, &ai.AgentInteraction{
		UserID:    testOtherUserID,
		SessionID: session.ID,
		Kind:      ai.InteractionKindApproval,
	}); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("AppendInteraction(other user) error = %v, want ErrSessionNotFound", err)
	}
}

// TestNativeStore_AppendMessageWithContent 测试追加消息时 message_kind 缺省写为 chat，正文与草稿
// 照常密文落库并可解密回读，既有更新方法不受新增列影响。
func TestNativeStore_AppendMessageWithContent(t *testing.T) {
	ctx := context.Background()
	db := aitest.NewSessionDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	session, err := store.CreateSession(ctx, "default", "content", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	msg, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID:     session.ID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleAssistant,
		Content:       encTestValue("plain-answer"),
		DraftJSON:     encTestValue(`{"draft":"plain-draft"}`),
	})
	if err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if msg.MessageKind != llm.APIModeChat {
		t.Errorf("AppendMessage().MessageKind = %q, want %q", msg.MessageKind, llm.APIModeChat)
	}

	var raw struct {
		MessageKind llm.APIMode
		Content     []byte
	}
	if err := db.Raw("SELECT message_kind, content FROM agent_messages WHERE id = ?", msg.ID).Scan(&raw).Error; err != nil {
		t.Fatalf("reading the raw message columns error = %v", err)
	}
	if raw.MessageKind != llm.APIModeChat {
		t.Errorf("raw message_kind = %q, want %q", raw.MessageKind, llm.APIModeChat)
	}
	assertEncryptedColumn(t, "agent_messages.content", raw.Content, "plain-answer")

	got, err := store.GetMessage(ctx, msg.ID, session.NamespaceName, testUserID)
	if err != nil {
		t.Fatalf("GetMessage error = %v", err)
	}
	if string(got.Content.Value()) != "plain-answer" || string(got.DraftJSON.Value()) != `{"draft":"plain-draft"}` {
		t.Errorf("GetMessage() = (content=%q, draft=%q), want the stored values", got.Content.Value(), got.DraftJSON.Value())
	}
}
