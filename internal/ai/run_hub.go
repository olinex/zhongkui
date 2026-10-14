// 本文件承载驻留运行执行器 RunHub：把一次 Agent 生成的生命周期与浏览器连接解耦。运行在
// 独立 goroutine 中以脱管上下文执行，事件写入每会话的内存环形缓冲并异步落库
// （agent_run_events），HTTP 处理器只负责订阅与转发。断线（关抽屉 / 刷新 / 断网）仅使订阅者
// 退订，运行继续；若某 running 运行的最后一名订阅者退订后经过 agent.runDetachGraceSeconds
// 宽限期仍无订阅者，自动取消该运行（避免无人观看仍持续烧 token）。

package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"sync"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 驻留运行的运行期常量。
const (
	// agentLiveRunTTL 运行登记（agent_live_runs）的存活时长：超过该时刻的登记与事件可被 TTL 清理
	agentLiveRunTTL = time.Hour
	// agentRunMaxDuration 单次运行的最大时长：脱管上下文丢失了请求 deadline，须补上界兜底
	agentRunMaxDuration = 30 * time.Minute
	// runEventBufferSize 每运行内存环形缓冲的最大事件条数（回放超出部分回落数据库）
	runEventBufferSize = 512
	// subscriberBufferSize 单订阅者实时通道的缓冲条数（满时丢弃，慢订阅者不阻塞运行）
	subscriberBufferSize = 256
	// persistQueueSize 事件落库队列的缓冲条数
	persistQueueSize = 1024
	// defaultRunDetachGraceSeconds 退订宽限秒数的兜底值（参数缺失时使用）
	defaultRunDetachGraceSeconds = 90
	// supersedeWaitTimeout 被新运行取代的在途运行的收敛等待上限：超时仅告警（新运行仍继续登记）
	supersedeWaitTimeout = 15 * time.Second
)

// 包级错误哨兵：驻留运行的订阅/取消失败类别，供上层 errors.Is 映射业务码。
var (
	// ErrAgentLiveRunNotFound 驻留运行不存在
	ErrAgentLiveRunNotFound = errors.New("agent live run not found")
	// ErrAgentLiveRunExpired 驻留运行登记已过期
	ErrAgentLiveRunExpired = errors.New("agent live run expired")
	// ErrAgentLiveRunForbidden 驻留运行归属不符（命名空间不一致）
	ErrAgentLiveRunForbidden = errors.New("agent live run forbidden")
	// ErrAgentLiveRunAlreadyRunning 会话已有在途驻留运行（部分唯一索引 uq_agent_live_runs_running 拒绝登记）
	ErrAgentLiveRunAlreadyRunning = errors.New("agent live run already running")
)

// DetachedContext 承载可安全脱管的取值（仅普通 context.Context）：由视图层在启动 goroutine 前
// 从请求上下文抽取，禁止在其中持有 *gin.Context（handler 返回后不可再用）。
type DetachedContext struct {
	// ctx 脱管基础上下文（通常取自 c.Request.Context()）
	ctx context.Context
}

// NewDetachedContext 从普通上下文构造 DetachedContext；nil 时回退为 Background。
func NewDetachedContext(ctx context.Context) DetachedContext {
	return DetachedContext{ctx: ctx}
}

// base 返回脱管基础上下文；未携带时回退为 Background。
func (d DetachedContext) base() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

// RunExecutor 是 RunHub 依赖的对话执行能力（由 *Agent 实现）：以脱管上下文执行一次运行并把
// 输出抽干到回调。抽出接口便于测试注入替身，RunHub 只依赖该单一方法。
type RunExecutor interface {
	// RunDetached 以脱管上下文执行一次带会话持久化的运行，onEvent 收每个流项、onToolRequest 收
	// 挂起时下发的客户端工具请求；正常结束返回 nil。
	RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error
}

