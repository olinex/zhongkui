package turns

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"

	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/knowledge"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// sessionLoop 是类型擦除后的接口，Manager 用它在不知道 M 的情况下
// 驱动一个会话的 TurnLoop。
type sessionLoop interface {
	// Chat 推入一个查询轮次（必要时抢占进行中的轮次）。
	Chat(ctx context.Context, item ChatItem, stream *turnStream) error
	// Action 推入一个恢复挂起运行的操作项。
	Action(ctx context.Context, item ChatItem, stream *turnStream) error
	// Abort 立即停止循环。
	Abort()
	// Wait 阻塞直到循环退出。
	Wait()
	// Slot 返回循环固定的模型槽位。
	Slot() modelsai.Slot
	// Kind 返回循环的消息表示（对话模型接口形态）。
	Kind() llm.APIMode
}

// turnBinding 把一个 ChatItem 与它使用的轮次流（以及之后的知识收集器）
// 关联起来。它通过运行上下文传递。
type turnBinding struct {
	stream    *turnStream
	collector *knowledge.Collector
	resume    bool
	// userMessage 是本轮提问落库后的事实，供 turn.started 替换前端本地回显。
	userMessage *assistant.UserMessageSnapshot
}

type turnBindingKey struct{}

func withTurnBinding(ctx context.Context, binding *turnBinding) context.Context {
	return context.WithValue(ctx, turnBindingKey{}, binding)
}

func turnBindingFrom(ctx context.Context) (*turnBinding, bool) {
	binding, ok := ctx.Value(turnBindingKey{}).(*turnBinding)
	return binding, ok && binding != nil
}

// genericSessionLoop 封装一个 adk.TurnLoop[*ChatItem, M] 及其会话、
// 逐轮次绑定和持久化状态访问。
type genericSessionLoop[M adk.MessageType] struct {
	owner        *Manager
	deps         Deps
	handle       *agentkit.SlotHandle[M]
	checkpointID string
	detached     context.Context

	loop             *adk.TurnLoop[*ChatItem, M]
	initialCitations []retrieval.RetrievalResult

	startOnce sync.Once
	waitOnce  sync.Once
	finished  chan struct{}

	mu      sync.Mutex
	sess    *modelsai.AgentSession
	turns   map[*ChatItem]*turnBinding
	started bool
	running bool
}

func newGenericSessionLoop[M adk.MessageType](
	owner *Manager,
	handle *agentkit.SlotHandle[M],
	sess *modelsai.AgentSession,
	checkpointID string,
	initialCitations []retrieval.RetrievalResult,
	ctx context.Context,
) *genericSessionLoop[M] {
	l := &genericSessionLoop[M]{
		owner:            owner,
		deps:             owner.deps,
		handle:           handle,
		checkpointID:     checkpointID,
		detached:         context.WithoutCancel(ctx),
		initialCitations: append([]retrieval.RetrievalResult(nil), initialCitations...),
		finished:         make(chan struct{}),
		sess:             sess,
		turns:            make(map[*ChatItem]*turnBinding),
	}
	l.loop = adk.NewTurnLoop(adk.TurnLoopConfig[*ChatItem, M]{
		GenInput:      l.genInput,
		GenResume:     l.genResume,
		PrepareAgent:  l.prepareAgent,
		OnAgentEvents: l.onAgentEvents,
		Store:         owner.checkpointStore(),
		CheckpointID:  checkpointID,
	})
	return l
}

// Slot 返回循环固定的模型槽位。
func (l *genericSessionLoop[M]) Slot() modelsai.Slot { return l.handle.Slot }

// Kind 返回循环的消息表示（对话模型接口形态）。
func (l *genericSessionLoop[M]) Kind() llm.APIMode { return l.handle.Kind }

// Chat 推入一个查询轮次（必要时抢占进行中的轮次）。
func (l *genericSessionLoop[M]) Chat(_ context.Context, item ChatItem, stream *turnStream) error {
	ptr := &item
	l.bindTurn(ptr, &turnBinding{stream: stream})
	preempt := l.isStarted()
	if err := l.push(ptr, preempt); err != nil {
		l.unbind([]*ChatItem{ptr})
		return errors.WithStack(err)
	}
	l.start()
	return nil
}

