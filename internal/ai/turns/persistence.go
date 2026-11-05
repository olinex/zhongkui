package turns

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/adk"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// appendUserMessage 把当前查询作为原生用户消息持久化，并把本轮选用的技能锚定在这条用户消息上
// （技能归属轮次而非会话，同一会话的不同轮次可各自选用不同技能）。模型历史在此追加之前加载，
// 以免用户消息被 GenModelInput 重复添加。
func (l *genericSessionLoop[M]) appendUserMessage(ctx context.Context, sess *modelsai.AgentSession, item *ChatItem) (*modelsai.AgentMessage, error) {
	msg := msgops.NewUser[M](item.Query)
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	row := &modelsai.AgentMessage{
		SessionID:     sess.ID,
		NamespaceName: sess.NamespaceName,
		UserID:        sess.UserID,
		Role:          modelsai.MessageRoleUser,
		MessageKind:   msgops.APIModeOf[M](),
		Payload:       serializer.NewCrypto(payload),
		SkillID:       modelsai.SkillIDColumn(item.SkillID),
		SkillName:     item.SkillName,
	}
	return l.deps.Sessions.AppendNativeMessage(ctx, row)
}

// persistIntermediates 存储运行产生的每条 assistant/tool 消息。
// 最后一条 assistant 消息会带上引用、token 用量和终止阶段，
// 并作为挂起/下一轮次的锚点返回。
func (l *genericSessionLoop[M]) persistIntermediates(ctx context.Context, sess *modelsai.AgentSession, res StreamResult[M], phase modelsai.AgentRunPhase, elapsedMs int64) (lastMessageID, lastAssistantID uint) {
	messages := res.Intermediates
	if len(messages) == 0 && res.LastContent != "" {
		messages = []M{msgops.NewAssistant[M](res.LastContent, nil)}
	}
	if len(messages) == 0 {
		return 0, 0
	}

	lastAssistantIdx := -1
	for i, msg := range messages {
		if msgops.RoleLabel[M](msg) == "Agent" {
			lastAssistantIdx = i
		}
	}

	var citations []byte
	if lastAssistantIdx >= 0 && len(res.Citations) > 0 {
		if data, err := json.Marshal(res.Citations); err == nil {
			citations = data
		}
	}

	for i, msg := range messages {
		payload, err := json.Marshal(msg)
		if err != nil {
			l.warnf(ctx, "turns: marshal intermediate message failed: %v", err)
			continue
		}
		row := &modelsai.AgentMessage{
			SessionID:     sess.ID,
			NamespaceName: sess.NamespaceName,
			UserID:        sess.UserID,
			Role:          persistedRole[M](msg),
			MessageKind:   msgops.APIModeOf[M](),
			Payload:       serializer.NewCrypto(payload),
		}
		if i == lastAssistantIdx {
			row.Phase = phase
			row.PromptTokens = res.PromptTokens
			row.CompletionTokens = res.CompletionTokens
			row.TotalTokens = res.TotalTokens
			row.ElapsedMs = elapsedMs
			if len(citations) > 0 {
				row.CitationsJSON = serializer.NewCrypto(citations)
			}
		}
		appended, err := l.deps.Sessions.AppendNativeMessage(ctx, row)
		if err != nil {
			l.warnf(ctx, "turns: append intermediate message failed: %v", err)
			continue
		}
		lastMessageID = appended.ID
		if i == lastAssistantIdx {
			lastAssistantID = appended.ID
		}
	}
	return lastMessageID, lastAssistantID
}

