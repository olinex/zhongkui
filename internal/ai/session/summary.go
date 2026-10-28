package session

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// latestCompactionBefore 在事务内返回会话中「upto_seq < beforeSeq」的最近一条压缩检查点（按 upto_seq
// 降序取首条），供回滚把上下文起点回退到该检查点；未找到时返回 ErrMessageNotFound（语义即「无更早
// 检查点」，由调用方据此清空摘要）。
func latestCompactionBefore(tx *gorm.DB, sessionID, userID uint, beforeSeq uint64) (*ai.AgentSessionCompaction, error) {
	var checkpoint ai.AgentSessionCompaction
	err := tx.Where("session_id = ? AND user_id = ? AND upto_seq < ?", sessionID, userID, beforeSeq).
		Order("upto_seq DESC").First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &checkpoint, nil
}

// UpdateSummaryAndCheckpoint 在单事务内推进会话的滚动摘要：更新 agent_sessions 的 summary 与
// summary_upto_seq，并追加一条 agent_session_compactions 压缩检查点（记录推进后的游标、摘要全文与
// 展示度量，供回滚逐级回退 start bound，以及刷新后回显历史压缩记录）。reason 取 rolling（静默滚动）/
// auto（自动压缩）/ manual（手动压缩）；metrics 为该次推进的真实展示度量（三种原因都必须填真值，
// 取值与是否展示无关——是否回显交由读取侧按 reason 决定）。
// 摘要经 crypto 序列化器加密落库，故会话更新必须以「结构体 + Select」写这两列、不触碰其它字段
// （避免并发改写会话标题/档位等；map 形式会绕过序列化器把明文直写库）；检查点同样是密文列，走
// Create 结构体路径加密。session 必须带有 ID / UserID / NamespaceName（调用点均持有已加载的会话）。
func (s *Store) UpdateSummaryAndCheckpoint(ctx context.Context, session *ai.AgentSession, summary string, uptoSeq uint, reason string, metrics contract.CompactionMetrics) error {
	if session == nil {
		return errors.New("session store: nil session")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := &ai.AgentSession{
			Summary:        serializer.NewCrypto([]byte(summary)),
			SummaryUptoSeq: uptoSeq,
		}
		if err := tx.Model(update).
			Where("id = ? AND user_id = ?", session.ID, session.UserID).
			Select("summary", "summary_upto_seq").
			Updates(update).Error; err != nil {
			return errors.WithStack(err)
		}
		checkpoint := &ai.AgentSessionCompaction{
			NamespaceName: session.NamespaceName,
			SessionID:     session.ID,
			UserID:        session.UserID,
			UptoSeq:       uint64(uptoSeq),
			Summary:       serializer.NewCrypto([]byte(summary)),
			Reason:        reason,
			TokensBefore:  metrics.TokensBefore,
			FoldedCount:   metrics.FoldedCount,
			TokensAfter:   metrics.TokensAfter,
		}
		if err := tx.Create(checkpoint).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// ListCompactions 返回会话内按 upto_seq 升序的全部压缩检查点（限定属主用户），供消息列表在刷新后
// 回显历史压缩记录。仅取展示所需列——刻意不 Select summary，避免为回显而无谓解密摘要。
func (s *Store) ListCompactions(ctx context.Context, sessionID, userID uint) ([]*ai.AgentSessionCompaction, error) {
	var checkpoints []*ai.AgentSessionCompaction
	if err := s.db.WithContext(ctx).
		Select("id", "upto_seq", "reason", "tokens_before", "folded_count", "tokens_after").
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("upto_seq ASC").
		Find(&checkpoints).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return checkpoints, nil
}

// ListCompactionSummaries 返回会话内各压缩检查点的摘要全文（键为 upto_seq，限定属主用户），供历史
// 回放中的压缩块在展开时查看已完成内容。摘要为密文列，方法只取 upto_seq + summary，避免 ListCompactions
// 为消息列表回显而解密全部摘要；无检查点时返回空映射。
func (s *Store) ListCompactionSummaries(ctx context.Context, sessionID, userID uint) (map[uint64]string, error) {
	var checkpoints []*ai.AgentSessionCompaction
	if err := s.db.WithContext(ctx).
		Select("upto_seq", "summary").
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Find(&checkpoints).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	summaries := make(map[uint64]string, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.Summary == nil {
			continue
		}
		summaries[checkpoint.UptoSeq] = string(checkpoint.Summary.Value())
	}
	return summaries, nil
}
