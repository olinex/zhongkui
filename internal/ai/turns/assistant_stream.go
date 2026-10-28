package turns

// 本文件把 agent 事件流投影为 AI 助手领域事件（reasoning / content / tool_calls /
// approval / questionnaire / next_steps 块 + 交互与终态事件），并返回持久化所需的结果。

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// resumeBlock 是挂起交互块在助手消息中的定位与创建时载荷。
type resumeBlock struct {
	// Substate 交互子状态（审批 / 作答）。
	Substate assistant.InteractionSubstate
	// Seq 块在助手消息内的序号。
	Seq int
	// Kind 块类型（approval / questionnaire）。
	Kind assistant.BlockKind
	// Payload 块创建时的载荷（含调用清单或题目）。
	Payload any
}

// resumeBlockUpdate 是续跑时要原位下发的块终态。
type resumeBlockUpdate struct {
	// Seq 目标块的序号。
	Seq int
	// Kind 块类型。
	Kind assistant.BlockKind
	// Payload 更新后的整块载荷。
	Payload any
}

// projectionSeed 是事件投影的起始状态：续跑时承接上一轮的块序号、工具调用定位与
// 待更新块，使续跑内容与挂起前的内容处于同一条助手消息、同一组块序号中。
type projectionSeed struct {
	// BlockSeq 已分配的最大块序号。
	BlockSeq int
	// Round 已使用的最大工具轮次。
	Round int
	// ToolSeqs 调用 ID 到 tool_calls 块的定位。
	ToolSeqs map[string]int
	// ToolCalls 调用 ID 到当前调用事实。
	ToolCalls map[string]assistant.ToolCallRecord
	// Pending 挂起交互块。
	Pending map[assistant.InteractionSubstate]resumeBlock
	// Updates 续跑时需原位更新的块。
	Updates []resumeBlockUpdate
	// Interrupts 续跑消费的根中断 ID。
	Interrupts []string
}

// ProjectionSeed 返回事件投影起始状态的安全副本。
func (s *turnStream) ProjectionSeed() projectionSeed {
	s.mu.Lock()
	defer s.mu.Unlock()
	seed := projectionSeed{
		BlockSeq:   s.blockSeq,
		Round:      s.round,
		ToolSeqs:   make(map[string]int, len(s.toolSeqs)),
		ToolCalls:  make(map[string]assistant.ToolCallRecord, len(s.toolCalls)),
		Pending:    make(map[assistant.InteractionSubstate]resumeBlock, len(s.pendingBlocks)),
		Updates:    append([]resumeBlockUpdate(nil), s.resumeUpdates...),
		Interrupts: append([]string(nil), s.resumeIDs...),
	}
	for callID, seq := range s.toolSeqs {
		seed.ToolSeqs[callID] = seq
	}
	for callID, record := range s.toolCalls {
		seed.ToolCalls[callID] = record
	}
	for substate, block := range s.pendingBlocks {
		seed.Pending[substate] = block
	}
	return seed
}

