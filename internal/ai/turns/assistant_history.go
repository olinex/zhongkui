package turns

// 本文件把持久化的原生消息与交互重建为历史快照：同一用户轮次之间的助手 / 工具消息合并为
// 一条助手消息，块按真实产生顺序编号；已作答与待处理交互各自落为 approval / questionnaire /
// next_steps 块，使刷新 / 重进会话与实时终态同构。

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// toolBlockLocation 定位一个调用在 tool_calls 块中的位置。
type toolBlockLocation struct {
	// block 块在消息 blocks 中的下标。
	block int
	// call 调用在块 calls 中的下标。
	call int
}

// assistantHistoryGroup 聚合一轮回答期间的全部助手 / 工具消息。
type assistantHistoryGroup struct {
	snapshot  assistant.MessageSnapshot
	blocks    []assistant.Block
	blockSeq  int
	round     int
	content   []string
	toolIndex map[string]toolBlockLocation
	nextSteps bool
}

func newAssistantHistoryGroup() *assistantHistoryGroup {
	return &assistantHistoryGroup{
		snapshot:  assistant.MessageSnapshot{Role: "assistant"},
		toolIndex: make(map[string]toolBlockLocation),
	}
}

// appendBlock 追加一块并返回块下标。
func (g *assistantHistoryGroup) appendBlock(kind assistant.BlockKind, payload any) int {
	g.blockSeq++
	g.blocks = append(g.blocks, assistant.Block{Seq: g.blockSeq, Kind: kind, Payload: payload})
	if kind == assistant.BlockKindNextSteps {
		g.nextSteps = true
	}
	return len(g.blocks) - 1
}

// appendContent 记录一条助手正文，供快照 content 纯文本兜底使用。
func (g *assistantHistoryGroup) appendContent(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	g.content = append(g.content, text)
}

// appendToolCalls 把一条消息内的工具调用追加为一个 tool_calls 块：同批调用共用一个块，
// 已存在的 callID 原位更新，避免续跑 / 重复行产生重影。
func (g *assistantHistoryGroup) appendToolCalls(calls []msgops.ToolCall) {
	if len(calls) == 0 {
		return
	}
	g.round++
	round := g.round
	records := make([]assistant.ToolCallRecord, 0, len(calls))
	blockIdx := -1
	for _, call := range calls {
		if call.Name == "" {
			continue
		}
		if loc, ok := g.toolIndex[call.ID]; ok && call.ID != "" {
			payload := g.blocks[loc.block].Payload.(assistant.ToolCallsBlockPayload)
			payload.Calls[loc.call].ToolName = call.Name
			payload.Calls[loc.call].Arguments = call.Args
			g.blocks[loc.block].Payload = payload
			continue
		}
		record := assistant.ToolCallRecord{
			CallID:    call.ID,
			ToolName:  call.Name,
			Arguments: call.Args,
			ExecOwner: contract.ToolExecOwnerBackend,
			Round:     round,
			Status:    assistant.ToolCallStatusStart,
		}
		if blockIdx < 0 {
			records = append(records, record)
			blockIdx = g.appendBlock(assistant.BlockKindToolCalls, assistant.ToolCallsBlockPayload{Calls: records})
			continue
		}
		payload := g.blocks[blockIdx].Payload.(assistant.ToolCallsBlockPayload)
		payload.Calls = append(payload.Calls, record)
		g.blocks[blockIdx].Payload = payload
	}
	// 登记块内每条调用的位置。
	if blockIdx >= 0 {
		payload := g.blocks[blockIdx].Payload.(assistant.ToolCallsBlockPayload)
		for i, record := range payload.Calls {
			if record.CallID != "" {
				g.toolIndex[record.CallID] = toolBlockLocation{block: blockIdx, call: i}
			}
		}
	}
}

