package ai

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 包级错误哨兵：会话/消息查询不命中的统一错误，供上层 errors.Is 判断。
var (
	// ErrSessionNotFound 会话不存在
	ErrSessionNotFound = errors.New("agent session not found")
	// ErrMessageNotFound 消息不存在
	ErrMessageNotFound = errors.New("agent message not found")
)

// SessionStore 提供 Agent 会话（AgentSession）与消息（AgentMessage）的持久化 CRUD，
// 全部操作基于 GORM（database.DB，即 *gorm.DB）。会话删除时消息在同一事务内级联删除；
// 消息查询默认按创建时间升序，与 LLM 对话顺序一致。
type SessionStore struct {
	// db core 库连接（agent_sessions / agent_messages 读写）。
	db *database.DB
}

// NewSessionStore 构造 SessionStore；db 为空时返回英文错误。
func NewSessionStore(db *database.DB) (*SessionStore, error) {
	if db == nil {
		return nil, errors.New("session store: nil database")
	}
	return &SessionStore{db: db}, nil
}

// CreateSession 创建一个新会话，初始状态为 active；slot 为该会话的缺省档位
// (flash/standard/advanced)，空表示生成时按 agent.defaultSlot 参数解析；agentKind 为会话所属
// Agent 类型，空取缺省 knowledge，非法取值直接返回英文错误且不落库。
func (s *SessionStore) CreateSession(ctx context.Context, namespaceName, title string, slot ai.Slot, agentKind ai.AgentKind) (*ai.AgentSession, error) {
	kind, err := ai.ParseAgentKind(agentKind)
	if err != nil {
		return nil, err
	}
	session := &ai.AgentSession{
		NamespaceName: namespaceName,
		Title:         title,
		TargetModel:   slot,
		Status:        ai.SessionStatusActive,
		AgentKind:     kind,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// GetSession 按 ID 查询会话；不存在时返回 ErrSessionNotFound。
func (s *SessionStore) GetSession(ctx context.Context, id uint) (*ai.AgentSession, error) {
	var session ai.AgentSession
	err := s.db.WithContext(ctx).First(&session, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &session, nil
}

// ListSessions 列出命名空间下的会话，按更新时间倒序（最近活跃在前）；
// limit/offset 非正数时不限定。
func (s *SessionStore) ListSessions(ctx context.Context, namespaceName string, limit, offset int) ([]*ai.AgentSession, error) {
	q := s.db.WithContext(ctx).Where("namespace_name = ?", namespaceName).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var sessions []*ai.AgentSession
	if err := q.Find(&sessions).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return sessions, nil
}

// RenameSession 重命名会话标题，返回更新后的会话。
func (s *SessionStore) RenameSession(ctx context.Context, id uint, title string) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(session).Update("title", title).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// SetSessionStatus 更新会话状态（如归档），返回更新后的会话。
func (s *SessionStore) SetSessionStatus(ctx context.Context, id uint, status ai.SessionStatus) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(session).Update("status", status).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// DeleteSession 在同一事务内删除会话及其全部消息（会话软删除、消息硬删除，
// DB 级联不生效，须显式清理，与 Knowledge.DeleteDocument 同模式）。
func (s *SessionStore) DeleteSession(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&ai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ?", id).Delete(&ai.AgentSession{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// AppendMessage 追加一条消息到会话；msg 为空时返回英文错误。
func (s *SessionStore) AppendMessage(ctx context.Context, msg *ai.AgentMessage) (*ai.AgentMessage, error) {
	if msg == nil {
		return nil, errors.New("session store: nil message")
	}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return msg, nil
}

// ListMessages 分页列出会话消息（按创建时间升序）；limit/offset 非正数时不限定。
func (s *SessionStore) ListMessages(ctx context.Context, sessionID uint, limit, offset int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at ASC, id ASC")
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

// HistoryMessages 返回会话最近 maxCount 条消息（按时间升序返回，最新在后），
// 供注入 LLM 上下文保持对话顺序；maxCount 非正数时返回全部消息。
func (s *SessionStore) HistoryMessages(ctx context.Context, sessionID uint, maxCount int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if maxCount > 0 {
		q = q.Order("created_at DESC, id DESC").Limit(maxCount)
	} else {
		q = q.Order("created_at ASC, id ASC")
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

// ListMessagesAfter 返回会话中 id 大于 afterID 的消息（按 id 升序），供会话压缩取出
// 「游标之后尚未摘要的全部消息」。
func (s *SessionStore) ListMessagesAfter(ctx context.Context, sessionID uint, afterID uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id > ?", sessionID, afterID).
		Order("id ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// ListMessagesBefore 返回会话中 afterID 与 beforeID 之间的消息（afterID < id < beforeID，
// 按 id 升序），供滚动摘要取出「窗口外且尚未摘要」的消息（窗口最旧消息 ID 作上界）。
func (s *SessionStore) ListMessagesBefore(ctx context.Context, sessionID uint, beforeID uint, afterID uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id > ? AND id < ?", sessionID, afterID, beforeID).
		Order("id ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// UpdateSummary 更新会话的滚动摘要与其覆盖到的最后一条消息 ID，只写这两列、不触碰其它字段
// （避免并发改写会话标题/档位等）。摘要经 crypto 序列化器加密落库，故必须以「结构体 + Select」
// 更新以触发序列化器（map 形式会绕过序列化器把明文直写库）。
func (s *SessionStore) UpdateSummary(ctx context.Context, sessionID uint, summary string, uptoMessageID uint) error {
	update := &ai.AgentSession{
		Summary:              serializer.NewCrypto([]byte(summary)),
		SummaryUptoMessageID: uptoMessageID,
	}
	if err := s.db.WithContext(ctx).Model(update).
		Where("id = ?", sessionID).
		Select("summary", "summary_upto_message_id").
		Updates(update).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// LatestAssistantMessage 返回会话最近一条助手消息（含草稿/引用载荷）；
// 不存在时返回 ErrMessageNotFound。
func (s *SessionStore) LatestAssistantMessage(ctx context.Context, sessionID uint) (*ai.AgentMessage, error) {
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND role = ?", sessionID, ai.MessageRoleAssistant).
		Order("created_at DESC, id DESC").
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &msg, nil
}