// Action 推入一个恢复挂起运行的操作项。
func (l *genericSessionLoop[M]) Action(_ context.Context, item ChatItem, stream *turnStream) error {
	ptr := &item
	l.bindTurn(ptr, &turnBinding{
		stream: stream,
		resume: true,
	})
	preempt := l.isStarted()
	if err := l.push(ptr, preempt); err != nil {
		l.unbind([]*ChatItem{ptr})
		return errors.WithStack(err)
	}
	l.start()
	return nil
}

func (l *genericSessionLoop[M]) push(item *ChatItem, preempt bool) error {
	var ok bool
	if preempt {
		ok, _ = l.loop.Push(item, adk.WithPreempt[*ChatItem, M](adk.AfterToolCalls))
	} else {
		ok, _ = l.loop.Push(item)
	}
	if !ok {
		l.setRunning(false)
		return ErrLoopClosed
	}
	l.setRunning(true)
	return nil
}

func (l *genericSessionLoop[M]) start() {
	l.startOnce.Do(func() {
		l.mu.Lock()
		l.started = true
		l.mu.Unlock()
		l.loop.Run(l.detached)
		go func() {
			l.loop.Wait()
			l.finish()
		}()
	})
}

func (l *genericSessionLoop[M]) finish() {
	l.waitOnce.Do(func() {
		l.setRunning(false)
		close(l.finished)
	})
}

func (l *genericSessionLoop[M]) isStarted() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.started
}

func (l *genericSessionLoop[M]) setRunning(running bool) {
	l.mu.Lock()
	l.running = running
	l.mu.Unlock()
}

// Abort 立即停止循环。
func (l *genericSessionLoop[M]) Abort() {
	l.loop.Stop(adk.WithImmediate())
}

// Wait 阻塞直到循环退出。
func (l *genericSessionLoop[M]) Wait() {
	<-l.finished
}

func (l *genericSessionLoop[M]) bindTurn(item *ChatItem, binding *turnBinding) {
	l.mu.Lock()
	l.turns[item] = binding
	l.mu.Unlock()
}

func (l *genericSessionLoop[M]) turnFor(item *ChatItem) *turnBinding {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.turns[item]
}

func (l *genericSessionLoop[M]) unbind(items []*ChatItem) {
	l.mu.Lock()
	for _, item := range items {
		delete(l.turns, item)
	}
	l.mu.Unlock()
}

func (l *genericSessionLoop[M]) setSession(sess *modelsai.AgentSession) {
	l.mu.Lock()
	l.sess = sess
	l.mu.Unlock()
}

func (l *genericSessionLoop[M]) session() *modelsai.AgentSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sess
}

// genInput 选择第一个查询项，追加用户消息，加载规范化历史并构建
// agent 输入。
func (l *genericSessionLoop[M]) genInput(ctx context.Context, loop *adk.TurnLoop[*ChatItem, M], items []*ChatItem) (*adk.GenInputResult[*ChatItem, M], error) {
	var (
		queryItem *ChatItem
		remaining []*ChatItem
	)
	for _, item := range items {
		if item == nil {
			continue
		}
		if queryItem == nil && strings.TrimSpace(item.Query) != "" {
			queryItem = item
			continue
		}
		remaining = append(remaining, item)
	}
	if queryItem == nil {
		loop.Stop(adk.WithStopCause("no query"))
		return nil, ErrNoQuery
	}

	binding := l.turnFor(queryItem)
	if binding == nil || binding.stream == nil {
		return nil, errors.New("turns: query item has no turn stream")
	}

	sess, err := l.loadSession(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	slot, err := l.effectiveSlot(queryItem.Slot, sess)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if slot != l.handle.Slot {
		return nil, errors.Newf("turns: loop slot %q does not match resolved slot %q", l.handle.Slot, slot)
	}

	maxCount := 0
	if l.deps.Params != nil {
		maxCount = l.deps.Params.Int("session.maxContextMessages")
	}
	stored, err := l.deps.Sessions.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, maxCount)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	history := decodeStoredMessages[M](stored)

	appended, err := l.appendUserMessage(ctx, sess, queryItem.Query)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	rc, runCtx := l.buildRunContext(ctx, sess, queryItem, slot, history, binding.stream, l.userMessageSnapshot(ctx, sess, queryItem.Query, appended))
	return &adk.GenInputResult[*ChatItem, M]{
		RunCtx:    runCtx,
		Input:     l.handle.NewInput(rc),
		RunOpts:   l.agentRunOptions(),
		Consumed:  []*ChatItem{queryItem},
		Remaining: remaining,
	}, nil
}