// ensureAnchor 创建锚点 assistant 消息，用于在任何 assistant 消息发出
// 之前就发生中断的情况。
func (l *genericSessionLoop[M]) ensureAnchor(ctx context.Context, sess *modelsai.AgentSession) uint {
	// 锚点不携带原生内容：它只把挂起的交互/运行绑定到一条 assistant 记录上，
	// 因此历史重放会跳过它。
	row := &modelsai.AgentMessage{
		SessionID:     sess.ID,
		NamespaceName: sess.NamespaceName,
		UserID:        sess.UserID,
		Role:          modelsai.MessageRoleAssistant,
		MessageKind:   msgops.APIModeOf[M](),
		Payload:       serializer.NewCrypto(nil),
		Phase:         modelsai.AgentRunPhaseSuspended,
	}
	appended, err := l.deps.Sessions.AppendNativeMessage(ctx, row)
	if err != nil {
		l.warnf(ctx, "turns: append anchor message failed: %v", err)
		return 0
	}
	return appended.ID
}

// persistInteractions 存储投影产生的审批/问卷/next_steps 卡片，
// 并关联到 assistant 锚点消息。
func (l *genericSessionLoop[M]) persistInteractions(ctx context.Context, sess *modelsai.AgentSession, interactions []contract.Interaction, lastAssistantID, lastMessageID uint) {
	messageID := lastAssistantID
	if messageID == 0 {
		messageID = lastMessageID
	}
	for _, interaction := range interactions {
		payload, err := json.Marshal(interaction.Payload)
		if err != nil {
			l.warnf(ctx, "turns: marshal interaction failed: %v", err)
			continue
		}
		row := &modelsai.AgentInteraction{
			SessionID:     sess.ID,
			NamespaceName: sess.NamespaceName,
			UserID:        sess.UserID,
			MessageID:     messageID,
			Kind:          interactionKind(interaction.Kind),
			Status:        interactionStatus(interaction),
			Payload:       serializer.NewCrypto(payload),
		}
		if _, err := l.deps.Sessions.AppendInteraction(ctx, row); err != nil {
			l.warnf(ctx, "turns: append interaction failed: %v", err)
		}
	}
}

// saveSuspendedRun 记录业务挂起状态。权威的恢复状态是 TurnLoop
// 检查点；该记录携带待处理调用、引用、锚点消息和
// 检查点键。
func (l *genericSessionLoop[M]) saveSuspendedRun(ctx context.Context, sess *modelsai.AgentSession, res StreamResult[M], anchorMessageID uint, interruptIDs []string) {
	pending, err := json.Marshal(res.PendingCalls)
	if err != nil {
		l.warnf(ctx, "turns: marshal pending calls failed: %v", err)
		return
	}
	citations, err := json.Marshal(res.Citations)
	if err != nil {
		citations = nil
	}
	run := &modelsai.AgentRun{
		NamespaceName:    sess.NamespaceName,
		UserID:           sess.UserID,
		SessionID:        sess.ID,
		Status:           modelsai.AgentRunStatusPending,
		Slot:             l.handle.Slot,
		PendingCallsJSON: serializer.NewCrypto(pending),
		CheckpointID:     l.checkpointID,
	}
	if len(citations) > 0 {
		run.CitationsJSON = serializer.NewCrypto(citations)
	}
	run.AnchorMessageID = anchorMessageID
	if err := l.deps.Sessions.SaveAgentRun(ctx, run); err != nil {
		l.warnf(ctx, "turns: save suspended run failed: %v", err)
		return
	}
	if run.ID != 0 && anchorMessageID != 0 {
		if err := l.deps.Sessions.SetAgentRunAnchor(ctx, run.ID, anchorMessageID); err != nil {
			l.warnf(ctx, "turns: set agent run anchor failed: %v", err)
		}
	}
	_ = interruptIDs
}

// interruptIDsForResult 返回根中断 ID；当投影未返回任何 ID 时，
// 回退使用待处理调用携带的 ID。
func interruptIDsForResult[M adk.MessageType](res StreamResult[M]) []string {
	ids := make([]string, 0, len(res.InterruptIDs))
	seen := make(map[string]bool, len(res.InterruptIDs))
	for _, id := range res.InterruptIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		return ids
	}
	for _, call := range res.PendingCalls {
		if call.InterruptID == "" || seen[call.InterruptID] {
			continue
		}
		seen[call.InterruptID] = true
		ids = append(ids, call.InterruptID)
	}
	return ids
}

