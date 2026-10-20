package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// seedTurn 在会话内追加一轮问答（一条用户提问 + 一条已终局的助手回答），供轮次口径用例构造消息事实。
func seedTurn(t *testing.T, store *SessionStore, sessionID, userID uint, question string, phase ai.AgentRunPhase) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID:     sessionID,
		NamespaceName: "default",
		UserID:        userID,
		Role:          ai.MessageRoleUser,
		Content:       encTestValue(question),
	}); err != nil {
		t.Fatalf("AppendMessage(user) error = %v", err)
	}
	if _, err := store.AppendMessage(ctx, &ai.AgentMessage{
		SessionID:     sessionID,
		NamespaceName: "default",
		UserID:        userID,
		Role:          ai.MessageRoleAssistant,
		Content:       encTestValue("answer"),
		Phase:         phase,
	}); err != nil {
		t.Fatalf("AppendMessage(assistant) error = %v", err)
	}
}

// TestSessionTurnLimit_LocksAtLimit 覆盖「达上限自动锁定」：以会话内现存用户消息条数为有效轮次，
// 达到上限时 StateOf 落 locked 且 Locked=true——锁定是既有事实的全函数推导结果，不写任何状态列。
func TestSessionTurnLimit_LocksAtLimit(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	const maxTurns = 2
	for i := 0; i < maxTurns; i++ {
		seedTurn(t, store, session.ID, testUserID, "q", ai.AgentRunPhaseDone)
	}

	turns, err := store.CountUserTurns(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("CountUserTurns error = %v", err)
	}
	if turns != maxTurns {
		t.Fatalf("CountUserTurns() = %d, want %d", turns, maxTurns)
	}
	snapshot := StateOf(InteractionFacts{
		UserTurns: turns, MaxUserTurns: maxTurns, LastAssistantPhase: ai.AgentRunPhaseDone,
	})
	if snapshot.State != ai.AgentInteractionStateLocked || !snapshot.Locked {
		t.Errorf("StateOf(full turns) = (%q, locked=%v), want (locked, true)", snapshot.State, snapshot.Locked)
	}
	if err := CanSend(InteractionFacts{UserTurns: turns, MaxUserTurns: maxTurns}); err != ErrInteractionLocked {
		t.Errorf("CanSend(full turns) = %v, want ErrInteractionLocked", err)
	}
}

// TestSessionTurnLimit_RollbackUnlocks 覆盖「回滚解锁」：已满会话回滚删除一条提问后有效轮次下降，
// StateOf 自然回到 paused（无在途活动与挂起现场时）——派生即解锁，无需任何额外状态写入。
func TestSessionTurnLimit_RollbackUnlocks(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	const maxTurns = 2
	for i := 0; i < maxTurns; i++ {
		seedTurn(t, store, session.ID, testUserID, "q", ai.AgentRunPhaseDone)
	}
	if snapshot := StateOf(InteractionFacts{UserTurns: maxTurns, MaxUserTurns: maxTurns}); snapshot.State != ai.AgentInteractionStateLocked {
		t.Fatalf("precondition: StateOf(full) = %q, want locked", snapshot.State)
	}

	// 回滚到第 2 条提问发送前：删除该提问及其之后的助手回复。
	if err := store.TruncateMessagesFromUserMessage(ctx, session.ID, testUserID, 2); err != nil {
		t.Fatalf("TruncateMessagesFromUserMessage error = %v", err)
	}
	turns, err := store.CountUserTurns(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("CountUserTurns error = %v", err)
	}
	if turns != maxTurns-1 {
		t.Fatalf("CountUserTurns() after rollback = %d, want %d", turns, maxTurns-1)
	}
	snapshot := StateOf(InteractionFacts{
		UserTurns: turns, MaxUserTurns: maxTurns, LastAssistantPhase: ai.AgentRunPhaseDone,
	})
	if snapshot.State != ai.AgentInteractionStatePaused || snapshot.Locked {
		t.Errorf("StateOf(after rollback) = (%q, locked=%v), want (paused, false)", snapshot.State, snapshot.Locked)
	}
}

// TestSessionStore_PendingRunSubstate 覆盖挂起现场种类读取：问卷类 ask_user → answer，其余工具 →
// approval，无可用现场 → 空串（表示无挂起现场）。用例走 PostgreSQL 测试库（testdb.Core）：读取运行
// 现场会整行回读 agent_runs（含 TIMESTAMPTZ 的 expires_at 列），sqlite 方言无法扫描该列。
func TestSessionStore_PendingRunSubstate(t *testing.T) {
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	savePending := func(tools ...string) {
		t.Helper()
		pending := make([]pendingToolCall, 0, len(tools))
		for _, name := range tools {
			pending = append(pending, pendingToolCall{
				CallID: name, ToolName: name, Arguments: "{}", ExecOwner: ToolExecOwnerClient,
			})
		}
		raw, merr := json.Marshal(pending)
		if merr != nil {
			t.Fatalf("marshal pending calls error = %v", merr)
		}
		if serr := store.SaveAgentRun(ctx, &ai.AgentRun{
			NamespaceName: "default", UserID: testUserID, SessionID: session.ID,
			Status:           ai.AgentRunStatusPending,
			PendingCallsJSON: serializer.NewCrypto(raw),
			ExpiresAt:        time.Now().Add(time.Hour),
		}); serr != nil {
			t.Fatalf("SaveAgentRun error = %v", serr)
		}
	}

	if kind, err := store.PendingRunSubstate(ctx, session.ID, testUserID); err != nil || kind != "" {
		t.Errorf("PendingRunSubstate(no run) = (%q, %v), want empty", kind, err)
	}
	savePending("ask_user")
	if kind, err := store.PendingRunSubstate(ctx, session.ID, testUserID); err != nil || kind != ai.AgentInteractionSubstateAnswer {
		t.Errorf("PendingRunSubstate(ask_user) = (%q, %v), want answer", kind, err)
	}
	savePending("api_invoke")
	if kind, err := store.PendingRunSubstate(ctx, session.ID, testUserID); err != nil || kind != ai.AgentInteractionSubstateApproval {
		t.Errorf("PendingRunSubstate(api_invoke) = (%q, %v), want approval", kind, err)
	}
}
