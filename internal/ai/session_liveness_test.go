// 本文件覆盖 Task 5「等待中收敛与在途运行存活性收敛」的数据库层行为：用户在等待中主动中止、挂起现场
// TTL 过期、进程重启后遗留的 running 登记，三者均须使会话派生回落 暂停（用户中止 / 失败），且收敛
// 只改「阶段」而不删除会话消息、长期记忆与压缩检查点。运行现场（agent_runs）与运行登记（agent_live_runs）
// 含 TIMESTAMPTZ 列，故一律走 PostgreSQL 测试库（testdb.Core）。

package ai

import (
	// 标准库
	"context"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// newLivenessStore 构造走 PostgreSQL 测试库的会话存储，供本文件各用例复用。
func newLivenessStore(t *testing.T) *SessionStore {
	t.Helper()
	store, err := NewSessionStore(testdb.Core(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	return store
}

// countRows 统计某表在某会话下的现存行数（供「收敛不删除数据」断言）。
func countRows(t *testing.T, store *SessionStore, model any, sessionID uint) int64 {
	t.Helper()
	var n int64
	if err := store.db.Model(model).Where("session_id = ?", sessionID).Count(&n).Error; err != nil {
		t.Fatalf("count rows of %T error = %v", model, err)
	}
	return n
}

// seedAssistantAnchor 追加一条承载挂起锚点的助手消息（阶段可指定），返回该行 ID。
func seedAssistantAnchor(t *testing.T, store *SessionStore, sessionID uint, phase ai.AgentRunPhase) uint {
	t.Helper()
	msg, err := store.AppendMessage(context.Background(), &ai.AgentMessage{
		SessionID:     sessionID,
		NamespaceName: "default",
		UserID:        testUserID,
		Role:          ai.MessageRoleAssistant,
		Content:       encTestValue("answer"),
		Phase:         phase,
	})
	if err != nil {
		t.Fatalf("AppendMessage(assistant) error = %v", err)
	}
	return msg.ID
}

// TestConvergeOrphanLiveRun_ProcessRestart 覆盖「进程重启后读取在途运行」：内存中已无对应活动的 running
// 登记被收敛为 failed，会话最近助手消息终局一并置 failed（使派生回落 暂停/失败），且消息 / 长期记忆 /
// 压缩检查点均未被删除。
func TestConvergeOrphanLiveRun_ProcessRestart(t *testing.T) {
	store := newLivenessStore(t)
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	seedTurn(t, store, session.ID, testUserID, "q", "") // 在途运行阶段的阶段性落库行（阶段为空）
	if err := store.db.WithContext(ctx).Create(&ai.AgentMemory{
		NamespaceName: "default", UserID: testUserID, AgentKind: ai.AgentKindKnowledge,
		SessionID: session.ID, Kind: ai.MemoryKindFact, Title: "m", Content: encTestValue("body"),
		// PostgreSQL 的 embedding 列（vector(1536)）为 not null：本用例只关心行是否留存，填零向量即可。
		Embedding: make([]float32, 1536),
	}).Error; err != nil {
		t.Fatalf("create memory error = %v", err)
	}
	if err := store.db.WithContext(ctx).Create(&ai.AgentSessionCompaction{
		NamespaceName: "default", SessionID: session.ID, UserID: testUserID,
		UptoSeq: 2, Summary: encTestValue("s"), Reason: "rolling",
	}).Error; err != nil {
		t.Fatalf("create compaction error = %v", err)
	}
	runID := serializer.NewHexUUIDv4()
	if err := store.CreateLiveRun(ctx, &ai.AgentLiveRun{
		NamespaceName: "default", UserID: testUserID, RunID: runID, SessionID: session.ID,
		Phase: ai.AgentRunPhaseRunning, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateLiveRun error = %v", err)
	}

	msgsBefore := countRows(t, store, &ai.AgentMessage{}, session.ID)

	if err := store.ConvergeOrphanLiveRun(ctx, runID, session.ID, testUserID); err != nil {
		t.Fatalf("ConvergeOrphanLiveRun error = %v", err)
	}

	lr, err := store.LatestLiveRun(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("LatestLiveRun error = %v", err)
	}
	if lr.Phase != ai.AgentRunPhaseFailed {
		t.Errorf("orphan live run phase = %q, want %q", lr.Phase, ai.AgentRunPhaseFailed)
	}
	last, err := store.LatestAssistantMessage(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if last.Phase != ai.AgentRunPhaseFailed {
		t.Errorf("latest assistant phase = %q, want %q", last.Phase, ai.AgentRunPhaseFailed)
	}
	if snapshot := StateOf(InteractionFacts{LastAssistantPhase: last.Phase, UserTurns: 1, MaxUserTurns: 20}); snapshot.State != ai.AgentInteractionStatePaused || snapshot.Substate != ai.AgentInteractionSubstateFailed {
		t.Errorf("StateOf(orphan converged) = (%q, %q), want (paused, failed)", snapshot.State, snapshot.Substate)
	}
	if got := countRows(t, store, &ai.AgentMessage{}, session.ID); got != msgsBefore {
		t.Errorf("messages after convergence = %d, want %d (must not be deleted)", got, msgsBefore)
	}
	if got := countRows(t, store, &ai.AgentMemory{}, session.ID); got != 1 {
		t.Errorf("memories after convergence = %d, want 1 (must not be deleted)", got)
	}
	if got := countRows(t, store, &ai.AgentSessionCompaction{}, session.ID); got != 1 {
		t.Errorf("compactions after convergence = %d, want 1 (must not be deleted)", got)
	}
}

// TestConvergeExpiredPendingRuns 覆盖「待回答超时」：过期挂起现场被作废（status=expired），锚点助手消息
// 终局置 failed，会话派生回落 暂停/失败。
func TestConvergeExpiredPendingRuns(t *testing.T) {
	store := newLivenessStore(t)
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	anchorID := seedAssistantAnchor(t, store, session.ID, ai.AgentRunPhaseSuspended)
	run := &ai.AgentRun{
		NamespaceName: "default", UserID: testUserID, SessionID: session.ID,
		Status:           ai.AgentRunStatusPending,
		PendingCallsJSON: encTestValue(`[{"callID":"ask_user","toolName":"ask_user"}]`),
		AnchorMessageID:  anchorID,
		ExpiresAt:        time.Now().Add(-time.Minute),
	}
	if err := store.SaveAgentRun(ctx, run); err != nil {
		t.Fatalf("SaveAgentRun error = %v", err)
	}

	converged, err := store.ConvergeExpiredPendingRuns(ctx, time.Now(), 0)
	if err != nil {
		t.Fatalf("ConvergeExpiredPendingRuns error = %v", err)
	}
	if converged != 1 {
		t.Errorf("ConvergeExpiredPendingRuns = %d, want 1", converged)
	}
	var got ai.AgentRun
	if err := store.db.WithContext(ctx).First(&got, run.ID).Error; err != nil {
		t.Fatalf("reload agent run error = %v", err)
	}
	if got.Status != ai.AgentRunStatusExpired {
		t.Errorf("converged run status = %q, want %q", got.Status, ai.AgentRunStatusExpired)
	}
	last, err := store.LatestAssistantMessage(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if last.Phase != ai.AgentRunPhaseFailed {
		t.Errorf("anchor phase = %q, want %q", last.Phase, ai.AgentRunPhaseFailed)
	}
	if kind, err := store.PendingRunSubstate(ctx, session.ID, testUserID); err != nil || kind != "" {
		t.Errorf("PendingRunSubstate after convergence = (%q, %v), want empty", kind, err)
	}
	if snapshot := StateOf(InteractionFacts{LastAssistantPhase: last.Phase}); snapshot.State != ai.AgentInteractionStatePaused || snapshot.Substate != ai.AgentInteractionSubstateFailed {
		t.Errorf("StateOf(expired pending) = (%q, %q), want (paused, failed)", snapshot.State, snapshot.Substate)
	}
}

// TestConvergeExpiredRunningLiveRuns 覆盖清理任务对超期 running 登记的批量收敛（逐行置 failed）。
func TestConvergeExpiredRunningLiveRuns(t *testing.T) {
	store := newLivenessStore(t)
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	seedAssistantAnchor(t, store, session.ID, "")
	runID := serializer.NewHexUUIDv4()
	if err := store.CreateLiveRun(ctx, &ai.AgentLiveRun{
		NamespaceName: "default", UserID: testUserID, RunID: runID, SessionID: session.ID,
		Phase: ai.AgentRunPhaseRunning, ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateLiveRun error = %v", err)
	}

	converged, err := store.ConvergeExpiredRunningLiveRuns(ctx, time.Now(), 0)
	if err != nil {
		t.Fatalf("ConvergeExpiredRunningLiveRuns error = %v", err)
	}
	if converged != 1 {
		t.Errorf("ConvergeExpiredRunningLiveRuns = %d, want 1", converged)
	}
	lr, err := store.LatestLiveRun(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("LatestLiveRun error = %v", err)
	}
	if lr.Phase != ai.AgentRunPhaseFailed {
		t.Errorf("expired running live run phase = %q, want %q", lr.Phase, ai.AgentRunPhaseFailed)
	}
}

// TestAbortPendingRun_UserAbortWhileWaiting 覆盖「用户放弃待授权 / 待回答」：现场被作废（consumed），锚点
// 助手消息终局置 cancelled，会话派生回落 暂停/用户中止；重复中止幂等。
func TestAbortPendingRun_UserAbortWhileWaiting(t *testing.T) {
	store := newLivenessStore(t)
	ctx := context.Background()
	session, err := store.CreateSession(ctx, "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	anchorID := seedAssistantAnchor(t, store, session.ID, ai.AgentRunPhaseSuspended)
	run := &ai.AgentRun{
		NamespaceName: "default", UserID: testUserID, SessionID: session.ID,
		Status:           ai.AgentRunStatusPending,
		PendingCallsJSON: encTestValue(`[{"callID":"c1","toolName":"api_invoke"}]`),
		AnchorMessageID:  anchorID,
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	if err := store.SaveAgentRun(ctx, run); err != nil {
		t.Fatalf("SaveAgentRun error = %v", err)
	}

	if err := store.AbortPendingRun(ctx, session.ID, testUserID); err != nil {
		t.Fatalf("AbortPendingRun error = %v", err)
	}
	var got ai.AgentRun
	if err := store.db.WithContext(ctx).First(&got, run.ID).Error; err != nil {
		t.Fatalf("reload agent run error = %v", err)
	}
	if got.Status != ai.AgentRunStatusConsumed {
		t.Errorf("aborted run status = %q, want %q", got.Status, ai.AgentRunStatusConsumed)
	}
	last, err := store.LatestAssistantMessage(ctx, session.ID, testUserID)
	if err != nil {
		t.Fatalf("LatestAssistantMessage error = %v", err)
	}
	if last.Phase != ai.AgentRunPhaseCancelled {
		t.Errorf("anchor phase = %q, want %q", last.Phase, ai.AgentRunPhaseCancelled)
	}
	if kind, err := store.PendingRunSubstate(ctx, session.ID, testUserID); err != nil || kind != "" {
		t.Errorf("PendingRunSubstate after abort = (%q, %v), want empty", kind, err)
	}
	if snapshot := StateOf(InteractionFacts{LastAssistantPhase: last.Phase}); snapshot.State != ai.AgentInteractionStatePaused || snapshot.Substate != ai.AgentInteractionSubstateAborted {
		t.Errorf("StateOf(user abort) = (%q, %q), want (paused, aborted)", snapshot.State, snapshot.Substate)
	}
	// 重复中止：无 pending 现场，幂等返回 nil。
	if err := store.AbortPendingRun(ctx, session.ID, testUserID); err != nil {
		t.Errorf("AbortPendingRun(repeat) error = %v, want nil", err)
	}
}