// ResumeFrom 承接上一轮投影状态，并写入续跑要更新的块与消费的中断 ID。
func (s *turnStream) ResumeFrom(previous *turnStream, updates []resumeBlockUpdate, interrupts []string) {
	seed := projectionSeed{}
	if previous != nil {
		seed = previous.ProjectionSeed()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockSeq = seed.BlockSeq
	s.round = seed.Round
	s.toolSeqs = seed.ToolSeqs
	s.toolCalls = seed.ToolCalls
	s.pendingBlocks = seed.Pending
	s.resumeUpdates = updates
	s.resumeIDs = interrupts
	// 本次续跑会消费的挂起块已原位更新，不能再留给下一轮续跑重复下发。
	for substate, block := range s.pendingBlocks {
		for _, update := range updates {
			if update.Seq == block.Seq {
				delete(s.pendingBlocks, substate)
				break
			}
		}
	}
}

// PendingBlocks 返回挂起交互块，供续跑时构建原位更新。
func (s *turnStream) PendingBlocks() []resumeBlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]resumeBlock, 0, len(s.pendingBlocks))
	for _, block := range s.pendingBlocks {
		out = append(out, block)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// NoteBlockSeq 记录已分配的最大块序号。
func (s *turnStream) NoteBlockSeq(seq int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq > s.blockSeq {
		s.blockSeq = seq
	}
}

// NoteRound 记录已使用的最大工具轮次。
func (s *turnStream) NoteRound(round int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if round > s.round {
		s.round = round
	}
}

// NoteToolCall 记录调用所属的 tool_calls 块与最新事实。
func (s *turnStream) NoteToolCall(callID string, seq int, record assistant.ToolCallRecord) {
	if callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolSeqs[callID] = seq
	s.toolCalls[callID] = record
}

// NoteInteraction 记录挂起交互块的位置与载荷，供续跑原位更新。
func (s *turnStream) NoteInteraction(substate assistant.InteractionSubstate, block resumeBlock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingBlocks[substate] = block
}

// buildResumeUpdates 根据挂起块与用户提交的操作构建续跑原位更新：审批回填决策与执行结果，
// 问卷回填作答并置 awaiting=false。载荷类型不匹配的块跳过。
func buildResumeUpdates(blocks []resumeBlock, request contract.ActionRequest) []resumeBlockUpdate {
	if len(blocks) == 0 {
		return nil
	}
	decisions := make(map[string]contract.ApprovalDecision, len(request.Decisions))
	for _, decision := range request.Decisions {
		decisions[decision.CallID] = decision
	}
	answers := make([]contract.QuestionnaireAnswer, 0, len(request.Answers))
	for _, answer := range request.Answers {
		answers = append(answers, contract.QuestionnaireAnswer{
			QuestionID: answer.QuestionID,
			Answered:   answer.Answered,
			Answer:     answer.Answer,
			Custom:     answer.Custom,
		})
	}

	out := make([]resumeBlockUpdate, 0, len(blocks))
	for _, block := range blocks {
		switch block.Kind {
		case assistant.BlockKindApproval:
			payload, ok := block.Payload.(assistant.ApprovalBlockPayload)
			if !ok {
				continue
			}
			calls := append([]assistant.ApprovalCallRecord(nil), payload.Calls...)
			for i := range calls {
				decision, ok := decisions[calls[i].CallID]
				if !ok {
					continue
				}
				approved := decision.Approved
				calls[i].Decision = &approved
				calls[i].Result = decision.Result
				calls[i].Error = decision.Error
			}
			payload.Calls = calls
			out = append(out, resumeBlockUpdate{Seq: block.Seq, Kind: block.Kind, Payload: payload})
		case assistant.BlockKindQuestionnaire:
			payload, ok := block.Payload.(assistant.QuestionnaireBlockPayload)
			if !ok {
				continue
			}
			payload.Answers = answers
			payload.Awaiting = false
			out = append(out, resumeBlockUpdate{Seq: block.Seq, Kind: block.Kind, Payload: payload})
		}
	}
	return out
}

// sortedKeys 返回 map 键的升序切片，保证下发顺序稳定。
func sortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// assistantNextStepsFilter 剥离模型输出末尾的内联下一步段。
type assistantNextStepsFilter interface {
	Write(chunk string) string
	Close() string
	Steps() (steps []contract.NextStep, seen bool, err error)
	Seen() bool
}

// assistantStreamOptions 是事件投影的可选行为。
type assistantStreamOptions struct {
	newNextStepsFilter func() assistantNextStepsFilter
}

// assistantStreamOption 修改投影选项。
type assistantStreamOption func(*assistantStreamOptions)

// withAssistantNextStepsFilter 注入内联下一步过滤器工厂。
func withAssistantNextStepsFilter(factory func() assistantNextStepsFilter) assistantStreamOption {
	return func(options *assistantStreamOptions) {
		options.newNextStepsFilter = factory
	}
}

func resolveAssistantStreamOptions(opts []assistantStreamOption) assistantStreamOptions {
	var options assistantStreamOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func (o assistantStreamOptions) newFilter() assistantNextStepsFilter {
	if o.newNextStepsFilter == nil {
		return nil
	}
	return o.newNextStepsFilter()
}

// assistantProjector 把一条 agent 事件流投影为领域事件。
type assistantProjector[M adk.MessageType] struct {
	stream    *turnStream
	options   assistantStreamOptions
	seed      projectionSeed
	res       StreamResult[M]
	blockSeq  int
	round     int
	toolSeqs  map[string]int
	toolCalls map[string]assistant.ToolCallRecord

	reasoningSeq  int
	reasoningText strings.Builder

	contentSeq int
}

// streamAssistantEvents 投影一个回合的 agent 事件流，返回持久化所需结果。
// started 为 turn.started 载荷；续跑时还会下发 interaction.resumed 与挂起块终态。
func streamAssistantEvents[M adk.MessageType](stream *turnStream, events *adk.AsyncIterator[*adk.TypedAgentEvent[M]], started assistant.TurnStartedPayload, opts ...assistantStreamOption) (StreamResult[M], error) {
	p := &assistantProjector[M]{
		stream:    stream,
		options:   resolveAssistantStreamOptions(opts),
		seed:      stream.ProjectionSeed(),
		toolSeqs:  make(map[string]int),
		toolCalls: make(map[string]assistant.ToolCallRecord),
	}
	p.blockSeq = p.seed.BlockSeq
	p.round = p.seed.Round
	for callID, seq := range p.seed.ToolSeqs {
		p.toolSeqs[callID] = seq
	}
	for callID, record := range p.seed.ToolCalls {
		p.toolCalls[callID] = record
	}

	p.emitTurnStarted(started)
	if started.Resumed {
		if len(p.seed.Interrupts) > 0 {
			ids := append([]string(nil), p.seed.Interrupts...)
			p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
				return e.InteractionResumed(ids)
			})
		}
		for _, update := range p.seed.Updates {
			update := update
			p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
				return e.BlockUpdated(update.Seq, update.Kind, update.Payload)
			})
		}
	}
	p.emitMessageStarted(started)
	return p.streamEvents(events)
}

