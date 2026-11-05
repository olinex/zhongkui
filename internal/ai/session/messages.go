package session

import (
	// 标准库
	"context"
	"database/sql"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// allocateMessageSeq 在 tx 内为某会话原子分配 count 个连续序号：以会话行的 message_seq 计数列
// 一次性自增并取回新的最大值；count 即本次分配到的最大序号（count=1 时即该条消息的 seq）。
// 会话不存在或不属于该命名空间 / 属主用户（id + namespace_name + user_id 不匹配）时返回
// ErrSessionNotFound。
func allocateMessageSeq(tx *gorm.DB, sessionID uint, namespaceName string, userID uint, count int64) (uint64, error) {
	var seq int64
	err := tx.Raw("UPDATE agent_sessions SET message_seq = message_seq + ? WHERE id = ? AND namespace_name = ? AND user_id = ? RETURNING message_seq", count, sessionID, namespaceName, userID).
		Row().Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSessionNotFound
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return uint64(seq), nil
}

// AppendMessage 在单事务内追加一条消息到会话：先以会话行的 message_seq 计数列原子分配一个 seq
// （并发追加以该计数列为锁点全局串行，不重复），赋给 msg.Seq 后再落库。msg 为空时返回英文错误；
// 会话不存在或不属于消息携带的命名空间 / 属主用户时返回 ErrSessionNotFound。
func (s *Store) AppendMessage(ctx context.Context, msg *ai.AgentMessage) (*ai.AgentMessage, error) {
	if msg == nil {
		return nil, errors.New("session store: nil message")
	}
	if msg.MessageKind == "" {
		msg.MessageKind = llm.APIModeChat
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seq, err := allocateMessageSeq(tx, msg.SessionID, msg.NamespaceName, msg.UserID, 1)
		if err != nil {
			return errors.WithStack(err)
		}
		msg.Seq = seq
		if err := tx.Create(msg).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return msg, nil
}

// GetMessage 按消息 ID 查询一条消息（限定命名空间与属主用户），供运行层取回问卷锚点行上的载荷；
// 不存在（含归属不符）时返回 ErrMessageNotFound。
func (s *Store) GetMessage(ctx context.Context, id uint, namespaceName string, userID uint) (*ai.AgentMessage, error) {
	if id == 0 {
		return nil, errors.New("session store: unidentified message")
	}
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).Where("id = ? AND namespace_name = ? AND user_id = ?", id, namespaceName, userID).First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &msg, nil
}

// AssistantSeqAtMessage 返回指定助手消息（id）在所属会话内的 seq，供手动压缩把折叠上界定位到该助手
// 回答；该消息不存在、不属于该会话的命名空间与属主，或不是助手消息时返回 ErrMessageNotFound。按角色
// 过滤而非按「第 N 轮」定位：折叠边界以选中的助手回答为准，与前端选中的那条消息一一对应。
func (s *Store) AssistantSeqAtMessage(ctx context.Context, sessionID uint, namespaceName string, userID uint, messageID uint) (uint64, error) {
	var message ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND id = ? AND role = ?", sessionID, namespaceName, userID, messageID, ai.MessageRoleAssistant).
		First(&message).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrMessageNotFound
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return message.Seq, nil
}

// UpdateAssistantMessage 原地更新一条已落库的助手消息（按消息 ID 定位，并限定属主用户），供运行
// 途中把阶段性落库的那一行继续累积正文，以及运行终结时写入生命周期终态阶段（phase：done / failed /
// cancelled / suspended）。不重分配 seq，故消息在会话内的顺序与只读游标不变。必须用「结构体 + Select」
// 写这几列（正文 / 草稿 / 引用为密文列，走 crypto 序列化器；map 形式会绕过序列化器把明文直写库，见
// UpdateSummaryAndCheckpoint）。思考 / 问卷 / 授权清单不再有对应列，展示事实由事件块与
// agent_interactions 承载。msg 为空或 ID 为 0 时返回英文错误；目标消息不存在（含归属不符）时返回
// ErrMessageNotFound。
func (s *Store) UpdateAssistantMessage(ctx context.Context, msg *ai.AgentMessage) error {
	if msg == nil || msg.ID == 0 {
		return errors.New("session store: nil or unidentified message")
	}
	update := &ai.AgentMessage{
		Content:          msg.Content,
		DraftJSON:        msg.DraftJSON,
		CitationsJSON:    msg.CitationsJSON,
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
		TotalTokens:      msg.TotalTokens,
		ElapsedMs:        msg.ElapsedMs,
		Phase:            msg.Phase,
	}
	columns := []string{"content", "draft_json", "citations_json", "prompt_tokens", "completion_tokens", "total_tokens", "elapsed_ms", "phase"}
	res := s.db.WithContext(ctx).Model(update).
		Where("id = ? AND namespace_name = ? AND user_id = ?", msg.ID, msg.NamespaceName, msg.UserID).
		Select(columns).
		Updates(update)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// DeleteMessage 硬删除一条消息行（按消息 ID 定位，并限定命名空间与属主用户），供回合在「本次流以挂起
// 结束」时回收其阶段性落库的那一行：挂起不是终局，续跑会把本回合已外发的正文并回最终 done，留着
// 这一行会在续跑结束后形成同一回合的两条助手消息。消息行本无 deleted_at 列（见 DeleteSession），
// 故这里即硬删；目标行不存在（含归属不符）时返回 ErrMessageNotFound。
func (s *Store) DeleteMessage(ctx context.Context, id uint, namespaceName string, userID uint) error {
	if id == 0 {
		return errors.New("session store: unidentified message")
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ? AND user_id = ?", id, namespaceName, userID).
		Delete(&ai.AgentMessage{})
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// ListMessages 分页列出会话（限定命名空间与属主用户）消息（按会话内 seq 升序）；limit/offset 非正数时
// 不限定。
func (s *Store) ListMessages(ctx context.Context, sessionID uint, namespaceName string, userID uint, limit, offset int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ? AND namespace_name = ? AND user_id = ?", sessionID, namespaceName, userID).Order("seq ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var messages []*ai.AgentMessage
	if err := q.Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// HistoryMessages 返回会话最近 maxCount 条消息（限定命名空间与属主用户，按会话内 seq 升序返回，最新在
// 后），供注入 LLM 上下文保持对话顺序；maxCount 非正数时返回全部消息。
func (s *Store) HistoryMessages(ctx context.Context, sessionID uint, namespaceName string, userID uint, maxCount int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ? AND namespace_name = ? AND user_id = ?", sessionID, namespaceName, userID)
	if maxCount > 0 {
		q = q.Order("seq DESC").Limit(maxCount)
	} else {
		q = q.Order("seq ASC")
	}
	var messages []*ai.AgentMessage
	if err := q.Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	// 倒序取最近 maxCount 条后反转回升序，与对话顺序一致。
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// ListMessagesAfter 返回会话中 seq 大于 afterSeq 的消息（限定命名空间与属主用户，按会话内 seq 升序），
// 供会话压缩取出「游标之后尚未摘要的全部消息」。
func (s *Store) ListMessagesAfter(ctx context.Context, sessionID uint, namespaceName string, userID uint, afterSeq uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND seq > ?", sessionID, namespaceName, userID, afterSeq).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// ListMessagesBefore 返回会话中 afterSeq 与 beforeSeq 之间的消息（afterSeq < seq < beforeSeq，限定命名
// 空间与属主用户，按会话内 seq 升序），供滚动摘要取出「窗口外且尚未摘要」的消息（窗口最旧消息 seq 作
// 上界）。
func (s *Store) ListMessagesBefore(ctx context.Context, sessionID uint, namespaceName string, userID uint, beforeSeq uint, afterSeq uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND seq > ? AND seq < ?", sessionID, namespaceName, userID, afterSeq, beforeSeq).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// LatestAssistantMessage 返回会话最近一条助手消息（含草稿/引用载荷），并限定命名空间与属主用户；
// 不存在时返回 ErrMessageNotFound。
func (s *Store) LatestAssistantMessage(ctx context.Context, sessionID uint, namespaceName string, userID uint) (*ai.AgentMessage, error) {
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND role = ?", sessionID, namespaceName, userID, ai.MessageRoleAssistant).
		Order("seq DESC").
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &msg, nil
}