func (l *genericSessionLoop[M]) prepareAgent(_ context.Context, _ *adk.TurnLoop[*ChatItem, M], _ []*ChatItem) (adk.TypedAgent[M], error) {
	return l.handle.Agent, nil
}

// genResume 把第一个操作项映射为 ResumeParams.Targets。
func (l *genericSessionLoop[M]) genResume(ctx context.Context, _ *adk.TurnLoop[*ChatItem, M], interruptedItems, unhandledItems, newItems []*ChatItem) (*adk.GenResumeResult[*ChatItem, M], error) {
	item := findActionItem(interruptedItems, unhandledItems, newItems)
	if item == nil || item.Action == nil {
		return nil, ErrNoActionItem
	}
	binding := l.turnFor(item)
	if binding == nil || binding.stream == nil {
		return nil, errors.New("turns: action item has no turn stream")
	}

	targets := make(map[string]any, len(item.Action.Targets))
	for interruptID, target := range item.Action.Targets {
		value := target
		targets[interruptID] = &value
	}

	sess, err := l.loadSession(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	slot, err := l.effectiveSlot(item.Slot, sess)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if slot != l.handle.Slot {
		return nil, errors.Newf("turns: loop slot %q does not match resolved slot %q", l.handle.Slot, slot)
	}

	runCtx := l.buildResumeContext(ctx, sess, item, slot, binding.stream)
	return &adk.GenResumeResult[*ChatItem, M]{
		RunCtx:       runCtx,
		RunOpts:      l.agentRunOptions(),
		ResumeParams: &adk.ResumeParams{Targets: targets},
		Consumed:     interruptedItems,
		Remaining:    unhandledItems,
	}, nil
}

// agentRunOptions 为每次 agent run（新提问 / 审批续跑）生成独立的工具轮数闸门：
// ADK 的剩余轮数会随检查点跨挂起累计，审批次数会把额度耗尽；改为每个 run 段重新计数后，
// 人工交互不消耗工具轮额度，纯自主循环仍受 agent.maxToolRounds 约束。
func (l *genericSessionLoop[M]) agentRunOptions() []adk.AgentRunOption {
	if rt := l.deps.Agents(); rt != nil {
		return rt.RunOptions()
	}
	return nil
}

// onAgentEvents 把事件流投影为实时输出（缓冲 + 持久化），并在
// 运行因中断结束时记录挂起元数据。
func (l *genericSessionLoop[M]) onAgentEvents(ctx context.Context, tc *adk.TurnContext[*ChatItem, M], events *adk.AsyncIterator[*adk.TypedAgentEvent[M]]) error {
	binding, ok := turnBindingFrom(ctx)
	if !ok || binding.stream == nil {
		drainEvents(events)
		return nil
	}
	stream := binding.stream
	defer func() {
		stream.Close()
		l.unbind(tc.Consumed)
		l.setRunning(false)
		if l.owner != nil {
			if sess := l.session(); sess != nil {
				l.owner.markRunningForStream(sess.ID, stream, false)
			}
		}
	}()

	// 持久化必须能在请求取消（中止/抢占）后继续完成。
	persistCtx := context.WithoutCancel(ctx)

	res, err := streamAssistantEvents[M](stream, events,
		turnStartedPayload(stream, binding),
		withAssistantNextStepsFilter(func() assistantNextStepsFilter {
			return agentkit.NewInlineNextStepsFilter()
		}))
	if err == nil {
		err = res.Err
	}
	if binding.collector != nil {
		res.Citations = binding.collector.Snapshot()
	}
	// 实时引用以知识收集器的权威快照下发：投影期的 event-output 引用可能为空，而持久化与
	// 历史快照使用的是收集器快照，二者必须同源（否则实时终态没有引用列表、刷新后才出现）。
	if len(res.Citations) > 0 {
		citations := append([]retrieval.RetrievalResult(nil), res.Citations...)
		stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.Next(assistant.EventTypeMessageCitations, assistant.MessageCitationsPayload{Citations: citations})
		})
	}

	sess := l.session()
	phase := phaseForResult(res)
	// 终态耗时同时落库，使刷新后的历史用量与实时 turn.completed 一致。
	elapsedMs := time.Since(stream.CreatedAt()).Milliseconds()
	lastMessageID, lastAssistantID := l.persistIntermediates(persistCtx, sess, res, phase, elapsedMs)

	if res.Interrupted && lastAssistantID == 0 {
		if anchor := l.ensureAnchor(persistCtx, sess); anchor != 0 {
			lastAssistantID = anchor
			lastMessageID = anchor
		}
	}
	l.persistInteractions(persistCtx, sess, res.Interactions, lastAssistantID, lastMessageID)

	if res.Interrupted {
		ids := interruptIDsForResult(res)
		l.saveSuspendedRun(persistCtx, sess, res, lastAssistantID, ids)
		if setErr := l.deps.Sessions.SetPendingInterrupt(persistCtx, sess.ID, sess.UserID, strings.Join(ids, ",")); setErr != nil {
			l.warnf(persistCtx, "turns: persist pending interrupt failed: %v", setErr)
		}
		return nil
	}

	if setErr := l.deps.Sessions.SetPendingInterrupt(persistCtx, sess.ID, sess.UserID, ""); setErr != nil {
		l.warnf(persistCtx, "turns: clear pending interrupt failed: %v", setErr)
	}

	if err != nil {
		l.setLastErr(sess.ID, stream, err.Error())
	} else {
		l.setLastErr(sess.ID, stream, "")
	}

	l.emitTerminalEvents(stream, phase, lastAssistantID, res, elapsedMs)
	l.owner.deleteCheckpoint(persistCtx, l.checkpointID)
	return nil
}

