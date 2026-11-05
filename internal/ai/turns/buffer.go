package turns

import (
	"sync"
	"time"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// defaultBufferCapacity 是每轮次为断线重放保留的输出条目数量。
// 较旧的条目会从环形缓冲区中淘汰；/render 会从会话存储重建
// 持久历史。
const defaultBufferCapacity = 512

// indexedItem 是一条带重放索引的缓冲条目。
type indexedItem struct {
	index int
	item  StreamItem
}

// turnStream 是一轮次内输出条目的缓冲区。它是有界环形缓冲区，并向实时
// 订阅者扇出。索引在每个轮次内单调递增（从 1 开始，与事件游标一致），
// 订阅者可用 after=<最后看到的索引> 重连。
type turnStream struct {
	mu        sync.Mutex
	capacity  int
	sessionID uint
	runID     string
	createdAt time.Time
	emitter   *assistant.Emitter

	items     []indexedItem
	nextIndex int
	subs      map[uint64]chan StreamItem
	nextSub   uint64
	closed    bool

	// 以下字段是事件投影跨轮续接所需的状态，受 mu 保护。
	blockSeq      int
	round         int
	toolSeqs      map[string]int
	toolCalls     map[string]assistant.ToolCallRecord
	pendingBlocks map[assistant.InteractionSubstate]resumeBlock
	resumeUpdates []resumeBlockUpdate
	resumeIDs     []string
}

func newTurnStream(capacity int, sessionID uint, runID string) *turnStream {
	if capacity <= 0 {
		capacity = defaultBufferCapacity
	}
	return &turnStream{
		capacity:      capacity,
		sessionID:     sessionID,
		runID:         runID,
		createdAt:     time.Now(),
		emitter:       assistant.NewEmitter(sessionID, runID),
		items:         make([]indexedItem, 0, capacity),
		nextIndex:     1,
		subs:          make(map[uint64]chan StreamItem),
		toolSeqs:      make(map[string]int),
		toolCalls:     make(map[string]assistant.ToolCallRecord),
		pendingBlocks: make(map[assistant.InteractionSubstate]resumeBlock),
	}
}

// RunID 返回该轮次的运行 ID。
func (s *turnStream) RunID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runID
}

// CreatedAt 返回该轮次流的创建时刻，用于计算终态耗时。
func (s *turnStream) CreatedAt() time.Time {
	if s == nil {
		return time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createdAt
}

// SetMessageID 回填后续事件携带的助手消息 ID。
func (s *turnStream) SetMessageID(messageID uint) {
	if s == nil || messageID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitter.SetMessageID(messageID)
}

// MessageID 返回当前事件所属助手消息 ID。
func (s *turnStream) MessageID() uint {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emitter.MessageID()
}

// appendLocked 记录一条条目并扇出。调用方需持有 mu。
func (s *turnStream) appendLocked(item StreamItem) {
	if s.closed {
		return
	}
	event := indexedItem{index: s.nextIndex, item: item}
	s.nextIndex++
	if len(s.items) >= s.capacity {
		copy(s.items, s.items[1:])
		s.items[len(s.items)-1] = event
	} else {
		s.items = append(s.items, event)
	}
	for _, ch := range s.subs {
		select {
		case ch <- item:
		default:
			// 订阅者过慢：丢弃这条条目。它可以带上 after 重连并重放环形缓冲区，
			// 而不是阻塞 agent。
		}
	}
}

// Append 记录一条输出条目并扇出给实时订阅者。
func (s *turnStream) Append(item StreamItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendLocked(item)
}

// AppendEvent 记录一条事件。
func (s *turnStream) AppendEvent(event assistant.Event) {
	s.Append(StreamItem{Event: &event})
}

// Emit 生成并追加一条实时事件：游标分配与入队在同一临界区完成，
// 保证重放顺序与事件 seq 一致。
func (s *turnStream) Emit(build func(*assistant.Emitter) assistant.Event) assistant.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := build(s.emitter)
	s.appendLocked(StreamItem{Event: &event})
	return event
}

// Closed 报告流是否已关闭（回合已收敛）。
func (s *turnStream) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Items 返回索引大于 after 的缓冲条目（-1 返回全部）。
func (s *turnStream) Items(after int) []StreamItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StreamItem, 0, len(s.items))
	for _, event := range s.items {
		if event.index > after {
			out = append(out, event.item)
		}
	}
	return out
}

// Range 返回当前缓冲的最小索引（空缓冲时等于 NextIndex）与下一个待分配索引。
func (s *turnStream) Range() (first, next int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next = s.nextIndex
	if len(s.items) == 0 {
		return next, next
	}
	return s.items[0].index, next
}

// Subscribe 返回索引大于 after 的缓冲条目以及一个实时通道。返回的
// cancel 函数会解除订阅并关闭通道。
func (s *turnStream) Subscribe(after int) ([]StreamItem, <-chan StreamItem, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	replay := make([]StreamItem, 0, len(s.items))
	for _, event := range s.items {
		if event.index > after {
			replay = append(replay, event.item)
		}
	}

	ch := make(chan StreamItem, 64)
	if s.closed {
		close(ch)
		return replay, ch, func() {}
	}

	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if existing, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(existing)
		}
	}
	return replay, ch, cancel
}

// Close 关闭流以及所有实时订阅者通道。之后的写入会被
// 忽略。
func (s *turnStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

// ClosePreempted 以被抢占事件收敛流。
func (s *turnStream) ClosePreempted(byRunID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	event := s.emitter.Next(assistant.EventTypeTurnPreempted, assistant.TurnPreemptedPayload{ByRunID: byRunID})
	s.appendLocked(StreamItem{Event: &event})
	s.closeLocked()
}

// CloseNotice 以中止 / 错误提示收敛流：下发 turn.error（仅 error）与 turn.completed。
func (s *turnStream) CloseNotice(kind, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if kind == "error" {
		event := s.emitter.TurnError(text, "")
		s.appendLocked(StreamItem{Event: &event})
	}
	phase := modelsai.AgentRunPhaseCancelled
	if kind == "error" {
		phase = modelsai.AgentRunPhaseFailed
	}
	event := s.emitter.TurnCompleted(phase, s.emitter.MessageID(), nil)
	s.appendLocked(StreamItem{Event: &event})
	s.closeLocked()
}

// CloseTurn 以 turn.completed 收敛一条流；已关闭时忽略。
func (s *turnStream) CloseTurn(phase modelsai.AgentRunPhase, messageID uint, usage *assistant.Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if messageID == 0 {
		// 本轮未产生新的助手消息时沿用续跑锚点，便于前端把终态归到同一条气泡。
		messageID = s.emitter.MessageID()
	}
	if messageID != 0 {
		s.emitter.SetMessageID(messageID)
	}
	event := s.emitter.TurnCompleted(phase, messageID, usage)
	s.appendLocked(StreamItem{Event: &event})
	s.closeLocked()
}

func (s *turnStream) closeLocked() {
	if s.closed {
		return
	}
	s.closed = true
	for id, ch := range s.subs {
		delete(s.subs, id)
		close(ch)
	}
}
