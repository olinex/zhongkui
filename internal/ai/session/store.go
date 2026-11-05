package session

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 包级错误哨兵：会话/消息查询不命中的统一错误，供上层 errors.Is 判断。定义归 contract
// （跨层共享），此处保留同值别名以兼容既有调用点。
var (
	// ErrSessionNotFound 会话不存在
	ErrSessionNotFound = contract.ErrSessionNotFound
	// ErrMessageNotFound 消息不存在
	ErrMessageNotFound = contract.ErrMessageNotFound
)

// Store 提供 Agent 会话（AgentSession）与消息（AgentMessage）的持久化 CRUD，
// 全部操作基于 GORM（database.DB，即 *gorm.DB）。会话删除时消息在同一事务内级联删除；
// 消息查询默认按会话内 seq 升序，与 LLM 对话顺序一致。
type Store struct {
	// db core 库连接（agent_sessions / agent_messages 读写）。
	db *database.DB
}

// NewStore 构造 Store；db 为空时返回英文错误。
func NewStore(db *database.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("session store: nil database")
	}
	return &Store{db: db}, nil
}

// CreateSession 创建一个新会话，初始状态为 active；slot 为该会话的缺省档位
// (flash/standard/advanced)，空表示生成时按 agent.defaultSlot 参数解析；agentKind 为会话所属
// Agent 类型，空取缺省 knowledge，非法取值直接返回英文错误且不落库。会话来由取普通助手会话。
func (s *Store) CreateSession(ctx context.Context, namespaceName, title string, slot ai.Slot, agentKind ai.AgentKind, userID uint) (*ai.AgentSession, error) {
	return s.CreateSessionWithOrigin(ctx, namespaceName, title, slot, agentKind, ai.SessionOriginChat, userID)
}

// CreateSessionWithOrigin 按指定会话来由创建会话；内部运行会话（如周期报告每次运行）以
// periodic_report 创建，从而不出现在 AI 助手会话列表中。
func (s *Store) CreateSessionWithOrigin(ctx context.Context, namespaceName, title string, slot ai.Slot, agentKind ai.AgentKind, origin ai.SessionOrigin, userID uint) (*ai.AgentSession, error) {
	if userID == 0 {
		// user_id 为 users(id) 外键，0 不是合法属主。
		return nil, errors.New("session store: user id is required")
	}
	kind, err := ai.ParseAgentKind(agentKind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	session := &ai.AgentSession{
		NamespaceName: namespaceName,
		UserID:        userID,
		Title:         title,
		TargetModel:   slot,
		Status:        ai.SessionStatusActive,
		AgentKind:     kind,
		Origin:        origin,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// GetSession 按 ID 查询会话，并限定其属于给定命名空间与属主用户（跨用户隔离）；不存在时返回
// ErrSessionNotFound。
func (s *Store) GetSession(ctx context.Context, id uint, namespaceName string, userID uint) (*ai.AgentSession, error) {
	var session ai.AgentSession
	err := s.db.WithContext(ctx).
		Where("id = ? AND namespace_name = ? AND user_id = ?", id, namespaceName, userID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &session, nil
}

// ListSessions 列出命名空间下属于该用户、状态为 active 且会话来由为普通助手的会话，按更新时间
// 倒序（最近活跃在前）；limit/offset 非正数时不限定。归档（archived）等非 active 会话与内部
// 运行会话（如周期报告每次运行的会话）不出现在列表中。
func (s *Store) ListSessions(ctx context.Context, namespaceName string, userID uint, limit, offset int) ([]*ai.AgentSession, error) {
	q := s.db.WithContext(ctx).
		Where("namespace_name = ? AND user_id = ? AND status = ? AND origin = ?",
			namespaceName, userID, ai.SessionStatusActive, ai.SessionOriginChat).
		Order("updated_at DESC")
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

// RenameSession 重命名会话标题，返回更新后的会话；会话不属于给定命名空间 / 属主用户时返回
// ErrSessionNotFound。
func (s *Store) RenameSession(ctx context.Context, id uint, namespaceName string, userID uint, title string) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := s.db.WithContext(ctx).Model(session).Update("title", title).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// UpdateSessionValve 更新会话的审核阀门（each/auto/none），返回更新后的会话；会话不存在时返回
// ErrSessionNotFound。阀门取值合法性由调用方经绑定标签校验，本方法只负责落库。
func (s *Store) UpdateSessionValve(ctx context.Context, id uint, namespaceName string, userID uint, valve ai.ReviewValve) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := s.db.WithContext(ctx).Model(session).Update("review_valve", valve).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// SetSessionStatus 更新会话状态（如归档），返回更新后的会话；会话不属于给定命名空间 / 属主用户
// 时返回 ErrSessionNotFound。
func (s *Store) SetSessionStatus(ctx context.Context, id uint, namespaceName string, userID uint, status ai.SessionStatus) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := s.db.WithContext(ctx).Model(session).Update("status", status).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// DeleteSession 在同一事务内以真实（硬）删除移除会话及其从属数据：会话行不再软删除，连同其消息、
// 持久化交互、压缩检查点、长期记忆与挂起运行现场一并整行删除。会话对各子表均为真 FK
// （ON DELETE CASCADE），但不依赖数据库是否强制外键（如 sqlite 默认关闭），仍显式清理：先删持久化
// 交互与消息、压缩检查点，再删长期记忆（agent_memories）与挂起运行现场（agent_runs），最后经
// Unscoped 硬删会话本身（消息/交互/检查点本就无 deleted_at 列为硬删；记忆与运行现场亦经 Unscoped
// 硬删）。
func (s *Store) DeleteSession(ctx context.Context, id uint, namespaceName string, userID uint) error {
	scope := "session_id = ? AND namespace_name = ? AND user_id = ?"
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentInteraction{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentSessionCompaction{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Unscoped().Delete(&ai.AgentMemory{}).Error; err != nil {
			return errors.WithStack(err)
		}
		// 挂起现场的图状态字节（agent_checkpoints）以 agent_runs.checkpoint_id 关联，且无外键；
		// 先按该会话运行现场的键清掉，避免会话删除后留下孤儿现场。审批回退快照（redo:<会话ID>:
		// <交互ID>）不挂在 agent_runs 上，按会话前缀一并清理。
		if err := tx.Exec(`DELETE FROM agent_checkpoints WHERE checkpoint_id IN (
			SELECT checkpoint_id FROM agent_runs
			WHERE session_id = ? AND namespace_name = ? AND user_id = ? AND checkpoint_id <> '')`,
			id, namespaceName, userID).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("checkpoint_id LIKE ?", RedoCheckpointPrefix(id)+"%").
			Delete(&ai.AgentCheckpoint{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Unscoped().Delete(&ai.AgentRun{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ? AND namespace_name = ? AND user_id = ?", id, namespaceName, userID).Unscoped().Delete(&ai.AgentSession{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// DB 返回底层数据库句柄，仅供测试与诊断读取原始行（加密列校验等）；生产链路不得使用。
func (s *Store) DB() *database.DB {
	return s.db
}