func (p *assistantProjector[M]) emitTurnStarted(started assistant.TurnStartedPayload) {
	payload := started
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.Next(assistant.EventTypeTurnStarted, payload)
	})
}

func (p *assistantProjector[M]) emitMessageStarted(started assistant.TurnStartedPayload) {
	payload := assistant.MessageStartedPayload{Role: "assistant"}
	if started.Resumed {
		payload.MessageID = started.AssistantMessageID
	}
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.Next(assistant.EventTypeMessageStarted, payload)
	})
}

// nextBlock 分配块序号、记录跨轮状态并下发 block.created。
func (p *assistantProjector[M]) nextBlock(kind assistant.BlockKind, payload any) int {
	p.blockSeq++
	p.stream.NoteBlockSeq(p.blockSeq)
	seq := p.blockSeq
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.BlockCreated(seq, kind, payload)
	})
	return seq
}

func (p *assistantProjector[M]) emitDelta(seq int, kind assistant.BlockKind, text string) {
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.BlockDelta(seq, kind, text)
	})
}

func (p *assistantProjector[M]) emitUpdated(seq int, kind assistant.BlockKind, payload any) {
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.BlockUpdated(seq, kind, payload)
	})
}

func (p *assistantProjector[M]) emitError(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.TurnError(message, "")
	})
}

// emitRetrying 把可重试错误作为非终态提示下发：code=retrying 表示模型调用将重试。
func (p *assistantProjector[M]) emitRetrying(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
		return e.TurnError(message, "retrying")
	})
}

// appendReasoning 追加思考增量：首个增量建块，其后发 block.delta。
func (p *assistantProjector[M]) appendReasoning(text string) {
	if text == "" {
		return
	}
	p.reasoningText.WriteString(text)
	if p.reasoningSeq == 0 {
		p.reasoningSeq = p.nextBlock(assistant.BlockKindReasoning, assistant.ReasoningBlockPayload{Text: text})
		return
	}
	p.emitDelta(p.reasoningSeq, assistant.BlockKindReasoning, text)
}

// sealReasoning 收敛当前思考块；下一段思考会另起一块。
func (p *assistantProjector[M]) sealReasoning() {
	if p.reasoningSeq == 0 {
		return
	}
	p.emitUpdated(p.reasoningSeq, assistant.BlockKindReasoning, assistant.ReasoningBlockPayload{Text: p.reasoningText.String()})
	p.reasoningSeq = 0
	p.reasoningText.Reset()
}