// completeToolCall 把工具结果回填到发起时的调用记录；找不到时新建一个已完成块。
func (g *assistantHistoryGroup) completeToolCall(id, name, content string) {
	if id == "" {
		return
	}
	if loc, ok := g.toolIndex[id]; ok {
		payload := g.blocks[loc.block].Payload.(assistant.ToolCallsBlockPayload)
		record := payload.Calls[loc.call]
		record.Status = assistant.ToolCallStatusDone
		record.Result = content
		if name != "" {
			record.ToolName = name
		}
		payload.Calls[loc.call] = record
		g.blocks[loc.block].Payload = payload
		return
	}
	g.round++
	record := assistant.ToolCallRecord{
		CallID:    id,
		ToolName:  name,
		ExecOwner: contract.ToolExecOwnerBackend,
		Round:     g.round,
		Status:    assistant.ToolCallStatusDone,
		Result:    content,
	}
	block := g.appendBlock(assistant.BlockKindToolCalls, assistant.ToolCallsBlockPayload{Calls: []assistant.ToolCallRecord{record}})
	g.toolIndex[id] = toolBlockLocation{block: block, call: 0}
}

// buildAssistantSnapshots 把持久消息与交互重建为历史快照。
func buildAssistantSnapshots[M adk.MessageType](
	stored []*modelsai.AgentMessage,
	interactions []contract.Interaction,
	pendingCalls map[uint][]contract.PendingCall,
	compressBySeq map[uint64]*assistant.CompressState,
) []assistant.MessageSnapshot {
	var groups []*assistantHistoryGroup
	var current *assistantHistoryGroup
	userIndex := 0

	// 交互按锚点消息归组：随该消息所在行之后追加，保持与实时流一致的块顺序
	// （多轮问卷 / 审批不再被统一堆到消息末尾）。
	byAnchor := make(map[uint][]contract.Interaction)
	for _, interaction := range interactions {
		if interaction.Status == contract.InteractionCancelled || interaction.MessageID == 0 {
			continue
		}
		byAnchor[interaction.MessageID] = append(byAnchor[interaction.MessageID], interaction)
	}
	// present 现存消息 ID 集合：锚点已不存在的交互（典型为回退截断后残留的旧数据）不再回放，
	// 否则会被挂到残留轮次或合成一条空回答，凭空复现已删除的卡片（如「推荐的下一步」）。
	present := make(map[uint]struct{}, len(stored))
	for _, row := range stored {
		if row != nil && row.ID != 0 {
			present[row.ID] = struct{}{}
		}
	}
	used := make(map[uint]struct{})
	appendInteractions := func(group *assistantHistoryGroup, anchor uint) {
		if _, ok := used[anchor]; ok {
			return
		}
		list := byAnchor[anchor]
		if len(list) == 0 {
			return
		}
		used[anchor] = struct{}{}
		for _, interaction := range list {
			addHistoryInteraction(group, interaction)
			if interaction.Status == contract.InteractionPending {
				for _, call := range pendingCalls[anchor] {
					group.snapshot.PendingCalls = append(group.snapshot.PendingCalls, assistant.PendingCallSnapshot{
						PendingCall: call,
						MessageID:   anchor,
					})
				}
			}
		}
	}

	for _, row := range stored {
		if row == nil {
			continue
		}
		msg, ok := decodeStoredMessage[M](row)
		if !ok {
			continue
		}
		switch row.Role {
		case modelsai.MessageRoleUser:
			userIndex++
			current = nil
			groups = append(groups, &assistantHistoryGroup{
				snapshot: assistant.MessageSnapshot{
					ID:               row.ID,
					Role:             string(row.Role),
					Content:          msgops.Text(msg),
					CreatedAt:        formatMessageTime(row.CreatedAt),
					UserMessageIndex: userIndex,
				},
			})
			continue
		case modelsai.MessageRoleSystem:
			current = nil
			groups = append(groups, &assistantHistoryGroup{
				snapshot: assistant.MessageSnapshot{
					ID:        row.ID,
					Role:      string(row.Role),
					Content:   msgops.Text(msg),
					CreatedAt: formatMessageTime(row.CreatedAt),
				},
			})
			continue
		}

		if current == nil {
			current = newAssistantHistoryGroup()
			groups = append(groups, current)
		}
		current.snapshot.ID = row.ID
		current.snapshot.CreatedAt = formatMessageTime(row.CreatedAt)
		// 用量取本轮最后一个有计量的助手行（挂起 + 续跑时只保留续跑终态，避免跨运行重复累加，
		// 使历史与实时 turn.completed 一致）。
		if row.TotalTokens > 0 || row.PromptTokens > 0 || row.CompletionTokens > 0 {
			current.snapshot.PromptTokens = row.PromptTokens
			current.snapshot.CompletionTokens = row.CompletionTokens
			current.snapshot.TotalTokens = row.TotalTokens
		}
		if row.ElapsedMs > 0 {
			current.snapshot.ElapsedMs = row.ElapsedMs
		}
		if row.Phase != "" {
			current.snapshot.Phase = row.Phase
		}
		if citations := decodeMessageCitations(row); len(citations) > 0 {
			current.snapshot.Citations = citations
		}
		if state := compressBySeq[row.Seq]; state != nil {
			current.snapshot.Compress = state
		}

		results := msgops.ToolResults(msg)
		for _, result := range results {
			current.completeToolCall(result.ID, result.Name, result.Content)
		}
		if len(results) == 0 {
			if reasoning := msgops.ReasoningText(msg); reasoning != "" {
				current.appendBlock(assistant.BlockKindReasoning, assistant.ReasoningBlockPayload{Text: reasoning})
			}
			body := msgops.Text(msg)
			calls := msgops.ToolCalls(msg)
			if strings.TrimSpace(body) != "" {
				current.appendBlock(assistant.BlockKindContent, assistant.ContentBlockPayload{Text: body})
				current.appendContent(body)
			}
			current.appendToolCalls(calls)
		}
		appendInteractions(current, row.ID)
	}

	// 锚点行缺失的交互：指定了锚点但该消息已不存在（如回退截断）的直接跳过——其所属轮次已删除，
	// 挂到残留轮次只会复现已作废的卡片；空锚点（MessageID=0）的交互仍挂到最后一个助手组或新建一组。
	for _, interaction := range interactions {
		if interaction.Status == contract.InteractionCancelled {
			continue
		}
		if interaction.MessageID != 0 {
			if _, ok := used[interaction.MessageID]; ok {
				continue
			}
			if _, ok := present[interaction.MessageID]; !ok {
				continue
			}
			used[interaction.MessageID] = struct{}{}
		}
		group := lastAssistantGroup(groups)
		if group == nil {
			group = newAssistantHistoryGroup()
			groups = append(groups, group)
		}
		if group.snapshot.ID == 0 {
			group.snapshot.ID = interaction.MessageID
		}
		addHistoryInteraction(group, interaction)
		if interaction.Status == contract.InteractionPending && interaction.MessageID != 0 {
			for _, call := range pendingCalls[interaction.MessageID] {
				group.snapshot.PendingCalls = append(group.snapshot.PendingCalls, assistant.PendingCallSnapshot{
					PendingCall: call,
					MessageID:   interaction.MessageID,
				})
			}
		}
	}

	out := make([]assistant.MessageSnapshot, 0, len(groups))
	for _, group := range groups {
		group.snapshot.Blocks = group.blocks
		if len(group.content) > 0 {
			group.snapshot.Content = strings.Join(group.content, "\n\n")
		}
		group.snapshot.PendingCalls = dedupePendingCalls(group.snapshot.PendingCalls)
		out = append(out, group.snapshot)
	}
	return out
}