// turnStartedPayload 装配 turn.started：新提问携带用户消息事实与 userIndex，续跑携带
// 助手消息锚点。
func turnStartedPayload(stream *turnStream, binding *turnBinding) assistant.TurnStartedPayload {
	payload := assistant.TurnStartedPayload{RunID: stream.RunID(), Resumed: binding.resume}
	if binding.resume {
		payload.AssistantMessageID = stream.MessageID()
		return payload
	}
	payload.UserMessage = binding.userMessage
	return payload
}

// emitTerminalEvents 在持久化完成后下发 message.persisted 与 turn.completed，并收敛流。
func (l *genericSessionLoop[M]) emitTerminalEvents(stream *turnStream, phase modelsai.AgentRunPhase, messageID uint, res StreamResult[M], elapsedMs int64) {
	if messageID != 0 {
		stream.SetMessageID(messageID)
		stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.Next(assistant.EventTypeMessagePersisted, assistant.MessagePersistedPayload{MessageID: messageID})
		})
	}
	usage := &assistant.Usage{
		PromptTokens:     res.PromptTokens,
		CompletionTokens: res.CompletionTokens,
		TotalTokens:      res.TotalTokens,
		ElapsedMs:        elapsedMs,
	}
	stream.CloseTurn(phase, messageID, usage)
}

func (l *genericSessionLoop[M]) buildRunContext(ctx context.Context, sess *modelsai.AgentSession, item *ChatItem, slot modelsai.Slot, history []M, stream *turnStream, userMessage *assistant.UserMessageSnapshot) (agentkit.RunContext[M], context.Context) {
	request := agentkit.Request{
		Query:         item.Query,
		NamespaceName: firstNonEmpty(item.NamespaceName, sess.NamespaceName),
		DocIDs:        append([]uint(nil), item.DocIDs...),
		SessionID:     sess.ID,
		UserID:        firstNonZero(item.UserID, sess.UserID),
		AgentKind:     sess.AgentKind,
		BusinessData:  decodeBusinessData(item.BusinessDataJSON),
		Thinking:      item.Thinking,
		Slot:          slot,
		ReviewValve:   sess.ReviewValve,
	}
	rc := agentkit.RunContext[M]{
		History: history,
		Request: request,
		Summary: string(sess.Summary.Value()),
	}

	collector := knowledge.NewCollector(nil)
	runCtx := agentkit.WithRunContext[M](ctx, rc)
	runCtx = withTurnBinding(runCtx, &turnBinding{
		stream:      stream,
		collector:   collector,
		userMessage: userMessage,
	})
	runCtx = l.withToolRuntimes(runCtx, request, collector)
	return rc, runCtx
}