// decodeStoredMessages 解码全部持久化消息（跳过缺失载荷或解码失败的行）。
func decodeStoredMessages[M adk.MessageType](stored []*modelsai.AgentMessage) []M {
	out := make([]M, 0, len(stored))
	for _, row := range stored {
		if msg, ok := decodeStoredMessage[M](row); ok {
			out = append(out, msg)
		}
	}
	return out
}

// decodeStoredMessage 解码单条持久化消息；缺载荷或解码失败时返回 false。
func decodeStoredMessage[M adk.MessageType](row *modelsai.AgentMessage) (M, bool) {
	var zero M
	if row == nil || row.Payload == nil || len(row.Payload.Value()) == 0 {
		return zero, false
	}
	kind := msgops.APIModeOf[M]()
	if row.MessageKind != "" {
		if parsed, err := llm.ParseAPIMode(row.MessageKind); err == nil {
			kind = parsed
		}
	}
	msg, err := msgops.DecodeForModel[M](row.Payload.Value(), kind)
	if err != nil {
		return zero, false
	}
	return msg, true
}

func persistedRole[M adk.MessageType](msg M) modelsai.MessageRole {
	switch msgops.RoleLabel[M](msg) {
	case "You":
		return modelsai.MessageRoleUser
	case "System":
		return modelsai.MessageRoleSystem
	case "Tool":
		return modelsai.MessageRole("tool")
	default:
		return modelsai.MessageRoleAssistant
	}
}

func phaseForResult[M adk.MessageType](res StreamResult[M]) modelsai.AgentRunPhase {
	switch {
	case res.Interrupted:
		return modelsai.AgentRunPhaseSuspended
	case res.Err == nil:
		return modelsai.AgentRunPhaseDone
	case isCancellation(res.Err):
		return modelsai.AgentRunPhaseCancelled
	default:
		return modelsai.AgentRunPhaseFailed
	}
}

func isCancellation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	var cancel *adk.CancelError
	return errors.As(err, &cancel)
}

func interactionKind(kind contract.InteractionKind) modelsai.InteractionKind {
	switch kind {
	case contract.InteractionApproval:
		return modelsai.InteractionKindApproval
	case contract.InteractionQuestionnaire:
		return modelsai.InteractionKindQuestionnaire
	case contract.InteractionNextSteps:
		return modelsai.InteractionKindNextSteps
	default:
		return modelsai.InteractionKind(kind)
	}
}

func interactionStatus(interaction contract.Interaction) modelsai.InteractionStatus {
	if interaction.Kind == contract.InteractionNextSteps {
		return modelsai.InteractionStatusAnswered
	}
	switch interaction.Status {
	case contract.InteractionPending:
		return modelsai.InteractionStatusPending
	case contract.InteractionAnswered:
		return modelsai.InteractionStatusAnswered
	case contract.InteractionCancelled:
		return modelsai.InteractionStatusCancelled
	default:
		return modelsai.InteractionStatusPending
	}
}

func decodePendingCalls(run *modelsai.AgentRun) []contract.PendingCall {
	if run == nil || run.PendingCallsJSON == nil {
		return nil
	}
	var calls []contract.PendingCall
	if err := json.Unmarshal(run.PendingCallsJSON.Value(), &calls); err != nil {
		return nil
	}
	return calls
}

func decodeCitations(run *modelsai.AgentRun) []retrieval.RetrievalResult {
	if run == nil || run.CitationsJSON == nil {
		return nil
	}
	var citations []retrieval.RetrievalResult
	if err := json.Unmarshal(run.CitationsJSON.Value(), &citations); err != nil {
		return nil
	}
	return citations
}