// RunStore 是 RunHub 依赖的运行持久化能力（由 *SessionStore 实现），抽出接口便于测试注入替身。
type RunStore interface {
	// AppendRunEvent 追加一条运行事件
	AppendRunEvent(ctx context.Context, ev *ai.AgentRunEvent) error
	// ListRunEventsAfter 返回某运行中序号大于 after 的事件（按 seq 升序），并限定属主用户
	ListRunEventsAfter(ctx context.Context, runID serializer.HexUUID, userID uint, after uint64, limit int) ([]*ai.AgentRunEvent, error)
	// CreateLiveRun 登记一条驻留运行
	CreateLiveRun(ctx context.Context, lr *ai.AgentLiveRun) error
	// FinishLiveRun 把驻留运行收敛到终态阶段并写入最终事件序号（限定属主用户）
	FinishLiveRun(ctx context.Context, runID serializer.HexUUID, userID uint, phase ai.AgentRunPhase, lastSeq uint64) error
	// LatestLiveRun 返回某会话最近一条驻留运行登记（限定属主用户）
	LatestLiveRun(ctx context.Context, sessionID, userID uint) (*ai.AgentLiveRun, error)
}

// RunHubDeps 是 RunHub 的组件访问器：AI 运行时在配置热重载时会整体替换 *Components，故
// 这里保存访问器（而非组件指针），在「使用时」解析当前装配的组件，使 hub 不随重载重建、
// 也不打断在途运行。
type RunHubDeps struct {
	// Agent 返回当前的对话执行器（未装配时返回 nil）
	Agent func() RunExecutor
	// Params 返回当前行为参数读取器（未装配时返回 nil）
	Params func() *Params
	// Store 返回当前运行持久化能力（未装配时返回 nil）
	Store func() RunStore
	// Logger 日志记录器（可空）
	Logger *logging.Logger
}

// Published 是一条已发布的事件：序号 + 类型 + JSON 负载，供订阅者回放与实时转发。
type Published struct {
	// Seq 事件在所属运行内的单调自增序号（从 1 起）
	Seq uint64
	// Type 事件类型（delta/stage/tool_request/done/error）
	Type ai.AgentRunEventType
	// Payload 事件 JSON 负载（含用户内容）
	Payload []byte
}

// PendingCall 是一条待执行的工具调用（供刷新后重建待审批现场）。
type PendingCall struct {
	// CallID 与模型 tool call id 一致
	CallID string `json:"callID"`
	// ToolName 待执行的工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级（挂起前的批量评估产出）
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 审核重要级与会话阀门合成的最终结论：true 表示需人工审核
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属（client / backend）；为空时按 client 处理
	ExecOwner ToolExecOwner `json:"execOwner"`
}

// LiveRunStatus 是一次驻留运行的当前状态快照（供刷新后发现在途运行）。
type LiveRunStatus struct {
	// RunID 运行标识
	RunID serializer.HexUUID
	// Phase 运行生命周期阶段
	Phase ai.AgentRunPhase
	// LastSeq 已发布的最大事件序号
	LastSeq uint64
	// PendingCalls 挂起待审批时的待执行调用列表（非挂起时为空）
	PendingCalls []PendingCall
}

// subscriber 是一个订阅者：通过带缓冲的事件通道接收实时事件。
type subscriber struct {
	// ch 实时事件通道（发布侧非阻塞写入，满时丢弃该条）
	ch chan Published
}

// liveRun 是 RunHub 持有的一次在途运行。map 层（RunHub.runs）由 RunHub.mu 保护，本结构体内
// 字段由自身 mu 保护；两把锁不同时持有（先取 RunHub.mu 定位、释放后再取 run.mu）。
type liveRun struct {
	// runID 运行标识
	runID serializer.HexUUID
	// sessionID 所属会话 ID
	sessionID uint
	// userID 所属属主用户 ID（users.id），用于跨用户隔离
	userID uint
	// namespace 所属命名空间
	namespace string
	// ctx 脱管基础上下文（WithoutCancel，用于持久化与登记，不随请求结束取消）
	ctx context.Context
	// cancel 取消本次运行（Cancel 与宽限到点据此中止图循环）
	cancel context.CancelFunc

	// mu 保护本结构体其余可变字段
	mu sync.Mutex
	// phase 当前生命周期阶段
	phase ai.AgentRunPhase
	// seq 已发布的最大事件序号
	seq uint64
	// buffer 有界内存环形缓冲（回放优先取内存）
	buffer *runEventBuffer
	// subs 当前订阅者集合
	subs map[*subscriber]struct{}
	// graceTimer 退订宽限定时器（仅 running 且无订阅者时启动）
	graceTimer *time.Timer
	// finished 是否已收敛到终态
	finished bool
	// cancelled 是否已被显式或宽限取消
	cancelled bool

	// persistCh 事件落库队列（保持发布顺序）
	persistCh chan *ai.AgentRunEvent
	// persistCloseCh 关闭以通知落库 goroutine 抽干队列后退出
	persistCloseCh chan struct{}
	// persistDone 落库 goroutine 退出信号
	persistDone chan struct{}
	// finishDone 终态处理完成信号（Shutdown 据此等待）
	finishDone chan struct{}
}

