package ai

import (
	// 标准库
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// testSessionID 由小整数派生一个确定性的会话键，供测试使用（Start / Subscribe / Cancel 复用
// 同一值，保证键一致）。
func testSessionID(n int) uint {
	return uint(n)
}

// testUserID 是 RunHub 测试使用的属主用户 ID；跨用户隔离用例另用 testOtherUserID 验证越权否定。
const (
	testUserID      = uint(1)
	testOtherUserID = uint(2)
)

// hubExecutor 是 RunHub 测试用的对话执行器替身：可选在发事件前阻塞于 release（使运行在内存中
// 停留以便测试订阅回放），或发出工具请求 / 增量后挂起等待 ctx 取消（用于测试取消、退订宽限与
// 最大时长）；未被拦截时发出 done 完成项收敛运行。
type hubExecutor struct {
	deltas       []string      // 依次发出的增量文本
	reasonings   []string      // 依次发出的思考过程增量文本
	toolCalls    []ToolRequest // 发增量前先发出的客户端工具请求
	release      chan struct{} // 非 nil 时发事件前等待其关闭
	blockOnCtx   bool          // 为真时发完工具请求 / 增量后挂起等待 ctx 取消
	ignoreCancel bool          // blockOnCtx 且为真时忽略 ctx 取消，改等 hold 关闭（模拟无法收敛的运行）
	hold         chan struct{} // ignoreCancel 时的释放通道
	err          error         // 非 nil 时在发出 done 前以此错误收尾
}

// RunDetached 按配置顺序发出工具请求、增量与思考过程增量，随后视配置挂起或发出 done 收敛。
func (e *hubExecutor) RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	if e.release != nil {
		<-e.release
	}
	for _, req := range e.toolCalls {
		if onToolRequest != nil {
			if err := onToolRequest(req); err != nil {
				return err
			}
		}
	}
	for _, d := range e.deltas {
		if onEvent != nil {
			if err := onEvent(AgentOutput{Delta: d}); err != nil {
				return err
			}
		}
	}
	for _, r := range e.reasonings {
		if onEvent != nil {
			if err := onEvent(AgentOutput{Reasoning: r}); err != nil {
				return err
			}
		}
	}
	if e.blockOnCtx {
		if e.ignoreCancel {
			if e.hold != nil {
				<-e.hold
			}
			return e.err
		}
		<-ctx.Done()
		return ctx.Err()
	}
	if e.err != nil {
		return e.err
	}
	if onEvent != nil {
		return onEvent(AgentOutput{Done: true, Content: strings.Join(e.deltas, ""), Reasoning: strings.Join(e.reasonings, "")})
	}
	return nil
}

// fakeStore 是 RunStore 的内存替身：以内存 map 保存运行登记与事件日志。用它避免 sqlite 对
// TIMESTAMPTZ 列的时间扫描限制（真实 *SessionStore 在 Postgres 下等价）。
type fakeStore struct {
	mu     sync.Mutex
	runs   map[serializer.HexUUID]*ai.AgentLiveRun
	order  []serializer.HexUUID
	events map[serializer.HexUUID][]*ai.AgentRunEvent
}

// newFakeStore 构造空的运行持久化替身。
func newFakeStore() *fakeStore {
	return &fakeStore{
		runs:   map[serializer.HexUUID]*ai.AgentLiveRun{},
		events: map[serializer.HexUUID][]*ai.AgentRunEvent{},
	}
}

// AppendRunEvent 追加一条事件到对应运行的日志。
func (s *fakeStore) AppendRunEvent(ctx context.Context, ev *ai.AgentRunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[ev.RunID] = append(s.events[ev.RunID], ev)
	return nil
}

