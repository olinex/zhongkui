package interaction

import (
	// 标准库
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// runningFacts 构造「存在在途活动」的事实快照，仅用于推导用例。
func runningFacts(substate ai.AgentInteractionSubstate) InteractionFacts {
	return InteractionFacts{LiveRunning: true, LiveSubstate: substate}
}

// waitingFacts 构造「存在挂起现场」的事实快照。
func waitingFacts(kind ai.AgentInteractionSubstate) InteractionFacts {
	return InteractionFacts{PendingKind: kind}
}

// pausedFacts 构造「无在途活动、无挂起现场、未锁定」的事实快照，终局阶段决定暂停子状态。
func pausedFacts(phase ai.AgentRunPhase) InteractionFacts {
	return InteractionFacts{LastAssistantPhase: phase}
}

// lockedFacts 构造「有效轮次已达上限」的事实快照。
func lockedFacts() InteractionFacts {
	return InteractionFacts{UserTurns: 3, MaxUserTurns: 3}
}

// derivedStateCase 是「事实 → 推导结果」的代表性用例，供推导断言与守卫矩阵两组测试共用，
// 保证守卫判定与推导结果始终对齐同源。
type derivedStateCase struct {
	name     string
	facts    InteractionFacts
	state    ai.AgentInteractionState
	substate ai.AgentInteractionSubstate
	locked   bool
}

// representativeStates 覆盖四个主状态（含全部子状态）共 10 组代表事实。
func representativeStates() []derivedStateCase {
	return []derivedStateCase{
		{"running/generating", runningFacts(ai.AgentInteractionSubstateGenerating), ai.AgentInteractionStateRunning, ai.AgentInteractionSubstateGenerating, false},
		{"running/thinking", runningFacts(ai.AgentInteractionSubstateThinking), ai.AgentInteractionStateRunning, ai.AgentInteractionSubstateThinking, false},
		{"running/tool_calling", runningFacts(ai.AgentInteractionSubstateToolCalling), ai.AgentInteractionStateRunning, ai.AgentInteractionSubstateToolCalling, false},
		{"running/compacting", runningFacts(ai.AgentInteractionSubstateCompacting), ai.AgentInteractionStateRunning, ai.AgentInteractionSubstateCompacting, false},
		{"waiting/approval", waitingFacts(ai.AgentInteractionSubstateApproval), ai.AgentInteractionStateWaiting, ai.AgentInteractionSubstateApproval, false},
		{"waiting/answer", waitingFacts(ai.AgentInteractionSubstateAnswer), ai.AgentInteractionStateWaiting, ai.AgentInteractionSubstateAnswer, false},
		{"paused/success", pausedFacts(ai.AgentRunPhaseDone), ai.AgentInteractionStatePaused, ai.AgentInteractionSubstateSuccess, false},
		{"paused/failed", pausedFacts(ai.AgentRunPhaseFailed), ai.AgentInteractionStatePaused, ai.AgentInteractionSubstateFailed, false},
		{"paused/aborted", pausedFacts(ai.AgentRunPhaseCancelled), ai.AgentInteractionStatePaused, ai.AgentInteractionSubstateAborted, false},
		{"locked", lockedFacts(), ai.AgentInteractionStateLocked, "", true},
	}
}

// assertSnapshotEqual 断言推导出参与期望完全一致（含 locked 与空子状态口径）。
func assertSnapshotEqual(t *testing.T, got InteractionSnapshot, want derivedStateCase) {
	t.Helper()
	if got.State != want.state || got.Substate != want.substate || got.Locked != want.locked {
		t.Errorf("StateOf(%s) = (%q, %q, locked=%v), want (%q, %q, locked=%v)",
			want.name, got.State, got.Substate, got.Locked, want.state, want.substate, want.locked)
	}
}

// TestStateOf_Representative 表驱动断言每个代表性事实落点正确（四主状态与全部子状态各一例），
// 轮次字段透传不失真。
func TestStateOf_Representative(t *testing.T) {
	for _, tc := range representativeStates() {
		t.Run(tc.name, func(t *testing.T) {
			got := StateOf(tc.facts)
			assertSnapshotEqual(t, got, tc)
			if got.UserTurns != tc.facts.UserTurns || got.MaxUserTurns != tc.facts.MaxUserTurns {
				t.Errorf("StateOf(%s) turns = (%d, %d), want (%d, %d)",
					tc.name, got.UserTurns, got.MaxUserTurns, tc.facts.UserTurns, tc.facts.MaxUserTurns)
			}
		})
	}
}

// TestStateOf_EmptySession 断言无任何消息的空会话落为 暂停/成功。
func TestStateOf_EmptySession(t *testing.T) {
	got := StateOf(InteractionFacts{})
	if got.State != ai.AgentInteractionStatePaused || got.Substate != ai.AgentInteractionSubstateSuccess {
		t.Errorf("StateOf(empty) = (%q, %q), want (paused, success)", got.State, got.Substate)
	}
	if got.Locked {
		t.Error("StateOf(empty).Locked = true, want false")
	}
}

// TestStateOf_Priority 断言推导优先级：在途活动覆盖挂起现场与锁定；挂起现场覆盖锁定。
func TestStateOf_Priority(t *testing.T) {
	// 轮次已满但存在在途活动：仍为运行中，活动优先于锁定。
	running := StateOf(InteractionFacts{
		LiveRunning: true, LiveSubstate: ai.AgentInteractionSubstateThinking,
		PendingKind: ai.AgentInteractionSubstateApproval, UserTurns: 9, MaxUserTurns: 9,
	})
	if running.State != ai.AgentInteractionStateRunning || running.Substate != ai.AgentInteractionSubstateThinking {
		t.Errorf("StateOf(running over locked) = (%q, %q), want (running, thinking)", running.State, running.Substate)
	}
	// 轮次已满但存在挂起现场：仍为等待中，挂起现场优先于锁定。
	waiting := StateOf(InteractionFacts{
		PendingKind: ai.AgentInteractionSubstateAnswer, UserTurns: 9, MaxUserTurns: 9,
	})
	if waiting.State != ai.AgentInteractionStateWaiting || waiting.Substate != ai.AgentInteractionSubstateAnswer {
		t.Errorf("StateOf(waiting over locked) = (%q, %q), want (waiting, answer)", waiting.State, waiting.Substate)
	}
}

// TestStateOf_Normalization 断言归一化兜底：运行子状态为空 / 非法 → generating；挂起种类非法 →
// approval；MaxUserTurns <= 0 时轮次再大也永不锁定。
func TestStateOf_Normalization(t *testing.T) {
	if got := StateOf(runningFacts("")); got.Substate != ai.AgentInteractionSubstateGenerating {
		t.Errorf("StateOf(running, empty substate).Substate = %q, want generating", got.Substate)
	}
	if got := StateOf(runningFacts("bogus")); got.Substate != ai.AgentInteractionSubstateGenerating {
		t.Errorf("StateOf(running, invalid substate).Substate = %q, want generating", got.Substate)
	}
	if got := StateOf(waitingFacts("bogus")); got.Substate != ai.AgentInteractionSubstateApproval {
		t.Errorf("StateOf(waiting, invalid kind).Substate = %q, want approval", got.Substate)
	}
	// 暂停子状态对未知终局阶段亦兜底为成功（例如空串与运行中阶段）。
	if got := StateOf(pausedFacts("bogus")); got.Substate != ai.AgentInteractionSubstateSuccess {
		t.Errorf("StateOf(paused, unknown phase).Substate = %q, want success", got.Substate)
	}
	for _, max := range []int{0, -1, -100} {
		got := StateOf(InteractionFacts{UserTurns: 1000, MaxUserTurns: max})
		if got.State != ai.AgentInteractionStatePaused || got.Locked {
			t.Errorf("StateOf(turns=1000, max=%d) = (%q, locked=%v), want (paused, false)", max, got.State, got.Locked)
		}
	}
}

// TestStateOf_Total 遍历字段笛卡尔积（含各字段零值组合）断言全函数性：State 恒为四个主状态
// 之一，且 Locked 与 State==locked 严格一致，不存在空档或并列。
func TestStateOf_Total(t *testing.T) {
	validStates := make(map[ai.AgentInteractionState]bool)
	for _, state := range ai.AllAgentInteractionStates() {
		validStates[state] = true
	}
	running := []bool{false, true}
	liveSubstates := []ai.AgentInteractionSubstate{"", ai.AgentInteractionSubstateGenerating, ai.AgentInteractionSubstateThinking, ai.AgentInteractionSubstateApproval, "bogus"}
	pendingKinds := []ai.AgentInteractionSubstate{"", ai.AgentInteractionSubstateAnswer, ai.AgentInteractionSubstateApproval, "bogus"}
	turns := [][2]int{{0, 0}, {0, 20}, {5, 0}, {5, 5}, {6, 5}, {20, 20}}
	phases := []ai.AgentRunPhase{"", ai.AgentRunPhaseDone, ai.AgentRunPhaseFailed, ai.AgentRunPhaseCancelled, ai.AgentRunPhaseRunning, ai.AgentRunPhaseSuspended, "bogus"}
	count := 0
	for _, runningFlag := range running {
		for _, live := range liveSubstates {
			for _, pending := range pendingKinds {
				for _, turn := range turns {
					for _, phase := range phases {
						facts := InteractionFacts{
							LiveRunning: runningFlag, LiveSubstate: live, PendingKind: pending,
							UserTurns: turn[0], MaxUserTurns: turn[1], LastAssistantPhase: phase,
						}
						got := StateOf(facts)
						count++
						if !validStates[got.State] {
							t.Fatalf("StateOf(%+v).State = %q, want one of the four main states", facts, got.State)
						}
						if got.Locked != (got.State == ai.AgentInteractionStateLocked) {
							t.Fatalf("StateOf(%+v).Locked = %v, want it derived from State (%q)", facts, got.Locked, got.State)
						}
						if got.State == ai.AgentInteractionStateLocked && got.Substate != "" {
							t.Fatalf("StateOf(%+v) = locked with substate %q, want an empty substate", facts, got.Substate)
						}
					}
				}
			}
		}
	}
	if count != len(running)*len(liveSubstates)*len(pendingKinds)*len(turns)*len(phases) {
		t.Fatalf("covered cases = %d, want the full cartesian product", count)
	}
}

// assertGuard 断言守卫返回值与期望一致：允许时为 nil，拒绝时 errors.Is 命中对应哨兵。
func assertGuard(t *testing.T, guard string, got, want error, label string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s(%s) = %v, want nil", guard, label, got)
	case want != nil && !errors.Is(got, want):
		t.Errorf("%s(%s) = %v, want %v", guard, label, got, want)
	}
}

