package turns

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	sessionstore "zhongkuisec.com/zhongkui/internal/ai/session"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// sessionCheckpointPrefix 是 TurnLoop 使用的检查点键前缀。完整键为
// "session:<id>"。
const sessionCheckpointPrefix = "session:"

// sessionState 是由管理器拥有的每会话生命周期状态。
type sessionState struct {
	// turnMu 串行化同一会话的轮次启动/停止。
	turnMu sync.Mutex

	mu      sync.Mutex
	loop    sessionLoop
	current *turnStream
	// suspended 是挂起轮次的流：保留事件投影的块序号、工具调用定位与待更新块，
	// 供续跑在同一助手消息内原位更新。
	suspended *turnStream
	lastErr   string
	running   bool
	slot      modelsai.Slot
	version   uint64
}

// Manager 为每个会话持有一个活跃的 TurnLoop，并管理内存中的实时领域事件
// 缓冲区。
type Manager struct {
	// deps 是装配依赖集合。
	deps Deps

	// ctx 是脱离请求取消的管理器上下文。
	ctx context.Context
	// cancel 取消管理器上下文。
	cancel context.CancelFunc

	// mu 保护 states 与 closed。
	mu sync.Mutex
	// states 是每会话的生命周期与缓冲区状态。
	states map[uint]*sessionState
	// closed 表示管理器已关闭。
	closed bool
}

// New 构建会话轮次管理器。
func New(ctx context.Context, deps Deps) (*Manager, error) {
	if deps.Sessions == nil {
		return nil, errors.New("turns: nil session store")
	}
	mctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &Manager{
		deps:   deps,
		ctx:    mctx,
		cancel: cancel,
		states: make(map[uint]*sessionState),
	}, nil
}

func (m *Manager) runtime() *agentkit.Runtime {
	if m.deps.Agents == nil {
		return nil
	}
	return m.deps.Agents()
}

func (m *Manager) checkpointStore() adk.CheckPointStore {
	if m.deps.Checkpoints != nil {
		return m.deps.Checkpoints
	}
	if rt := m.runtime(); rt != nil {
		return rt.Checkpoints()
	}
	return nil
}

func (m *Manager) ensureOpen() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerClosed
	}
	return nil
}

func (m *Manager) stateFor(sessionID uint) *sessionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[sessionID]
	if !ok {
		state = &sessionState{}
		m.states[sessionID] = state
	}
	return state
}

func (m *Manager) peekState(sessionID uint) *sessionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[sessionID]
}

// markRunningForStream 仅在 stream 仍是会话当前轮次时更新 running；
// 被抢占的轮次不得清除新轮次的 running 标志。
func (m *Manager) markRunningForStream(sessionID uint, stream *turnStream, running bool) {
	if state := m.peekState(sessionID); state != nil {
		state.mu.Lock()
		if state.current == stream {
			state.running = running
		}
		state.mu.Unlock()
	}
}

func (m *Manager) markLastErrForStream(sessionID uint, stream *turnStream, message string) {
	if state := m.peekState(sessionID); state != nil {
		state.mu.Lock()
		if state.current == stream {
			state.lastErr = message
		}
		state.mu.Unlock()
	}
}

func (s *sessionState) beginTurn(stream *turnStream, slot modelsai.Slot) *turnStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.current
	s.current = stream
	s.slot = slot
	s.running = true
	s.version++
	return previous
}

func (s *sessionState) currentStream() *turnStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// takeSuspended 取出并清除挂起轮次的流。
func (s *sessionState) takeSuspended() *turnStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.suspended
	s.suspended = nil
	return stream
}

func (s *sessionState) loopInfo() (sessionLoop, modelsai.Slot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loop, s.slot
}

func (s *sessionState) takeLoop() sessionLoop {
	s.mu.Lock()
	defer s.mu.Unlock()
	loop := s.loop
	s.loop = nil
	return loop
}

func (s *sessionState) installLoop(loop sessionLoop, slot modelsai.Slot, stream *turnStream) {
	s.mu.Lock()
	s.loop = loop
	s.slot = slot
	s.current = stream
	s.running = true
	s.version++
	s.mu.Unlock()
}

func (s *sessionState) clearLoop(loop sessionLoop) {
	s.mu.Lock()
	if s.loop == loop {
		s.loop = nil
	}
	s.mu.Unlock()
}

func (s *sessionState) setRunning(running bool) {
	s.mu.Lock()
	s.running = running
	s.mu.Unlock()
}

func (s *sessionState) snapshot() (running bool, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.lastErr
}

func (m *Manager) deleteCheckpoint(ctx context.Context, checkpointID string) {
	if checkpointID == "" {
		return
	}
	store := m.checkpointStore()
	if store == nil {
		return
	}
	deleter, ok := store.(adk.CheckPointDeleter)
	if !ok {
		return
	}
	_ = deleter.Delete(ctx, checkpointID)
}