// ListRunEventsAfter 返回某运行中序号大于 after 的事件（按 seq 升序），并限定属主用户。
func (s *fakeStore) ListRunEventsAfter(ctx context.Context, runID serializer.HexUUID, userID uint, after uint64, limit int) ([]*ai.AgentRunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ai.AgentRunEvent
	for _, ev := range s.events[runID] {
		if ev.UserID == userID && ev.Seq > after {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// CreateLiveRun 登记一条驻留运行；若同一 (session_id, user_id) 已有 running 登记则返回
// ErrAgentLiveRunAlreadyRunning（对应 uq_agent_live_runs_running 部分唯一索引的 ON CONFLICT 未写入）。
func (s *fakeStore) CreateLiveRun(ctx context.Context, lr *ai.AgentLiveRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.RunID != lr.RunID && existing.SessionID == lr.SessionID &&
			existing.UserID == lr.UserID && existing.Phase == ai.AgentRunPhaseRunning {
			return ErrAgentLiveRunAlreadyRunning
		}
	}
	s.runs[lr.RunID] = lr
	s.order = append(s.order, lr.RunID)
	return nil
}

// FinishLiveRun 把驻留运行收敛到终态并写入最终序号（限定属主用户）。
func (s *fakeStore) FinishLiveRun(ctx context.Context, runID serializer.HexUUID, userID uint, phase ai.AgentRunPhase, lastSeq uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lr, ok := s.runs[runID]; ok && lr.UserID == userID {
		lr.Phase = phase
		lr.LastSeq = lastSeq
	}
	return nil
}

// LatestLiveRun 返回某会话最近一条运行登记（限定属主用户）；不存在时返回 ErrAgentRunNotFound。
func (s *fakeStore) LatestLiveRun(ctx context.Context, sessionID, userID uint) (*ai.AgentLiveRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *ai.AgentLiveRun
	for _, runID := range s.order {
		if lr := s.runs[runID]; lr.SessionID == sessionID && lr.UserID == userID {
			latest = lr
		}
	}
	if latest == nil {
		return nil, ErrAgentRunNotFound
	}
	return latest, nil
}

// phaseOf 返回某会话最近一条运行登记的阶段（无登记时返回空串），供测试轮询收敛结果。
func (s *fakeStore) phaseOf(sessionID uint) ai.AgentRunPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.order) - 1; i >= 0; i-- {
		if lr := s.runs[s.order[i]]; lr.SessionID == sessionID {
			return lr.Phase
		}
	}
	return ""
}

// runningCount 返回某会话当前仍为 running 的登记条数，供取代用例断言不残留第二个在途运行。
func (s *fakeStore) runningCount(sessionID uint) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, lr := range s.runs {
		if lr.SessionID == sessionID && lr.Phase == ai.AgentRunPhaseRunning {
			n++
		}
	}
	return n
}

// phaseOfRun 返回指定 runID 的登记阶段（无登记时返回空串），供取代用例按运行标识断言而非取最新。
func (s *fakeStore) phaseOfRun(runID serializer.HexUUID) ai.AgentRunPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lr, ok := s.runs[runID]; ok {
		return lr.Phase
	}
	return ""
}

// waitRunPhase 轮询等待指定 runID 的登记收敛到指定阶段（超时即失败）。
func waitRunPhase(t *testing.T, store *fakeStore, runID serializer.HexUUID, phase ai.AgentRunPhase) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.phaseOfRun(runID) == phase {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("live run %s did not reach phase %q in time (last %q)", runID, phase, store.phaseOfRun(runID))
}

// inMemoryRunID 返回 RunHub 内存中该会话当前在途运行的 runID（无则返回零值），供取代用例断言。
func inMemoryRunID(hub *RunHub, sessionID uint) serializer.HexUUID {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if run := hub.runs[sessionID]; run != nil {
		return run.runID
	}
	return serializer.NewNilHexUUIDv4()
}

// newTestHub 构造注入了替身执行器与内存运行持久化的 RunHub。
func newTestHub(t *testing.T, exec RunExecutor) (*RunHub, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	params := testParams(t, nil)
	hub := NewRunHub(RunHubDeps{
		Agent:  func() RunExecutor { return exec },
		Params: func() *Params { return params },
		Store:  func() RunStore { return store },
	})
	return hub, store
}

// intPtr 返回指向给定整数的指针（测试覆盖 RunHub 的退订宽限秒数）。
func intPtr(v int) *int { return &v }

// waitLiveRunPhase 轮询等待某会话的驻留运行收敛到指定阶段（超时即失败）。
func waitLiveRunPhase(t *testing.T, store *fakeStore, sessionID uint, phase ai.AgentRunPhase) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.phaseOf(sessionID) == phase {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("live run of session %d did not reach phase %q in time (last %q)", sessionID, phase, store.phaseOf(sessionID))
}

