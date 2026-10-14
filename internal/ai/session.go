package ai

import (
	// 标准库
	"context"
	"database/sql"

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
// 消息查询默认按会话内 seq 升序，与 LLM 对话顺序一致。
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
func (s *SessionStore) CreateSession(ctx context.Context, namespaceName, title string, slot ai.Slot, agentKind ai.AgentKind, userID uint) (*ai.AgentSession, error) {
	kind, err := ai.ParseAgentKind(agentKind)
	if err != nil {
		return nil, err
	}
	session := &ai.AgentSession{
		NamespaceName: namespaceName,
		UserID:        userID,
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

// GetSession 按 ID 查询会话，并限定其属于给定命名空间与属主用户（跨用户隔离）；不存在时返回
// ErrSessionNotFound。
func (s *SessionStore) GetSession(ctx context.Context, id uint, namespaceName string, userID uint) (*ai.AgentSession, error) {
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

// ListSessions 列出命名空间下属于该用户且状态为 active 的会话，按更新时间倒序（最近活跃在前）；
// limit/offset 非正数时不限定。归档（archived）等非 active 会话不出现在列表中。
func (s *SessionStore) ListSessions(ctx context.Context, namespaceName string, userID uint, limit, offset int) ([]*ai.AgentSession, error) {
	q := s.db.WithContext(ctx).Where("namespace_name = ? AND user_id = ? AND status = ?", namespaceName, userID, ai.SessionStatusActive).Order("updated_at DESC")
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
func (s *SessionStore) RenameSession(ctx context.Context, id uint, namespaceName string, userID uint, title string) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(session).Update("title", title).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// UpdateSessionValve 更新会话的审核阀门（each/auto/none），返回更新后的会话；会话不存在时返回
// ErrSessionNotFound。阀门取值合法性由调用方经绑定标签校验，本方法只负责落库。
func (s *SessionStore) UpdateSessionValve(ctx context.Context, id uint, namespaceName string, userID uint, valve ai.ReviewValve) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(session).Update("review_valve", valve).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// SetSessionStatus 更新会话状态（如归档），返回更新后的会话；会话不属于给定命名空间 / 属主用户
// 时返回 ErrSessionNotFound。
func (s *SessionStore) SetSessionStatus(ctx context.Context, id uint, namespaceName string, userID uint, status ai.SessionStatus) (*ai.AgentSession, error) {
	session, err := s.GetSession(ctx, id, namespaceName, userID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(session).Update("status", status).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// DeleteSession 在同一事务内以真实（硬）删除移除会话及其从属数据：会话行不再软删除，连同其消息、
// 压缩检查点、长期记忆、挂起运行现场与驻留运行登记及事件一并整行删除。会话对各子表均为真 FK
// （ON DELETE CASCADE），但不依赖数据库是否强制外键（如 sqlite 默认关闭），仍显式清理：先删消息与
// 压缩检查点，再删长期记忆（agent_memories）、挂起运行现场（agent_runs）与驻留运行登记及事件
// （agent_live_runs / agent_run_events），最后经 Unscoped 硬删会话本身（消息/检查点/事件/登记本就无
// deleted_at 列为硬删；记忆与运行现场亦经 Unscoped 硬删）。
func (s *SessionStore) DeleteSession(ctx context.Context, id uint, namespaceName string, userID uint) error {
	scope := "session_id = ? AND namespace_name = ? AND user_id = ?"
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentSessionCompaction{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Unscoped().Delete(&ai.AgentMemory{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Unscoped().Delete(&ai.AgentRun{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentRunEvent{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where(scope, id, namespaceName, userID).Delete(&ai.AgentLiveRun{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ? AND namespace_name = ? AND user_id = ?", id, namespaceName, userID).Unscoped().Delete(&ai.AgentSession{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// allocateMessageSeq 在 tx 内为某会话原子分配 count 个连续序号：以会话行的 message_seq 计数列
// 一次性自增并取回新的最大值；count 即本次分配到的最大序号（count=1 时即该条消息的 seq）。
// 会话不存在（id + user_id 不匹配）时返回 ErrSessionNotFound。
func allocateMessageSeq(tx *gorm.DB, sessionID, userID uint, count int64) (uint64, error) {
	var seq int64
	err := tx.Raw("UPDATE agent_sessions SET message_seq = message_seq + ? WHERE id = ? AND user_id = ? RETURNING message_seq", count, sessionID, userID).
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
// 会话不存在时返回 ErrSessionNotFound。
func (s *SessionStore) AppendMessage(ctx context.Context, msg *ai.AgentMessage) (*ai.AgentMessage, error) {
	if msg == nil {
		return nil, errors.New("session store: nil message")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seq, err := allocateMessageSeq(tx, msg.SessionID, msg.UserID, 1)
		if err != nil {
			return err
		}
		msg.Seq = seq
		if err := tx.Create(msg).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// TruncateMessagesFromUserMessage 截断会话消息：删除会话内第 userMessageIndex 条（从 1 起，仅按
// 用户消息计数、按 seq 升序定位）用户消息，以及该条之后的全部消息（含助手回复），供「回退 / 删除到
// 某一轮对话」使用。目标用户消息不存在（序号越界）时返回 ErrMessageNotFound。
// 单事务执行，避免截断中途留下不一致的会话内容。截断落在已压缩区间内（summary_upto_seq ≥ 被截断
// 起始 seq）时，按压缩检查点回退上下文起点：恢复「upto_seq < 起始 seq」的最近一个检查点的摘要与
// 游标，无可用检查点则清空摘要、summary_upto_seq 置 0（回到最开始）；并作废（删除）upto_seq ≥ 起始
// seq 的检查点。截断点尚未被摘要覆盖（summary_upto_seq < 起始 seq）时摘要与游标保持不变。回滚不删除
// 压缩期间抽取的长期记忆（agent_memories），也不做级联删除。后续追加仍按会话 message_seq 单调分配，
// 序号不复用。
func (s *SessionStore) TruncateMessagesFromUserMessage(ctx context.Context, sessionID, userID uint, userMessageIndex int) error {
	if userMessageIndex < 1 {
		return errors.New("session store: user message index must be >= 1")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target ai.AgentMessage
		err := tx.Where("session_id = ? AND user_id = ? AND role = ?", sessionID, userID, ai.MessageRoleUser).
			Order("seq ASC").Offset(userMessageIndex - 1).Limit(1).First(&target).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMessageNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("session_id = ? AND user_id = ? AND seq >= ?", sessionID, userID, target.Seq).
			Delete(&ai.AgentMessage{}).Error; err != nil {
			return errors.WithStack(err)
		}
		// 读取会话当前上下文起点（仅取所需两列，避免解密摘要）：截断点落在已压缩区间内时按检查点回退。
		var session ai.AgentSession
		if err := tx.Select("id", "summary_upto_seq").Where("id = ? AND user_id = ?", sessionID, userID).
			First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSessionNotFound
			}
			return errors.WithStack(err)
		}
		if uint64(session.SummaryUptoSeq) < target.Seq {
			// 回滚点尚未被摘要覆盖：摘要与游标（及其检查点）保持不变。
			return nil
		}
		checkpoint, cpErr := latestCompactionBefore(tx, sessionID, userID, target.Seq)
		if cpErr != nil && !errors.Is(cpErr, ErrMessageNotFound) {
			return cpErr
		}
		// 摘要是密文列，恢复/清空一律走「结构体 + Select」以触发 crypto 序列化器（map 会绕过序列化器
		// 直写明文）；无可用检查点即清空（Summary 置 nil）并回到最开始。
		restore := &ai.AgentSession{Summary: nil, SummaryUptoSeq: 0}
		if checkpoint != nil {
			restore = &ai.AgentSession{Summary: checkpoint.Summary, SummaryUptoSeq: uint(checkpoint.UptoSeq)}
		}
		if err := tx.Model(restore).Where("id = ? AND user_id = ?", sessionID, userID).
			Select("summary", "summary_upto_seq").Updates(restore).Error; err != nil {
			return errors.WithStack(err)
		}
		// 作废 upto_seq ≥ 起始 seq 的检查点。
		if err := tx.Where("session_id = ? AND user_id = ? AND upto_seq >= ?", sessionID, userID, target.Seq).
			Delete(&ai.AgentSessionCompaction{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

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

// AssistantSeqAtUserMessage 返回「第 index 条用户消息（从 1 起，仅计 role=user、按 seq 升序）之后
// 的首条助手消息」的会话内 seq，供手动压缩把折叠上界定位到该轮助手回复；目标用户消息不存在或其后
// 尚无助手回复时返回 ErrMessageNotFound。
func (s *SessionStore) AssistantSeqAtUserMessage(ctx context.Context, sessionID, userID uint, index int) (uint64, error) {
	if index < 1 {
		return 0, errors.New("session store: user message index must be >= 1")
	}
	var target ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND role = ?", sessionID, userID, ai.MessageRoleUser).
		Order("seq ASC").Offset(index - 1).Limit(1).First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrMessageNotFound
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	var assistant ai.AgentMessage
	err = s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND role = ? AND seq > ?", sessionID, userID, ai.MessageRoleAssistant, target.Seq).
		Order("seq ASC").Limit(1).First(&assistant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrMessageNotFound
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return assistant.Seq, nil
}

// BranchSessionFromUserMessage 以第 userMessageIndex 条（从 1 起，仅按用户消息计数、按 seq 升序
// 定位）用户消息所在轮次为结尾，把该会话从最早一条消息起、直到「下一条用户消息之前」的全部消息
// 复制为一个新会话，供前端「分支」使用（新会话可独立演进，源会话不受影响）。目标用户消息不存在
// （序号越界）时返回 ErrMessageNotFound；源会话不属于给定属主用户时返回 ErrSessionNotFound。
// 单事务执行，避免复制中途留下半截会话。新会话归属同一命名空间与属主，标题为空时沿用源会话标题，
// 档位 / Agent 类型 / 审核阀门与源会话一致，状态 active；滚动摘要不复制（消息 seq 会重新编号，
// 原摘要覆盖范围失去意义）。复制出的消息在新会话内 seq 从 1 连续重新编号（新 session_id 不同，
// 不受 UNIQUE(session_id,user_id,seq) 影响），保留 role/content/draft/citations/created_at 等载荷，
// 不复制 id 与源 seq。新会话的 message_seq 直接置为复制条数（在插入时一次算好，供后续追加消息从
// count+1 继续分配 seq）。
func (s *SessionStore) BranchSessionFromUserMessage(ctx context.Context, sessionID, userID uint, userMessageIndex int, title string) (*ai.AgentSession, error) {
	if userMessageIndex < 1 {
		return nil, errors.New("session store: user message index must be >= 1")
	}
	var branched *ai.AgentSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 校验源会话归属（id + user_id）：不存在或跨用户均视为不存在。这里单独查一次而非并入下方
		// CTE，是因为「源会话不存在」与「用户消息序号越界」在 CTE 里都只表现为插入 0 行、无法区分，
		// 而两者必须返回不同的哨兵错误；这次查询同时顺带取出源标题用于缺省。
		var source ai.AgentSession
		err := tx.Where("id = ? AND user_id = ?", sessionID, userID).First(&source).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSessionNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		// 标题为空时沿用源会话标题，非空则用传入值覆盖。
		newTitle := source.Title
		if title != "" {
			newTitle = title
		}

		// 用单条 CTE 完成原本串行的「定位分支上界 → 插入新会话 → 逐条复制消息 → 回写 message_seq」。
		// 之所以值得这样合并：逐条复制消息原先是最多 N 次 INSERT，是这里唯一随消息数增长的 SQL，
		// 改成 INSERT ... SELECT + ROW_NUMBER() 后一次成型，SQL 次数由 N 次降为常数次；整段仍在同一
		// 事务内执行，对并发与回滚的语义与逐条复制完全等价。
		//   anchor：作为分支结尾的第 userMessageIndex 条用户消息（仅计 role=user、按 seq 升序）。
		//   bounds：anchor_seq 为该消息 seq（序号越界时为 NULL），upper_seq 为「下一条用户消息」的
		//           seq（不存在则为 NULL，代表复制到会话末尾）；seq 从 1 起，故 NULL 可安全充当「无上界」。
		//   new_session：插入新会话，message_seq 用与 copied 相同的 where 谓词计数一次算好，省掉原先
		//           末尾的一次 UPDATE；anchor_seq 为 NULL 时 WHERE 过滤成立即插入 0 行，外层读到空行
		//           即可判定序号越界。
		//   copied：按 seq 升序把范围内消息复制进新会话，seq 用 ROW_NUMBER() 从 1 连续重编号；
		//           content/draft_json/citations_json 是密文列，原样搬运字节即可，无需解密再加密。
		// 源会话的软删除（deleted_at IS NULL）已由上面的 GORM First 过滤，CTE 内不再重复判断。
		row := tx.Raw(`
WITH anchor AS (
    SELECT seq FROM agent_messages
    WHERE session_id = ? AND user_id = ? AND role = ?
    ORDER BY seq ASC
    OFFSET ? LIMIT 1
),
bounds AS (
    SELECT
        (SELECT seq FROM anchor) AS anchor_seq,
        (SELECT MIN(m.seq) FROM agent_messages m
         WHERE m.session_id = ? AND m.user_id = ? AND m.role = ?
           AND m.seq > (SELECT seq FROM anchor)) AS upper_seq
),
new_session AS (
    INSERT INTO agent_sessions
        (created_at, updated_at, namespace_name, user_id, title, target_model, status, agent_kind, review_valve, message_seq)
    SELECT
        NOW(), NOW(), a.namespace_name, a.user_id, ?, a.target_model, ?, a.agent_kind, a.review_valve,
        (SELECT COUNT(*) FROM agent_messages m
         WHERE m.session_id = ? AND m.user_id = ? AND (b.upper_seq IS NULL OR m.seq < b.upper_seq))
    FROM agent_sessions a, bounds b
    WHERE a.id = ? AND a.user_id = ? AND b.anchor_seq IS NOT NULL
    RETURNING id, created_at, updated_at, namespace_name, user_id, title, target_model, status, agent_kind, review_valve, message_seq
),
copied AS (
    INSERT INTO agent_messages
        (created_at, updated_at, session_id, namespace_name, user_id, seq, role, content, draft_json, citations_json, prompt_tokens, completion_tokens, total_tokens, elapsed_ms)
    SELECT
        m.created_at, NOW(), ns.id, m.namespace_name, m.user_id, ROW_NUMBER() OVER (ORDER BY m.seq),
        m.role, m.content, m.draft_json, m.citations_json,
        m.prompt_tokens, m.completion_tokens, m.total_tokens, m.elapsed_ms
    FROM agent_messages m, bounds b, new_session ns
    WHERE m.session_id = ? AND m.user_id = ? AND (b.upper_seq IS NULL OR m.seq < b.upper_seq)
)
SELECT id, created_at, updated_at, namespace_name, user_id, title, target_model, status, agent_kind, review_valve, message_seq
FROM new_session
`,
			sessionID, userID, ai.MessageRoleUser, userMessageIndex-1,
			sessionID, userID, ai.MessageRoleUser,
			newTitle, ai.SessionStatusActive,
			sessionID, userID,
			sessionID, userID,
			sessionID, userID,
		).Row()

		var branch ai.AgentSession
		if err := row.Scan(&branch.ID, &branch.CreatedAt, &branch.UpdatedAt, &branch.NamespaceName,
			&branch.UserID, &branch.Title, &branch.TargetModel, &branch.Status, &branch.AgentKind,
			&branch.ReviewValve, &branch.MessageSeq); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrMessageNotFound
			}
			return errors.WithStack(err)
		}
		branched = &branch
		return nil
	})
	if err != nil {
		return nil, err
	}
	return branched, nil
}

// ListMessages 分页列出会话（限定属主用户）消息（按会话内 seq 升序）；limit/offset 非正数时不限定。
func (s *SessionStore) ListMessages(ctx context.Context, sessionID, userID uint, limit, offset int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).Order("seq ASC")
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

// HistoryMessages 返回会话最近 maxCount 条消息（按会话内 seq 升序返回，最新在后），
// 供注入 LLM 上下文保持对话顺序；maxCount 非正数时返回全部消息。
func (s *SessionStore) HistoryMessages(ctx context.Context, sessionID, userID uint, maxCount int) ([]*ai.AgentMessage, error) {
	q := s.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID)
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

// ListMessagesAfter 返回会话中 seq 大于 afterSeq 的消息（按会话内 seq 升序），供会话压缩取出
// 「游标之后尚未摘要的全部消息」。
func (s *SessionStore) ListMessagesAfter(ctx context.Context, sessionID, userID uint, afterSeq uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND seq > ?", sessionID, userID, afterSeq).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// ListMessagesBefore 返回会话中 afterSeq 与 beforeSeq 之间的消息（afterSeq < seq < beforeSeq，
// 按会话内 seq 升序），供滚动摘要取出「窗口外且尚未摘要」的消息（窗口最旧消息 seq 作上界）。
func (s *SessionStore) ListMessagesBefore(ctx context.Context, sessionID, userID uint, beforeSeq uint, afterSeq uint) ([]*ai.AgentMessage, error) {
	var messages []*ai.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND seq > ? AND seq < ?", sessionID, userID, afterSeq, beforeSeq).
		Order("seq ASC").Find(&messages).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return messages, nil
}

// UpdateSummaryAndCheckpoint 在单事务内推进会话的滚动摘要：更新 agent_sessions 的 summary 与
// summary_upto_seq，并追加一条 agent_session_compactions 压缩检查点（记录推进后的游标与摘要全文，
// 供回滚逐级回退 start bound）。reason 取 rolling（静默滚动）/ auto（自动压缩）/ manual（手动压缩）。
// 摘要经 crypto 序列化器加密落库，故会话更新必须以「结构体 + Select」写这两列、不触碰其它字段
// （避免并发改写会话标题/档位等；map 形式会绕过序列化器把明文直写库）；检查点同样是密文列，走
// Create 结构体路径加密。session 必须带有 ID / UserID / NamespaceName（调用点均持有已加载的会话）。
func (s *SessionStore) UpdateSummaryAndCheckpoint(ctx context.Context, session *ai.AgentSession, summary string, uptoSeq uint, reason string) error {
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
		}
		if err := tx.Create(checkpoint).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// LatestAssistantMessage 返回会话最近一条助手消息（含草稿/引用载荷），并限定属主用户；
// 不存在时返回 ErrMessageNotFound。
func (s *SessionStore) LatestAssistantMessage(ctx context.Context, sessionID, userID uint) (*ai.AgentMessage, error) {
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND role = ?", sessionID, userID, ai.MessageRoleAssistant).
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

// ContextTokens 返回会话最近一条助手消息的输入 token 数（PromptTokens），即当前会话的上下文
// 占用度量；无助手消息（ErrMessageNotFound）时返回 0，不视为错误。
func (s *SessionStore) ContextTokens(ctx context.Context, sessionID, userID uint) (int, error) {
	msg, err := s.LatestAssistantMessage(ctx, sessionID, userID)
	if errors.Is(err, ErrMessageNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return msg.PromptTokens, nil
}

// ContextTokensBySession 批量返回每个会话最近一条助手消息的输入 token 数（PromptTokens），
// 供会话列表一次性取回各会话的上下文占用。以「按 session_id 分组的 MAX(seq) 子查询 JOIN
// agent_messages」单条 SQL 完成，避免逐会话查询的 N+1；无助手消息的会话不出现在结果中，
// sessionIDs 为空时返回空 map。
func (s *SessionStore) ContextTokensBySession(ctx context.Context, userID uint, sessionIDs []uint) (map[uint]int, error) {
	if len(sessionIDs) == 0 {
		return map[uint]int{}, nil
	}
	var rows []struct {
		SessionID    uint
		PromptTokens int
	}
	err := s.db.WithContext(ctx).Raw(`
SELECT m.session_id, m.prompt_tokens
FROM agent_messages m
JOIN (
    SELECT session_id, MAX(seq) AS max_seq
    FROM agent_messages
    WHERE user_id = ? AND role = ? AND session_id IN ?
    GROUP BY session_id
) latest ON m.session_id = latest.session_id AND m.seq = latest.max_seq
WHERE m.user_id = ? AND m.role = ?
`, userID, ai.MessageRoleAssistant, sessionIDs, userID, ai.MessageRoleAssistant).Scan(&rows).Error
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint]int, len(rows))
	for _, row := range rows {
		out[row.SessionID] = row.PromptTokens
	}
	return out, nil
}
