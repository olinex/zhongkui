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
	// cancelWaitTimeout 取消端点等待生成 goroutine 真正退出的安全上限：取消信号已下发，超时仅告警
	// （不阻塞调用方返回），仅用于防止极端卡死（如执行器忽略 ctx 取消）时长挂请求。
	cancelWaitTimeout = 15 * time.Second
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

// base 返回脱管基础上下文；未携带时回退为 Background（兜底，非活动链路：仅用于构造脱管上下文，
// 真正的活动根 context 由 newLiveRun 经 WithTimeout 从其派生，取消经该根的 cancel 传播）。
func (d DetachedContext) base() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

// RunExecutor 是 RunHub 依赖的对话执行能力（由 *Agent 实现）：以脱管上下文执行一次运行（首轮生成
// 或续跑）并把输出抽干到回调。抽出接口便于测试注入替身。
type RunExecutor interface {
	// RunDetached 以脱管上下文执行一次带会话持久化的运行，onEvent 收每个流项、onToolRequest 收
	// 挂起时下发的客户端工具请求；正常结束返回 nil。
	RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error
	// PrepareResume 认领挂起运行现场并构建续跑输入（不启动运行）；认领失败返回哨兵错误
	// （不存在 / 归属不符 / 已消费 / 已过期），供调用方在登记与构造 SSE 写手前同步映射业务码。
	PrepareResume(ctx context.Context, in ResumeInput) (AgentInput, error)
	// ResumeDetached 以 PrepareResume 备好的输入脱管执行一次续跑，抽干语义同 RunDetached
	// （不重载历史、不追加用户消息）。
	ResumeDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error
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

// InteractionStateEvent 是流式 state 事件的负载：仅承载交互状态与子状态（StateOf 出参的子集）。
// 其余出参（locked/userTurns/maxUserTurns）随 HTTP 快照出参下发，不占用流式事件负载。
type InteractionStateEvent struct {
	// State 交互主状态（waiting/running/paused/locked 之一）
	State ai.AgentInteractionState `json:"state"`
	// Substate 交互子状态（随主状态取值；已锁定为空串）
	Substate ai.AgentInteractionSubstate `json:"substate"`
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
	// InMemory 该快照是否取自内存在途运行；false 表示取自数据库登记（本进程内存中已无对应活动）。
	// 调用方据此判定「running 却非内存」的孤儿运行（进程重启 / goroutine 丢失），把它按已收敛处理。
	InMemory bool
	// Substate 在途活动当前上报的交互子状态（generating/thinking/tool_calling/compacting）；仅在
	// 内存分支有意义（InMemory 为 true 时），DB 回落分支为空串。
	Substate ai.AgentInteractionSubstate
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
	// substate 当前上报的交互子状态（变更时经 state 事件广播）
	substate ai.AgentInteractionSubstate
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
	// cancelOverride 取消端点等待执行 goroutine 退出的上限测试覆盖
	cancelOverride *time.Duration
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

// cancelTimeout 返回取消端点等待执行 goroutine 退出的上限（测试覆盖优先）。
func (h *RunHub) cancelTimeout() time.Duration {
	if h.cancelOverride != nil {
		return *h.cancelOverride
	}
	return cancelWaitTimeout
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

// runFunc 是一次驻留运行的执行闭包：在给定脱管上下文下执行并把输出抽干到回调（首轮生成经
// RunDetached、续跑经 ResumeDetached）。由 Start / StartResume 在登记前构造，交给 execute 调用。
type runFunc func(ctx context.Context, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error

// Start 启动一次驻留运行：登记（agent_live_runs）后在独立 goroutine 中执行，事件先广播给
// 订阅者再异步落库。started 非空时在登记完成后立即以 runID 回调（供调用方在 goroutine 启动前
// 完成订阅，避免漏掉早期事件）。返回运行标识。
func (h *RunHub) Start(detached DetachedContext, in AgentInput, sessionID uint, userID uint, namespace string, started func(runID serializer.HexUUID)) (serializer.HexUUID, error) {
	agent := h.agent()
	return h.launch(detached, sessionID, userID, namespace, func(run *liveRun) (runFunc, error) {
		// 首轮生成的初始交互子状态为「思考中」：登记后 start 会据此发布初始 state 事件，
		// 使订阅方（含回放）先收到 running 状态。
		run.substate = ai.AgentInteractionSubstateThinking
		// 属主用户随运行注入：既用于隔离，也确保脱管执行时落库/召回归属一致。
		in.UserID = userID
		// 阶段事件与压缩事件的上报回调在此注入（首轮生成会触发两者），执行闭包据此持有已配好的输入。
		in.ToolReporter = func(ev StageEvent) error {
			h.publishStage(run, ev)
			return nil
		}
		in.CompressReporter = func(ev CompressEvent) error {
			// 压缩开始 / 摘要增量 → compacting；压缩完成 → 回到正文生成中（随后继续产出正文）。
			switch ev.Type {
			case ai.AgentRunEventCompressStart, ai.AgentRunEventCompressDelta:
				h.reportSubstate(run, ai.AgentInteractionSubstateCompacting)
			case ai.AgentRunEventCompressDone:
				h.reportSubstate(run, ai.AgentInteractionSubstateGenerating)
			}
			h.publishJSON(run, ev.Type, ev)
			return nil
		}
		return func(ctx context.Context, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
			return agent.RunDetached(ctx, in, onEvent, onToolRequest)
		}, nil
	}, started)
}

// StartResume 启动一次续跑（客户端工具调用结果回传后恢复挂起运行）的驻留运行：先同步认领运行现场并
// 构建续跑输入（PrepareResume——认领失败即返回哨兵错误，此时尚未登记、也未构造 SSE 写手，调用方仍可
// 返回 JSON 业务码），再以与 Start 一致的登记 / 广播 / 收敛路径执行（ResumeDetached）。如此续跑与首轮
// 生成一样拥有独立的 live-run 登记并在收敛时写入终态（done/cancelled/failed）：否则挂起现场留下的
// suspended 登记会一直残留，前端刷新后据它误判「仍在等待审批」，并以陈旧的挂起事件回放覆盖已落库的
// 正文。返回运行标识。
func (h *RunHub) StartResume(detached DetachedContext, in ResumeInput, sessionID uint, userID uint, namespace string, started func(runID serializer.HexUUID)) (serializer.HexUUID, error) {
	agent := h.agent()
	return h.launch(detached, sessionID, userID, namespace, func(run *liveRun) (runFunc, error) {
		// 续跑的初始交互子状态同首轮：「思考中」，登记后由 start 发布初始 state 事件。
		run.substate = ai.AgentInteractionSubstateThinking
		in.SessionID = sessionID
		in.NamespaceName = namespace
		in.UserID = userID
		// 认领现场并回灌 tool message 的阶段（PrepareResume）就要上报 toolResult 阶段终态，故先注入阶段上报。
		in.ToolReporter = func(ev StageEvent) error {
			h.publishStage(run, ev)
			return nil
		}
		prepared, err := agent.PrepareResume(run.ctx, in)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return func(ctx context.Context, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
			return agent.ResumeDetached(ctx, prepared, onEvent, onToolRequest)
		}, nil
	}, started)
}

// ActivityInput 描述一次通用在途活动（如手动上下文压缩）的启动参数：会话 / 属主 / 命名空间为登记
// 必需字段，Run 是活动本体。与生成（Start / StartResume）共用同一套登记互斥、运行根 context、取消、
// 终局化与落库路径，从而满足「会话内至多一个在途活动」——压缩与生成冲突时同样返回既有的
// ErrAgentLiveRunAlreadyRunning 哨兵。
type ActivityInput struct {
	// SessionID 所属会话 ID
	SessionID uint
	// UserID 属主用户 ID
	UserID uint
	// Namespace 所属命名空间
	Namespace string
	// Run 活动本体：在活动根 context 下执行；report 用于下发活动自己的流式事件
	// （delta / reasoning / stage / tool_request / compress_*，取消后内容类事件被自动抑制）。
	Run func(ctx context.Context, report func(typ ai.AgentRunEventType, payload any) error) error
}

// StartActivity 启动一次通用在途活动（如上下文压缩）：与 Start / StartResume 共用同一内部启动路径
// （登记互斥、运行根 context、取消、终局化与落库）。Run 在活动根 ctx 下执行；Run 正常返回（nil）时由
// RunHub 以 done 事件收口，使 execute 的既有终局分支（sawDone → done）自然成立——故无需为活动另写一套
// 收敛逻辑。活动被取消时 Run 应随 ctx 返回错误，此时既不产出 done、也不产生半截检查点。
func (h *RunHub) StartActivity(detached DetachedContext, in ActivityInput, started func(runID serializer.HexUUID)) (serializer.HexUUID, error) {
	return h.launch(detached, in.SessionID, in.UserID, in.Namespace, func(run *liveRun) (runFunc, error) {
		// 通用在途活动（如手动上下文压缩）的初始交互子状态为「上下文压缩中」；start 据此发布初始
		// state 事件，订阅方先收到 running/compacting。
		run.substate = ai.AgentInteractionSubstateCompacting
		return func(ctx context.Context, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
			report := func(typ ai.AgentRunEventType, payload any) error {
				h.publishJSON(run, typ, payload)
				return nil
			}
			if err := in.Run(ctx, report); err != nil {
				return errors.WithStack(err)
			}
			// 成功收口：以 done 事件收尾，令 execute 的 sawDone → done 分支自然成立。
			if onEvent != nil {
				return onEvent(AgentOutput{Done: true})
			}
			return nil
		}, nil
	}, started)
}

// launch 是 Start / StartResume / StartActivity 共用的内部启动路径：构造运行（newLiveRun，含根
// context 与取消载体）→ 由 build 装配执行闭包（装配期可做同步准备，如续跑认领现场）→ 走 start 完成
// 登记、冲突判定与收敛。build 返回错误时就地取消刚构造的运行（避免泄漏其最大时长计时器）并透传错误。
func (h *RunHub) launch(detached DetachedContext, sessionID, userID uint, namespace string, build func(run *liveRun) (runFunc, error), started func(runID serializer.HexUUID)) (serializer.HexUUID, error) {
	run, runCtx, err := h.newLiveRun(detached, sessionID, userID, namespace)
	if err != nil {
		return serializer.NewNilHexUUIDv4(), errors.WithStack(err)
	}
	exec, err := build(run)
	if err != nil {
		run.cancel()
		return serializer.NewNilHexUUIDv4(), errors.WithStack(err)
	}
	if err := h.start(run, runCtx, exec, started); err != nil {
		return serializer.NewNilHexUUIDv4(), errors.WithStack(err)
	}
	return run.runID, nil
}

// newLiveRun 构造一次运行的运行期结构（不登记、不执行）：校验组件已装配，分配运行标识并派生脱管上下文
// 与「单次运行最大时长」上界。返回运行结构、带超时的执行上下文（执行期供用）与错误。
func (h *RunHub) newLiveRun(detached DetachedContext, sessionID, userID uint, namespace string) (*liveRun, context.Context, error) {
	if h.agent() == nil {
		return nil, nil, errors.New("ai: run hub: no agent is assembled")
	}
	if h.store() == nil {
		return nil, nil, errors.New("ai: run hub: no session store is assembled")
	}
	ctx := context.WithoutCancel(detached.base())
	runCtx, cancel := context.WithTimeout(ctx, h.maxRunDuration())
	run := &liveRun{
		runID:          newAgentRunID(),
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
	return run, runCtx, nil
}

// start 登记并启动一次已构造好的运行：先占位再收敛（保证同一会话不出现两个在途运行共存），随后登记
// agent_live_runs（部分唯一索引 uq_agent_live_runs_running 据此拒绝并发在途运行）并启动落库与执行
// goroutine。登记失败时就地撤销占位并取消运行，返回错误。exec 为本次运行的执行闭包。
func (h *RunHub) start(run *liveRun, runCtx context.Context, exec runFunc, started func(runID serializer.HexUUID)) error {
	store := h.store()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		run.cancel()
		return errors.New("ai: run hub is closed")
	}
	// 先占位再收敛：并发 Start 时后到者能取到先到者并收敛它，保证同一会话不出现两个在途运行共存。
	prev := h.runs[run.sessionID]
	h.runs[run.sessionID] = run
	h.mu.Unlock()
	if prev != nil && prev != run {
		h.supersede(prev)
	}

	// 收敛完成后才登记：这样部分唯一索引 uq_agent_live_runs_running 不会与旧运行的在途登记冲突。
	if err := store.CreateLiveRun(run.ctx, &ai.AgentLiveRun{
		NamespaceName: run.namespace,
		UserID:        run.userID,
		RunID:         run.runID,
		SessionID:     run.sessionID,
		Phase:         ai.AgentRunPhaseRunning,
		LastSeq:       0,
		ExpiresAt:     time.Now().Add(agentLiveRunTTL),
	}); err != nil {
		h.mu.Lock()
		if h.runs[run.sessionID] == run {
			delete(h.runs, run.sessionID)
		}
		h.mu.Unlock()
		run.cancel()
		// 会话已有在途运行（CreateLiveRun 的 ON CONFLICT DO NOTHING 未写入）转为明确错误并记录
		// 告警，避免与其它登记失败混淆。
		if errors.Is(err, ErrAgentLiveRunAlreadyRunning) {
			if l := h.logger(); l != nil {
				l.Ctx(run.ctx).Warnf("ai: session %d already has an in-flight live run: %v", run.sessionID, err)
			}
			return errors.WithStack(errors.Newf("ai: run hub: session %d already has an in-flight live run", run.sessionID))
		}
		return errors.WithStack(err)
	}
	go h.persistLoop(run)
	// 发布初始 state 事件（running + 初始子状态）：在订阅回调与执行 goroutine 之前发布，使订阅方
	// （含按 after 回放）必然先收到「运行中」状态，而不必等第一个阶段 / 增量事件才出现状态。
	h.publishJSON(run, ai.AgentRunEventState, InteractionStateEvent{
		State:    ai.AgentInteractionStateRunning,
		Substate: run.substate,
	})
	if started != nil {
		started(run.runID)
	}
	go h.execute(run, runCtx, exec)
	return nil
}

// execute 在 runCtx 下执行一次运行并把输出转为事件：增量与完成项、思考过程、内容块、客户端工具请求经
// 各自的发布路径全部发布（阶段事件由执行闭包注入到输入里的 ToolReporter 上报）；结束后据「是否被取消 /
// 是否出错 / 是否挂起 / 是否有完成项」收敛终态并调用 finish。
func (h *RunHub) execute(run *liveRun, runCtx context.Context, exec runFunc) {
	var sawDone, sawToolRequest bool
	var runErr error
	if exec == nil {
		runErr = errors.New("ai: run hub: no agent is assembled")
	} else {
		runErr = exec(runCtx,
			func(out AgentOutput) error {
				// 块事件与增量同源同序：同一流项可能**同时**含增量与块事件（如正文增量开块），故块事件
				// 独立于下方「增量 / done / 思考」互斥分支之外逐条下发——先块后增量，使前端先据 block
				// 事件建立 / 收敛块，再把随之而来的增量归并到当前开放块。块创建与收敛各下发一次，
				// 收敛事件的 payload 为该块终态全文（前端 upsert by seq 即收敛）。
				for _, ev := range out.Blocks {
					h.publishJSON(run, ai.AgentRunEventBlock, ev)
				}
				switch {
				case out.Delta != "":
					h.reportSubstate(run, ai.AgentInteractionSubstateGenerating)
					h.publishDelta(run, "", out.Delta)
				case out.Done:
					sawDone = true
					h.publishJSON(run, ai.AgentRunEventDone, out)
				case out.Reasoning != "":
					h.reportSubstate(run, ai.AgentInteractionSubstateThinking)
					h.publishJSON(run, ai.AgentRunEventReasoning, map[string]string{"delta": out.Reasoning})
				}
				return nil
			},
			func(req ToolRequest) error {
				sawToolRequest = true
				h.reportSubstate(run, ai.AgentInteractionSubstateToolCalling)
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

// finish 收敛一次运行到终态：failed/cancelled 追加一条 error 事件（负载同时携带阶段，供前端据以
// 展示「本轮被中断」而非仅弹瞬时错误），随后停止宽限定时器、封存落库 goroutine 并等待其抽干、写回
// 终态阶段与最终序号、关闭全部订阅者通道并从在途表中移除。
func (h *RunHub) finish(run *liveRun, phase ai.AgentRunPhase, errMsg string) {
	if errMsg != "" {
		h.publishJSON(run, ai.AgentRunEventError, map[string]string{"error": errMsg, "phase": string(phase)})
	}
	// 终局 state 事件：必须在 run.finished 置位之前发布——publish 对 finished 的运行直接丢弃。
	// 终局状态由本运行的 phase 与内存缓冲里已发布的 tool_request 事件本地推导（不查库）。
	h.publishJSON(run, ai.AgentRunEventState, terminalStateEvent(run, phase))
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

// terminalStateEvent 由运行终态阶段本地推导终局的 state 事件负载（不查库）：cancelled → paused/aborted、
// failed → paused/failed、done → paused/success，suspended → waiting 并按下述判定子状态。挂起子状态
// 的口径与 SessionStore.PendingRunSubstate **同源**：用本运行内存缓冲里已发布的 tool_request 事件
// 判定——存在问卷类 ask_user 调用 → answer，否则（含列表为空）→ approval；ask_user 字面量仍只在
// client_tools.go 一处声明（isQuestionnaireTool）。暂停类的三个子状态与 StateOf 的 pausedSubstateOf
// 同源，故流式 state 事件与刷新后的 HTTP 快照出参口径一致。
func terminalStateEvent(run *liveRun, phase ai.AgentRunPhase) InteractionStateEvent {
	switch phase {
	case ai.AgentRunPhaseCancelled:
		return InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateAborted}
	case ai.AgentRunPhaseFailed:
		return InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateFailed}
	case ai.AgentRunPhaseDone:
		return InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateSuccess}
	case ai.AgentRunPhaseSuspended:
		run.mu.Lock()
		calls := pendingCallsOf(run.buffer.events)
		run.mu.Unlock()
		for _, call := range calls {
			if isQuestionnaireTool(call.ToolName) {
				return InteractionStateEvent{State: ai.AgentInteractionStateWaiting, Substate: ai.AgentInteractionSubstateAnswer}
			}
		}
		return InteractionStateEvent{State: ai.AgentInteractionStateWaiting, Substate: ai.AgentInteractionSubstateApproval}
	default:
		// 相位空间仅上述五个；其余（理论上不可达）保守归为暂停/成功。
		return InteractionStateEvent{State: ai.AgentInteractionStatePaused, Substate: ai.AgentInteractionSubstateSuccess}
	}
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

// contentEventType 判定事件是否为「内容类」事件（受取消前置条件抑制）：delta / reasoning / stage /
// tool_request / block / compress_start / compress_delta / compress_done。放行事件为 done / error /
// state——finish 需要在取消后下发携带 phase=cancelled 的 error 事件，state 是交互状态广播（Task 6 用）。
func contentEventType(typ ai.AgentRunEventType) bool {
	switch typ {
	case ai.AgentRunEventDelta, ai.AgentRunEventReasoning, ai.AgentRunEventStage,
		ai.AgentRunEventToolRequest, ai.AgentRunEventBlock, ai.AgentRunEventCompressStart,
		ai.AgentRunEventCompressDelta, ai.AgentRunEventCompressDone:
		return true
	default:
		return false
	}
}

// publish 发布一条事件：持锁自增序号、写内存缓冲、非阻塞扇出给订阅者，解锁后异步入落库队列。
// 取消前置条件在此一处判定：run.cancelled 为真（显式取消或退订宽限自动取消）时抑制一切内容类事件，
// 使「中止生效后不得再下发任何 delta / reasoning / stage / tool_request / block / compress_* 事件」成为
// 可断言的事实——所有发布路径（publishDelta / publishStage / publishJSON）都经
// 此处，无需在每个调用点各写一遍。done / error / state 照常放行。
func (h *RunHub) publish(run *liveRun, typ ai.AgentRunEventType, payload []byte) {
	run.mu.Lock()
	if run.finished {
		run.mu.Unlock()
		return
	}
	if run.cancelled && contentEventType(typ) {
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
// 阶段切换即子状态切换，故在此先上报子状态（stage → 思考 / 生成 / 工具调用），驱动 state 事件。
func (h *RunHub) publishStage(run *liveRun, ev StageEvent) {
	h.reportSubstate(run, substateForStage(ev.Stage))
	if ev.Delta != "" {
		h.publishDelta(run, string(ev.Stage), ev.Delta)
		return
	}
	h.publishJSON(run, ai.AgentRunEventStage, ev)
}

// substateForStage 把阶段标识映射为「运行中」的交互子状态：正文生成阶段 → generating，工具调用/结果
// 阶段 → tool_calling，其余（检索等前置与评估阶段）→ thinking。这是「后端上报子状态」的唯一映射点。
func substateForStage(stage EvalStage) ai.AgentInteractionSubstate {
	switch stage {
	case EvalStageAnswer:
		return ai.AgentInteractionSubstateGenerating
	case EvalStageToolCall, EvalStageToolResult:
		return ai.AgentInteractionSubstateToolCalling
	default:
		return ai.AgentInteractionSubstateThinking
	}
}

// reportSubstate 上报一次运行子状态并在值发生变化时发布 state 事件（State 恒为 running）：
// 已收敛（finished）或已取消（cancelled）的运行不再上报（与 publish 的放行口径一致）；值未变更时
// 不重复发布，避免同一子状态下每次增量都产生一条 state 事件。持锁比较、更新字段，**释放锁后**再
// 发布——publish 内部要取 run.mu，持锁发布会自锁死锁。
func (h *RunHub) reportSubstate(run *liveRun, substate ai.AgentInteractionSubstate) {
	run.mu.Lock()
	if run.finished || run.cancelled || run.substate == substate {
		run.mu.Unlock()
		return
	}
	run.substate = substate
	run.mu.Unlock()
	h.publishJSON(run, ai.AgentRunEventState, InteractionStateEvent{State: ai.AgentInteractionStateRunning, Substate: substate})
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
//
// 命中内存中的在途运行时，先置取消标记并调用根 context 的 cancel()（取消信号沿 context 树即时切断
// 上游模型流式请求与工具执行，从源头停止 token 消耗），再等待该运行的执行 goroutine 真正退出
// （finish 完成终局化并 close(finishDone)）后才返回，并设安全上限兜底（超时仅告警）。因此「取消
// 返回后不可能再有正文事件」是调用方可依赖的事实：返回时该活动已完成终局化。等待不在持有
// h.mu / run.mu 时进行，避免与 finish 的加锁互等。无内存运行（纯 DB 登记）时维持既有幂等语义：
// 仅校验归属 / 存在性后返回，不等待。
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
	done := run.finishDone
	run.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// 等待执行 goroutine 真正退出后再返回：finish 已 close(finishDone)，返回时该活动已完成终局化，
	// 故「取消返回后不可能再有正文事件」成为可断言的事实。安全上限兜底，超时仅告警、不阻塞返回。
	select {
	case <-done:
	case <-time.After(h.cancelTimeout()):
		if l := h.logger(); l != nil {
			l.Ctx(run.ctx).Warnf("ai: cancelled run %s did not finish within %s", run.runID, h.cancelTimeout())
		}
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
		st := &LiveRunStatus{RunID: run.runID, Phase: run.phase, LastSeq: run.seq, InMemory: true, Substate: run.substate}
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

// BroadcastInteractionState 在指定会话的内存在途运行上发布一条 state 事件；无内存运行（暂停 /
// 已锁定 / 等待中：后端此时无生成 goroutine）时为 no-op——此时状态随 HTTP 出参下发。属主不符同样
// 视为无在途活动并直接返回（不报错：「无在途活动」是正常事实，不是异常）。广播路径不写 DB：DB 登记
// 由运行自身的终局化写入，视图层只负责把重算出的状态推给当前在途运行的订阅者。
func (h *RunHub) BroadcastInteractionState(ctx context.Context, sessionID, userID uint, ev InteractionStateEvent) error {
	h.mu.Lock()
	run := h.runs[sessionID]
	h.mu.Unlock()
	if run == nil || run.userID != userID {
		return nil
	}
	h.publishJSON(run, ai.AgentRunEventState, ev)
	return nil
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