// TestRunHub_ReplayAndLive 测试 Start 启动运行后 Subscribe 依次收到实时转发与完成收敛：事件序号
// 单调自增、done 事件携带聚合正文，事件落库且运行时登记收敛到 done。
func TestRunHub_ReplayAndLive(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"a", "b", "c"}, release: make(chan struct{})}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(11), testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	replay, live, cancel, err := hub.Subscribe(ctx, testSessionID(11), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	if len(replay) != 0 {
		t.Fatalf("replay before release len = %d, want 0", len(replay))
	}
	close(exec.release)

	var got []Published
	for ev := range live {
		got = append(got, ev)
	}
	if len(got) != 4 {
		t.Fatalf("live events len = %d, want 4", len(got))
	}
	for i, ev := range got {
		if ev.Seq != uint64(i+1) {
			t.Errorf("event[%d].Seq = %d, want %d", i, ev.Seq, i+1)
		}
		if i < 3 && ev.Type != ai.AgentRunEventDelta {
			t.Errorf("event[%d].Type = %q, want delta", i, ev.Type)
		}
	}
	if got[3].Type != ai.AgentRunEventDone || !strings.Contains(string(got[3].Payload), "abc") {
		t.Errorf("done event = %+v, want the aggregated content", got[3])
	}

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	if len(events) != 4 {
		t.Errorf("persisted events len = %d, want 4", len(events))
	}
	waitLiveRunPhase(t, store, testSessionID(11), ai.AgentRunPhaseDone)
}

// TestRunHub_ReasoningReplayAndPersist 测试思考过程事件经发布、落库与回放一致：reasoning 增量事件
// 与 delta 事件同序发布并落库，done 事件仍照常发布（其负载内含思考全文，不因 reasoning 非空被吞），
// 运行收敛到 done；运行结束后回落数据库回放同样含 reasoning 事件。
func TestRunHub_ReasoningReplayAndPersist(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"a"}, reasonings: []string{"t1", "t2"}, release: make(chan struct{})}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(40), testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancel, err := hub.Subscribe(ctx, testSessionID(40), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	close(exec.release)

	var got []Published
	for ev := range live {
		got = append(got, ev)
	}
	if len(got) != 4 {
		t.Fatalf("live events len = %d, want 4 (delta + 2 reasoning + done)", len(got))
	}
	if got[0].Type != ai.AgentRunEventDelta {
		t.Errorf("event[0].Type = %q, want delta", got[0].Type)
	}
	for i, want := range []string{"t1", "t2"} {
		ev := got[1+i]
		if ev.Type != ai.AgentRunEventReasoning || !strings.Contains(string(ev.Payload), want) {
			t.Errorf("event[%d] = %+v, want a reasoning event carrying %q", 1+i, ev, want)
		}
	}
	if got[3].Type != ai.AgentRunEventDone {
		t.Errorf("event[3].Type = %q, want done (the terminal event must not be swallowed)", got[3].Type)
	}
	waitLiveRunPhase(t, store, testSessionID(40), ai.AgentRunPhaseDone)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("persisted events len = %d, want 4", len(events))
	}
	if events[1].Type != ai.AgentRunEventReasoning || events[2].Type != ai.AgentRunEventReasoning {
		t.Errorf("persisted events[1:3] = %+v, want reasoning events", events[1:3])
	}

	replay, _, cancel2, err := hub.Subscribe(ctx, testSessionID(40), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe(from DB) error = %v", err)
	}
	defer cancel2()
	replayed := 0
	for _, ev := range replay {
		if ev.Type == ai.AgentRunEventReasoning {
			replayed++
		}
	}
	if replayed != 2 {
		t.Errorf("replayed reasoning events = %d, want 2", replayed)
	}
}

// TestRunHub_ReplayFromDBAfterFinish 测试运行收敛并从内存移除后，Subscribe 回落数据库回放已落库
// 事件（实时通道立即关闭，表示无后续事件），且仅回放 seq > after 的事件。
func TestRunHub_ReplayFromDBAfterFinish(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"x", "y"}}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(12), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	waitLiveRunPhase(t, store, testSessionID(12), ai.AgentRunPhaseDone)

	replay, live, cancel, err := hub.Subscribe(ctx, testSessionID(12), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe(from DB) error = %v", err)
	}
	defer cancel()
	if len(replay) != 3 {
		t.Fatalf("replay from DB len = %d, want 3", len(replay))
	}
	if _, ok := <-live; ok {
		t.Error("live channel of a finished run should be closed")
	}
	replay, _, _, err = hub.Subscribe(ctx, testSessionID(12), testUserID, 2)
	if err != nil {
		t.Fatalf("Subscribe(from DB, after=2) error = %v", err)
	}
	if len(replay) != 1 {
		t.Errorf("replay(after=2) len = %d, want 1", len(replay))
	}
}

// TestRunHub_SubscribeNotFound 测试订阅无任何运行登记的会话返回 ErrAgentLiveRunNotFound。
func TestRunHub_SubscribeNotFound(t *testing.T) {
	hub, _ := newTestHub(t, &hubExecutor{})
	if _, _, _, err := hub.Subscribe(context.Background(), testSessionID(99), testUserID, 0); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("Subscribe(no run) error = %v, want ErrAgentLiveRunNotFound", err)
	}
}

