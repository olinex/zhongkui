// 本文件覆盖 Task 6「状态广播」的领域层行为：交互 state 事件的初始下发、子状态流转、终局口径、
// 广播端点（BroadcastInteractionState）以及 LiveRun 快照的子状态字段。复用 run_hub_test.go 的
// 测试脚手架（hubExecutor / fakeStore / newTestHub 等）。

package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// stateEventsOf 从事件切片中筛出 state 事件并解析其负载（{state, substate}），供状态广播用例断言。
func stateEventsOf(t *testing.T, evs []Published) []InteractionStateEvent {
	t.Helper()
	var out []InteractionStateEvent
	for _, ev := range evs {
		if ev.Type != ai.AgentRunEventState {
			continue
		}
		var s InteractionStateEvent
		if err := json.Unmarshal(ev.Payload, &s); err != nil {
			t.Fatalf("unmarshal state event payload error = %v (payload=%s)", err, ev.Payload)
		}
		out = append(out, s)
	}
	return out
}

// persistedStateEvents 从落库事件里筛出并解析 state 事件负载（用于验证 state 事件可续播回放）。
func persistedStateEvents(t *testing.T, events []*ai.AgentRunEvent) []InteractionStateEvent {
	t.Helper()
	var out []InteractionStateEvent
	for _, ev := range events {
		if ev.Type != ai.AgentRunEventState {
			continue
		}
		var s InteractionStateEvent
		if err := json.Unmarshal(ev.Payload.Value(), &s); err != nil {
			t.Fatalf("unmarshal persisted state event payload error = %v", err)
		}
		out = append(out, s)
	}
	return out
}

// assertStateEvents 断言实际 state 事件序列与期望逐项相等。
func assertStateEvents(t *testing.T, got, want []InteractionStateEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("state events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// terminalStateOf 返回事件切片（含 state）中最后一条 state 事件负载。
func terminalStateOf(t *testing.T, evs []Published) InteractionStateEvent {
	t.Helper()
	states := stateEventsOf(t, evs)
	if len(states) == 0 {
		t.Fatal("no state event received")
	}
	return states[len(states)-1]
}

// suspendExecutor 是发完给定工具请求即正常返回（不产出 done）的最小执行器替身，用于构造「工具
// 调用挂起」终局并验证其终局 state 子状态（问卷类 ask_user → waiting/answer，其余 → waiting/approval）。
type suspendExecutor struct {
	calls []ToolRequest
}

// RunDetached 依次下发客户端工具请求后正常返回（不产出 done），使运行收敛为 suspended。
func (e *suspendExecutor) RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	for _, c := range e.calls {
		if onToolRequest != nil {
			if err := onToolRequest(c); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

// PrepareResume 是 ResumeInput → AgentInput 的桩实现（本替身不涉及续跑认领）。
func (e *suspendExecutor) PrepareResume(ctx context.Context, in ResumeInput) (AgentInput, error) {
	return AgentInput{SessionID: in.SessionID, NamespaceName: in.NamespaceName, UserID: in.UserID}, nil
}

// ResumeDetached 复用 RunDetached 的编排。
func (e *suspendExecutor) ResumeDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	return e.RunDetached(ctx, in, onEvent, onToolRequest)
}

// compressExecutor 在 RunDetached 中依次经注入的 CompressReporter 上报给定压缩事件，随后以 done
// 收尾，用于验证「上下文压缩中 → 正文生成中」的子状态流转。
type compressExecutor struct{ events []CompressEvent }

// RunDetached 上报压缩事件后以 done 收尾。
func (e *compressExecutor) RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	if in.CompressReporter != nil {
		for _, ev := range e.events {
			if err := in.CompressReporter(ev); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	if onEvent != nil {
		return onEvent(AgentOutput{Done: true})
	}
	return nil
}

// PrepareResume 是 ResumeInput → AgentInput 的桩实现。
func (e *compressExecutor) PrepareResume(ctx context.Context, in ResumeInput) (AgentInput, error) {
	return AgentInput{SessionID: in.SessionID, NamespaceName: in.NamespaceName, UserID: in.UserID}, nil
}

// ResumeDetached 复用 RunDetached 的编排。
func (e *compressExecutor) ResumeDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	return e.RunDetached(ctx, in, onEvent, onToolRequest)
}

// TestRunHub_StateEventTransitionsAndReplay 测试 Start 路径的 state 事件：登记后即发布初始
// running/thinking；随后随工具请求与正文增量依次流转 tool_calling → generating；终局发布
// paused/success；且全部 state 事件落库可续播回放（口径与事件一致）。
func TestRunHub_StateEventTransitionsAndReplay(t *testing.T) {
	exec := &hubExecutor{
		toolCalls: []ToolRequest{{CallID: "c1", ToolName: "api_invoke", Arguments: "{}"}},
		deltas:    []string{"a"},
		release:   make(chan struct{}),
	}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(70)

	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	replay, live, cancel, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	// 初始 state 事件在登记后、执行前发布，故订阅回放中必然先出现 running/thinking。
	if got := stateEventsOf(t, replay); len(got) != 1 ||
		got[0] != (InteractionStateEvent{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateThinking}) {
		t.Fatalf("initial state replay = %+v, want a single running/thinking", got)
	}
	close(exec.release)

	all := append(append([]Published{}, replay...), drainUntilClosed(t, live)...)
	want := []InteractionStateEvent{
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateThinking},
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateToolCalling},
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateGenerating},
		{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateSuccess},
	}
	assertStateEvents(t, stateEventsOf(t, all), want)

	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseDone)
	// 落库回放：state 事件与流式一致（可续播）。
	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	assertStateEvents(t, persistedStateEvents(t, events), want)
}