// addHistoryInteraction 把一条已作答 / 待处理交互追加为块。
func addHistoryInteraction(group *assistantHistoryGroup, interaction contract.Interaction) {
	switch interaction.Kind {
	case contract.InteractionApproval:
		payload, ok := interaction.Payload.(contract.ApprovalPayload)
		if !ok || len(payload.Calls) == 0 {
			return
		}
		decisions := make(map[string]contract.ApprovalDecision, len(payload.Decisions))
		for _, decision := range payload.Decisions {
			decisions[decision.CallID] = decision
		}
		records := make([]assistant.ApprovalCallRecord, 0, len(payload.Calls))
		callIDs := make([]string, 0, len(payload.Calls))
		for _, call := range payload.Calls {
			record := assistant.ApprovalCallRecord{
				CallID:           call.CallID,
				ToolName:         call.ToolName,
				Arguments:        call.Arguments,
				AuditLevel:       call.AuditLevel,
				ApprovalRequired: call.ApprovalRequired,
				ExecOwner:        call.ExecOwner,
			}
			if decision, ok := decisions[call.CallID]; ok {
				approved := decision.Approved
				record.Decision = &approved
				record.Result = decision.Result
				record.Error = decision.Error
			}
			if call.CallID != "" {
				callIDs = append(callIDs, call.CallID)
			}
			records = append(records, record)
		}
		group.appendBlock(assistant.BlockKindApproval, assistant.ApprovalBlockPayload{Calls: records, CallIDs: callIDs})
	case contract.InteractionQuestionnaire:
		payload, ok := interaction.Payload.(contract.QuestionnairePayload)
		if !ok || len(payload.Questions) == 0 {
			return
		}
		answers := make([]contract.QuestionnaireAnswer, 0, len(payload.Answers))
		for _, answer := range payload.Answers {
			answers = append(answers, contract.QuestionnaireAnswer{
				QuestionID: answer.QuestionID,
				Answered:   answer.Answered,
				Answer:     answer.Answer,
				Custom:     answer.Custom,
			})
		}
		group.appendBlock(assistant.BlockKindQuestionnaire, assistant.QuestionnaireBlockPayload{
			Questions:    payload.Questions,
			Answers:      answers,
			Awaiting:     interaction.Status == contract.InteractionPending,
			InterruptIDs: payload.InterruptIDs,
		})
	case contract.InteractionNextSteps:
		payload, ok := interaction.Payload.(contract.NextStepsPayload)
		if !ok || len(payload.Steps) == 0 || group.nextSteps {
			return
		}
		group.appendBlock(assistant.BlockKindNextSteps, assistant.NextStepsBlockPayload{Steps: payload.Steps})
	}
}