// TestRunHub_Cancel 测试显式取消：取消后运行收敛到 cancelled；重复取消幂等；命名空间不符返回
// ErrAgentLiveRunForbidden；不存在的会话返回 ErrAgentLiveRunNotFound。
func TestRunHub_Cancel(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(13), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancel, err := hub.Subscribe(ctx, testSessionID(13), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	if err := hub.Cancel(ctx, testSessionID(13), testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	if err := hub.Cancel(ctx, testSessionID(13), testUserID, "default"); err != nil {
		t.Fatalf("Cancel(again) error = %v, want nil (idempotent)", err)
	}
	if err := hub.Cancel(ctx, testSessionID(13), testUserID, "other"); !errors.Is(err, ErrAgentLiveRunForbidden) {
		t.Errorf("Cancel(wrong namespace) error = %v, want ErrAgentLiveRunForbidden", err)
	}
	for range live {
	}
	waitLiveRunPhase(t, store, testSessionID(13), ai.AgentRunPhaseCancelled)
	if err := hub.Cancel(ctx, testSessionID(77), testUserID, "default"); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("Cancel(no run) error = %v, want ErrAgentLiveRunNotFound", err)
	}
}

// TestRunHub_GraceCancelsDetachedRun 测试退订宽限：最后一名订阅者退订后经过宽限期仍无订阅者时
// 运行被自动取消（宽限秒数经测试覆盖提速）。
func TestRunHub_GraceCancelsDetachedRun(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, store := newTestHub(t, exec)
	hub.graceOverride = intPtr(1)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(14), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancel, err := hub.Subscribe(ctx, testSessionID(14), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	cancel() // 退订：启动退订宽限定时器
	waitLiveRunPhase(t, store, testSessionID(14), ai.AgentRunPhaseCancelled)
}

// TestRunHub_MaxDuration 测试单次运行最大时长兜底：脱管运行丢失请求 deadline 后超时以失败收尾。
func TestRunHub_MaxDuration(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, store := newTestHub(t, exec)
	d := 100 * time.Millisecond
	hub.maxOverride = &d
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(15), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancel, err := hub.Subscribe(ctx, testSessionID(15), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	for range live {
	}
	waitLiveRunPhase(t, store, testSessionID(15), ai.AgentRunPhaseFailed)
}

// TestRunHub_LiveRunSnapshot 测试 LiveRun 状态快照：运行中携带工具请求重建的待执行调用列表，
// 待审批阶段（suspended）由前端据此重建现场。
func TestRunHub_LiveRunSnapshot(t *testing.T) {
	exec := &hubExecutor{
		toolCalls:  []ToolRequest{{CallID: "c1", ToolName: "api_invoke", Arguments: "{}"}},
		blockOnCtx: true,
	}
	hub, _ := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(16), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancel, err := hub.Subscribe(ctx, testSessionID(16), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := hub.LiveRun(ctx, testSessionID(16), testUserID)
		if err == nil && len(st.PendingCalls) == 1 {
			if st.PendingCalls[0].CallID != "c1" || st.PendingCalls[0].ToolName != "api_invoke" {
				t.Errorf("pending call = %+v, want the tool request published", st.PendingCalls[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("LiveRun did not expose the pending call in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRunHub_Shutdown 测试关闭：取消全部在途运行并等待终态，关闭后拒绝新运行。
func TestRunHub_Shutdown(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, _ := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(17), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancel, err := hub.Subscribe(ctx, testSessionID(17), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := hub.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(18), testUserID, "default", nil); err == nil {
		t.Error("Start after Shutdown = nil error, want a rejection")
	}
}

// TestRunHub_CrossUserIsolation 测试跨用户隔离：以属主启动的在途运行对其它用户不可见 —— 订阅、
// 取消、状态查询均视为不存在（内存运行属主不符时回落数据库，数据库同样按属主过滤）。
func TestRunHub_CrossUserIsolation(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, _ := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(20), testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancel, err := hub.Subscribe(ctx, testSessionID(20), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe(owner) error = %v", err)
	}
	defer cancel()
	if _, _, _, err := hub.Subscribe(ctx, testSessionID(20), testOtherUserID, 0); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("Subscribe(other user) error = %v, want ErrAgentLiveRunNotFound", err)
	}
	if _, err := hub.LiveRun(ctx, testSessionID(20), testOtherUserID); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("LiveRun(other user) error = %v, want ErrAgentLiveRunNotFound", err)
	}
	if err := hub.Cancel(ctx, testSessionID(20), testOtherUserID, "default"); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("Cancel(other user) error = %v, want ErrAgentLiveRunNotFound", err)
	}
}

// TestRunHub_SupersedeInFlightRun 测试新提问取代在途运行：同一会话再次 Start 时，先到的在途运行
// 被取消并收敛（不残留），内存登记与返回的 runID 以新运行为准。
func TestRunHub_SupersedeInFlightRun(t *testing.T) {
	exec := &hubExecutor{blockOnCtx: true}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	runID1, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q1"}, testSessionID(30), testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start(run1) error = %v", err)
	}

	runID2, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q2"}, testSessionID(30), testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start(run2) error = %v", err)
	}
	if runID2 == runID1 {
		t.Fatal("Start(run2) returned the same runID as run1")
	}
	// run1 已被取消并收敛到 cancelled，且内存登记指向 run2。
	waitRunPhase(t, store, runID1, ai.AgentRunPhaseCancelled)
	if got := inMemoryRunID(hub, testSessionID(30)); got != runID2 {
		t.Errorf("in-memory run = %s, want the superseding run %s", got, runID2)
	}
	if n := store.runningCount(testSessionID(30)); n != 1 {
		t.Errorf("running registrations = %d, want 1 (only the superseding run)", n)
	}
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = hub.Shutdown(shutdownCtx)
}

// TestRunHub_SupersedeSuspendedRun 测试取代挂起中的运行：执行器先发 tool_request 再阻塞于 ctx
// （模拟挂起），第二次 Start 取代它，断言不残留第二个 running 登记。
func TestRunHub_SupersedeSuspendedRun(t *testing.T) {
	exec := &hubExecutor{
		toolCalls:  []ToolRequest{{CallID: "c1", ToolName: "api_invoke", Arguments: "{}"}},
		blockOnCtx: true,
	}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q1"}, testSessionID(31), testUserID, "default", nil); err != nil {
		t.Fatalf("Start(run1) error = %v", err)
	}
	waitLiveRunPhase(t, store, testSessionID(31), ai.AgentRunPhaseRunning)
	// 等待挂起现场（tool_request）落库，确保 run1 确已到挂起态。
	deadline := time.Now().Add(2 * time.Second)
	var runID1 serializer.HexUUID
	for {
		lr, err := store.LatestLiveRun(ctx, testSessionID(31), testUserID)
		if err == nil {
			runID1 = lr.RunID
			if events, _ := store.ListRunEventsAfter(ctx, lr.RunID, testUserID, 0, 0); len(events) >= 1 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("run1 did not publish its tool request in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q2"}, testSessionID(31), testUserID, "default", nil); err != nil {
		t.Fatalf("Start(run2) error = %v", err)
	}
	waitRunPhase(t, store, runID1, ai.AgentRunPhaseCancelled)
	if n := store.runningCount(testSessionID(31)); n != 1 {
		t.Errorf("running registrations = %d, want 1 (no residual suspended run)", n)
	}
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = hub.Shutdown(shutdownCtx)
}

// TestRunHub_SupersedeTimeoutConflict 测试并发登记的兜底：被取代运行忽略取消、在收敛时限内未走到
// 终态，导致新运行的 CreateLiveRun 撞上唯一约束冲突；此时 Start 返回明确冲突错误且不残留悬空
// 内存登记。
func TestRunHub_SupersedeTimeoutConflict(t *testing.T) {
	hold := make(chan struct{})
	exec := &hubExecutor{blockOnCtx: true, ignoreCancel: true, hold: hold}
	hub, store := newTestHub(t, exec)
	d := 50 * time.Millisecond
	hub.supersedeOverride = &d
	ctx := context.Background()

	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q1"}, testSessionID(32), testUserID, "default", nil); err != nil {
		t.Fatalf("Start(run1) error = %v", err)
	}
	waitLiveRunPhase(t, store, testSessionID(32), ai.AgentRunPhaseRunning)

	_, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q2"}, testSessionID(32), testUserID, "default", nil)
	if err == nil {
		t.Fatal("Start(run2) = nil error, want an explicit in-flight conflict")
	}
	if !strings.Contains(err.Error(), "already has an in-flight live run") {
		t.Errorf("Start(run2) error = %v, want the explicit in-flight conflict", err)
	}
	if got := inMemoryRunID(hub, testSessionID(32)); got != serializer.NewNilHexUUIDv4() {
		t.Errorf("in-memory run = %s, want it cleared after the failed registration", got)
	}

	// 放行被取代运行，令其收敛，避免 goroutine 泄漏。
	close(hold)
	shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	_ = hub.Shutdown(shutdownCtx)
}