// openContent 新建正文块，载荷为首个增量。
func (p *assistantProjector[M]) openContent(text string) {
	p.contentSeq = p.nextBlock(assistant.BlockKindContent, assistant.ContentBlockPayload{Text: text})
}

func (p *assistantProjector[M]) appendContent(text string) {
	if text == "" {
		return
	}
	p.emitDelta(p.contentSeq, assistant.BlockKindContent, text)
}

// sealContent 收敛当前正文块为终态全文。
func (p *assistantProjector[M]) sealContent(final string) {
	if p.contentSeq == 0 {
		return
	}
	p.emitUpdated(p.contentSeq, assistant.BlockKindContent, assistant.ContentBlockPayload{Text: final})
	p.contentSeq = 0
}

// streamEvents 是投影循环：解释 agent 事件并产出实时事件与持久化结果。
func (p *assistantProjector[M]) streamEvents(events *adk.AsyncIterator[*adk.TypedAgentEvent[M]]) (StreamResult[M], error) {
	timings := newAssistantToolTimer()

	for {
		event, ok := events.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			if isRetryableAssistantError(event.Err) {
				p.sealReasoning()
				p.emitRetrying(event.Err)
				continue
			}
			p.sealReasoning()
			// 用户主动取消以 turn.completed(cancelled) 收敛，不再额外下发错误。
			if !isCancellation(event.Err) {
				p.emitError(event.Err)
			}
			p.res.Err = event.Err
			return p.res, event.Err
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			p.handleInterrupt(event.Action.Interrupted)
			return p.res, nil
		}

		if event.Output != nil {
			collectAssistantCitations(event.Output.CustomizedOutput, &p.res)
			if mv := event.Output.MessageOutput; mv != nil {
				if msgops.VariantIsToolResult(mv) {
					content, callID, name := msgops.DrainToolResult(mv)
					p.completeToolCall(callID, name, content, timings.elapsed(callID))
					p.res.Intermediates = append(p.res.Intermediates, msgops.NewToolResult[M](callID, name, content))
				} else if mv.IsStreaming && mv.MessageStream != nil {
					if err := p.consumeStream(mv, timings); err != nil {
						return p.res, errors.WithStack(err)
					}
				} else if !msgops.IsNil(mv.Message) {
					p.consumeMessage(mv.Message, timings)
				}
			}
		}

		if event.Action != nil && event.Action.Exit {
			break
		}
	}

	return p.res, nil
}

