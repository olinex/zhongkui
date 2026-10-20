package ai

import (
	// 标准库
	"context"
	"encoding/json"
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
	deltas       []string            // 依次发出的增量文本
	reasonings   []string            // 依次发出的思考过程增量文本
	blockGroups  [][]AgentBlockEvent // 依次发出的内容块事件组（每组作为一个仅含 Blocks 的流项下发）
	toolCalls    []ToolRequest       // 发增量前先发出的客户端工具请求
	release      chan struct{}       // 非 nil 时发事件前等待其关闭
	blockOnCtx   bool                // 为真时发完工具请求 / 增量后挂起等待 ctx 取消
	ignoreCancel bool                // blockOnCtx 且为真时忽略 ctx 取消，改等 hold 关闭（模拟无法收敛的运行）
	hold         chan struct{}       // ignoreCancel 时的释放通道
	err          error               // 非 nil 时在发出 done 前以此错误收尾
	postCancel   []AgentOutput       // blockOnCtx 时：ctx 取消后仍尝试外发的流项（应被取消前置条件抑制）
}

// RunDetached 按配置顺序发出客户端工具请求、内容块事件组、增量与思考过程增量，随后视配置挂起或发出
// done 收敛。
func (e *hubExecutor) RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	if e.release != nil {
		<-e.release
	}
	for _, req := range e.toolCalls {
		if onToolRequest != nil {
			if err := onToolRequest(req); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	for _, grp := range e.blockGroups {
		if onEvent != nil {
			if err := onEvent(AgentOutput{Blocks: grp}); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	for _, d := range e.deltas {
		if onEvent != nil {
			if err := onEvent(AgentOutput{Delta: d}); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	for _, r := range e.reasonings {
		if onEvent != nil {
			if err := onEvent(AgentOutput{Reasoning: r}); err != nil {
				return errors.WithStack(err)
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
		// 取消后再尝试外发配置的流项：用于验证取消前置条件确实抑制内容类事件（这些项不应被订阅者收到）。
		for _, out := range e.postCancel {
			if onEvent != nil {
				if err := onEvent(out); err != nil {
					return errors.WithStack(err)
				}
			}
		}
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

// PrepareResume 是 ResumeInput → AgentInput 的桩实现：原样透传会话 / 命名空间 / 属主用户后返回，供
// StartResume 用例验证「认领 + 登记 + 执行 + 收敛」全链路（真实认领逻辑由 *Agent 承担）。
func (e *hubExecutor) PrepareResume(ctx context.Context, in ResumeInput) (AgentInput, error) {
	return AgentInput{SessionID: in.SessionID, NamespaceName: in.NamespaceName, UserID: in.UserID}, nil
}

// ResumeDetached 复用 RunDetached 的事件编排：续跑与首轮生成在 RunHub 侧的事件语义一致。
func (e *hubExecutor) ResumeDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	return e.RunDetached(ctx, in, onEvent, onToolRequest)
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
	// 释放前缓冲中仅有 Start 发布的初始 state 事件，不含任何内容事件。
	if len(nonStateEvents(replay)) != 0 {
		t.Fatalf("replay before release len = %d, want 0 content events", len(nonStateEvents(replay)))
	}
	close(exec.release)

	var got []Published
	for ev := range live {
		got = append(got, ev)
	}
	got = nonStateEvents(got)
	if len(got) != 4 {
		t.Fatalf("live content events len = %d, want 4", len(got))
	}
	for i, ev := range got {
		if i > 0 && ev.Seq <= got[i-1].Seq {
			t.Errorf("event[%d].Seq = %d, want strictly increasing from %d", i, ev.Seq, got[i-1].Seq)
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
	if len(nonStateRunEvents(events)) != 4 {
		t.Errorf("persisted content events len = %d, want 4", len(nonStateRunEvents(events)))
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
	got = nonStateEvents(got)
	if len(got) != 4 {
		t.Fatalf("live content events len = %d, want 4 (delta + 2 reasoning + done)", len(got))
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
	content := nonStateRunEvents(events)
	if len(content) != 4 {
		t.Fatalf("persisted content events len = %d, want 4", len(content))
	}
	if content[1].Type != ai.AgentRunEventReasoning || content[2].Type != ai.AgentRunEventReasoning {
		t.Errorf("persisted content events[1:3] = %+v, want reasoning events", content[1:3])
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
	if len(nonStateEvents(replay)) != 3 {
		t.Fatalf("replay content events from DB len = %d, want 3", len(nonStateEvents(replay)))
	}
	if _, ok := <-live; ok {
		t.Error("live channel of a finished run should be closed")
	}
	// after=4 越过初始 state 事件、子状态流转 state 事件与首条增量，仅回放其后的内容事件（done）。
	replay, _, _, err = hub.Subscribe(ctx, testSessionID(12), testUserID, 4)
	if err != nil {
		t.Fatalf("Subscribe(from DB, after=4) error = %v", err)
	}
	if len(nonStateEvents(replay)) != 1 {
		t.Errorf("replay(after=4) content events len = %d, want 1", len(nonStateEvents(replay)))
	}
}

// TestRunHub_SubscribeNotFound 测试订阅无任何运行登记的会话返回 ErrAgentLiveRunNotFound。
func TestRunHub_SubscribeNotFound(t *testing.T) {
	hub, _ := newTestHub(t, &hubExecutor{})
	if _, _, _, err := hub.Subscribe(context.Background(), testSessionID(99), testUserID, 0); !errors.Is(err, ErrAgentLiveRunNotFound) {
		t.Errorf("Subscribe(no run) error = %v, want ErrAgentLiveRunNotFound", err)
	}
}

// TestRunHub_StartResumeConvergesToDone 测试续跑经 StartResume 启动后拥有独立的在途登记并在收敛时
// 写入 done：会话先留有一条 suspended 登记（模拟问卷挂起留下的现场），续跑登记不被它阻拦（部分唯一
// 索引只约束 running），收敛后最新登记的 phase 变为 done——这正是修复「刷新后据陈旧 suspended 登记
// 误判仍在等待审批、并以陈旧挂起事件回放覆盖已落库正文」的关键。
func TestRunHub_StartResumeConvergesToDone(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"r1", "r2"}}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sessionID := testSessionID(50)

	// 预置一条 suspended 登记：模拟上一回合命中 client 工具挂起后留下的现场。
	if err := store.CreateLiveRun(ctx, &ai.AgentLiveRun{
		NamespaceName: "default",
		UserID:        testUserID,
		RunID:         newAgentRunID(),
		SessionID:     sessionID,
		Phase:         ai.AgentRunPhaseSuspended,
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateLiveRun(suspended) error = %v", err)
	}

	runID, err := hub.StartResume(NewDetachedContext(ctx), ResumeInput{
		SessionID:     sessionID,
		NamespaceName: "default",
		UserID:        testUserID,
	}, sessionID, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("StartResume error = %v", err)
	}
	waitLiveRunPhase(t, store, sessionID, ai.AgentRunPhaseDone)

	latest, err := store.LatestLiveRun(ctx, sessionID, testUserID)
	if err != nil {
		t.Fatalf("LatestLiveRun error = %v", err)
	}
	if latest.RunID != runID {
		t.Errorf("LatestLiveRun.RunID = %s, want the resumed run %s", latest.RunID, runID)
	}
	if latest.Phase != ai.AgentRunPhaseDone {
		t.Errorf("LatestLiveRun.Phase = %q, want done (the suspended record must not remain the latest)", latest.Phase)
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

// drainUntilClosed 读取实时通道直到其关闭（超时即失败），返回关闭前收到的全部事件。
func drainUntilClosed(t *testing.T, live <-chan Published) []Published {
	t.Helper()
	var out []Published
	for {
		select {
		case ev, ok := <-live:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-time.After(3 * time.Second):
			t.Fatal("live channel was not closed in time; the run did not finalize")
		}
	}
}

// assertNoContentEvents 断言给定事件中不含任何内容类事件（delta / reasoning / stage / tool_request /
// compress_*），失败信息明确指出违规事件。
func assertNoContentEvents(t *testing.T, evs []Published) {
	t.Helper()
	for _, ev := range evs {
		if contentEventType(ev.Type) {
			t.Errorf("received a content event after cancellation: type=%q seq=%d payload=%s", ev.Type, ev.Seq, string(ev.Payload))
		}
	}
}

// nonStateEvents 过滤掉交互状态广播（state），仅保留内容与终局事件：state 事件由 Task 6 的
// 交互状态上报引入，既有用例聚焦内容 / 终局事件序列，过滤后仍可按原口径断言。
func nonStateEvents(evs []Published) []Published {
	out := make([]Published, 0, len(evs))
	for _, ev := range evs {
		if ev.Type != ai.AgentRunEventState {
			out = append(out, ev)
		}
	}
	return out
}

// nonStateRunEvents 过滤落库事件里的 state 事件，供既有用例按内容事件计数与索引。
func nonStateRunEvents(events []*ai.AgentRunEvent) []*ai.AgentRunEvent {
	out := make([]*ai.AgentRunEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type != ai.AgentRunEventState {
			out = append(out, ev)
		}
	}
	return out
}

// readContentEvents 从实时通道读取 n 条非 state 事件（跳过交互状态广播，每条含超时），用于在取消
// 前确定地取到已发布的内容事件（state 事件不计入读取条数）。
func readContentEvents(t *testing.T, live <-chan Published, n int) []Published {
	t.Helper()
	out := make([]Published, 0, n)
	for len(out) < n {
		select {
		case ev, ok := <-live:
			if !ok {
				t.Fatalf("live channel closed before reading content event %d of %d", len(out)+1, n)
			}
			if ev.Type == ai.AgentRunEventState {
				continue
			}
			out = append(out, ev)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for content event %d of %d", len(out)+1, n)
		}
	}
	return out
}

// assertTerminalCancelled 断言事件切片（去除 state 广播后）以一条携带 phase=cancelled 的 error
// 事件收尾：终局 state 事件在 error 之后发布，故须先滤除 state 再取末条。
func assertTerminalCancelled(t *testing.T, evs []Published) {
	t.Helper()
	content := nonStateEvents(evs)
	if len(content) == 0 {
		t.Fatal("no terminal event received")
	}
	last := content[len(content)-1]
	if last.Type != ai.AgentRunEventError || !strings.Contains(string(last.Payload), string(ai.AgentRunPhaseCancelled)) {
		t.Errorf("terminal event = %+v, want an error carrying phase=cancelled", last)
	}
}

// TestRunHub_CancelDuringGeneratingAndThinking 表驱动覆盖「正文生成中」与「思考中」两个子状态的中止：
// (a) Cancel 命中内存运行时等待执行 goroutine 真正退出后才返回（运行已从在途表移除）；
// (b) 取消后订阅者不再收到任何内容类事件（取消后试发的 late 流项被 publish 取消前置条件抑制）；
// (c) liveRun 终局收敛到 Phase=cancelled，落库事件同样不含 late 内容。
func TestRunHub_CancelDuringGeneratingAndThinking(t *testing.T) {
	cases := []struct {
		name    string
		session uint
		exec    *hubExecutor
		pre     int
		wantPre []ai.AgentRunEventType
	}{
		{
			name:    "generating",
			session: testSessionID(60),
			exec:    &hubExecutor{deltas: []string{"d1", "d2"}, blockOnCtx: true, postCancel: []AgentOutput{{Delta: "late"}}},
			pre:     2,
			wantPre: []ai.AgentRunEventType{ai.AgentRunEventDelta, ai.AgentRunEventDelta},
		},
		{
			name:    "thinking",
			session: testSessionID(61),
			exec:    &hubExecutor{reasonings: []string{"t1", "t2"}, blockOnCtx: true, postCancel: []AgentOutput{{Reasoning: "late"}}},
			pre:     2,
			wantPre: []ai.AgentRunEventType{ai.AgentRunEventReasoning, ai.AgentRunEventReasoning},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hub, store := newTestHub(t, tc.exec)
			ctx := context.Background()
			runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, tc.session, testUserID, "default", nil)
			if err != nil {
				t.Fatalf("Start error = %v", err)
			}
			replay, live, cancelSub, err := hub.Subscribe(ctx, tc.session, testUserID, 0)
			if err != nil {
				t.Fatalf("Subscribe error = %v", err)
			}
			defer cancelSub()
			if len(nonStateEvents(replay)) != 0 {
				t.Fatalf("replay content events len = %d, want 0", len(nonStateEvents(replay)))
			}

			pre := readContentEvents(t, live, tc.pre)
			for i, ev := range pre {
				if ev.Type != tc.wantPre[i] {
					t.Errorf("pre event[%d].Type = %q, want %q", i, ev.Type, tc.wantPre[i])
				}
			}

			// 取消：命中内存运行时会等待执行 goroutine 真正退出后才返回。
			if err := hub.Cancel(ctx, tc.session, testUserID, "default"); err != nil {
				t.Fatalf("Cancel error = %v", err)
			}
			// (a) Cancel 返回时运行已终局化并从在途表移除（finish 在 execute goroutine 内完成）。
			if got := inMemoryRunID(hub, tc.session); got != serializer.NewNilHexUUIDv4() {
				t.Errorf("in-memory run = %s right after Cancel, want none (the goroutine must have exited)", got)
			}
			// (b) 取消后订阅者不再收到任何内容类事件。
			rest := drainUntilClosed(t, live)
			assertNoContentEvents(t, rest)
			assertTerminalCancelled(t, rest)
			// (c) 终局 Phase=cancelled。
			waitLiveRunPhase(t, store, tc.session, ai.AgentRunPhaseCancelled)

			events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
			if err != nil {
				t.Fatalf("ListRunEventsAfter error = %v", err)
			}
			for _, ev := range events {
				if strings.Contains(string(ev.Payload.Value()), "late") {
					t.Errorf("a late content event (%q) was persisted after cancellation", string(ev.Payload.Value()))
				}
			}
		})
	}
}

// TestRunHub_CancelDuringToolCall 测试「工具调用中」中止：假执行器发完客户端工具请求后阻塞于
// ctx.Done()，取消后该阶段随取消返回、终局 cancelled，且不再下发任何内容事件。
func TestRunHub_CancelDuringToolCall(t *testing.T) {
	exec := &hubExecutor{
		toolCalls:  []ToolRequest{{CallID: "c1", ToolName: "api_invoke", Arguments: "{}"}},
		blockOnCtx: true,
		postCancel: []AgentOutput{{Delta: "late"}},
	}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(62)
	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancelSub, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancelSub()

	pre := readContentEvents(t, live, 1)
	if pre[0].Type != ai.AgentRunEventToolRequest {
		t.Errorf("pre event type = %q, want tool_request", pre[0].Type)
	}
	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	rest := drainUntilClosed(t, live)
	assertNoContentEvents(t, rest)
	assertTerminalCancelled(t, rest)
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)
}

// TestRunHub_CancelDuringCompactionActivity 测试「上下文压缩中」中止：用 StartActivity 起一个「模拟
// 压缩」活动（阻塞于 ctx.Done()，取消后试发 compress_done），取消后断言不产生半截 compress_done、
// 终局 cancelled。这为 Task 8 的真实压缩活动预置了可复用的验证方式。
func TestRunHub_CancelDuringCompactionActivity(t *testing.T) {
	hub, store := newTestHub(t, &hubExecutor{})
	ctx := context.Background()
	sid := testSessionID(63)
	_, err := hub.StartActivity(NewDetachedContext(ctx), ActivityInput{
		SessionID: sid, UserID: testUserID, Namespace: "default",
		Run: func(ctx context.Context, report func(typ ai.AgentRunEventType, payload any) error) error {
			_ = report(ai.AgentRunEventCompressStart, map[string]any{"reason": "manual"})
			<-ctx.Done()
			// 取消后仍尝试下发压缩完成：须被抑制，绝不产生半截 compress_done。
			_ = report(ai.AgentRunEventCompressDone, map[string]any{"foldedCount": 1})
			return ctx.Err()
		},
	}, nil)
	if err != nil {
		t.Fatalf("StartActivity error = %v", err)
	}
	_, live, cancelSub, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancelSub()

	pre := readContentEvents(t, live, 1)
	if pre[0].Type != ai.AgentRunEventCompressStart {
		t.Errorf("pre event type = %q, want compress_start", pre[0].Type)
	}
	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	rest := drainUntilClosed(t, live)
	for _, ev := range rest {
		if ev.Type == ai.AgentRunEventCompressDone {
			t.Errorf("received a half-written compress_done after cancellation: %+v", ev)
		}
	}
	assertNoContentEvents(t, rest)
	assertTerminalCancelled(t, rest)
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)
}

// TestRunHub_StartActivityConvergesToDone 测试通用在途活动的成功路径：Run 正常返回时由 RunHub 以
// done 事件收口，活动登记收敛到 done——证明 StartActivity 复用既有 execute 终局分支（sawDone → done）。
func TestRunHub_StartActivityConvergesToDone(t *testing.T) {
	hub, store := newTestHub(t, &hubExecutor{})
	ctx := context.Background()
	sid := testSessionID(64)
	runID, err := hub.StartActivity(NewDetachedContext(ctx), ActivityInput{
		SessionID: sid, UserID: testUserID, Namespace: "default",
		Run: func(ctx context.Context, report func(typ ai.AgentRunEventType, payload any) error) error {
			_ = report(ai.AgentRunEventCompressStart, map[string]any{"reason": "manual"})
			_ = report(ai.AgentRunEventCompressDone, map[string]any{"foldedCount": 2})
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
	content := nonStateRunEvents(events)
	if len(content) != 3 {
		t.Fatalf("persisted content events len = %d, want 3 (compress_start + compress_done + done)", len(content))
	}
	if content[0].Type != ai.AgentRunEventCompressStart || content[1].Type != ai.AgentRunEventCompressDone {
		t.Errorf("persisted content events[0:2] = %q,%q, want compress_start,compress_done", content[0].Type, content[1].Type)
	}
	if content[2].Type != ai.AgentRunEventDone {
		t.Errorf("persisted terminal event = %q, want done", content[2].Type)
	}
}

// TestRunHub_CancelReturnsNoContentEvents 明确针对「取消返回后无后续内容事件」：执行器在取消后又
// 尝试试发两条内容流项，但取消生效后它们一律被抑制；取消返回后只要有事件，只能是携带 phase=cancelled
// 的 error，绝无内容类事件。
func TestRunHub_CancelReturnsNoContentEvents(t *testing.T) {
	exec := &hubExecutor{
		deltas:     []string{"d1"},
		blockOnCtx: true,
		postCancel: []AgentOutput{{Delta: "late-a"}, {Delta: "late-b"}},
	}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(65)
	if _, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancelSub, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancelSub()

	readContentEvents(t, live, 1) // 先取到首条增量，再在生成过程中取消
	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	rest := drainUntilClosed(t, live)
	assertNoContentEvents(t, rest)
	assertTerminalCancelled(t, rest)
	for _, ev := range rest {
		if strings.Contains(string(ev.Payload), "late") {
			t.Errorf("content event %+v was delivered after Cancel returned", ev)
		}
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)
}

// TestRunHub_GraceExpiredCancelsWithoutContentEvents 测试退订宽限自动取消：最后一名订阅者 detach 且
// 宽限到点后，运行经根 context 的 cancel() 收敛到 cancelled，且取消后不再下发 / 落库任何内容类事件。
func TestRunHub_GraceExpiredCancelsWithoutContentEvents(t *testing.T) {
	exec := &hubExecutor{deltas: []string{"d1"}, blockOnCtx: true, postCancel: []AgentOutput{{Delta: "late"}}}
	hub, store := newTestHub(t, exec)
	hub.graceOverride = intPtr(1)
	ctx := context.Background()
	sid := testSessionID(66)
	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, _, cancelSub, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	cancelSub() // 最后一名订阅者退订 → 启动退订宽限定时器
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	for _, ev := range events {
		if strings.Contains(string(ev.Payload.Value()), "late") {
			t.Errorf("content %q was persisted after the grace expiry cancelled the run", string(ev.Payload.Value()))
		}
	}
	content := nonStateRunEvents(events)
	last := content[len(content)-1]
	if last.Type != ai.AgentRunEventError || !strings.Contains(string(last.Payload.Value()), string(ai.AgentRunPhaseCancelled)) {
		t.Errorf("terminal persisted event = %+v, want an error carrying phase=cancelled", last)
	}
}

// decodeBlockEvent 把一个 block 事件的 payload 解码为 AgentBlockEvent；解码失败即测试失败。
func decodeBlockEvent(t *testing.T, ev Published) AgentBlockEvent {
	t.Helper()
	var out AgentBlockEvent
	if err := json.Unmarshal(ev.Payload, &out); err != nil {
		t.Fatalf("decode block event payload %s: %v", string(ev.Payload), err)
	}
	return out
}

// TestRunHub_BlockEventsPublishedPersistedAndReplayed 测试内容块事件的发布路径：一个流项既可含增量亦可含
// 块事件，块事件以独立于增量互斥分支之外的路径逐条下发（type=block，payload 为 {seq, kind, payload}）。
// 用真实 blockBuilder 产出「创建 + 收敛」两个事件喂给替身执行器：创建事件在该块首现时下发、收敛事件在
// 该块收拢为终态时下发（各一次），收敛事件的 payload 为该块终态全文。断言事件经发布、落库后，回落数据库
// 回放得到同一组 block 事件（续播回放一致）。
func TestRunHub_BlockEventsPublishedPersistedAndReplayed(t *testing.T) {
	builder := newBlockBuilder(7, 0)
	builder.AddContent("hi")     // 开 content 块 → 创建事件
	builder.AddContent(" there") // 追加进当前开放块 → 无新事件
	builder.Close()              // 收敛 → 收敛事件（终态全文）
	group := builder.DrainEvents()
	if len(group) != 2 {
		t.Fatalf("builder events len = %d, want 2 (创建 + 收敛)", len(group))
	}

	exec := &hubExecutor{blockGroups: [][]AgentBlockEvent{group}, release: make(chan struct{})}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, testSessionID(70), testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancel, err := hub.Subscribe(ctx, testSessionID(70), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancel()
	close(exec.release)

	var got []Published
	for ev := range live {
		got = append(got, ev)
	}
	content := nonStateEvents(got)
	if len(content) != 3 {
		t.Fatalf("content events len = %d, want 3 (block + block + done)", len(content))
	}
	if content[0].Type != ai.AgentRunEventBlock || content[1].Type != ai.AgentRunEventBlock {
		t.Fatalf("content events[0:2] = %q,%q, want block,block", content[0].Type, content[1].Type)
	}
	if content[2].Type != ai.AgentRunEventDone {
		t.Errorf("terminal event = %q, want done", content[2].Type)
	}
	created := decodeBlockEvent(t, content[0])
	converged := decodeBlockEvent(t, content[1])
	if created.Seq != 1 || created.Kind != ai.AgentMessageBlockKindContent {
		t.Errorf("created event = {seq:%d, kind:%q}, want {seq:1, kind:content}", created.Seq, created.Kind)
	}
	if converged.Seq != created.Seq || converged.Kind != created.Kind {
		t.Errorf("converged event = {seq:%d, kind:%q}, want same block as created {seq:%d, kind:%q}", converged.Seq, converged.Kind, created.Seq, created.Kind)
	}
	if !strings.Contains(string(created.Payload), "hi") {
		t.Errorf("created payload = %s, want it to carry the opening text", string(created.Payload))
	}
	if !strings.Contains(string(converged.Payload), "hi there") {
		t.Errorf("converged payload = %s, want the terminal full text", string(converged.Payload))
	}
	if string(created.Payload) == string(converged.Payload) {
		t.Errorf("收敛事件 payload 应随追加内容更新为终态全文，不应与创建事件相同")
	}
	waitLiveRunPhase(t, store, testSessionID(70), ai.AgentRunPhaseDone)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	persisted := 0
	for _, ev := range nonStateRunEvents(events) {
		if ev.Type == ai.AgentRunEventBlock {
			persisted++
		}
	}
	if persisted != 2 {
		t.Errorf("persisted block events = %d, want 2", persisted)
	}

	replay, _, cancel2, err := hub.Subscribe(ctx, testSessionID(70), testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe(from DB) error = %v", err)
	}
	defer cancel2()
	replayed := 0
	for _, ev := range replay {
		if ev.Type == ai.AgentRunEventBlock {
			replayed++
		}
	}
	if replayed != 2 {
		t.Errorf("replayed block events = %d, want 2（续播应回放同一组块事件）", replayed)
	}
}

// TestRunHub_CancelSuppressesBlockEvents 测试取消抑制：中止生效后不再下发 / 落库任何 block 事件，与
// delta / reasoning / stage / compress_* 同口径。执行器在取消后仍试发一个 block 事件组，须被 publish
// 的取消前置条件抑制；订阅者仅收到携带 phase=cancelled 的 error，落库同样无该块事件。
func TestRunHub_CancelSuppressesBlockEvents(t *testing.T) {
	lateBlocks := []AgentBlockEvent{{
		Seq:     1,
		Kind:    ai.AgentMessageBlockKindContent,
		Payload: json.RawMessage(`{"text":"late"}`),
	}}
	exec := &hubExecutor{
		deltas:     []string{"d1"},
		blockOnCtx: true,
		postCancel: []AgentOutput{{Blocks: lateBlocks}},
	}
	hub, store := newTestHub(t, exec)
	ctx := context.Background()
	sid := testSessionID(71)
	runID, err := hub.Start(NewDetachedContext(ctx), AgentInput{Query: "q"}, sid, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	_, live, cancelSub, err := hub.Subscribe(ctx, sid, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancelSub()
	readContentEvents(t, live, 1) // 先取到首条增量，再在生成过程中取消
	if err := hub.Cancel(ctx, sid, testUserID, "default"); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}
	rest := drainUntilClosed(t, live)
	assertNoContentEvents(t, rest)
	assertTerminalCancelled(t, rest)
	for _, ev := range rest {
		if ev.Type == ai.AgentRunEventBlock {
			t.Errorf("received a block event after cancellation: %+v", ev)
		}
	}
	waitLiveRunPhase(t, store, sid, ai.AgentRunPhaseCancelled)

	events, err := store.ListRunEventsAfter(ctx, runID, testUserID, 0, 0)
	if err != nil {
		t.Fatalf("ListRunEventsAfter error = %v", err)
	}
	for _, ev := range events {
		if ev.Type == ai.AgentRunEventBlock {
			t.Errorf("a block event was persisted after cancellation: %+v", ev)
		}
	}
}