// runEventBuffer 是每运行的有界事件环形缓冲：按序号升序保存最近 runEventBufferSize 条事件，
// 超出时丢弃最旧条目（更旧的事件由数据库兜底回放）。仅在持有 liveRun.mu 时访问，无自有锁。
type runEventBuffer struct {
	// events 缓冲中的事件（按序号升序）
	events []Published
	// max 最大条数
	max int
}

// newRunEventBuffer 构造指定容量的环形缓冲。
func newRunEventBuffer(max int) *runEventBuffer {
	return &runEventBuffer{max: max}
}

// append 追加一条事件，超出容量时丢弃最旧条目。
func (b *runEventBuffer) append(ev Published) {
	b.events = append(b.events, ev)
	if len(b.events) > b.max {
		b.events = b.events[len(b.events)-b.max:]
	}
}

// after 返回序号大于 seq 的事件副本。
func (b *runEventBuffer) after(seq uint64) []Published {
	out := make([]Published, 0, len(b.events))
	for _, ev := range b.events {
		if ev.Seq > seq {
			out = append(out, ev)
		}
	}
	return out
}

// earliestSeq 返回缓冲中最小事件序号；缓冲为空时返回 0。
func (b *runEventBuffer) earliestSeq() uint64 {
	if len(b.events) == 0 {
		return 0
	}
	return b.events[0].Seq
}

// RunHub 是进程级的驻留运行执行器：持有 map[sessionID]*liveRun，负责启动运行、订阅（含回放）、
// 取消与关闭。跨配置热重载存活（组件经访问器实时解析）。
type RunHub struct {
	// deps 组件访问器
	deps RunHubDeps
	// mu 保护 runs 与 closed
	mu sync.Mutex
	// runs 会话 → 在途运行（终态后移除）
	runs map[uint]*liveRun
	// closed 是否已关闭（关闭后拒绝新运行）
	closed bool
	// graceOverride 退订宽限秒数的测试覆盖（非 nil 时优先于参数）
	graceOverride *int
	// maxOverride 最大运行时长的测试覆盖
	maxOverride *time.Duration
	// supersedeOverride 被取代运行收敛等待上限的测试覆盖
	supersedeOverride *time.Duration
}

// NewRunHub 构造运行执行器。
func NewRunHub(deps RunHubDeps) *RunHub {
	return &RunHub{deps: deps, runs: map[uint]*liveRun{}}
}

// agent 解析当前的对话执行器（未装配时返回 nil）。
func (h *RunHub) agent() RunExecutor {
	if h.deps.Agent == nil {
		return nil
	}
	return h.deps.Agent()
}

// params 解析当前行为参数读取器（未装配时返回 nil）。
func (h *RunHub) params() *Params {
	if h.deps.Params == nil {
		return nil
	}
	return h.deps.Params()
}

// store 解析当前运行持久化能力（未装配时返回 nil）。
func (h *RunHub) store() RunStore {
	if h.deps.Store == nil {
		return nil
	}
	return h.deps.Store()
}

// logger 返回日志记录器（可空）。
func (h *RunHub) logger() *logging.Logger {
	return h.deps.Logger
}

// graceSeconds 返回退订宽限秒数：测试覆盖优先，其次取参数 agent.runDetachGraceSeconds，
// 参数缺失时用兜底值；<=0 表示关闭自动取消。
func (h *RunHub) graceSeconds() int {
	if h.graceOverride != nil {
		return *h.graceOverride
	}
	if p := h.params(); p != nil {
		return p.Int("agent.runDetachGraceSeconds")
	}
	return defaultRunDetachGraceSeconds
}

// maxRunDuration 返回单次运行的最大时长（测试覆盖优先）。
func (h *RunHub) maxRunDuration() time.Duration {
	if h.maxOverride != nil {
		return *h.maxOverride
	}
	return agentRunMaxDuration
}