// consumeStream 投影一条流式 assistant 消息。
func (p *assistantProjector[M]) consumeStream(mv *adk.TypedMessageVariant[M], timings *assistantToolTimer) error {
	var textAcc, reasoningAcc strings.Builder
	var chunks []M
	var callOrder []int
	calls := make(map[int]*assistantStreamToolCall)
	nextSteps := p.options.newFilter()

	for {
		chunk, err := mv.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			p.sealReasoning()
			if isRetryableAssistantError(err) {
				// 模型调用将重试：丢弃部分输出。
				p.emitRetrying(err)
				return nil
			}
			if !isCancellation(err) {
				p.emitError(err)
			}
			p.res.Err = err
			return errors.WithStack(err)
		}

		chunks = append(chunks, chunk)

		for _, call := range msgops.ToolCalls(chunk) {
			acc, ok := calls[call.Index]
			if !ok {
				acc = &assistantStreamToolCall{index: call.Index}
				calls[call.Index] = acc
				callOrder = append(callOrder, call.Index)
			}
			if acc.id == "" {
				acc.id = call.ID
			}
			if acc.name == "" {
				acc.name = call.Name
			}
			acc.args.WriteString(call.Args)
		}

		if reasoning := msgops.ReasoningText(chunk); reasoning != "" {
			reasoningAcc.WriteString(reasoning)
			p.appendReasoning(reasoning)
		}

		if text := msgops.AssistantDeltaText(chunk); text != "" {
			if nextSteps != nil {
				text = nextSteps.Write(text)
			}
			if text != "" {
				textAcc.WriteString(text)
				if p.contentSeq == 0 {
					p.sealReasoning()
					p.openContent(text)
				} else {
					p.appendContent(text)
				}
			}
		}

		addAssistantUsage(chunk, &p.res)
	}

	p.sealReasoning()

	if reasoningAcc.Len() > 0 {
		appendAssistantReasoning(&p.res, reasoningAcc.String())
	}
	// 内联下一步段可能横跨多个分片，流结束时补发其前的正文尾量。
	if nextSteps != nil {
		if tail := nextSteps.Close(); tail != "" {
			textAcc.WriteString(tail)
			if p.contentSeq != 0 {
				p.appendContent(tail)
			} else {
				// 整段正文都因过滤器的标记前缀保留而延迟到关流才外发：此时补建正文块，
				// 避免只持久化正文却没有任何 content 块。
				p.openContent(tail)
			}
		}
	}
	if textAcc.Len() > 0 {
		text := textAcc.String()
		p.sealContent(text)
		p.res.LastContent = text
	}

	toolCalls := flattenAssistantToolCalls(calls, callOrder)
	p.emitToolCalls(toolCalls, timings)

	// 推荐的下一步只属于最终回答（不再调用工具）；中间轮的同一标记不生成块。
	if len(toolCalls) == 0 && p.res.LastContent != "" {
		p.appendInlineNextSteps(nextSteps)
	}

	if len(chunks) == 0 {
		return nil
	}
	if merged, err := msgops.ConcatChunks(chunks); err == nil && msgops.HasContent(merged) {
		// 内联段已从外发正文剥离，持久化的原生消息也必须是剥离后的正文，刷新 / 回放才不会再展示。
		if nextSteps != nil && nextSteps.Seen() {
			merged = msgops.ReplaceAssistantText(merged, p.res.LastContent)
		}
		p.res.Intermediates = append(p.res.Intermediates, msgops.NormalizeForSession(merged))
	} else if textAcc.Len() > 0 || len(toolCalls) > 0 {
		message := msgops.NewAssistant[M](textAcc.String(), toolCalls)
		p.res.Intermediates = append(p.res.Intermediates, msgops.NormalizeForSession(message))
	}
	return nil
}

// consumeMessage 投影一条完整（非流式）assistant 消息。
func (p *assistantProjector[M]) consumeMessage(msg M, timings *assistantToolTimer) {
	content := msgops.AssistantText(msg)
	reasoning := msgops.ReasoningText(msg)
	calls := msgops.ToolCalls(msg)
	nextSteps := p.options.newFilter()
	if nextSteps != nil {
		content = nextSteps.Write(content) + nextSteps.Close()
		if nextSteps.Seen() {
			msg = msgops.ReplaceAssistantText(msg, content)
		}
	}

	if reasoning != "" {
		p.appendReasoning(reasoning)
		p.sealReasoning()
		appendAssistantReasoning(&p.res, reasoning)
	}
	if content != "" {
		p.openContent(content)
		p.sealContent(content)
		p.res.LastContent = content
	}
	p.emitToolCalls(calls, timings)

	if content != "" && len(calls) == 0 {
		p.appendInlineNextSteps(nextSteps)
	}

	addAssistantUsage(msg, &p.res)

	if content != "" || len(calls) > 0 || msgops.HasContent(msg) {
		p.res.Intermediates = append(p.res.Intermediates, msgops.NormalizeForSession(msg))
	}
}

// emitToolCalls 把同一条消息内的工具调用投影为一个 tool_calls 块：首条随块创建，
// 其余按 callID 逐条下发。
func (p *assistantProjector[M]) emitToolCalls(calls []msgops.ToolCall, timings *assistantToolTimer) {
	if len(calls) == 0 {
		return
	}
	p.round++
	p.stream.NoteRound(p.round)
	round := p.round

	blockSeq := 0
	for _, call := range calls {
		if call.Name == "" {
			continue
		}
		timings.start(call.ID)
		record := assistant.ToolCallRecord{
			CallID:    call.ID,
			ToolName:  call.Name,
			Arguments: call.Args,
			ExecOwner: contract.ToolExecOwnerBackend,
			Round:     round,
			Status:    assistant.ToolCallStatusStart,
		}
		if seq, ok := p.toolSeqs[call.ID]; ok && call.ID != "" {
			// 同一调用（跨轮续跑 / 重复分片）：原位更新，不重复建块。
			p.toolCalls[call.ID] = record
			p.stream.NoteToolCall(call.ID, seq, record)
			p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
				return e.ToolCallUpdated(seq, record)
			})
			continue
		}
		if blockSeq == 0 {
			blockSeq = p.nextBlock(assistant.BlockKindToolCalls, assistant.ToolCallsBlockPayload{Calls: []assistant.ToolCallRecord{record}})
		} else {
			p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
				return e.ToolCallUpdated(blockSeq, record)
			})
		}
		p.toolCalls[call.ID] = record
		p.stream.NoteToolCall(call.ID, blockSeq, record)
	}
}