// TestCanSend_DerivedStates 断言发送守卫矩阵：仅暂停允许；已锁定返回 ErrInteractionLocked；
// 运行中 / 等待中返回 ErrInteractionBusy。
func TestCanSend_DerivedStates(t *testing.T) {
	for _, tc := range representativeStates() {
		var want error
		switch tc.state {
		case ai.AgentInteractionStatePaused:
			want = nil
		case ai.AgentInteractionStateLocked:
			want = ErrInteractionLocked
		default:
			want = ErrInteractionBusy
		}
		assertGuard(t, "CanSend", CanSend(tc.facts), want, tc.name)
	}
}

// TestCanCancel_DerivedStates 断言中止守卫矩阵：仅运行中 / 等待中允许，其余 ErrInteractionBusy。
func TestCanCancel_DerivedStates(t *testing.T) {
	for _, tc := range representativeStates() {
		var want error = ErrInteractionBusy
		if tc.state == ai.AgentInteractionStateRunning || tc.state == ai.AgentInteractionStateWaiting {
			want = nil
		}
		assertGuard(t, "CanCancel", CanCancel(tc.facts), want, tc.name)
	}
}

// TestCanRollback_DerivedStates 断言回滚守卫矩阵：仅运行中拒绝，暂停 / 已锁定 / 等待中放行。
func TestCanRollback_DerivedStates(t *testing.T) {
	for _, tc := range representativeStates() {
		var want error = ErrInteractionBusy
		if tc.state == ai.AgentInteractionStatePaused || tc.state == ai.AgentInteractionStateLocked ||
			tc.state == ai.AgentInteractionStateWaiting {
			want = nil
		}
		assertGuard(t, "CanRollback", CanRollback(tc.facts), want, tc.name)
	}
}

// TestCanCompact_DerivedStates 断言压缩守卫矩阵：仅暂停 / 已锁定允许；压缩是一次在途活动，等待中也不允许。
func TestCanCompact_DerivedStates(t *testing.T) {
	for _, tc := range representativeStates() {
		var want error = ErrInteractionBusy
		if tc.state == ai.AgentInteractionStatePaused || tc.state == ai.AgentInteractionStateLocked {
			want = nil
		}
		assertGuard(t, "CanCompact", CanCompact(tc.facts), want, tc.name)
	}
}

// TestCanRerunUserInput_DerivedStates 断言回退待输入回合守卫矩阵：仅运行中拒绝，暂停 / 已锁定 / 等待中放行。
func TestCanRerunUserInput_DerivedStates(t *testing.T) {
	for _, tc := range representativeStates() {
		var want error = ErrInteractionBusy
		if tc.state == ai.AgentInteractionStatePaused || tc.state == ai.AgentInteractionStateLocked ||
			tc.state == ai.AgentInteractionStateWaiting {
			want = nil
		}
		assertGuard(t, "CanRerunUserInput", CanRerunUserInput(tc.facts), want, tc.name)
	}
}
