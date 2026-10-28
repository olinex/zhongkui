package session

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// ErrInteractionNotFound 交互不存在（含归属不符）。
var ErrInteractionNotFound = errors.New("agent interaction not found")

// AppendInteraction 在单事务内追加一条持久化交互（审批 / 问卷 / 下一步）：先校验会话属于该属主用户，
// 再按会话内已有最大 seq 分配下一个序号（从 1 起），赋给 item.Seq 后落库。Status 为空时取缺省
// pending；Payload 由调用方以 crypto 序列化器包装（serializer.NewCrypto(业务 JSON)）。
// item 为空或 Kind 为空时返回英文错误；会话不存在（含归属不符）时返回 ErrSessionNotFound。
func (s *Store) AppendInteraction(ctx context.Context, item *ai.AgentInteraction) (*ai.AgentInteraction, error) {
	if item == nil {
		return nil, errors.New("session store: nil interaction")
	}
	if item.Kind == "" {
		return nil, errors.New("session store: interaction with empty kind")
	}
	if item.Status == "" {
		item.Status = ai.InteractionStatusPending
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sessions int64
		if err := tx.Model(&ai.AgentSession{}).
			Where("id = ? AND user_id = ?", item.SessionID, item.UserID).
			Count(&sessions).Error; err != nil {
			return errors.WithStack(err)
		}
		if sessions == 0 {
			return ErrSessionNotFound
		}
		var maxSeq uint64
		if err := tx.Model(&ai.AgentInteraction{}).
			Where("session_id = ? AND user_id = ?", item.SessionID, item.UserID).
			Select("COALESCE(MAX(seq), 0)").
			Scan(&maxSeq).Error; err != nil {
			return errors.WithStack(err)
		}
		item.Seq = maxSeq + 1
		if err := tx.Create(item).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return item, nil
}

// ListInteractions 按会话内 seq 升序返回会话的全部持久化交互，并限定属主用户；无交互时返回空切片。
// 供刷新 / 重进会话时把已作答的审批、问卷与下一步卡片一并重建。
func (s *Store) ListInteractions(ctx context.Context, sessionID, userID uint) ([]*ai.AgentInteraction, error) {
	var items []*ai.AgentInteraction
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("seq ASC").
		Find(&items).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return items, nil
}

// UpdateInteractionStatus 更新一条持久化交互的状态（pending/answered/cancelled），按交互 ID + 属主
// 用户定位；payload 非 nil 时一并替换业务 JSON（以 crypto 序列化器加密后经「结构体 + Select」写入，
// map 形式会绕过序列化器把明文直写库），nil 表示只更新状态、保留原载荷。
// id 为 0 或 status 为空时返回英文错误；目标交互不存在（含归属不符）时返回 ErrInteractionNotFound。
func (s *Store) UpdateInteractionStatus(ctx context.Context, id, userID uint, status ai.InteractionStatus, payload []byte) error {
	if id == 0 {
		return errors.New("session store: unidentified interaction")
	}
	if status == "" {
		return errors.New("session store: interaction status is empty")
	}
	update := &ai.AgentInteraction{Status: status}
	columns := []string{"status"}
	if payload != nil {
		update.Payload = serializer.NewCrypto(payload)
		columns = append(columns, "payload")
	}
	res := s.db.WithContext(ctx).Model(update).
		Where("id = ? AND user_id = ?", id, userID).
		Select(columns).
		Updates(update)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrInteractionNotFound
	}
	return nil
}
