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
)

// TruncateMessagesFromUserMessage 截断会话消息：删除会话内第 userMessageIndex 条（从 1 起，仅按
// 用户消息计数、按 seq 升序定位）用户消息，以及该条之后的全部消息（含助手回复），供
// 「回退 / 删除到某一轮对话」使用。目标用户消息不存在（序号越界）时返回 ErrMessageNotFound。
// 锚点落在被截断区间内的运行现场（agent_runs）一并置为已消费：其锚点消息已删除，现场无从续跑，
// 留着会让前端把已删除回合当作挂起现场。锚点落在被截断区间内的持久化交互（agent_interactions）
// 连同其 redo checkpoint 一并删除：交互卡片依附于助手消息，消息已删而卡片仍留在登记里时，历史回放
// 会把它挂到残留轮次或合成一条空回答，表现为已回退的「推荐的下一步」等卡片刷新后仍在。
// 单事务执行，避免截断中途留下不一致的会话内容。截断落在已压缩区间内（summary_upto_seq ≥ 被截断
// 起始 seq）时，按压缩检查点回退上下文起点：恢复「upto_seq < 起始 seq」的最近一个检查点的摘要与
// 游标，无可用检查点则清空摘要、summary_upto_seq 置 0（回到最开始）；并作废（删除）upto_seq ≥ 起始
// seq 的检查点。截断点尚未被摘要覆盖（summary_upto_seq < 起始 seq）时摘要与游标保持不变。回滚不删除
// 压缩期间抽取的长期记忆（agent_memories），也不做级联删除。后续追加仍按会话 message_seq 单调分配，
// 序号不复用。
func (s *Store) TruncateMessagesFromUserMessage(ctx context.Context, sessionID, userID uint, userMessageIndex int) error {
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
		// 作废（置为已消费）该会话的全部 pending 挂起现场：截断意味着当前等待回合被整体放弃，现场已无从
		// 续跑；若仍留在登记里，前端刷新 / 重开抽屉探测在途运行时会据其续播，把已删除回合的回答重新拼回
		// 消息区（并闸住输入区）。等待中现在允许直接回滚。
		if err := tx.Model(&ai.AgentRun{}).
			Where("session_id = ? AND user_id = ? AND status = ?", sessionID, userID, ai.AgentRunStatusPending).
			Update("status", ai.AgentRunStatusConsumed).Error; err != nil {
			return errors.WithStack(err)
		}
		// 作废锚点落在被截断区间内的交互（审批 / 问卷 / 下一步）：先按待删消息取回其 ID 集合，再按锚点
		// 定位交互。交互回退用的 checkpoint 快照由交互 ID 派生（见 RedoCheckpointID），随交互一并清理。
		var messageIDs []uint
		if err := tx.Model(&ai.AgentMessage{}).
			Where("session_id = ? AND user_id = ? AND seq >= ?", sessionID, userID, target.Seq).
			Pluck("id", &messageIDs).Error; err != nil {
			return errors.WithStack(err)
		}
		if len(messageIDs) > 0 {
			var interactionIDs []uint
			if err := tx.Model(&ai.AgentInteraction{}).
				Where("session_id = ? AND user_id = ? AND message_id IN ?", sessionID, userID, messageIDs).
				Pluck("id", &interactionIDs).Error; err != nil {
				return errors.WithStack(err)
			}
			if len(interactionIDs) > 0 {
				keys := make([]string, 0, len(interactionIDs))
				for _, interactionID := range interactionIDs {
					keys = append(keys, RedoCheckpointID(sessionID, interactionID))
				}
				if err := tx.Where("checkpoint_id IN ?", keys).Delete(&ai.AgentCheckpoint{}).Error; err != nil {
					return errors.WithStack(err)
				}
				if err := tx.Where("id IN ?", interactionIDs).Delete(&ai.AgentInteraction{}).Error; err != nil {
					return errors.WithStack(err)
				}
			}
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
			return errors.WithStack(cpErr)
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

// BranchSessionFromUserMessage 以第 userMessageIndex 条（从 1 起，仅按用户消息计数、按 seq 升序
// 定位）用户消息所在轮次为结尾，把该会话从最早一条消息起、直到「下一条用户消息之前」的全部消息
// 复制为一个新会话，供前端「分支」使用（新会话可独立演进，源会话不受影响）。目标用户消息不存在
// （序号越界）时返回 ErrMessageNotFound；源会话不属于给定属主用户时返回 ErrSessionNotFound。
// 单事务执行，避免复制中途留下半截会话。新会话归属同一命名空间与属主，标题为空时沿用源会话标题，
// 档位 / Agent 类型 / 审核阀门与源会话一致，状态 active；滚动摘要不复制（消息 seq 会重新编号，
// 原摘要覆盖范围失去意义）。复制出的消息在新会话内 seq 从 1 连续重新编号（新 session_id 不同，
// 不受 UNIQUE(session_id,user_id,seq) 影响），保留 role/message_kind/payload/content/draft/citations/
// created_at 等载荷，不复制 id 与源 seq。新会话的 message_seq 直接置为复制条数（在插入时一次算好，
// 供后续追加消息从 count+1 继续分配 seq）。
func (s *Store) BranchSessionFromUserMessage(ctx context.Context, sessionID, userID uint, userMessageIndex int, title string) (*ai.AgentSession, error) {
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
		//           message_kind/payload 与 content/draft_json/citations_json 均为原样搬运，原生消息
		//           载荷与正文一并保留（密文列直接搬字节，无需解密再加密）。
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
        (created_at, updated_at, session_id, namespace_name, user_id, seq, role, message_kind, payload, content, draft_json, citations_json, prompt_tokens, completion_tokens, total_tokens, elapsed_ms)
    SELECT
        m.created_at, NOW(), ns.id, m.namespace_name, m.user_id, ROW_NUMBER() OVER (ORDER BY m.seq),
        m.role, m.message_kind, m.payload, m.content, m.draft_json, m.citations_json,
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
		return nil, errors.WithStack(err)
	}
	return branched, nil
}
