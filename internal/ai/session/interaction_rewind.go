package session

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"strings"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// redoCheckpointPrefix 是交互回退用的 checkpoint 快照键前缀：完整键为
// "redo:<会话ID>:<交互ID>"。挂起时把当时的图现场另存一份快照，使审批 / 问卷被作答后仍能回退到该次
// 挂起重新作答；快照与交互同寿命（交互删除时一并清理）。
const redoCheckpointPrefix = "redo:"

// ErrInteractionRewindUnavailable 交互已无法回退：该交互之后的产物已被上下文摘要覆盖，或载荷缺少
// 可续跑的调用清单。
var ErrInteractionRewindUnavailable = errors.New("agent interaction rewind unavailable")

// RedoCheckpointID 返回某条交互（审批 / 问卷）的 checkpoint 快照键。
func RedoCheckpointID(sessionID, interactionID uint) string {
	return fmt.Sprintf("%s%d:%d", redoCheckpointPrefix, sessionID, interactionID)
}

// RedoCheckpointPrefix 返回某会话全部交互回退快照键的公共前缀，供会话删除时按前缀清理。
func RedoCheckpointPrefix(sessionID uint) string {
	return fmt.Sprintf("%s%d:", redoCheckpointPrefix, sessionID)
}

// InteractionRewind 是一次交互回退写入挂起现场所需的参数。
type InteractionRewind struct {
	// Calls 该交互的待执行调用清单（审批调用 / ask_user 调用，原样回填运行现场，供续跑按 callID 与
	// 中断 ID 映射 ResumeParams.Targets）
	Calls []contract.PendingCall
	// PayloadJSON 重置后的交互载荷（审批保留调用清单并清空决策；问卷保留题目并清空作答、置回待回答）
	PayloadJSON []byte
	// Slot 续跑使用的模型档位
	Slot ai.Slot
	// CheckpointID 续跑使用的 checkpoint 键（已由调用方写入挂起时的图现场快照）
	CheckpointID string
}

// ListInteractionsByKind 按会话内 seq 倒序返回指定类型的交互（限定属主用户），供回退时按调用 ID /
// 中断 ID 定位目标交互；无匹配交互时返回空切片。
func (s *Store) ListInteractionsByKind(ctx context.Context, sessionID, userID uint, kind ai.InteractionKind) ([]*ai.AgentInteraction, error) {
	var items []*ai.AgentInteraction
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND kind = ?", sessionID, userID, kind).
		Order("seq DESC").
		Find(&items).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return items, nil
}