// TestRunHub_StateEventFailedAndCancelled 表驱动测试失败 / 中止的终局 state 口径：运行出错 →
// paused/failed，用户中止 → paused/aborted（与 StateOf 的暂停子状态同源）。
func TestRunHub_StateEventFailedAndCancelled(t *testing.T) {
	cases := []struct {
		name      string
		exec      RunExecutor
		session   uint
		want      InteractionStateEvent
		wantPhase ai.AgentRunPhase
	}{
		{
			name:      "failed",
			exec:      &hubExecutor{release: make(chan struct{}), err: errors.New("boom")},
			session:   testSessionID(71),
			want:      InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateFailed},
			wantPhase: ai.AgentRunPhaseFailed,
		},
		{
			name:      "cancelled",
			exec:      &hubExecutor{deltas: []string{"d"}, blockOnCtx: true, release: make(chan struct{})},
			session:   testSessionID(72),
			want:      InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateAborted},
			wantPhase: ai.AgentRunPhaseCancelled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hub, store := newTestHub(t, tc.exec)
			ctx := context.Background()
			if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, tc.session, testUserID, "default", nil); err != nil {
				t.Fatalf("Start error = %v", err)
			}
			_, live, cancel, err := hub.Subscribe(ctx, tc.session, testUserID, 0)
			if err != nil {
				t.Fatalf("Subscribe error = %v", err)
			}
			defer cancel()
			release := tc.exec.(*hubExecutor).release
			if tc.name == "cancelled" {
				// 先取到首条增量（进入正文生成中）再中止，确保取消发生在运行中。
				close(release)
				readContentEvents(t, live, 1)
				if err := hub.Cancel(ctx, tc.session, testUserID, "default"); err != nil {
					t.Fatalf("Cancel error = %v", err)
				}
			} else {
				close(release)
			}
			rest := drainUntilClosed(t, live)
			if got := terminalStateOf(t, rest); got != tc.want {
				t.Errorf("terminal state event = %+v, want %+v", got, tc.want)
			}
			waitLiveRunPhase(t, store, tc.session, tc.wantPhase)
		})
	}
}