// supersedeTimeout 返回被取代运行的收敛等待上限（测试覆盖优先）。
func (h *RunHub) supersedeTimeout() time.Duration {
	if h.supersedeOverride != nil {
		return *h.supersedeOverride
	}
	return supersedeWaitTimeout
}

// supersede 收敛一个被新运行取代的在途运行：置取消并触发其 cancel，随后等待它走到终态
// （finish 中已通过 FinishLiveRun 把 DB 登记改为非 running），从而使新运行的登记不触发
// uq_agent_live_runs_running 冲突，也不产生孤儿运行。等待超时仅告警，不阻塞新运行登记。
func (h *RunHub) supersede(prev *liveRun) {
	prev.mu.Lock()
	if !prev.finished {
		prev.cancelled = true
		if prev.cancel != nil {
			prev.cancel()
		}
	}
	done := prev.finishDone
	prev.mu.Unlock()
	select {
	case <-done:
	case <-time.After(h.supersedeTimeout()):
		if l := h.logger(); l != nil {
			l.Ctx(prev.ctx).Warnf("ai: superseded run %s did not converge within %s", prev.runID, h.supersedeTimeout())
		}
	}
}

// Start 启动一次驻留运行：登记（agent_live_runs）后在独立 goroutine 中执行，事件先广播给
// 订阅者再异步落库。started 非空时在登记完成后立即以 runID 回调（供调用方在 goroutine 启动前
// 完成订阅，避免漏掉早期事件）。返回运行标识。
func (h *RunHub) Start(detached DetachedContext, in AgentInput, sessionID uint, userID uint, namespace string, started func(runID serializer.HexUUID)) (serializer.HexUUID, error) {
	if h.agent() == nil {
		return serializer.NewNilHexUUIDv4(), errors.New("ai: run hub: no agent is assembled")
	}
	store := h.store()
	if store == nil {
		return serializer.NewNilHexUUIDv4(), errors.New("ai: run hub: no session store is assembled")
	}
	// 属主用户随运行注入：既用于隔离，也确保脱管执行时落库/召回归属一致。
	in.UserID = userID
	runID := newAgentRunID()
	ctx := context.WithoutCancel(detached.base())
	runCtx, cancel := context.WithTimeout(ctx, h.maxRunDuration())
	run := &liveRun{
		runID:          runID,
		sessionID:      sessionID,
		userID:         userID,
		namespace:      namespace,
		ctx:            ctx,
		cancel:         cancel,
		phase:          ai.AgentRunPhaseRunning,
		buffer:         newRunEventBuffer(runEventBufferSize),
		subs:           map[*subscriber]struct{}{},
		persistCh:      make(chan *ai.AgentRunEvent, persistQueueSize),
		persistCloseCh: make(chan struct{}),
		persistDone:    make(chan struct{}),
		finishDone:     make(chan struct{}),
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		cancel()
		return serializer.NewNilHexUUIDv4(), errors.New("ai: run hub is closed")
	}
	// 先占位再收敛：并发 Start 时后到者能取到先到者并收敛它，保证同一会话不出现两个在途运行共存。
	prev := h.runs[sessionID]
	h.runs[sessionID] = run
	h.mu.Unlock()
	if prev != nil && prev != run {
		h.supersede(prev)
	}

	// 收敛完成后才登记：这样部分唯一索引 uq_agent_live_runs_running 不会与旧运行的在途登记冲突。
	if err := store.CreateLiveRun(ctx, &ai.AgentLiveRun{
		NamespaceName: namespace,
		UserID:        userID,
		RunID:         runID,
		SessionID:     sessionID,
		Phase:         ai.AgentRunPhaseRunning,
		LastSeq:       0,
		ExpiresAt:     time.Now().Add(agentLiveRunTTL),
	}); err != nil {
		h.mu.Lock()
		if h.runs[sessionID] == run {
			delete(h.runs, sessionID)
		}
		h.mu.Unlock()
		cancel()
		// 会话已有在途运行（CreateLiveRun 的 ON CONFLICT DO NOTHING 未写入）转为明确错误并记录
		// 告警，避免与其它登记失败混淆。
		if errors.Is(err, ErrAgentLiveRunAlreadyRunning) {
			if l := h.logger(); l != nil {
				l.Ctx(ctx).Warnf("ai: session %d already has an in-flight live run: %v", sessionID, err)
			}
			return serializer.NewNilHexUUIDv4(), errors.WithStack(errors.Newf("ai: run hub: session %d already has an in-flight live run", sessionID))
		}
		return serializer.NewNilHexUUIDv4(), err
	}
	go h.persistLoop(run)
	if started != nil {
		started(runID)
	}
	go h.execute(run, runCtx, in)
	return runID, nil
}