// completeToolCall 把工具结果落回对应调用：按 callID 原位更新状态、耗时与结果；
// 调用块不存在时（续跑现场缺失）新建一块终态调用。
func (p *assistantProjector[M]) completeToolCall(callID, name, content string, elapsed int64) {
	if callID == "" {
		return
	}
	record, known := p.toolCalls[callID]
	seq, located := p.toolSeqs[callID]
	if known && located {
		record.Status = assistant.ToolCallStatusDone
		record.Result = content
		record.ElapsedMs = elapsed
		if name != "" {
			record.ToolName = name
		}
		p.toolCalls[callID] = record
		p.stream.NoteToolCall(callID, seq, record)
		p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.ToolCallUpdated(seq, record)
		})
		return
	}
	p.round++
	p.stream.NoteRound(p.round)
	record = assistant.ToolCallRecord{
		CallID:    callID,
		ToolName:  name,
		ExecOwner: contract.ToolExecOwnerBackend,
		Round:     p.round,
		Status:    assistant.ToolCallStatusDone,
		ElapsedMs: elapsed,
		Result:    content,
	}
	seq = p.nextBlock(assistant.BlockKindToolCalls, assistant.ToolCallsBlockPayload{Calls: []assistant.ToolCallRecord{record}})
	p.toolCalls[callID] = record
	p.stream.NoteToolCall(callID, seq, record)
}

