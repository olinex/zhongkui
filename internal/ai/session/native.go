package session

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AppendNativeMessage 在单事务内追加一条承载 eino 原生消息载荷的消息：先复用会话行的 message_seq
// 计数列原子分配一个 seq（与 AppendMessage 同一分配口径），赋给 msg.Seq 后再落库。MessageKind 为空
// 时取缺省 chat；Payload 由调用方以 crypto 序列化器包装（serializer.NewCrypto(原生消息 JSON)）。
// Content 为空且提供了 Payload 时，按 MessageKind 解码载荷并回填正文，使滚动摘要与消息列表仍可读取
// 原生消息的文本；解码失败时保持 Content 为空，不影响追加。
// msg 为空时返回英文错误；会话不存在或不属于消息携带的命名空间 / 属主用户时返回 ErrSessionNotFound。
func (s *Store) AppendNativeMessage(ctx context.Context, msg *ai.AgentMessage) (*ai.AgentMessage, error) {
	if msg == nil {
		return nil, errors.New("session store: nil message")
	}
	if msg.MessageKind == "" {
		msg.MessageKind = llm.APIModeChat
	}
	backfillContentFromPayload(msg)
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

// ListNativeMessages 返回会话最近 maxCount 条消息（限定命名空间与属主用户，按会话内 seq 升序返回，最新
// 在后）；maxCount 非正数时返回全部消息。每条消息的 Payload 为原生消息 JSON（经 crypto 解密后由调用方
// 按 MessageKind 反序列化）；仅含 Content 的行也会返回，调用方按需回退。
func (s *Store) ListNativeMessages(ctx context.Context, sessionID uint, namespaceName string, userID uint, maxCount int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ? AND namespace_name = ? AND user_id = ?", sessionID, namespaceName, userID)
	limited := maxCount > 0
	if limited {
		q = q.Order("seq DESC").Limit(maxCount)
	} else {
		q = q.Order("seq ASC")
	}
	var messages []*ai.AgentMessage
	if err := q.Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if limited {
		// 倒序取最近 maxCount 条后反转回升序，与对话顺序一致。
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}
	return messages, nil
}

// UpdateNativeMessage 原地更新一条已落库的消息（按消息 ID + 会话 ID + 命名空间 + 属主用户定位），供运行
// 途中累积原生消息载荷以及终结时写入引用、用量与生命周期终态阶段。不重分配 seq，故消息在会话内的顺序
// 不变。必须用「结构体 + Select」写这几列（Payload / CitationsJSON 为密文列，走 crypto 序列化器；
// map 形式会绕过序列化器把明文直写库）。MessageKind 为空时按 chat 写回。
// msg 为空、ID 或 SessionID 为 0 时返回英文错误；目标消息不存在（含归属不符）时返回 ErrMessageNotFound。
func (s *Store) UpdateNativeMessage(ctx context.Context, msg *ai.AgentMessage) error {
	if msg == nil || msg.ID == 0 || msg.SessionID == 0 {
		return errors.New("session store: nil or unidentified message")
	}
	kind := msg.MessageKind
	if kind == "" {
		kind = llm.APIModeChat
	}
	update := &ai.AgentMessage{
		MessageKind:      kind,
		Payload:          msg.Payload,
		CitationsJSON:    msg.CitationsJSON,
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
		TotalTokens:      msg.TotalTokens,
		ElapsedMs:        msg.ElapsedMs,
		Phase:            msg.Phase,
	}
	columns := []string{"message_kind", "payload", "citations_json", "prompt_tokens", "completion_tokens", "total_tokens", "elapsed_ms", "phase"}
	res := s.db.WithContext(ctx).Model(update).
		Where("id = ? AND session_id = ? AND namespace_name = ? AND user_id = ?", msg.ID, msg.SessionID, msg.NamespaceName, msg.UserID).
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

// SetPendingInterrupt 写入会话当前挂起的根 interrupt ID（传空串即清除挂起），会话按 id + 属主用户
// 定位；会话不存在（含归属不符）时返回 ErrSessionNotFound。
func (s *Store) SetPendingInterrupt(ctx context.Context, sessionID, userID uint, interruptID string) error {
	res := s.db.WithContext(ctx).Model(&ai.AgentSession{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Update("pending_interrupt_id", interruptID)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// GetPendingInterrupt 读取会话当前挂起的根 interrupt ID（无挂起时为空串），会话按 id + 属主用户
// 定位；会话不存在（含归属不符）时返回 ErrSessionNotFound。
func (s *Store) GetPendingInterrupt(ctx context.Context, sessionID, userID uint) (string, error) {
	var session ai.AgentSession
	err := s.db.WithContext(ctx).
		Select("pending_interrupt_id").
		Where("id = ? AND user_id = ?", sessionID, userID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", errors.WithStack(err)
	}
	return session.PendingInterruptID, nil
}

// backfillContentFromPayload 在追加原生消息时补齐正文列：新运行时只写 Payload，而滚动摘要与消息
// 列表仍读 Content。Content 已显式提供时保持原值；Payload 缺失或解码失败时保持 Content 为空，绝不
// 因回填失败中断追加。
func backfillContentFromPayload(msg *ai.AgentMessage) {
	if msg == nil || msg.Content != nil || msg.Payload == nil {
		return
	}
	kind := llm.APIModeChat
	if msg.MessageKind != "" {
		if parsed, err := llm.ParseAPIMode(msg.MessageKind); err == nil {
			kind = parsed
		}
	}
	switch kind {
	case llm.APIModeResponses:
		native, err := msgops.DecodeForModel[*schema.AgenticMessage](msg.Payload.Value(), kind)
		if err != nil {
			return
		}
		msg.Content = serializer.NewCrypto([]byte(msgops.Text(native)))
	default:
		native, err := msgops.DecodeForModel[*schema.Message](msg.Payload.Value(), kind)
		if err != nil {
			return
		}
		msg.Content = serializer.NewCrypto([]byte(msgops.Text(native)))
	}
}
