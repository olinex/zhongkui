package turns

// 本文件构造历史快照流：/render 先下发 snapshot.begin → snapshot.message →
// snapshot.end，再追加在途轮次已缓冲的实时事件，使刷新后的历史与实时终态一致。

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// RenderAssistant 返回会话的历史快照流（含终态块与元信息）。
func (m *Manager) RenderAssistant(ctx context.Context, sessionID, userID uint, namespaceName string) (*Stream, error) {
	sess, err := m.deps.Sessions.GetSession(ctx, sessionID, namespaceName, userID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	slot, err := m.effectiveSlot("", sess.TargetModel)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kind := llm.APIModeChat
	if rt := m.runtime(); rt != nil {
		if resolved, ok := rt.Kind(slot); ok {
			kind = resolved
		}
	}
	switch kind {
	case llm.APIModeResponses:
		return m.renderAssistantFor[*schema.AgenticMessage](ctx, sess)
	default:
		return m.renderAssistantFor[*schema.Message](ctx, sess)
	}
}

func (m *Manager) renderAssistantFor[M adk.MessageType](ctx context.Context, sess *modelsai.AgentSession) (*Stream, error) {
	stored, err := m.deps.Sessions.ListNativeMessages(ctx, sess.ID, sess.NamespaceName, sess.UserID, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rows, err := m.deps.Sessions.ListInteractions(ctx, sess.ID, sess.UserID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	interactions := make([]contract.Interaction, 0, len(rows))
	pendingAnchors := make(map[uint]struct{})
	for _, row := range rows {
		interaction, ok := decodeInteraction(row)
		if !ok || interaction.Status == contract.InteractionCancelled {
			continue
		}
		interactions = append(interactions, interaction)
		if interaction.Status == contract.InteractionPending && interaction.MessageID != 0 {
			pendingAnchors[interaction.MessageID] = struct{}{}
		}
	}

	// 待处理交互携带的挂起调用：按锚点消息取回挂起现场（含根中断 ID），供前端直接提交续跑。
	pendingCalls := make(map[uint][]contract.PendingCall, len(pendingAnchors))
	for anchor := range pendingAnchors {
		run, runErr := m.deps.Sessions.LatestRunByAnchor(ctx, sess.ID, sess.UserID, anchor)
		if runErr != nil || run == nil || run.Status != modelsai.AgentRunStatusPending {
			continue
		}
		pendingCalls[anchor] = decodePendingCalls(run)
	}

	compressBySeq := make(map[uint64]*assistant.CompressState)
	if checkpoints, cpErr := m.deps.Sessions.ListCompactions(ctx, sess.ID, sess.UserID); cpErr == nil {
		// 摘要全文单独取（ListCompactions 刻意不解密摘要）：压缩块在历史回放中收起展示，用户点击
		// 展开时需要有完整内容，故与检查点一并装入展示态。
		summaries, _ := m.deps.Sessions.ListCompactionSummaries(ctx, sess.ID, sess.UserID)
		for _, cp := range checkpoints {
			if cp == nil {
				continue
			}
			compressBySeq[cp.UptoSeq] = &assistant.CompressState{
				Phase:        assistant.CompressPhaseDone,
				Reason:       cp.Reason,
				TokensBefore: cp.TokensBefore,
				FoldedCount:  cp.FoldedCount,
				TokensAfter:  cp.TokensAfter,
				Summary:      summaries[cp.UptoSeq],
			}
		}
	}

	snapshots := buildAssistantSnapshots[M](stored, interactions, pendingCalls, compressBySeq)

	emitter := assistant.NewSnapshotEmitter(sess.ID)
	items := make([]StreamItem, 0, len(snapshots)+3)
	begin := emitter.SnapshotBegin()
	items = append(items, StreamItem{Event: &begin})
	for _, snapshot := range snapshots {
		event := emitter.SnapshotMessage(snapshot)
		items = append(items, StreamItem{Event: &event})
	}
	end := emitter.SnapshotEnd()
	items = append(items, StreamItem{Event: &end})

	// 在途轮次的缓冲事件追加在快照之后：刷新跟实时终态一致，客户端随后可用 /events?after=
	// 继续续播（快照事件不携带 id，不推进客户端游标）。
	if state := m.peekState(sess.ID); state != nil {
		if current := state.currentStream(); current != nil && !current.Closed() {
			for _, item := range current.Items(-1) {
				if item.Event != nil {
					items = append(items, item)
				}
			}
		}
	}

	return &Stream{
		SessionID: sess.ID,
		Replay:    items,
		Events:    closedEvents(),
		Close:     func() {},
	}, nil
}