func (l *genericSessionLoop[M]) buildResumeContext(ctx context.Context, sess *modelsai.AgentSession, item *ChatItem, slot modelsai.Slot, stream *turnStream) context.Context {
	request := agentkit.Request{
		NamespaceName: firstNonEmpty(item.NamespaceName, sess.NamespaceName),
		DocIDs:        append([]uint(nil), item.DocIDs...),
		SessionID:     sess.ID,
		UserID:        firstNonZero(item.UserID, sess.UserID),
		AgentKind:     sess.AgentKind,
		Thinking:      item.Thinking,
		Slot:          slot,
		ReviewValve:   sess.ReviewValve,
	}

	collector := knowledge.NewCollector(l.initialCitations)
	runCtx := withTurnBinding(ctx, &turnBinding{
		stream:    stream,
		collector: collector,
		resume:    true,
	})
	runCtx = l.withToolRuntimes(runCtx, request, collector)
	return runCtx
}

func (l *genericSessionLoop[M]) withToolRuntimes(ctx context.Context, request agentkit.Request, collector *knowledge.Collector) context.Context {
	if l.deps.Retriever != nil {
		snippetLimit := 0
		if l.deps.Params != nil {
			snippetLimit = l.deps.Params.Int("agent.retrievedDocSnippetLimit")
		}
		searcher := &knowledge.Searcher{
			Retriever:     l.deps.Retriever,
			NamespaceName: request.NamespaceName,
			DocIDs:        request.DocIDs,
			Collector:     collector,
		}
		ctx = tools.WithKnowledgeRuntime(ctx, tools.KnowledgeRuntime{
			Searcher:     searcher,
			SnippetLimit: snippetLimit,
		})
	}
	if l.deps.Memory != nil {
		if writer := l.deps.Memory.Writer(); writer != nil {
			ctx = tools.WithMemoryRuntime(ctx, tools.MemoryRuntime{
				Writer:        writer,
				NamespaceName: request.NamespaceName,
				AgentKind:     request.AgentKind,
				SessionID:     request.SessionID,
				UserID:        request.UserID,
			})
		}
	}
	return ctx
}

func (l *genericSessionLoop[M]) loadSession(ctx context.Context) (*modelsai.AgentSession, error) {
	current := l.session()
	if current == nil {
		return nil, errors.New("turns: session is not loaded")
	}
	sess, err := l.deps.Sessions.GetSession(ctx, current.ID, current.NamespaceName, current.UserID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	l.setSession(sess)
	return sess, nil
}

// userMessageSnapshot 把刚落库的提问整理为 turn.started 的用户消息事实；userIndex 取
// 落库后的用户提问条数（从 1 起）。
func (l *genericSessionLoop[M]) userMessageSnapshot(ctx context.Context, sess *modelsai.AgentSession, query string, appended *modelsai.AgentMessage) *assistant.UserMessageSnapshot {
	if appended == nil {
		return nil
	}
	snapshot := &assistant.UserMessageSnapshot{
		MessageID: appended.ID,
		Content:   query,
		CreatedAt: appended.CreatedAt.UTC().Format(time.RFC3339),
	}
	if count, err := l.deps.Sessions.CountUserTurns(ctx, sess.ID, sess.NamespaceName, sess.UserID); err == nil {
		snapshot.UserIndex = count
	}
	return snapshot
}

func (l *genericSessionLoop[M]) effectiveSlot(requested modelsai.Slot, sess *modelsai.AgentSession) (modelsai.Slot, error) {
	fallback := modelsai.SlotStandard
	if l.deps.Params != nil {
		if value := l.deps.Params.String("agent.defaultSlot"); value != "" {
			fallback = modelsai.Slot(value)
		}
	}
	return modelsai.ResolveSlot(requested, sess.TargetModel, fallback)
}

func (l *genericSessionLoop[M]) setLastErr(sessionID uint, stream *turnStream, message string) {
	if l.owner != nil {
		l.owner.markLastErrForStream(sessionID, stream, message)
	}
}

func (l *genericSessionLoop[M]) warnf(ctx context.Context, format string, args ...any) {
	if l.deps.Logger == nil {
		return
	}
	l.deps.Logger.Ctx(ctx).Warnf(format, args...)
}

func decodeBusinessData(raw string) contract.BusinessData {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var data contract.BusinessData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil
	}
	return data
}

func findActionItem(groups ...[]*ChatItem) *ChatItem {
	for _, items := range groups {
		for _, item := range items {
			if item != nil && item.Action != nil {
				return item
			}
		}
	}
	return nil
}

func drainEvents[M adk.MessageType](events *adk.AsyncIterator[*adk.TypedAgentEvent[M]]) {
	for {
		if _, ok := events.Next(); !ok {
			return
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...uint) uint {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