// execute 在 runCtx 下执行一次运行并把输出转为事件：阶段事件经 ToolReporter、增量与完成项经
// onEvent、客户端工具请求经 onToolRequest 全部发布；结束后据「是否被取消 / 是否出错 / 是否
// 挂起 / 是否有完成项」收敛终态并调用 finish。
func (h *RunHub) execute(run *liveRun, runCtx context.Context, in AgentInput) {
	agent := h.agent()
	var sawDone, sawToolRequest bool
	var runErr error
	if agent == nil {
		runErr = errors.New("ai: run hub: no agent is assembled")
	} else {
		in.ToolReporter = func(ev StageEvent) error {
			h.publishStage(run, ev)
			return nil
		}
		in.CompressReporter = func(ev CompressEvent) error {
			h.publishJSON(run, ev.Type, ev)
			return nil
		}
		runErr = agent.RunDetached(runCtx, in,
			func(out AgentOutput) error {
				switch {
				case out.Delta != "":
					h.publishDelta(run, "", out.Delta)
				case out.Done:
					sawDone = true
					h.publishJSON(run, ai.AgentRunEventDone, out)
				case out.Reasoning != "":
					h.publishJSON(run, ai.AgentRunEventReasoning, map[string]string{"delta": out.Reasoning})
				}
				return nil
			},
			func(req ToolRequest) error {
				sawToolRequest = true
				h.publishJSON(run, ai.AgentRunEventToolRequest, req)
				return nil
			},
		)
	}
	run.mu.Lock()
	cancelled := run.cancelled
	run.mu.Unlock()

	var phase ai.AgentRunPhase
	var errMsg string
	switch {
	case cancelled:
		phase = ai.AgentRunPhaseCancelled
		errMsg = "run cancelled"
	case runErr != nil:
		phase = ai.AgentRunPhaseFailed
		errMsg = runErr.Error()
	case sawToolRequest && !sawDone:
		phase = ai.AgentRunPhaseSuspended
	case sawDone:
		phase = ai.AgentRunPhaseDone
	default:
		phase = ai.AgentRunPhaseFailed
		errMsg = "run ended without a terminal event"
	}
	h.finish(run, phase, errMsg)
}

// finish 收敛一次运行到终态：failed/cancelled 追加一条 error 事件，随后停止宽限定时器、封存
// 落库 goroutine 并等待其抽干、写回终态阶段与最终序号、关闭全部订阅者通道并从在途表中移除。
func (h *RunHub) finish(run *liveRun, phase ai.AgentRunPhase, errMsg string) {
	if errMsg != "" {
		h.publishJSON(run, ai.AgentRunEventError, map[string]string{"error": errMsg})
	}
	run.mu.Lock()
	run.finished = true
	run.phase = phase
	run.stopGraceLocked()
	seq := run.seq
	subs := make([]*subscriber, 0, len(run.subs))
	for sub := range run.subs {
		subs = append(subs, sub)
	}
	run.subs = map[*subscriber]struct{}{}
	close(run.persistCloseCh)
	run.mu.Unlock()

	<-run.persistDone

	if store := h.store(); store != nil {
		if err := store.FinishLiveRun(run.ctx, run.runID, run.userID, phase, seq); err != nil {
			if l := h.logger(); l != nil {
				l.Ctx(run.ctx).Warnf("ai: finishing the live run %s failed: %v", run.runID, err)
			}
		}
	}
	for _, sub := range subs {
		close(sub.ch)
	}
	h.mu.Lock()
	if h.runs[run.sessionID] == run {
		delete(h.runs, run.sessionID)
	}
	h.mu.Unlock()
	close(run.finishDone)
}

// persistLoop 是落库 goroutine：按顺序消费事件队列并落库，收到关闭信号后抽干残余事件再退出。
func (h *RunHub) persistLoop(run *liveRun) {
	defer close(run.persistDone)
	store := h.store()
	if store == nil {
		return
	}
	for {
		select {
		case ev := <-run.persistCh:
			h.appendEvent(run, store, ev)
		case <-run.persistCloseCh:
			for {
				select {
				case ev := <-run.persistCh:
					h.appendEvent(run, store, ev)
				default:
					return
				}
			}
		}
	}
}