// handleInterrupt 把一次中断投影为审批 / 问卷块与 interaction.waiting 事件。
func (p *assistantProjector[M]) handleInterrupt(info *adk.InterruptInfo) {
	p.res.Interrupted = true

	var approvals []contract.PendingCall
	var approvalIDs []string
	var questions []contract.QuestionnaireQuestion
	var questionnaireIDs []string
	var pending []contract.PendingCall
	var fallback string

	for _, ctx := range info.InterruptContexts {
		if ctx == nil || !ctx.IsRootCause {
			continue
		}
		p.res.InterruptIDs = append(p.res.InterruptIDs, ctx.ID)

		switch request := ctx.Info.(type) {
		case *contract.ApprovalRequest:
			if request == nil {
				continue
			}
			call := pendingCallFromApproval(request, ctx.ID)
			approvals = append(approvals, call)
			pending = append(pending, call)
			approvalIDs = append(approvalIDs, ctx.ID)
		case contract.ApprovalRequest:
			call := pendingCallFromApproval(&request, ctx.ID)
			approvals = append(approvals, call)
			pending = append(pending, call)
			approvalIDs = append(approvalIDs, ctx.ID)
		case *contract.QuestionnaireRequest:
			if request == nil {
				continue
			}
			questions = append(questions, request.Questions...)
			questionnaireIDs = append(questionnaireIDs, ctx.ID)
			pending = append(pending, pendingCallFromQuestionnaire(request, ctx.ID))
		case contract.QuestionnaireRequest:
			questions = append(questions, request.Questions...)
			questionnaireIDs = append(questionnaireIDs, ctx.ID)
			pending = append(pending, pendingCallFromQuestionnaire(&request, ctx.ID))
		default:
			if ctx.Info != nil && fallback == "" {
				fallback = formatInterruptInfo(ctx.Info)
			}
		}
	}

	p.res.PendingCalls = append(p.res.PendingCalls, pending...)

	if len(approvals) > 0 {
		records := make([]assistant.ApprovalCallRecord, 0, len(approvals))
		callIDs := make([]string, 0, len(approvals))
		for _, call := range approvals {
			records = append(records, assistant.ApprovalCallRecord{
				CallID:           call.CallID,
				ToolName:         call.ToolName,
				Arguments:        call.Arguments,
				AuditLevel:       call.AuditLevel,
				ApprovalRequired: call.ApprovalRequired,
				ExecOwner:        call.ExecOwner,
			})
			if call.CallID != "" {
				callIDs = append(callIDs, call.CallID)
			}
			// 待审批调用在块内补全审核级别与执行归属，前端据此展示审核 chip 与执行说明。
			if call.CallID != "" {
				if record, ok := p.toolCalls[call.CallID]; ok {
					record.AuditLevel = call.AuditLevel
					record.ExecOwner = call.ExecOwner
					if seq, located := p.toolSeqs[call.CallID]; located {
						p.toolCalls[call.CallID] = record
						p.stream.NoteToolCall(call.CallID, seq, record)
						p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
							return e.ToolCallUpdated(seq, record)
						})
					}
				}
			}
		}
		payload := assistant.ApprovalBlockPayload{Calls: records, CallIDs: callIDs}
		seq := p.nextBlock(assistant.BlockKindApproval, payload)
		p.stream.NoteInteraction(assistant.InteractionSubstateApproval, resumeBlock{
			Substate: assistant.InteractionSubstateApproval,
			Seq:      seq,
			Kind:     assistant.BlockKindApproval,
			Payload:  payload,
		})
		ids := append([]string(nil), approvalIDs...)
		p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.InteractionWaiting(assistant.InteractionSubstateApproval, ids, seq)
		})
		p.res.Interactions = append(p.res.Interactions, contract.Interaction{
			Kind:    contract.InteractionApproval,
			Status:  contract.InteractionPending,
			Payload: contract.ApprovalPayload{Calls: approvals},
		})
	}

	if len(questions) > 0 {
		payload := assistant.QuestionnaireBlockPayload{
			Questions:    questions,
			Awaiting:     true,
			InterruptIDs: append([]string(nil), questionnaireIDs...),
		}
		seq := p.nextBlock(assistant.BlockKindQuestionnaire, payload)
		p.stream.NoteInteraction(assistant.InteractionSubstateAnswer, resumeBlock{
			Substate: assistant.InteractionSubstateAnswer,
			Seq:      seq,
			Kind:     assistant.BlockKindQuestionnaire,
			Payload:  payload,
		})
		ids := append([]string(nil), questionnaireIDs...)
		p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.InteractionWaiting(assistant.InteractionSubstateAnswer, ids, seq)
		})
		p.res.Interactions = append(p.res.Interactions, contract.Interaction{
			Kind:    contract.InteractionQuestionnaire,
			Status:  contract.InteractionPending,
			Payload: contract.QuestionnairePayload{Questions: questions, Awaiting: true, InterruptIDs: questionnaireIDs},
		})
	}

	if len(approvals) == 0 && len(questions) == 0 && fallback != "" {
		ids := append([]string(nil), p.res.InterruptIDs...)
		p.stream.Emit(func(e *assistant.Emitter) assistant.Event {
			return e.InteractionWaiting(assistant.InteractionSubstateApproval, ids, 0)
		})
	}
}

// appendInlineNextSteps 把过滤器解析出的推荐项追加为 next_steps 块与可持久化交互。
func (p *assistantProjector[M]) appendInlineNextSteps(filter assistantNextStepsFilter) {
	if filter == nil {
		return
	}
	steps, seen, err := filter.Steps()
	if err != nil || !seen || len(steps) == 0 {
		return
	}
	p.nextBlock(assistant.BlockKindNextSteps, assistant.NextStepsBlockPayload{Steps: steps})
	p.res.Interactions = append(p.res.Interactions, contract.Interaction{
		Kind:    contract.InteractionNextSteps,
		Status:  contract.InteractionAnswered,
		Payload: contract.NextStepsPayload{Steps: steps},
	})
}

// assistantStreamToolCall 是流式分片累计的一次工具调用。
type assistantStreamToolCall struct {
	index int
	id    string
	name  string
	args  strings.Builder
}