// TestRunHub_StateEventSuspended 测试挂起终局的 state 子状态按挂起调用种类区分：问卷类 ask_user →
// waiting/answer，其余工具 → waiting/approval（与 SessionStore.PendingRunSubstate 同源）。
func TestRunHub_StateEventSuspended(t *testing.T) {
	cases := []struct {
		name string
		tool string
		want InteractionStateEvent
	}{
		{"approval", "api_invoke", InteractionStateEvent{State: ai.AgentInteractionStateWaiting, Substate: ai.AgentInteractionSubstateApproval}},
		{"answer", askUserToolName, InteractionStateEvent{State: ai.AgentInteractionStateWaiting, Substate: ai.AgentInteractionSubstateAnswer}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &suspendExecutor{calls: []ToolRequest{{CallID: "c1", ToolName: tc.tool, Arguments: "{}"}}}
			hub, store := newTestHub(t, exec)
			ctx := context.Background()
			sid := testSessionID(80 + i)
			runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil)
			if err != nil {
				t.Fatalf("Start error = %v", err)
			}
			// 挂起运行随即收敛（执行器发完工具请求即返回），直接从数据库取终局 state，避免订阅竞态。
			waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseSuspended)
			events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
			if err != nil {
				t.Fatalf("ListRunEventsAfter error = %v", err)
			}
			states := persistedStateEvents(t, events)
			if last := states[len(states)-1]; last != tc.want {
				t.Errorf("terminal state event = %+v, want %+v", last, tc.want)
			}
		})
	}
}

// TestRunHub_CompressReporterDrivesSubstate 测试压缩上报驱动子状态流转：compress_start/delta →
// compacting，compress_done → 回到 generating；随后的终局为 paused/success。
func TestRunHub_CompressReporterDrivesSubstate(t *testing.T) {
	exec := &compressExecutor{events: []CompressEvent{
		{Type: ai.AgentRunEventCompressStart, Reason: "auto"},
		{Type: ai.AgentRunEventCompressDelta, Delta: "s"},
		{Type: ai.AgentRunEventCompressDone, FoldedCount: 1},
	}}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(83)
	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseDone)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	want := []InteractionStateEvent{
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateThinking},
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateCompacting},
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateGenerating},
		{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateSuccess},
	}
	assertStateEvents(t, persistedStateEvents(t, events), want)
}