// appendEvent 落库一条事件，失败仅告警（不影响运行）。
func (h *RunHub) appendEvent(run *liveRun, store RunStore, ev *ai.AgentRunEvent) {
	if err := store.AppendRunEvent(run.ctx, ev); err != nil {
		if l := h.logger(); l != nil {
			l.Ctx(run.ctx).Warnf("ai: persisting the run event (run %s seq %d) failed: %v", run.runID, ev.Seq, err)
		}
	}
}

// publish 发布一条事件：持锁自增序号、写内存缓冲、非阻塞扇出给订阅者，解锁后异步入落库队列。
func (h *RunHub) publish(run *liveRun, typ ai.AgentRunEventType, payload []byte) {
	run.mu.Lock()
	if run.finished {
		run.mu.Unlock()
		return
	}
	run.seq++
	seq := run.seq
	pub := Published{Seq: seq, Type: typ, Payload: payload}
	run.buffer.append(pub)
	for sub := range run.subs {
		select {
		case sub.ch <- pub:
		default: // 慢订阅者：丢弃该条，避免阻塞运行
		}
	}
	run.mu.Unlock()

	select {
	case run.persistCh <- &ai.AgentRunEvent{
		RunID:         run.runID,
		SessionID:     run.sessionID,
		NamespaceName: run.namespace,
		UserID:        run.userID,
		Seq:           seq,
		Type:          typ,
		Payload:       serializer.NewCrypto(payload),
		ExpiresAt:     time.Now().Add(agentLiveRunTTL),
	}:
	case <-run.persistCloseCh:
	}
}

// publishJSON 把任意可序列化负载作为事件发布。
func (h *RunHub) publishJSON(run *liveRun, typ ai.AgentRunEventType, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.publish(run, typ, payload)
}

// publishDelta 发布一条增量文本事件（stage 非空时属某阶段的增量）。
func (h *RunHub) publishDelta(run *liveRun, stage string, delta string) {
	var payload []byte
	if stage != "" {
		payload, _ = json.Marshal(map[string]string{"stage": stage, "delta": delta})
	} else {
		payload, _ = json.Marshal(map[string]string{"delta": delta})
	}
	h.publish(run, ai.AgentRunEventDelta, payload)
}

// publishStage 发布一条阶段事件：带增量的阶段事件归入 delta（携带阶段标识），否则发布 stage。
func (h *RunHub) publishStage(run *liveRun, ev StageEvent) {
	if ev.Delta != "" {
		h.publishDelta(run, string(ev.Stage), ev.Delta)
		return
	}
	h.publishJSON(run, ai.AgentRunEventStage, ev)
}

// Subscribe 订阅某会话的在途运行：返回「回放事件（序号 > after）」与「实时事件通道」以及退订
// 函数。内存中无该运行时回落数据库（据此判定不存在 / 已过期，并回放已落库事件后即以 EOF 结束）。
// 传入的 ctx 被用于检测订阅者 detach（ctx 取消即退订），关页 / 刷新 / 断网都会触发。
func (h *RunHub) Subscribe(ctx context.Context, sessionID uint, userID uint, after uint64) ([]Published, <-chan Published, func(), error) {
	h.mu.Lock()
	run := h.runs[sessionID]
	h.mu.Unlock()
	// 内存运行存在但属主不符：视为不存在，回落数据库（同样按属主过滤），避免越权订阅。
	if run == nil || run.userID != userID {
		return h.subscribeFromDB(ctx, sessionID, userID, after)
	}

	run.mu.Lock()
	run.stopGraceLocked()
	finished := run.finished
	replayBuf := run.buffer.after(after)
	earliest := run.buffer.earliestSeq()
	var sub *subscriber
	if !finished {
		sub = &subscriber{ch: make(chan Published, subscriberBufferSize)}
		run.subs[sub] = struct{}{}
	}
	run.mu.Unlock()

	var prefix []Published
	if earliest > after+1 {
		prefix, _ = h.dbPrefix(ctx, run.runID, run.userID, after, earliest-1)
	}
	replay := make([]Published, 0, len(prefix)+len(replayBuf))
	replay = append(replay, prefix...)
	replay = append(replay, replayBuf...)

	if finished {
		ch := make(chan Published)
		close(ch)
		return replay, ch, func() {}, nil
	}
	var once sync.Once
	cancel := func() {
		once.Do(func() { h.subscriberDone(run, sub) })
	}
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return replay, sub.ch, cancel, nil
}