// RewindToInteraction 把会话回退到某条审批 / 问卷交互的挂起状态：删除该交互锚点消息之后的全部消息
// （上次作答后续跑产生的回答）与更晚的交互，把该交互重置为待处理（审批清空决策、问卷清空作答），
// 作废更晚的挂起现场，并以给定参数重建一条指向该锚点的 pending 运行现场——使状态重新落「等待审批 /
// 等待作答」，用户可重新选择，且续跑沿用挂起时的图现场而不是重跑整轮回答。
// 目标交互类型不符、不存在或归属不符时返回 ErrInteractionNotFound；锚点消息缺失返回
// ErrMessageNotFound；回退点已被滚动摘要覆盖时返回 ErrInteractionRewindUnavailable（摘要含被删内容，
// 回退会使上下文口径不一致）。目标已是 pending 时幂等返回 nil。单事务执行，避免回退中途留下
// 不一致状态。
func (s *Store) RewindToInteraction(ctx context.Context, sessionID, userID, interactionID uint, rewind InteractionRewind) error {
	if interactionID == 0 {
		return errors.New("session store: unidentified interaction")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target ai.AgentInteraction
		err := tx.Where("id = ? AND session_id = ? AND user_id = ?", interactionID, sessionID, userID).
			First(&target).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInteractionNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if target.Kind != ai.InteractionKindApproval && target.Kind != ai.InteractionKindQuestionnaire {
			return ErrInteractionNotFound
		}
		// 已是待处理：现场本就在等待该交互，无需回退（前端据此刷新后即可重新作答 / 决策）。
		if target.Status == ai.InteractionStatusPending {
			return nil
		}
		var anchor ai.AgentMessage
		err = tx.Where("id = ? AND session_id = ? AND user_id = ?", target.MessageID, sessionID, userID).
			First(&anchor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMessageNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		var session ai.AgentSession
		if err := tx.Select("id", "summary_upto_seq").Where("id = ? AND user_id = ?", sessionID, userID).
			First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSessionNotFound
			}
			return errors.WithStack(err)
		}
		// 回退点已被摘要覆盖：摘要中可能已折叠该交互之后的回答，删消息会造成上下文口径不一致。
		if uint64(session.SummaryUptoSeq) >= anchor.Seq {
			return ErrInteractionRewindUnavailable
		}
		// 作废更晚的挂起现场：其锚点位于被删除区间，已无从续跑。
		if err := tx.Model(&ai.AgentRun{}).
			Where("session_id = ? AND user_id = ? AND status = ?", sessionID, userID, ai.AgentRunStatusPending).
			Update("status", ai.AgentRunStatusConsumed).Error; err != nil {
			return errors.WithStack(err)
		}
		// 删除锚点之后的全部消息（上次作答后续跑产生的工具结果与最终回答）。
		if err := tx.Where("session_id = ? AND user_id = ? AND seq > ?", sessionID, userID, anchor.Seq).
			Delete(&ai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		// 删除更晚的交互，并清理其 checkpoint 快照（快照键由交互 ID 派生，随交互一起作废）。
		var laterIDs []uint
		if err := tx.Model(&ai.AgentInteraction{}).
			Where("session_id = ? AND user_id = ? AND seq > ?", sessionID, userID, target.Seq).
			Pluck("id", &laterIDs).Error; err != nil {
			return errors.WithStack(err)
		}
		if len(laterIDs) > 0 {
			keys := make([]string, 0, len(laterIDs))
			for _, id := range laterIDs {
				keys = append(keys, RedoCheckpointID(sessionID, id))
			}
			if err := tx.Where("checkpoint_id IN ?", keys).Delete(&ai.AgentCheckpoint{}).Error; err != nil {
				return errors.WithStack(err)
			}
			if err := tx.Where("id IN ?", laterIDs).Delete(&ai.AgentInteraction{}).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		// 重置目标交互：状态回到待处理，载荷按类型重置；保留目标自身的快照，使重复回退仍然可用。
		if err := tx.Model(&ai.AgentInteraction{}).Where("id = ?", target.ID).
			Select("status", "payload").
			Updates(&ai.AgentInteraction{
				Status:  ai.InteractionStatusPending,
				Payload: serializer.NewCrypto(rewind.PayloadJSON),
			}).Error; err != nil {
			return errors.WithStack(err)
		}
		// 作废锚点之后的压缩检查点（正常情况下回退点未被摘要覆盖时不存在，防御性清理）。
		if err := tx.Where("session_id = ? AND user_id = ? AND upto_seq >= ?", sessionID, userID, anchor.Seq).
			Delete(&ai.AgentSessionCompaction{}).Error; err != nil {
			return errors.WithStack(err)
		}
		// 重建挂起现场：待执行调用、档位、checkpoint 键与锚点消息原样回填，引用沿用锚点行的载荷。
		pendingJSON, err := json.Marshal(rewind.Calls)
		if err != nil {
			return errors.WithStack(err)
		}
		run := &ai.AgentRun{
			NamespaceName:    target.NamespaceName,
			UserID:           userID,
			SessionID:        sessionID,
			Status:           ai.AgentRunStatusPending,
			Slot:             rewind.Slot,
			PendingCallsJSON: serializer.NewCrypto(pendingJSON),
			CitationsJSON:    anchor.CitationsJSON,
			AnchorMessageID:  anchor.ID,
			CheckpointID:     rewind.CheckpointID,
		}
		if err := tx.Create(run).Error; err != nil {
			return errors.WithStack(err)
		}
		// 会话挂起中断 ID 取自调用清单（续跑按该 ID 映射 ResumeParams.Targets）。
		if err := tx.Model(&ai.AgentSession{}).Where("id = ? AND user_id = ?", sessionID, userID).
			Update("pending_interrupt_id", strings.Join(interruptIDsOf(rewind.Calls), ",")).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// interruptIDsOf 返回调用清单中非空的根中断 ID（去重、保持原序）。
func interruptIDsOf(calls []contract.PendingCall) []string {
	ids := make([]string, 0, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.InterruptID == "" {
			continue
		}
		if _, ok := seen[call.InterruptID]; ok {
			continue
		}
		seen[call.InterruptID] = struct{}{}
		ids = append(ids, call.InterruptID)
	}
	return ids
}