// TestRunHub_ActivityInitialSubstateCompacting 测试通用在途活动（如手动压缩）的初始与终局 state：
// 初始为 running/compacting（上下文压缩中），活动成功收敛后为 paused/success。
func TestRunHub_ActivityInitialSubstateCompacting(t *testing.T) {
	hub, store := newTestHub(t, &hubExecutor{})
	ctx := context.Background()
	sid := testSessionID(86)
	runID, err := hub.StartActivity(NewDetachedContext(ctx), ActivityInput{
		SessionID: sid, UserID: testUserID, Namespace: "default",
		Run: func(ctx context.Context, report func(typ ai.AgentRunEventType, payload any) error) error {
			return nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("StartActivity error = %v", err)
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseDone)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	want := []InteractionStateEvent{
		{State: ai.AgentInteractionStateRunning, Substate: ai.AgentInteractionSubstateCompacting},
		{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateSuccess},
	}
	assertStateEvents(t, persistedStateEvents(t, events), want)
}

// TestRunHub_ActivityCancelAbortsWithoutContent 测试通用在途活动（如手动压缩）被取消时的口径：取消
// 生效后不再下发任何内容类事件（尤其无 compress_done）、不产生 done，终局 state 为 paused/aborted，
// 且活动登记收敛为 cancelled——即「压缩中中止不留半截检查点、无 compress_done」的领域层依据。
func TestRunHub_ActivityCancelAbortsWithoutContent(t *testing.T) {
	hub, store := newTestHub(t, &hubExecutor{})
	ctx := context.Background()
	sid := testSessionID(87)
	if _, err := hub.StartActivity(NewDetachedContext(ctx), ActivityInput{
		SessionID: sid, UserID: testUserID, Namespace: "default",
		Run: func(ctx context.Context, report func(typ ai.AgentRunEventType, payload any) error) error {
			// 取消前上报 compress_start（应下发），随后阻塞等待取消。
			if err := report(ai.AgentRunEventCompressStart, CompressEvent{Type: ai.AgentRunEventCompressStart, Reason: "manual"}); err != nil {
				return errors.WithStack(err)
			}
			<-ctx.Done()
			// 取消后再尝试上报 compress_done：应被取消前置条件抑制（内容类事件不再放行）。
			_ = report(ai.AgentRunEventCompressDone, CompressEvent{Type: ai.AgentRunEventCompressDone, FoldedCount: 1})
			return ctx.Err()
		},
	}, nil); err != nil {
		t.Fatalf("StartActivity error = %v", err)
	}
	replay, live, cancel, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()

	// 先收集回放，并在实时通道上等待 compress_start 到达，确保在活动真正运行中再取消。
	all := append([]Published{}, replay...)
	sawStart := false
	for _, ev := range all {
		if ev.Type == ai.AgentRunEventCompressStart {
			sawStart = true
		}
	}
	for !sawStart {
		select {
		case ev, ok := <-live:
			if !ok {
				t.Fatal("live channel closed before compress_start arrived")
			}
			all = append(all, ev)
			if ev.Type == ai.AgentRunEventCompressStart {
				sawStart = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("compress_start was not observed before cancellation")
		}
	}

	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	all = append(all, drainUntilClosed(t, live)...)

	for _, ev := range all {
		if ev.Type == ai.AgentRunEventCompressDone {
			t.Errorf("received compress_done after cancellation: seq=%d", ev.Seq)
		}
	}
	if got := terminalStateOf(t, all); got != (InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateAborted}) {
		t.Errorf("terminal state = %+v, want paused/aborted", got)
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)
}

// TestRunHub_BroadcastInteractionState 测试广播端点：无内存运行（或属主不符）时为 no-op 且不报错；
// 存在内存运行送达订阅者一条 state 事件（供事实变更后重算广播）。
func TestRunHub_BroadcastInteractionState(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true, release: make(chan struct{})}
	hub, _ := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(84)
	want := InteractionStateEvent{State: ai.AgentInteractionStateWaiting, Substate: ai.AgentInteractionSubstateApproval}

	// 无内存运行：no-op。
	if err := hub.BroadcastInteractionState(ctx, testSessionID(999), testUserID, want); err != nil {
		t.Fatalf("BroadcastInteractionState(no run) error = %v, want nil", err)
	}

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancel, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	close(exec.release)

	// 属主不符：同样 no-op。
	if err := hub.BroadcastInteractionState(ctx, sid, testOtherUserID, want); err != nil {
		t.Fatalf("BroadcastInteractionState(other user) error = %v, want nil", err)
	}
	if err := hub.BroadcastInteractionState(ctx, sid, testUserID, want); err != nil {
		t.Fatalf("BroadcastInteractionState error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for delivered := false; !delivered; {
		select {
		case ev, ok := <-live:
			if !ok {
				t.Fatal("live channel closed before the broadcast arrived")
			}
			if ev.Type != ai.AgentRunEventState {
				continue
			}
			var got InteractionStateEvent
			if err := json.Unmarshal(ev.Payload, &got); err != nil {
				t.Fatalf("unmarshal broadcast state payload error = %v", err)
			}
			if got != want {
				t.Errorf("broadcast state event = %+v, want %+v", got, want)
			}
			delivered = true
		case <-time.After(time.Until(deadline)):
			t.Fatal("broadcast state event was not delivered in time")
		}
	}
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = hub.Shutdown(shutdownCtx)
}

// TestRunHub_LiveRunSubstate 测试 LiveRun 快照的子状态字段：内存分支随上报流转（thinking →
// generating），运行收敛后回落数据库分支时子状态为空串（数据库登记不承载子状态）。
func TestRunHub_LiveRunSubstate(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"a"}, blockOnCtx: true, release: make(chan struct{})}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(85)
	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancel, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	close(exec.release)

	// 内存分支：随正文增量上报从 thinking 流转到 generating。
	deadline := time.Now().Add(2 * time.Second)
	var st *LiveRunStatus
	for {
		st, err = hub.LiveRun(ctx, sid, testUserID)
		if err == nil && st.InMemory && st.Substate == ai.AgentInteractionSubstateGenerating {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("LiveRun.Substate did not reach generating in time (last %+v)", st)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 收敛后回落数据库分支：不携带子状态。
	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)
	st, err = hub.LiveRun(ctx, sid, testUserID)
	if err != nil {
		t.Fatalf("LiveRun(after finish) error = %v", err)
	}
	if st.InMemory {
		t.Errorf("LiveRun after finish = in-memory snapshot, want the DB fallback")
	}
	if st.Substate != "" {
		t.Errorf("LiveRun.Substate (DB fallback) = %q, want empty", st.Substate)
	}
}