// subscribeFromDB 在内存中无该会话运行时回落数据库：取最近一条运行登记，据其判定不存在 /
// 已过期，并回放已落库事件后以已关闭通道表示「无实时事件」。
func (h *RunHub) subscribeFromDB(ctx context.Context, sessionID uint, userID uint, after uint64) ([]Published, <-chan Published, func(), error) {
	store := h.store()
	if store == nil {
		return nil, nil, nil, ErrAgentLiveRunNotFound
	}
	lr, err := store.LatestLiveRun(ctx, sessionID, userID)
	if err != nil {
		if errors.Is(err, ErrAgentRunNotFound) {
			return nil, nil, nil, ErrAgentLiveRunNotFound
		}
		return nil, nil, nil, errors.WithStack(err)
	}
	if time.Now().After(lr.ExpiresAt) {
		return nil, nil, nil, ErrAgentLiveRunExpired
	}
	events, err := store.ListRunEventsAfter(ctx, lr.RunID, userID, after, 0)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	replay := make([]Published, 0, len(events))
	for _, ev := range events {
		replay = append(replay, Published{Seq: ev.Seq, Type: ev.Type, Payload: ev.Payload.Value()})
	}
	ch := make(chan Published)
	close(ch)
	return replay, ch, func() {}, nil
}

// dbPrefix 从事件日志读取序号落在 (after, until] 的事件（内存缓冲已丢弃的更旧前缀）。
func (h *RunHub) dbPrefix(ctx context.Context, runID serializer.HexUUID, userID uint, after, until uint64) ([]Published, error) {
	if until <= after {
		return nil, nil
	}
	store := h.store()
	if store == nil {
		return nil, nil
	}
	events, err := store.ListRunEventsAfter(ctx, runID, userID, after, int(until-after))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make([]Published, 0, len(events))
	for _, ev := range events {
		out = append(out, Published{Seq: ev.Seq, Type: ev.Type, Payload: ev.Payload.Value()})
	}
	return out, nil
}

// subscriberDone 退订一个订阅者：移出集合后若已无订阅者且运行仍在 running，启动退订宽限定时器。
func (h *RunHub) subscriberDone(run *liveRun, sub *subscriber) {
	run.mu.Lock()
	if _, ok := run.subs[sub]; ok {
		delete(run.subs, sub)
		if len(run.subs) == 0 && !run.finished && run.phase == ai.AgentRunPhaseRunning {
			run.stopGraceLocked()
			h.armGraceLocked(run)
		}
	}
	run.mu.Unlock()
}

// armGraceLocked 启动退订宽限定时器（须持 run.mu）：宽限秒数 <= 0 时关闭自动取消。定时器回调
// 在持锁下二次确认「仍无订阅者且仍 running」后才真正取消，避免与新订阅者竞态。
func (h *RunHub) armGraceLocked(run *liveRun) {
	dur := time.Duration(h.graceSeconds()) * time.Second
	if dur <= 0 {
		return
	}
	run.graceTimer = time.AfterFunc(dur, func() { h.onGraceExpired(run) })
}