func (m *Manager) watchLoop(state *sessionState, loop sessionLoop) {
	go func() {
		loop.Wait()
		// 检查点的清理归 TurnLoop 所有：正常收敛时其 cleanup 删除已加载的键，挂起 / 停止时另存新现场。
		// 此处不再重复删除——否则本轮清理可能晚于「重新审批」写回的挂起快照，把刚恢复的现场删掉。
		var current *turnStream
		state.mu.Lock()
		if state.loop == loop {
			state.loop = nil
			state.running = false
			current = state.current
			// 挂起轮次保留其事件投影状态，供续跑原位更新审批 / 问卷块与工具调用。
			if current != nil && len(current.PendingBlocks()) > 0 {
				state.suspended = current
			}
			// 回合已结束：清空缓冲快照，避免 /render 继续回放这段已完成的流。
			// 否则刷新时会把运行期实时事件重新覆盖到由持久历史重建的块上，导致内容被截断 / 丢失。
			state.current = nil
		}
		state.mu.Unlock()
		if current != nil {
			// 循环在该轮次关闭流之前就退出了（例如 GenInput/PrepareAgent
			// 失败）：终止订阅者，而不是让它们
			// 永久阻塞。
			current.CloseNotice("error", "the session loop exited before completing the turn")
		}
	}()
}

// snapshotRedoCheckpoint 为即将被续跑 / 放弃的审批或问卷挂起现场另存一份图现场快照（键
// redo:<会话ID>:<交互ID>），使交互被作答后仍能回退到该次挂起重来。必须在挂起现场被续跑覆盖或
// 删除之前调用。同一挂起可能同时落审批与问卷两条交互（它们共用同一图现场），故逐一留底；快照
// 已存在时不覆盖，先到者即挂起当时的现场。anchorMessageID 为 0 时不按锚点过滤。
func (m *Manager) snapshotRedoCheckpoint(ctx context.Context, sess *modelsai.AgentSession, checkpointID string, anchorMessageID uint) {
	store := m.checkpointStore()
	if store == nil {
		return
	}
	rows, err := m.deps.Sessions.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		return
	}
	var targets []*modelsai.AgentInteraction
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Kind != modelsai.InteractionKindApproval && row.Kind != modelsai.InteractionKindQuestionnaire {
			continue
		}
		if row.Status != modelsai.InteractionStatusPending {
			continue
		}
		if anchorMessageID != 0 && row.MessageID != anchorMessageID {
			continue
		}
		targets = append(targets, row)
	}
	if len(targets) == 0 {
		return
	}
	data, ok, getErr := store.Get(ctx, checkpointID)
	if getErr != nil || !ok {
		return
	}
	for _, target := range targets {
		key := sessionstore.RedoCheckpointID(sess.ID, target.ID)
		if _, exists, getErr := store.Get(ctx, key); getErr == nil && exists {
			continue
		}
		if setErr := store.Set(ctx, key, data); setErr != nil && m.deps.Logger != nil {
			m.deps.Logger.Ctx(ctx).Warnf("turns: snapshot interaction checkpoint failed: %v", setErr)
		}
	}
}