func flattenAssistantToolCalls(calls map[int]*assistantStreamToolCall, order []int) []msgops.ToolCall {
	out := make([]msgops.ToolCall, 0, len(order))
	for _, idx := range order {
		call := calls[idx]
		if call == nil || call.name == "" {
			continue
		}
		out = append(out, msgops.ToolCall{
			ID:    call.id,
			Name:  call.name,
			Args:  call.args.String(),
			Index: call.index,
		})
	}
	return out
}

// assistantToolTimer 记录一次运行内各工具调用的发起时刻，结果到达时算出执行耗时。
type assistantToolTimer struct {
	starts map[string]time.Time
}

func newAssistantToolTimer() *assistantToolTimer {
	return &assistantToolTimer{starts: make(map[string]time.Time)}
}

func (t *assistantToolTimer) start(callID string) {
	if callID == "" || t == nil {
		return
	}
	if _, ok := t.starts[callID]; ok {
		return
	}
	t.starts[callID] = time.Now()
}

func (t *assistantToolTimer) elapsed(callID string) int64 {
	if callID == "" || t == nil {
		return 0
	}
	started, ok := t.starts[callID]
	if !ok {
		return 0
	}
	delete(t.starts, callID)
	return time.Since(started).Milliseconds()
}

func appendAssistantReasoning[M adk.MessageType](res *StreamResult[M], text string) {
	if text == "" {
		return
	}
	if res.Reasoning == "" {
		res.Reasoning = text
		return
	}
	res.Reasoning += "\n" + text
}

func addAssistantUsage[M adk.MessageType](msg M, res *StreamResult[M]) {
	usage := assistantMessageUsage(msg)
	if usage == nil {
		return
	}
	res.PromptTokens += usage.PromptTokens
	res.CompletionTokens += usage.CompletionTokens
	res.TotalTokens += usage.TotalTokens
}

func collectAssistantCitations[M adk.MessageType](custom any, res *StreamResult[M]) {
	switch value := custom.(type) {
	case nil:
		return
	case []retrieval.RetrievalResult:
		res.Citations = append(res.Citations, value...)
	case retrieval.RetrievalResult:
		res.Citations = append(res.Citations, value)
	case *retrieval.RetrievalResult:
		if value != nil {
			res.Citations = append(res.Citations, *value)
		}
	}
}

// isRetryableAssistantError 判断哪些模型调用重试不应导致轮次失败。
func isRetryableAssistantError(err error) bool {
	if err == nil {
		return false
	}
	var willRetry *adk.WillRetryError
	if errors.As(err, &willRetry) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "429") ||
		strings.Contains(message, "Too Many Requests") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "qpm limit")
}

// pendingCallFromApproval 把审批中断负载转换为 PendingCall，并携带用于恢复定位的根中断 ID。
func pendingCallFromApproval(request *contract.ApprovalRequest, interruptID string) contract.PendingCall {
	return contract.PendingCall{
		CallID:           request.CallID,
		ToolName:         request.ToolName,
		Arguments:        request.Arguments,
		ExecOwner:        request.ExecOwner,
		AuditLevel:       request.AuditLevel,
		ApprovalRequired: request.ApprovalRequired,
		Round:            request.Round,
		InterruptID:      interruptID,
	}
}

// pendingCallFromQuestionnaire 把 ask_user 挂起表示为待处理的客户端调用（无需审批）。
func pendingCallFromQuestionnaire(request *contract.QuestionnaireRequest, interruptID string) contract.PendingCall {
	return contract.PendingCall{
		CallID:           request.CallID,
		ToolName:         contract.AskUserToolName,
		Arguments:        request.Arguments,
		ExecOwner:        contract.ToolExecOwnerClient,
		AuditLevel:       contract.AuditLevelNone,
		ApprovalRequired: false,
		Round:            request.Round,
		InterruptID:      interruptID,
	}
}

// assistantMessageUsage 从任一种消息表示中提取 token 用量。
func assistantMessageUsage[M adk.MessageType](msg M) *schema.TokenUsage {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil || m.ResponseMeta == nil {
			return nil
		}
		return m.ResponseMeta.Usage
	case *schema.AgenticMessage:
		if m == nil || m.ResponseMeta == nil {
			return nil
		}
		return m.ResponseMeta.TokenUsage
	default:
		return nil
	}
}

func formatInterruptInfo(info any) string {
	return strings.TrimSpace(fmt.Sprintf("%v", info))
}