// onGraceExpired 是宽限定时器回调：持锁确认仍无订阅者且仍 running 时才取消运行。
func (h *RunHub) onGraceExpired(run *liveRun) {
	run.mu.Lock()
	if run.finished || len(run.subs) > 0 || run.phase != ai.AgentRunPhaseRunning {
		run.mu.Unlock()
		return
	}
	run.cancelled = true
	cancel := run.cancel
	run.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// stopGraceLocked 停止并回收退订宽限定时器（须持 run.mu）。
func (run *liveRun) stopGraceLocked() {
	if run.graceTimer != nil {
		run.graceTimer.Stop()
		run.graceTimer = nil
	}
}

// Cancel 显式取消某会话的在途运行：命名空间不符返回 ErrAgentLiveRunForbidden，运行不存在返回
// ErrAgentLiveRunNotFound，已收敛终态时幂等返回 nil（不报错）。
func (h *RunHub) Cancel(ctx context.Context, sessionID uint, userID uint, namespace string) error {
	h.mu.Lock()
	run := h.runs[sessionID]
	h.mu.Unlock()
	if run == nil || run.userID != userID {
		store := h.store()
		if store == nil {
			return ErrAgentLiveRunNotFound
		}
		lr, err := store.LatestLiveRun(ctx, sessionID, userID)
		if err != nil {
			if errors.Is(err, ErrAgentRunNotFound) {
				return ErrAgentLiveRunNotFound
			}
			return errors.WithStack(err)
		}
		if lr.NamespaceName != namespace || lr.UserID != userID {
			return ErrAgentLiveRunForbidden
		}
		return nil
	}
	if run.namespace != namespace {
		return ErrAgentLiveRunForbidden
	}
	run.mu.Lock()
	if run.finished {
		run.mu.Unlock()
		return nil
	}
	run.cancelled = true
	cancel := run.cancel
	run.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// LiveRun 返回某会话当前在途运行的状态快照：内存优先，无内存运行回落数据库。运行不存在返回
// ErrAgentLiveRunNotFound，登记过期返回 ErrAgentLiveRunExpired。
func (h *RunHub) LiveRun(ctx context.Context, sessionID uint, userID uint) (*LiveRunStatus, error) {
	h.mu.Lock()
	run := h.runs[sessionID]
	h.mu.Unlock()
	if run != nil && run.userID == userID {
		run.mu.Lock()
		st := &LiveRunStatus{RunID: run.runID, Phase: run.phase, LastSeq: run.seq}
		st.PendingCalls = pendingCallsOf(run.buffer.events)
		run.mu.Unlock()
		return st, nil
	}
	store := h.store()
	if store == nil {
		return nil, ErrAgentLiveRunNotFound
	}
	lr, err := store.LatestLiveRun(ctx, sessionID, userID)
	if err != nil {
		if errors.Is(err, ErrAgentRunNotFound) {
			return nil, ErrAgentLiveRunNotFound
		}
		return nil, errors.WithStack(err)
	}
	if time.Now().After(lr.ExpiresAt) {
		return nil, ErrAgentLiveRunExpired
	}
	st := &LiveRunStatus{RunID: lr.RunID, Phase: lr.Phase, LastSeq: lr.LastSeq}
	events, err := store.ListRunEventsAfter(ctx, lr.RunID, userID, 0, 0)
	if err == nil {
		paid := make([]Published, 0, len(events))
		for _, ev := range events {
			paid = append(paid, Published{Seq: ev.Seq, Type: ev.Type, Payload: ev.Payload.Value()})
		}
		st.PendingCalls = pendingCallsOf(paid)
	}
	return st, nil
}

// pendingCallsOf 从事件序列中提取全部 tool_request 事件，重建待执行调用列表。
func pendingCallsOf(events []Published) []PendingCall {
	var calls []PendingCall
	for _, ev := range events {
		if ev.Type != ai.AgentRunEventToolRequest {
			continue
		}
		var req ToolRequest
		if err := json.Unmarshal(ev.Payload, &req); err != nil {
			continue
		}
		owner := req.ExecOwner
		if owner == "" {
			owner = ToolExecOwnerClient
		}
		calls = append(calls, PendingCall{
			CallID:           req.CallID,
			ToolName:         req.ToolName,
			Arguments:        req.Arguments,
			AuditLevel:       req.AuditLevel,
			ApprovalRequired: req.ApprovalRequired,
			ExecOwner:        owner,
		})
	}
	return calls
}

// Shutdown 关闭运行执行器：拒绝新运行、取消全部在途运行并等待各自终态处理完成（受 ctx 约束）。
func (h *RunHub) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	runs := make([]*liveRun, 0, len(h.runs))
	for _, run := range h.runs {
		runs = append(runs, run)
	}
	h.runs = map[uint]*liveRun{}
	h.mu.Unlock()

	for _, run := range runs {
		run.mu.Lock()
		if !run.finished {
			run.cancelled = true
			if run.cancel != nil {
				run.cancel()
			}
		}
		done := run.finishDone
		run.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// newAgentRunID 生成一个随机的运行标识（UUID v4）。
func newAgentRunID() serializer.HexUUID {
	return serializer.NewHexUUIDv4()
}