// ReopenInteraction 回退到一条已作答 / 已决策的交互（审批或问卷）：删除该交互锚点之后的全部消息与
// 更晚交互，把该交互重置为待处理，并以挂起时的图现场重建挂起现场，使会话重新落「等待审批 / 等待
// 作答」——用户可重新选择，而无需重跑整轮回答。会话运行中（有在途回合）时由调用方守卫拒绝；目标
// 交互仍待处理时幂等。
func (m *Manager) ReopenInteraction(ctx context.Context, in ReopenInteractionInput) error {
	if err := m.ensureOpen(); err != nil {
		return errors.WithStack(err)
	}
	if in.SessionID == 0 {
		return errors.New("turns: session id is required")
	}
	sess, err := m.deps.Sessions.GetSession(ctx, in.SessionID, in.NamespaceName, in.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	target, calls, payloadJSON, err := m.findInteractionTarget(ctx, sess, in.CallIDs, in.InterruptIDs)
	if err != nil {
		return errors.WithStack(err)
	}
	if target.Status == modelsai.InteractionStatusPending {
		return nil
	}
	store := m.checkpointStore()
	if store == nil {
		return ErrRedoUnavailable
	}
	// 快照缺失（旧数据 / 进程重启前未留底 / 已随更晚交互清理）时无法恢复挂起时的图现场。
	data, ok, err := store.Get(ctx, sessionstore.RedoCheckpointID(sess.ID, target.ID))
	if err != nil || !ok {
		return ErrRedoUnavailable
	}
	// 沿用原回合使用的档位：挂起现场（含已消费行）记录着该次运行实际使用的 slot，避免回退后
	// 静默切换模型。
	requested := modelsai.Slot("")
	if run, runErr := m.deps.Sessions.LatestRunByAnchor(ctx, sess.ID, sess.UserID, target.MessageID); runErr == nil && run != nil {
		requested = run.Slot
	}
	slot, err := m.effectiveSlot(requested, sess.TargetModel)
	if err != nil {
		return errors.WithStack(err)
	}

	state := m.stateFor(sess.ID)
	state.turnMu.Lock()
	defer state.turnMu.Unlock()
	// 回退后本会话不应再有在途回合：先退役内存循环，避免其继续写回即将被删除的回合。
	if loop := state.takeLoop(); loop != nil {
		loop.Abort()
		loop.Wait()
	}
	// 先把挂起时的图现场写回会话检查点键：回退后状态即为等待输入，用户随即提交时续跑必须
	// 能读到该现场。写失败则不落库，避免出现「等待输入但无现场」的空档。
	previous, hadPrevious, _ := store.Get(ctx, sessionCheckpointID(sess.ID))
	if err := store.Set(ctx, sessionCheckpointID(sess.ID), data); err != nil {
		return errors.WithStack(err)
	}
	rewind := sessionstore.InteractionRewind{
		Calls:        calls,
		PayloadJSON:  payloadJSON,
		Slot:         slot,
		CheckpointID: sessionCheckpointID(sess.ID),
	}
	if err := m.deps.Sessions.RewindToInteraction(ctx, sess.ID, sess.UserID, target.ID, rewind); err != nil {
		// 落库失败即撤销刚写回的现场，避免留下「有现场却无挂起运行」的孤儿检查点。
		if hadPrevious {
			_ = store.Set(ctx, sessionCheckpointID(sess.ID), previous)
		} else {
			m.deleteCheckpoint(ctx, sessionCheckpointID(sess.ID))
		}
		if errors.Is(err, sessionstore.ErrInteractionRewindUnavailable) ||
			errors.Is(err, sessionstore.ErrInteractionNotFound) ||
			errors.Is(err, sessionstore.ErrMessageNotFound) {
			return ErrRedoUnavailable
		}
		return errors.WithStack(err)
	}
	state.setRunning(false)
	return nil
}

// findInteractionTarget 定位回退目标并给出重建现场所需的调用清单与重置后的交互载荷：
//   - 审批：按卡片上的调用 ID 定位（调用 ID 随卡片初版即下发，含已决策卡片），载荷保留调用清单、
//     清空决策；
//   - 问卷：按卡片上的根中断 ID 定位（已作答卡片不再渲染提交按钮，中断 ID 由卡片元信息下发），
//     调用清单取自该挂起现场，载荷保留题目、清空作答并置回待回答。
//
// 找不到目标、载荷缺少可续跑的中断 ID 或现场缺失时返回 ErrRedoUnavailable。
func (m *Manager) findInteractionTarget(ctx context.Context, sess *modelsai.AgentSession, callIDs, interruptIDs []string) (*modelsai.AgentInteraction, []contract.PendingCall, []byte, error) {
	if len(callIDs) > 0 {
		return m.findApprovalTarget(ctx, sess, callIDs)
	}
	if len(interruptIDs) > 0 {
		return m.findQuestionnaireTarget(ctx, sess, interruptIDs)
	}
	return nil, nil, nil, ErrRedoUnavailable
}

// findApprovalTarget 按审批卡片上的工具调用 ID 定位审批交互；匹配到多个时取最新一条。
func (m *Manager) findApprovalTarget(ctx context.Context, sess *modelsai.AgentSession, callIDs []string) (*modelsai.AgentInteraction, []contract.PendingCall, []byte, error) {
	wanted := make(map[string]struct{}, len(callIDs))
	for _, id := range callIDs {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil, nil, ErrRedoUnavailable
	}
	rows, err := m.deps.Sessions.ListInteractionsByKind(ctx, sess.ID, sess.UserID, modelsai.InteractionKindApproval)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	for _, row := range rows {
		if row == nil || row.Payload == nil {
			continue
		}
		var payload contract.ApprovalPayload
		if err := json.Unmarshal(row.Payload.Value(), &payload); err != nil || len(payload.Calls) == 0 {
			continue
		}
		matched := false
		for _, call := range payload.Calls {
			if _, ok := wanted[call.CallID]; ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if !resumableCalls(payload.Calls) {
			return nil, nil, nil, ErrRedoUnavailable
		}
		payload.Decisions = nil
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, nil, errors.WithStack(err)
		}
		return row, payload.Calls, data, nil
	}
	return nil, nil, nil, ErrRedoUnavailable
}

// findQuestionnaireTarget 按问卷卡片上的根中断 ID 定位问卷交互；调用清单取自该审批挂起现场
// （问卷载荷本身不含调用），载荷重置为「保留题目与中断 ID、清空作答」。
func (m *Manager) findQuestionnaireTarget(ctx context.Context, sess *modelsai.AgentSession, interruptIDs []string) (*modelsai.AgentInteraction, []contract.PendingCall, []byte, error) {
	wanted := make(map[string]struct{}, len(interruptIDs))
	for _, id := range interruptIDs {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil, nil, ErrRedoUnavailable
	}
	rows, err := m.deps.Sessions.ListInteractionsByKind(ctx, sess.ID, sess.UserID, modelsai.InteractionKindQuestionnaire)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	for _, row := range rows {
		if row == nil || row.Payload == nil {
			continue
		}
		var payload contract.QuestionnairePayload
		if err := json.Unmarshal(row.Payload.Value(), &payload); err != nil || len(payload.Questions) == 0 {
			continue
		}
		matched := false
		for _, id := range payload.InterruptIDs {
			if _, ok := wanted[id]; ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		run, runErr := m.deps.Sessions.LatestRunByAnchor(ctx, sess.ID, sess.UserID, row.MessageID)
		if runErr != nil || run == nil || run.PendingCallsJSON == nil {
			continue
		}
		var calls []contract.PendingCall
		if err := json.Unmarshal(run.PendingCallsJSON.Value(), &calls); err != nil || len(calls) == 0 {
			continue
		}
		if !resumableCalls(calls) {
			return nil, nil, nil, ErrRedoUnavailable
		}
		reset := contract.QuestionnairePayload{
			Questions:    payload.Questions,
			Awaiting:     true,
			InterruptIDs: payload.InterruptIDs,
		}
		data, err := json.Marshal(reset)
		if err != nil {
			return nil, nil, nil, errors.WithStack(err)
		}
		return row, calls, data, nil
	}
	return nil, nil, nil, ErrRedoUnavailable
}

// resumableCalls 报告调用清单是否至少携带一个根中断 ID：续跑以中断 ID 为 ResumeParams.Targets 的键。
func resumableCalls(calls []contract.PendingCall) bool {
	for _, call := range calls {
		if call.InterruptID != "" {
			return true
		}
	}
	return false
}

func (m *Manager) effectiveSlot(requested, declared modelsai.Slot) (modelsai.Slot, error) {
	fallback := modelsai.SlotStandard
	if m.deps.Params != nil {
		if value := m.deps.Params.String("agent.defaultSlot"); value != "" {
			fallback = modelsai.Slot(value)
		}
	}
	return modelsai.ResolveSlot(requested, declared, fallback)
}

func sessionCheckpointID(sessionID uint) string {
	return fmt.Sprintf("%s%d", sessionCheckpointPrefix, sessionID)
}

// Chat 启动一个轮次，当会话循环繁忙时抢占当前轮次。
func (m *Manager) Chat(ctx context.Context, in ChatInput) (*Stream, error) {
	if err := m.ensureOpen(); err != nil {
		return nil, errors.WithStack(err)
	}
	if in.SessionID == 0 {
		return nil, errors.New("turns: session id is required")
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, ErrNoQuery
	}

	sess, err := m.deps.Sessions.GetSession(ctx, in.SessionID, in.NamespaceName, in.UserID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rt := m.runtime()
	if rt == nil {
		return nil, errors.New("turns: agent runtime is not configured")
	}
	slot, err := m.effectiveSlot(in.Slot, sess.TargetModel)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kind, ok := rt.Kind(slot)
	if !ok {
		return nil, errors.Newf("turns: slot %q has no configured agent", slot)
	}

	businessJSON, err := json.Marshal(in.BusinessData)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	item := ChatItem{
		Query:            in.Query,
		UserID:           sess.UserID,
		NamespaceName:    sess.NamespaceName,
		DocIDs:           append([]uint(nil), in.DocIDs...),
		BusinessDataJSON: string(businessJSON),
		Thinking:         in.Thinking,
		Slot:             slot,
	}

	state := m.stateFor(sess.ID)
	state.turnMu.Lock()
	defer state.turnMu.Unlock()

	// 新提问会作废挂起现场：丢弃上一轮保留的事件投影状态。
	state.takeSuspended()
	pending, pendingErr := m.deps.Sessions.GetPendingInterrupt(ctx, sess.ID, sess.UserID)
	hadPending := pendingErr == nil && pending != ""
	existing, existingSlot := state.loopInfo()
	reuse := existing != nil && existingSlot == slot && existing.Kind() == kind && !hadPending

	if !reuse && existing != nil {
		// 先退役上一个循环，使其清理逻辑无法在检查点被清除后覆盖或
		// 重建它。
		state.clearLoop(existing)
		existing.Abort()
		existing.Wait()
	}
	// 新查询会放弃之前的挂起运行：先在作废前为待处理审批留底图现场快照（供「重新审批」回退），
	// 再放弃现场；其检查点不得被新循环重放。
	if hadPending {
		m.snapshotRedoCheckpoint(ctx, sess, sessionCheckpointID(sess.ID), 0)
		_ = m.deps.Sessions.AbortPendingRun(ctx, sess.ID, sess.UserID)
		_ = m.deps.Sessions.SetPendingInterrupt(ctx, sess.ID, sess.UserID, "")
		m.cancelPendingInteractions(ctx, sess)
		m.deleteCheckpoint(ctx, sessionCheckpointID(sess.ID))
	}

	stream := newTurnStream(defaultBufferCapacity, sess.ID, uuid.NewString())
	if previous := state.beginTurn(stream, slot); previous != nil {
		previous.ClosePreempted(stream.RunID())
	}

	if reuse {
		if err := existing.Chat(ctx, item, stream); err == nil {
			return m.streamFor(stream), nil
		}
		state.clearLoop(existing)
	}

	loop, err := m.buildLoop(ctx, sess, slot, kind, sessionCheckpointID(sess.ID), nil)
	if err != nil {
		stream.Close()
		state.setRunning(false)
		return nil, errors.WithStack(err)
	}
	state.installLoop(loop, slot, stream)
	if err := loop.Chat(ctx, item, stream); err != nil {
		state.clearLoop(loop)
		state.setRunning(false)
		stream.Close()
		return nil, errors.WithStack(err)
	}
	m.watchLoop(state, loop)
	return m.streamFor(stream), nil
}

// Action 通过审批/问卷操作恢复一个挂起的运行。
func (m *Manager) Action(ctx context.Context, in ActionInput) (*Stream, error) {
	if err := m.ensureOpen(); err != nil {
		return nil, errors.WithStack(err)
	}
	if in.SessionID == 0 {
		return nil, errors.New("turns: session id is required")
	}
	if err := contract.ValidateActionRequest(in.Action); err != nil {
		return nil, errors.WithStack(err)
	}
	// /action 只接受审批与问卷提交：下一步发送统一走 /chat（chat.send 不再经 /action）。
	if in.Action.Action == contract.ActionChatSend {
		return nil, contract.ErrInvalidAction
	}

	sess, err := m.deps.Sessions.GetSession(ctx, in.SessionID, in.NamespaceName, in.UserID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	rt := m.runtime()
	if rt == nil {
		return nil, errors.New("turns: agent runtime is not configured")
	}
	run, err := m.deps.Sessions.ClaimAgentRun(ctx, sess.ID, sess.NamespaceName, sess.UserID)
	if err != nil {
		if errors.Is(err, contract.ErrAgentRunNotFound) || errors.Is(err, contract.ErrAgentRunConsumed) {
			return nil, ErrNoPendingRun
		}
		return nil, errors.WithStack(err)
	}
	if run.CheckpointID == "" {
		return nil, ErrCheckpointMissing
	}
	if err := m.waitCheckpoint(ctx, run.CheckpointID); err != nil {
		return nil, errors.WithStack(err)
	}
	// 续跑会覆盖同一检查点键：先为本次待处理审批留底图现场快照，供用户决策后「重新审批」回退。
	m.snapshotRedoCheckpoint(ctx, sess, run.CheckpointID, run.AnchorMessageID)

	pending := decodePendingCalls(run)
	targets, err := mapActionTargets(in.Action, pending)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := m.resolvePendingInteractions(ctx, sess, in.Action); err != nil {
		if m.deps.Logger != nil {
			m.deps.Logger.Ctx(ctx).Warnf("turns: mark pending interactions resolved failed: %v", err)
		}
	}
	slot, err := m.effectiveSlot(run.Slot, sess.TargetModel)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kind, ok := rt.Kind(slot)
	if !ok {
		return nil, errors.Newf("turns: slot %q has no configured agent", slot)
	}

	item := ChatItem{
		UserID:        sess.UserID,
		NamespaceName: sess.NamespaceName,
		Slot:          slot,
		InterruptID:   actionInterruptID(in.Action, pending),
		Action: &TurnAction{
			Kind:    in.Action.Action,
			Targets: targets,
		},
	}

	state := m.stateFor(sess.ID)
	state.turnMu.Lock()
	defer state.turnMu.Unlock()

	// 在安装新轮次之前退役旧循环：其退出回调不得把新轮次的流
	// 误认为自己的流。
	if loop := state.takeLoop(); loop != nil {
		loop.Abort()
		loop.Wait()
	}
	// 退役旧循环后，挂起的检查点必须仍然存在。
	if err := m.waitCheckpoint(ctx, run.CheckpointID); err != nil {
		state.setRunning(false)
		return nil, errors.WithStack(err)
	}

	previous := state.currentStream()
	suspended := state.takeSuspended()
	if previous == nil {
		previous = suspended
	}
	stream := newTurnStream(defaultBufferCapacity, sess.ID, uuid.NewString())
	if previous != nil {
		// 续跑承接挂起轮次的块序号与工具调用定位：审批 / 问卷块原位更新，后续内容
		// 与挂起前同属一条助手消息。
		stream.ResumeFrom(previous, buildResumeUpdates(previous.PendingBlocks(), in.Action), sortedKeys(targets))
	} else {
		stream.ResumeFrom(nil, nil, sortedKeys(targets))
	}
	if prev := state.beginTurn(stream, slot); prev != nil {
		prev.ClosePreempted(stream.RunID())
	} else if previous != nil {
		previous.ClosePreempted(stream.RunID())
	}
	stream.SetMessageID(run.AnchorMessageID)

	loop, err := m.buildLoop(ctx, sess, slot, kind, run.CheckpointID, decodeCitations(run))
	if err != nil {
		stream.Close()
		state.setRunning(false)
		return nil, errors.WithStack(err)
	}
	state.installLoop(loop, slot, stream)
	if err := loop.Action(ctx, item, stream); err != nil {
		state.clearLoop(loop)
		state.setRunning(false)
		stream.Close()
		return nil, errors.WithStack(err)
	}
	m.watchLoop(state, loop)
	return m.streamFor(stream), nil
}

func (m *Manager) waitCheckpoint(ctx context.Context, checkpointID string) error {
	store := m.checkpointStore()
	if store == nil {
		return ErrCheckpointMissing
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, ok, err := store.Get(ctx, checkpointID)
		if err == nil && ok {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return errors.Wrapf(ErrCheckpointMissing, "%v", err)
			}
			return ErrCheckpointMissing
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (m *Manager) buildLoop(ctx context.Context, sess *modelsai.AgentSession, slot modelsai.Slot, kind llm.APIMode, checkpointID string, citations []retrieval.RetrievalResult) (sessionLoop, error) {
	rt := m.runtime()
	if rt == nil {
		return nil, errors.New("turns: agent runtime is not configured")
	}
	switch kind {
	case llm.APIModeResponses:
		handle, ok := rt.AgenticHandle(slot)
		if !ok {
			return nil, errors.Newf("turns: responses slot %q has no handle", slot)
		}
		return newGenericSessionLoop[*schema.AgenticMessage](m, handle, sess, checkpointID, citations, ctx), nil
	default:
		handle, ok := rt.ChatHandle(slot)
		if !ok {
			return nil, errors.Newf("turns: chat slot %q has no handle", slot)
		}
		return newGenericSessionLoop[*schema.Message](m, handle, sess, checkpointID, citations, ctx), nil
	}
}

func (m *Manager) streamFor(stream *turnStream) *Stream {
	replay, live, cancel := stream.Subscribe(-1)
	return &Stream{SessionID: stream.sessionID, Replay: replay, Events: live, Close: cancel}
}

// Events 返回给定索引之后的当前轮次缓冲区重放内容以及一个
// 实时订阅。无在途回合时返回 nil，由视图层返回 204。
func (m *Manager) Events(ctx context.Context, sessionID, userID uint, namespaceName string, after int) (*Stream, error) {
	if _, err := m.deps.Sessions.GetSession(ctx, sessionID, namespaceName, userID); err != nil {
		return nil, errors.WithStack(err)
	}
	state := m.peekState(sessionID)
	if state == nil {
		return nil, nil
	}
	stream := state.currentStream()
	if stream == nil || stream.Closed() {
		// 已收敛的轮次不再回放其缓冲：让前端以 /render 的持久历史为准。
		return nil, nil
	}
	first, next := stream.Range()
	if after < first-1 || after > next-1 {
		// 游标已超出环形缓冲区（或来自上一轮运行）：要求客户端重新 /render。
		return &Stream{
			SessionID: sessionID,
			Events:    closedEvents(),
			Close:     func() {},
			Resume:    &ResumeInfo{RunID: stream.RunID(), FromSeq: after, Reset: "gap"},
		}, nil
	}
	replay, live, cancel := stream.Subscribe(after)
	return &Stream{
		SessionID: sessionID,
		Replay:    replay,
		Events:    live,
		Close:     cancel,
		Resume:    &ResumeInfo{RunID: stream.RunID(), FromSeq: after},
	}, nil
}

// Abort 立即停止活跃循环，并用一张 cancelled 卡片关闭
// 轮次流。
func (m *Manager) Abort(ctx context.Context, sessionID, userID uint, namespaceName string) error {
	sess, err := m.deps.Sessions.GetSession(ctx, sessionID, namespaceName, userID)
	if err != nil {
		return errors.WithStack(err)
	}

	state := m.peekState(sess.ID)
	if state == nil {
		// 等待中中止：先为待处理审批留底图现场快照，再作废现场。
		m.snapshotRedoCheckpoint(ctx, sess, sessionCheckpointID(sess.ID), 0)
		_ = m.deps.Sessions.AbortPendingRun(ctx, sess.ID, sess.UserID)
		_ = m.deps.Sessions.SetPendingInterrupt(ctx, sess.ID, sess.UserID, "")
		m.cancelPendingInteractions(ctx, sess)
		m.deleteCheckpoint(ctx, sessionCheckpointID(sess.ID))
		return nil
	}

	state.turnMu.Lock()
	defer state.turnMu.Unlock()
	state.takeSuspended()

	loop := state.takeLoop()
	if loop != nil {
		loop.Abort()
		// 等待循环持久化其部分轮次，然后再清除待处理状态并删除
		// 检查点。
		loop.Wait()
	}
	stream := state.currentStream()
	if stream != nil {
		stream.CloseNotice("cancelled", "the turn was aborted")
	}
	state.setRunning(false)
	state.mu.Lock()
	state.lastErr = ""
	state.mu.Unlock()

	// 等待中中止：先为待处理审批留底图现场快照，再作废现场。
	m.snapshotRedoCheckpoint(ctx, sess, sessionCheckpointID(sess.ID), 0)
	_ = m.deps.Sessions.AbortPendingRun(ctx, sess.ID, sess.UserID)
	if err := m.deps.Sessions.SetPendingInterrupt(ctx, sess.ID, sess.UserID, ""); err != nil {
		return errors.WithStack(err)
	}
	m.cancelPendingInteractions(ctx, sess)
	m.deleteCheckpoint(ctx, sessionCheckpointID(sess.ID))
	return nil
}

// State 返回会话推导出的生命周期状态。
func (m *Manager) State(ctx context.Context, sessionID, userID uint, namespaceName string) (State, error) {
	sess, err := m.deps.Sessions.GetSession(ctx, sessionID, namespaceName, userID)
	if err != nil {
		return State{}, errors.WithStack(err)
	}
	pending, err := m.deps.Sessions.GetPendingInterrupt(ctx, sess.ID, sess.UserID)
	if err != nil {
		pending = ""
	}
	ids := splitInterruptIDs(pending)

	userTurns := 0
	if count, err := m.deps.Sessions.CountUserTurns(ctx, sess.ID, sess.NamespaceName, sess.UserID); err == nil {
		userTurns = count
	}

	running := false
	lastErr := ""
	runID := ""
	if state := m.peekState(sess.ID); state != nil {
		running, lastErr = state.snapshot()
		// 在途轮次的运行标识：刷新 / 重开抽屉后前端据此订阅 /events?after= 续播；已收敛的流不再暴露。
		if running {
			if current := state.currentStream(); current != nil && !current.Closed() {
				runID = current.RunID()
			}
		}
	}

	return State{
		Running:             running,
		RunID:               runID,
		Interrupted:         len(ids) > 0,
		PendingInterruptIDs: ids,
		UserTurns:           userTurns,
		LastErr:             lastErr,
	}, nil
}

// StopSession 停止并遗忘会话的循环。它会在删除会话记录
// 之前被调用。
func (m *Manager) StopSession(ctx context.Context, sessionID uint) error {
	m.mu.Lock()
	state := m.states[sessionID]
	delete(m.states, sessionID)
	m.mu.Unlock()
	if state == nil {
		return nil
	}

	state.turnMu.Lock()
	loop := state.takeLoop()
	stream := state.currentStream()
	state.setRunning(false)
	state.turnMu.Unlock()

	if loop != nil {
		loop.Abort()
		done := make(chan struct{})
		go func() {
			loop.Wait()
			close(done)
		}()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	if stream != nil {
		stream.Close()
	}
	return nil
}

// Shutdown 停止全部会话循环并释放管理器。
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	states := make([]*sessionState, 0, len(m.states))
	for _, state := range m.states {
		states = append(states, state)
	}
	m.mu.Unlock()

	for _, state := range states {
		state.turnMu.Lock()
		loop := state.takeLoop()
		stream := state.currentStream()
		state.setRunning(false)
		state.turnMu.Unlock()
		if loop != nil {
			loop.Abort()
		}
		if stream != nil {
			stream.Close()
		}
	}
	m.cancel()
}

func closedEvents() <-chan StreamItem {
	ch := make(chan StreamItem)
	close(ch)
	return ch
}

func splitInterruptIDs(pending string) []string {
	if strings.TrimSpace(pending) == "" {
		return nil
	}
	parts := strings.Split(pending, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ids = append(ids, part)
		}
	}
	return ids
}

func decodeInteraction(row *modelsai.AgentInteraction) (contract.Interaction, bool) {
	kind := contract.InteractionKind(row.Kind)
	status := contract.InteractionStatus(row.Status)
	raw := []byte(nil)
	if row.Payload != nil {
		raw = row.Payload.Value()
	}
	if len(raw) == 0 {
		return contract.Interaction{}, false
	}

	var payload any
	switch kind {
	case contract.InteractionApproval:
		var value contract.ApprovalPayload
		if err := json.Unmarshal(raw, &value); err != nil {
			return contract.Interaction{}, false
		}
		payload = value
	case contract.InteractionQuestionnaire:
		var value contract.QuestionnairePayload
		if err := json.Unmarshal(raw, &value); err != nil {
			return contract.Interaction{}, false
		}
		if status == contract.InteractionAnswered {
			value.Awaiting = false
		}
		payload = value
	case contract.InteractionNextSteps:
		var value contract.NextStepsPayload
		if err := json.Unmarshal(raw, &value); err != nil {
			return contract.Interaction{}, false
		}
		payload = value
	default:
		return contract.Interaction{}, false
	}
	return contract.Interaction{Kind: kind, Status: status, Payload: payload, MessageID: row.MessageID}, true
}

// mapActionTargets 使用挂起时持久化的待处理调用，把审批 / 问卷操作请求
// 映射为 ResumeParams targets。
func mapActionTargets(request contract.ActionRequest, pending []contract.PendingCall) (map[string]contract.ToolResumeResult, error) {
	targets := make(map[string]contract.ToolResumeResult)
	switch request.Action {
	case contract.ActionApprovalDecide:
		for _, decision := range request.Decisions {
			call, found := findPendingCallForDecision(pending, decision)
			interruptID := decision.InterruptID
			if interruptID == "" && found {
				interruptID = call.InterruptID
			}
			if interruptID == "" {
				interruptID = requestInterruptID(request)
			}
			if interruptID == "" {
				continue
			}
			callID := decision.CallID
			if callID == "" && found {
				callID = call.CallID
			}
			targets[interruptID] = contract.ToolResumeResult{
				CallID:   callID,
				Approved: decision.Approved,
				Reason:   decision.Reason,
				Result:   decision.Result,
				Error:    decision.Error,
			}
		}
	case contract.ActionQuestionnaireSubmit:
		for _, call := range pending {
			if call.ToolName != contract.AskUserToolName || call.InterruptID == "" {
				continue
			}
			subset := answersForCall(call, request.Answers)
			if len(subset) == 0 {
				targets[call.InterruptID] = contract.ToolResumeResult{
					CallID:   call.CallID,
					Approved: false,
				}
				continue
			}
			result, err := json.Marshal(map[string]any{"answers": subset})
			if err != nil {
				continue
			}
			targets[call.InterruptID] = contract.ToolResumeResult{
				CallID:   call.CallID,
				Approved: true,
				Result:   string(result),
			}
		}
	default:
		return nil, contract.ErrInvalidAction
	}
	if len(targets) == 0 {
		return nil, ErrNoResumeTargets
	}
	return targets, nil
}

func findPendingCallForDecision(pending []contract.PendingCall, decision contract.ApprovalDecision) (contract.PendingCall, bool) {
	if decision.CallID != "" {
		for _, call := range pending {
			if call.CallID == decision.CallID {
				return call, true
			}
		}
	}
	if decision.InterruptID != "" {
		for _, call := range pending {
			if call.InterruptID == decision.InterruptID {
				return call, true
			}
		}
	}
	return contract.PendingCall{}, false
}

// answersForCall 按待处理 ask_user 调用参数中携带的问题 ID 过滤
// 提交的答案。
func answersForCall(call contract.PendingCall, answers []contract.QuestionnaireAnswer) []contract.QuestionnaireAnswer {
	ids := questionIDs(call.Arguments)
	if len(ids) == 0 {
		out := make([]contract.QuestionnaireAnswer, 0, len(answers))
		for _, answer := range answers {
			if answer.Answered && strings.TrimSpace(answer.Answer) != "" {
				out = append(out, answer)
			}
		}
		return out
	}
	out := make([]contract.QuestionnaireAnswer, 0, len(answers))
	for _, answer := range answers {
		if _, ok := ids[answer.QuestionID]; !ok {
			continue
		}
		if !answer.Answered && strings.TrimSpace(answer.Answer) == "" {
			continue
		}
		answer.Answered = true
		out = append(out, answer)
	}
	return out
}

func questionIDs(arguments string) map[string]struct{} {
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	var args struct {
		Questions []struct {
			ID string `json:"id"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil
	}
	ids := make(map[string]struct{}, len(args.Questions))
	for _, question := range args.Questions {
		if question.ID != "" {
			ids[question.ID] = struct{}{}
		}
	}
	return ids
}

// cancelPendingInteractions 在轮次被放弃或中止时使待处理卡片失效
// （被抢占的运行只会由新的 chat/abort 显式中止）。
func (m *Manager) cancelPendingInteractions(ctx context.Context, sess *modelsai.AgentSession) {
	rows, err := m.deps.Sessions.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row == nil || row.Status != modelsai.InteractionStatusPending {
			continue
		}
		_ = m.deps.Sessions.UpdateInteractionStatus(ctx, row.ID, sess.UserID, modelsai.InteractionStatusCancelled, nil)
	}
}

// resolvePendingInteractions 把匹配的持久交互标记为已作答（或已取消），
// 并为历史回填提交的决定/答案。
func (m *Manager) resolvePendingInteractions(ctx context.Context, sess *modelsai.AgentSession, request contract.ActionRequest) error {
	rows, err := m.deps.Sessions.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, row := range rows {
		if row == nil || row.Status != modelsai.InteractionStatusPending {
			continue
		}
		status, payload, ok := updatedInteractionPayload(row, request)
		if !ok {
			continue
		}
		if err := m.deps.Sessions.UpdateInteractionStatus(ctx, row.ID, sess.UserID, status, payload); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func updatedInteractionPayload(row *modelsai.AgentInteraction, request contract.ActionRequest) (modelsai.InteractionStatus, []byte, bool) {
	if row.Payload == nil {
		return "", nil, false
	}
	raw := row.Payload.Value()
	switch {
	case request.Action == contract.ActionApprovalDecide && row.Kind == modelsai.InteractionKindApproval:
		var payload contract.ApprovalPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", nil, false
		}
		payload.Decisions = request.Decisions
		status := modelsai.InteractionStatusAnswered
		if len(request.Decisions) == 0 {
			status = modelsai.InteractionStatusCancelled
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return "", nil, false
		}
		return status, data, true
	case request.Action == contract.ActionQuestionnaireSubmit && row.Kind == modelsai.InteractionKindQuestionnaire:
		var payload contract.QuestionnairePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return "", nil, false
		}
		payload.Answers = request.Answers
		payload.Awaiting = false
		status := modelsai.InteractionStatusAnswered
		answered := false
		for _, answer := range request.Answers {
			if answer.Answered || strings.TrimSpace(answer.Answer) != "" {
				answered = true
				break
			}
		}
		if !answered {
			status = modelsai.InteractionStatusCancelled
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return "", nil, false
		}
		return status, data, true
	default:
		return "", nil, false
	}
}

func actionInterruptID(request contract.ActionRequest, pending []contract.PendingCall) string {
	if id := requestInterruptID(request); id != "" {
		return id
	}
	for _, call := range pending {
		if call.InterruptID != "" {
			return call.InterruptID
		}
	}
	return ""
}

func requestInterruptID(request contract.ActionRequest) string {
	if request.InterruptID != "" {
		return request.InterruptID
	}
	if len(request.InterruptIDs) > 0 && request.InterruptIDs[0] != "" {
		return request.InterruptIDs[0]
	}
	return ""
}