func lastAssistantGroup(groups []*assistantHistoryGroup) *assistantHistoryGroup {
	for i := len(groups) - 1; i >= 0; i-- {
		if groups[i].snapshot.Role == "assistant" {
			return groups[i]
		}
	}
	return nil
}

// dedupePendingCalls 按 callID 去重：同一锚点可能同时挂审批与问卷交互，两者共用同一挂起现场。
func dedupePendingCalls(calls []assistant.PendingCallSnapshot) []assistant.PendingCallSnapshot {
	if len(calls) == 0 {
		return nil
	}
	out := make([]assistant.PendingCallSnapshot, 0, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.CallID == "" {
			out = append(out, call)
			continue
		}
		if _, ok := seen[call.CallID]; ok {
			continue
		}
		seen[call.CallID] = struct{}{}
		out = append(out, call)
	}
	return out
}

func decodeMessageCitations(row *modelsai.AgentMessage) []retrieval.RetrievalResult {
	if row == nil || row.CitationsJSON == nil {
		return nil
	}
	raw := row.CitationsJSON.Value()
	if len(raw) == 0 {
		return nil
	}
	var citations []retrieval.RetrievalResult
	if err := json.Unmarshal(raw, &citations); err != nil {
		return nil
	}
	return citations
}

func formatMessageTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
