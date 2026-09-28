package ai

import (
	// stdlib
	"context"

	// third-party
	"gorm.io/gorm"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// 包级错误哨兵：会话/消息查询不命中的统一错误，供上层 errors.Is 判断。
// Package-level error sentinels: unified not-found errors for session/message
// lookups, checkable via errors.Is by upper layers.
var (
	// ErrSessionNotFound 会话不存在
	// ErrSessionNotFound the session does not exist
	ErrSessionNotFound = errors.New("agent session not found")
	// ErrMessageNotFound 消息不存在
	// ErrMessageNotFound the message does not exist
	ErrMessageNotFound = errors.New("agent message not found")
)

// SessionStore 提供 Agent 会话（AgentSession）与消息（AgentMessage）的持久化 CRUD，
// 全部操作基于 GORM（database.DB，即 *gorm.DB）。会话删除时消息在同一事务内级联删除；
// 消息查询默认按创建时间升序，与 LLM 对话顺序一致。
// SessionStore provides the persistent CRUD of agent sessions (AgentSession) and
// messages (AgentMessage) over GORM (database.DB, i.e. *gorm.DB). Deleting a
// session cascades to its messages inside one transaction; message queries are
// ordered by creation time ascending by default, aligned with the LLM dialogue
// order.
type SessionStore struct {
	db *database.DB
}

// NewSessionStore 构造 SessionStore；db 为空时返回英文错误。
// NewSessionStore constructs a SessionStore; a nil db yields an English error.
func NewSessionStore(db *database.DB) (*SessionStore, error) {
	if db == nil {
		return nil, errors.New("session store: nil database")
	}
	return &SessionStore{db: db}, nil
}

// CreateSession 创建一个新会话，初始状态为 active；slot 为该会话的缺省档位
// (flash/standard/advanced)，空表示生成时按 agent.defaultSlot 参数解析；agentKind 为会话所属
// Agent 类型，空取缺省 knowledge，非法取值直接返回英文错误且不落库。
// CreateSession creates a new session with the initial status active; slot is the default
// slot of the session (flash/standard/advanced) and an empty value means resolving through the
// agent.defaultSlot parameter at generation time; agentKind is the agent kind the session belongs
// to, an empty value defaults to knowledge and an invalid value yields an English error without
// persisting anything.
func (s *SessionStore) CreateSession(ctx context.Context, namespaceName, title string, slot llm.Slot, agentKind llm.AgentKind) (*modelsai.AgentSession, error) {
	kind, err := llm.ParseAgentKind(agentKind)
	if err != nil {
		return nil, err
	}
	session := &modelsai.AgentSession{
		NamespaceName: namespaceName,
		Title:         title,
		TargetModel:   slot,
		Status:        llm.SessionStatusActive,
		AgentKind:     kind,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// GetSession 按 ID 查询会话；不存在时返回 ErrSessionNotFound。
// GetSession fetches a session by ID; a missing session yields ErrSessionNotFound.
func (s *SessionStore) GetSession(ctx context.Context, id uint) (*modelsai.AgentSession, error) {
	var session modelsai.AgentSession
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
// ListSessions lists the sessions of a namespace ordered by update time
// descending (most recently active first); non-positive limit/offset means no
// bound.
func (s *SessionStore) ListSessions(ctx context.Context, namespaceName string, limit, offset int) ([]*modelsai.AgentSession, error) {
	q := s.db.WithContext(ctx).Where("namespace_name = ?", namespaceName).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var sessions []*modelsai.AgentSession
	if err := q.Find(&sessions).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return sessions, nil
}

// RenameSession 重命名会话标题，返回更新后的会话。
// RenameSession renames the session title and returns the updated session.
func (s *SessionStore) RenameSession(ctx context.Context, id uint, title string) (*modelsai.AgentSession, error) {
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
// SetSessionStatus updates the session status (e.g. archived) and returns the
// updated session.
func (s *SessionStore) SetSessionStatus(ctx context.Context, id uint, status llm.SessionStatus) (*modelsai.AgentSession, error) {
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
// DeleteSession deletes the session and all its messages inside one transaction
// (the session is soft-deleted, the messages hard-deleted; the DB cascade does
// not fire, so the cleanup is explicit, same pattern as Knowledge.DeleteDocument).
func (s *SessionStore) DeleteSession(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&modelsai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ?", id).Delete(&modelsai.AgentSession{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// AppendMessage 追加一条消息到会话；msg 为空时返回英文错误。
// AppendMessage appends a message to a session; a nil msg yields an English error.
func (s *SessionStore) AppendMessage(ctx context.Context, msg *modelsai.AgentMessage) (*modelsai.AgentMessage, error) {
	if msg == nil {
		return nil, errors.New("session store: nil message")
	}
	if err := s.db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return msg, nil
}

// ListMessages 分页列出会话消息（按创建时间升序）；limit/offset 非正数时不限定。
// ListMessages pages the messages of a session (ordered by creation time
// ascending); non-positive limit/offset means no bound.
func (s *SessionStore) ListMessages(ctx context.Context, sessionID uint, limit, offset int) ([]*modelsai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at ASC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var messages []*modelsai.AgentMessage
	if err := q.Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// HistoryMessages 返回会话最近 maxCount 条消息（按时间升序返回，最新在后），
// 供注入 LLM 上下文保持对话顺序；maxCount 非正数时返回全部消息。
// HistoryMessages returns the latest maxCount messages of a session (ordered by
// time ascending, newest last) for LLM context injection; a non-positive
// maxCount returns all messages.
func (s *SessionStore) HistoryMessages(ctx context.Context, sessionID uint, maxCount int) ([]*modelsai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if maxCount > 0 {
		q = q.Order("created_at DESC, id DESC").Limit(maxCount)
	} else {
		q = q.Order("created_at ASC, id ASC")
	}
	var messages []*modelsai.AgentMessage
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
// ListMessagesAfter returns the messages of a session whose id is greater than afterID
// (ordered by id ascending), used by session compaction to take "every message after the
// cursor that has not been summarized yet".
func (s *SessionStore) ListMessagesAfter(ctx context.Context, sessionID uint, afterID uint) ([]*modelsai.AgentMessage, error) {
	var messages []*modelsai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id > ?", sessionID, afterID).
		Order("id ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// ListMessagesBefore 返回会话中 afterID 与 beforeID 之间的消息（afterID < id < beforeID，
// 按 id 升序），供滚动摘要取出「窗口外且尚未摘要」的消息（窗口最旧消息 ID 作上界）。
// ListMessagesBefore returns the messages of a session between afterID and beforeID
// (afterID < id < beforeID, ordered by id ascending), used by the rolling summary to take
// the "window-external and not yet summarized" messages (with the oldest window message id
// as the upper bound).
func (s *SessionStore) ListMessagesBefore(ctx context.Context, sessionID uint, beforeID uint, afterID uint) ([]*modelsai.AgentMessage, error) {
	var messages []*modelsai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND id > ? AND id < ?", sessionID, afterID, beforeID).
		Order("id ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// UpdateSummary 更新会话的滚动摘要与其覆盖到的最后一条消息 ID，只写这两列、不触碰其它字段
// （避免并发改写会话标题/档位等）。
// UpdateSummary updates the rolling summary of a session and the id of the last message it
// covers, writing only these two columns and leaving every other field untouched (so a
// concurrent update of the title/slot is never clobbered).
func (s *SessionStore) UpdateSummary(ctx context.Context, sessionID uint, summary string, uptoMessageID uint) error {
	if err := s.db.WithContext(ctx).Model(&modelsai.AgentSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"summary":                 summary,
			"summary_upto_message_id": uptoMessageID,
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// LatestAssistantMessage 返回会话最近一条助手消息（含草稿/引用载荷）；
// 不存在时返回 ErrMessageNotFound。
// LatestAssistantMessage returns the latest assistant message of a session
// (including the draft/citation payloads); a missing one yields
// ErrMessageNotFound.
func (s *SessionStore) LatestAssistantMessage(ctx context.Context, sessionID uint) (*modelsai.AgentMessage, error) {
	var msg modelsai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND role = ?", sessionID, llm.MessageRoleAssistant).
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
