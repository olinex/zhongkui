package ai

import (
	// 标准库
	"context"
	"database/sql"
	"encoding/json"
	"time"

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
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
	}
	if err := s.db.WithContext(ctx).Model(session).Update("status", status).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return session, nil
}

// DeleteSession 在同一事务内以真实（硬）删除移除会话及其从属数据：会话行不再软删除，连同其消息、
// 内容块、压缩检查点、长期记忆、挂起运行现场与驻留运行登记及事件一并整行删除。会话对各子表均为真 FK
// （ON DELETE CASCADE），但不依赖数据库是否强制外键（如 sqlite 默认关闭），仍显式清理：先删消息的
// 内容块与消息、压缩检查点，再删长期记忆（agent_memories）、挂起运行现场（agent_runs）与驻留运行登记
// 及事件（agent_live_runs / agent_run_events），最后经 Unscoped 硬删会话本身（消息/块/检查点/事件/
// 登记本就无 deleted_at 列为硬删；记忆与运行现场亦经 Unscoped 硬删）。
func (s *SessionStore) DeleteSession(ctx context.Context, id uint, namespaceName string, userID uint) error {
	scope := "session_id = ? AND namespace_name = ? AND user_id = ?"
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 块从属助手消息：先按消息子查询清掉该会话的块，不依赖数据库是否强制外键（message_id 真外键
		// ON DELETE CASCADE 在 Postgres 亦会随消息删除），避免不强制外键的库残留孤儿块。
		if err := tx.Exec(`DELETE FROM agent_message_blocks WHERE message_id IN (
			SELECT id FROM agent_messages WHERE session_id = ? AND namespace_name = ? AND user_id = ?)`,
			id, namespaceName, userID).Error; err != nil {
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

// GetMessage 按消息 ID 查询一条消息（限定属主用户），供运行层取回问卷锚点行上的载荷；不存在
// （含归属不符）时返回 ErrMessageNotFound。
func (s *SessionStore) GetMessage(ctx context.Context, id, userID uint) (*ai.AgentMessage, error) {
	if id == 0 {
		return nil, errors.New("session store: unidentified message")
	}
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &msg, nil
}

// UpdateAssistantMessage 原地更新一条已落库的助手消息（按消息 ID 定位，并限定属主用户），供运行
// 途中把阶段性落库的那一行继续累积正文，以及运行终结时写入生命周期终态阶段（phase：done / failed /
// cancelled / suspended）。不重分配 seq，故消息在会话内的顺序与只读游标不变。必须用「结构体 + Select」
// 写这几列（正文 / 草稿 / 引用为密文列，走 crypto 序列化器；map 形式会绕过序列化器把明文直写库，见
// UpdateSummaryAndCheckpoint）。思考 / 问卷 / 授权清单不再有对应列，一律由 agent_message_blocks 的
// reasoning / questionnaire / approval 块承载。msg 为空或 ID 为 0 时返回英文错误；目标消息不存在
// （含归属不符）时返回 ErrMessageNotFound。
func (s *SessionStore) UpdateAssistantMessage(ctx context.Context, msg *ai.AgentMessage) error {
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
		Where("id = ? AND user_id = ?", msg.ID, msg.UserID).
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

// DeleteMessage 硬删除一条消息行（按消息 ID 定位，并限定属主用户），供驻留运行在「本次流以挂起
// 结束」时回收其阶段性落库的那一行：挂起不是终局，续跑会把本回合已外发的正文并回最终 done，留着
// 这一行会在续跑结束后形成同一回合的两条助手消息。消息行本无 deleted_at 列（见 DeleteSession），
// 故这里即硬删；目标行不存在（含归属不符）时返回 ErrMessageNotFound。
func (s *SessionStore) DeleteMessage(ctx context.Context, id, userID uint) error {
	if id == 0 {
		return errors.New("session store: unidentified message")
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&ai.AgentMessage{})
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// TruncateMessagesFromUserMessage 截断会话消息：删除会话内第 userMessageIndex 条（从 1 起，仅按
// 用户消息计数、按 seq 升序定位）用户消息，以及该条之后的全部消息（含助手回复）与其内容块（显式删除，
// 不残留孤儿块），供「回退 / 删除到某一轮对话」使用。目标用户消息不存在（序号越界）时返回
// ErrMessageNotFound。
// 锚点落在被截断区间内的运行现场（agent_runs）一并置为已消费：其锚点消息已删除，现场无从续跑，
// 留着会让前端据 live-run 续播、把已删除回合的回答重新拼回消息区。
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
		// 作废（置为已消费）锚点落在被截断区间内的运行现场：这些现场的锚点消息即将随截断删除，
		// 现场已无从续跑，若仍留在登记里，前端刷新 / 重开抽屉探测在途运行时会据其续播，把已删除
		// 回合的回答重新拼回消息区（并闸住输入区）。须在删除消息之前取 id——消息为软删除，删除后
		// 再按 seq 取 id 会被 deleted_at 过滤掉。
		var anchorIDs []uint
		if err := tx.Model(&ai.AgentMessage{}).
			Where("session_id = ? AND user_id = ? AND seq >= ?", sessionID, userID, target.Seq).
			Pluck("id", &anchorIDs).Error; err != nil {
			return errors.WithStack(err)
		}
		if len(anchorIDs) > 0 {
			if err := tx.Model(&ai.AgentRun{}).
				Where("session_id = ? AND user_id = ? AND anchor_message_id IN ?", sessionID, userID, anchorIDs).
				Update("status", ai.AgentRunStatusConsumed).Error; err != nil {
				return errors.WithStack(err)
			}
			// 块从属助手消息：显式清掉将被删除消息的内容块（消息删除用 GORM，不保证触发 DB 级联；即便
			// message_id 真外键 ON DELETE CASCADE 在 Postgres 会跟随删除，也一并显式处理，使回滚后不残留
			// 孤儿块——不强制外键的库（如 sqlite 默认）尤其依赖这一步）。
			if derr := tx.Where("message_id IN ?", anchorIDs).Delete(&ai.AgentMessageBlock{}).Error; derr != nil {
				return errors.WithStack(derr)
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

// AssistantInputTurn 是历史「助手待输入回合」的回退结果：回合种类、问卷各轮载荷（仅问卷回合）与
// 该回合待执行调用清单（供重答 / 重新授权后经既有 tool-result 原样回传续跑）。
type AssistantInputTurn struct {
	// Kind 回合种类：answer（问卷作答）或 approval（工具授权）
	Kind ai.AgentInteractionSubstate
	// Questionnaires 问卷回合按轮次先后排列的各轮载荷（题目 + 上次答案），前端据此预填各问答框；
	// 授权回合为空
	Questionnaires []QuestionnairePayload
	// Calls 该回合的待执行调用清单（callID 须与挂起时一致）
	Calls []PendingCall
}

// toPendingCalls 把挂起现场中的待执行调用转为回传列表：审核重要级与执行归属原样透传；
// 审批合成结论未随现场持久化，问卷类调用由前端按工具名识别、豁免审批，其余调用保守置为
// 「需人工审批」，避免回退后未经用户决策即自动执行。
func toPendingCalls(pending []pendingToolCall) []PendingCall {
	calls := make([]PendingCall, 0, len(pending))
	for _, call := range pending {
		calls = append(calls, PendingCall{
			CallID:           call.CallID,
			ToolName:         call.ToolName,
			Arguments:        call.Arguments,
			AuditLevel:       call.AuditLevel,
			ExecOwner:        call.ExecOwner,
			ApprovalRequired: !isQuestionnaireTool(call.ToolName),
		})
	}
	return calls
}

// RerunAssistantInput 回退到一条历史「助手待输入回合」，供用户在锚点消息上重新作答问卷或重新授权工具
// 调用：定位该会话中承载该回合的助手锚点行，删除其后的全部消息（该回合此前的最终回答及其后各轮），把
// 锚点行的本轮产出（正文、草稿、引用与用量）一并清空并重置为 suspended（待输入），把锚点消息上承载本
// 回合输入的块之后的块一并清除、并把该块重置为待输入，再把承载该回合的运行现场重置为 pending、刷新其
// 存活期限——使用户经既有 tool-result 续跑时仍沿用同一现场与同一锚点行；该锚点之后的更晚现场一律置为
// 已消费（其对应回合已被截断，现场不再可用）。
//
// 回合种类由现场中的待执行调用判定：任一为问卷类 ask_user → answer（出参据锚点 questionnaire 块返回各
// 轮题目 + 上次答案，供前端预填重开问卷）；其余 → approval（出参据锚点 approval 块返回待审批调用清单，
// 供前端重建审批面板）。据此把能力从既有的「问卷重新回答」泛化为同时覆盖工具授权回合。
//
// 锚点消息不存在、不属于该会话与属主、不是助手消息，或该回合不是待输入回合（现场无待执行调用；问卷回合
// 锚点未携带 questionnaire 块、授权回合锚点未携带 approval 块）时返回 ErrMessageNotFound；承载该回合的
// 运行现场不存在时返回 ErrAgentRunNotFound；该现场已过期（超过 agentRunTTL）时返回 ErrAgentRunExpired
// （与恢复链路的「已过期」语义一致，前端据此给出本地化提示）。单事务执行，避免截断与现场重置中途留下
// 不一致状态。
func (s *SessionStore) RerunAssistantInput(ctx context.Context, sessionID, userID uint, messageID uint) (*AssistantInputTurn, error) {
	if messageID == 0 {
		return nil, errors.New("session store: unidentified message")
	}
	turn := &AssistantInputTurn{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var anchor ai.AgentMessage
		err := tx.Where("id = ? AND session_id = ? AND user_id = ? AND role = ?",
			messageID, sessionID, userID, ai.MessageRoleAssistant).First(&anchor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMessageNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		var run ai.AgentRun
		err = tx.Where("session_id = ? AND user_id = ? AND anchor_message_id = ?", sessionID, userID, messageID).
			Order("id DESC").First(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAgentRunNotFound
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if time.Now().After(run.ExpiresAt) {
			// 与 ClaimAgentRun 一致：过期现场尽力置为 expired，失败仅忽略（不影响「已过期」这一结论）。
			_ = tx.Model(&ai.AgentRun{}).Where("id = ?", run.ID).Update("status", ai.AgentRunStatusExpired).Error
			return ErrAgentRunExpired
		}
		// 取回现场中的待执行调用（nil 视为空）：空列表即该回合不是待输入回合，无挂起的用户输入可重来。
		var pending []pendingToolCall
		if run.PendingCallsJSON != nil {
			if derr := json.Unmarshal(run.PendingCallsJSON.Value(), &pending); derr != nil {
				return errors.WithStack(derr)
			}
		}
		if len(pending) == 0 {
			return ErrMessageNotFound
		}
		// 回合种类由现场调用判定：任一问卷类 ask_user → answer，否则 → approval。
		turn.Kind = ai.AgentInteractionSubstateApproval
		for _, call := range pending {
			if isQuestionnaireTool(call.ToolName) {
				turn.Kind = ai.AgentInteractionSubstateAnswer
				break
			}
		}
		// 载荷来源一律改为锚点消息上的内容块（旧 questionnaire_json 列已删除）：问卷回合读 questionnaire 块
		// （各轮题目 + 上次答案），授权回合读 approval 块（待审批调用清单）。按 block_seq 升序即块的真实产生顺序。
		var blocks []*ai.AgentMessageBlock
		if berr := tx.Where("message_id = ? AND namespace_name = ? AND user_id = ?", anchor.ID, anchor.NamespaceName, userID).
			Order("block_seq ASC").Find(&blocks).Error; berr != nil {
			return errors.WithStack(berr)
		}
		// resetSeq 为承载本回合输入的锚点块序号（问卷 → 末轮 questionnaire 块；授权 → approval 块）：其之前的
		// 块是该输入产生前的历史（思考 / 工具调用 / 更早各轮），保留不动；其之后的块是上次作答后续跑产生的
		// 产物，随本次回退一并清除，使重开后的续跑从该块之后重新产生。
		var resetSeq uint64
		if turn.Kind == ai.AgentInteractionSubstateAnswer {
			var anchorBlock *ai.AgentMessageBlock
			var last QuestionnaireBlockPayload
			for _, block := range blocks {
				if block.Kind != ai.AgentMessageBlockKindQuestionnaire || block.Payload == nil {
					continue
				}
				var payload QuestionnaireBlockPayload
				if derr := decodeBlockPayload(block.Payload.Value(), &payload); derr != nil || len(payload.Questions) == 0 {
					continue
				}
				turn.Questionnaires = append(turn.Questionnaires,
					QuestionnairePayload{Questions: payload.Questions, Answers: payload.Answers})
				anchorBlock, last = block, payload
			}
			if anchorBlock == nil {
				return ErrMessageNotFound
			}
			resetSeq = anchorBlock.BlockSeq
			// 把末轮 questionnaire 块重置为「待回答」：置 Awaiting=true，并保留上次答案——出参据该块返回题目与
			// 上次答案供前端预填，spec 要求重开面板以题目与上次答案预填，故答案不清空。
			last.Awaiting = true
			if uerr := updateBlockPayloadTx(tx, anchor.NamespaceName, userID, anchor.ID, resetSeq, &last); uerr != nil {
				return uerr
			}
			// 问卷类 ask_user 调用的 callID 不在问卷块内，须由挂起现场原样回传续跑，故出参调用取自现场。
			turn.Calls = toPendingCalls(pending)
		} else {
			var anchorBlock *ai.AgentMessageBlock
			var last ApprovalBlockPayload
			for _, block := range blocks {
				if block.Kind != ai.AgentMessageBlockKindApproval || block.Payload == nil {
					continue
				}
				var payload ApprovalBlockPayload
				if derr := decodeBlockPayload(block.Payload.Value(), &payload); derr != nil {
					continue
				}
				anchorBlock, last = block, payload
			}
			if anchorBlock == nil {
				return ErrMessageNotFound
			}
			resetSeq = anchorBlock.BlockSeq
			// 把 approval 块重置为「待输入」（清空用户决策与执行结果 / 错误），并据块装配待审批清单（callID /
			// 工具名 / 入参 / 审核重要级 / 执行归属）供前端重建审批面板——现场口径仅用于判定回合种类。
			for _, record := range last.Calls {
				record.Decision = nil
				record.Result = ""
				record.Error = ""
				turn.Calls = append(turn.Calls, record.PendingCall)
			}
			if uerr := updateBlockPayloadTx(tx, anchor.NamespaceName, userID, anchor.ID, resetSeq, &last); uerr != nil {
				return uerr
			}
		}
		// 清除锚点块之后新增的块：上次作答后续跑产生的思考 / 工具调用 / 正文等随回退一并删除（其之前的块为该
		// 输入产生前的历史，保留不动）。
		if derr := tx.Where("message_id = ? AND block_seq > ?", anchor.ID, resetSeq).
			Delete(&ai.AgentMessageBlock{}).Error; derr != nil {
			return errors.WithStack(derr)
		}
		// 截断锚点之后的消息：该回合此前的最终回答及其后各轮（更早轮次保持不变）。
		if derr := tx.Where("session_id = ? AND user_id = ? AND seq > ?", sessionID, userID, anchor.Seq).
			Delete(&ai.AgentMessage{}).Error; derr != nil {
			return errors.WithStack(derr)
		}
		// 锚点行把「本轮增量产物」整体清零（正文、草稿、引用与用量）并重置为待输入；问卷题目 / 作答由
		// questionnaire 块承载、保持不动，供前端预填重开。此前各轮的思考过程已随阶段性落库落为 reasoning
		// 块，清正文不再影响它。列集合与 UpdateAssistantMessage 的本轮产物一致。
		// 走「结构体 + Select」以触发 crypto 序列化器（map 会绕过序列化器把明文直写库）。
		blank := &ai.AgentMessage{
			Content:          serializer.NewCrypto([]byte("")),
			DraftJSON:        serializer.NewCrypto([]byte("")),
			CitationsJSON:    serializer.NewCrypto([]byte("")),
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
			ElapsedMs:        0,
			Phase:            ai.AgentRunPhaseSuspended,
		}
		columns := []string{"content", "draft_json", "citations_json",
			"prompt_tokens", "completion_tokens", "total_tokens", "elapsed_ms", "phase"}
		if uerr := tx.Model(blank).Where("id = ? AND user_id = ?", anchor.ID, userID).
			Select(columns).Updates(blank).Error; uerr != nil {
			return errors.WithStack(uerr)
		}
		// 重置承载现场为 pending 并刷新存活期限；更晚现场（其回合已被截断）一律置为已消费。
		res := tx.Model(&ai.AgentRun{}).Where("id = ?", run.ID).
			Updates(map[string]any{"status": ai.AgentRunStatusPending, "expires_at": time.Now().Add(agentRunTTL)})
		if res.Error != nil {
			return errors.WithStack(res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrAgentRunNotFound
		}
		if lerr := tx.Model(&ai.AgentRun{}).
			Where("session_id = ? AND user_id = ? AND id > ?", sessionID, userID, run.ID).
			Update("status", ai.AgentRunStatusConsumed).Error; lerr != nil {
			return errors.WithStack(lerr)
		}
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return turn, nil
}

// PendingInputCalls 只读返回会话当前「仍待用户输入」的挂起现场（agent_runs 中该会话最近一条未过期的
// status=pending 行）的锚点消息 ID 与该现场的待执行调用清单：供消息列表接口在刷新 / 重进会话后为承载该
// 现场的助手消息（问卷待作答 / 工具待授权）重建可交互面板——否则前端据出参的 awaiting / 授权记录渲染
// 出面板，却没有可回传的调用，提交 / 取消会静默无反应、不发任何请求。可用现场的口径：状态为 pending 且
// 未过期的现场中取 id 最大（最新）的一行；无可用现场（不存在 / 已消费 / 已过期 / 调用为空）时锚点为 0、
// 调用为 nil，调用方据此不向任何消息附加。
//
// 转换口径与 RerunAssistantInput 一致（共用 toPendingCalls）：CallID / ToolName / Arguments /
// ExecOwner / AuditLevel 原样透传；审批合成结论不随现场持久化，问卷类调用由前端按工具名识别、豁免
// 审批，其余调用保守置为「需人工审批」，避免未经用户决策即自动执行。
func (s *SessionStore) PendingInputCalls(ctx context.Context, sessionID, userID uint) (uint, []PendingCall, error) {
	var run ai.AgentRun
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND status = ? AND expires_at > ?",
			sessionID, userID, ai.AgentRunStatusPending, time.Now()).
		Order("id DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, errors.WithStack(err)
	}
	if run.PendingCallsJSON == nil {
		return 0, nil, nil
	}
	var pending []pendingToolCall
	if derr := json.Unmarshal(run.PendingCallsJSON.Value(), &pending); derr != nil {
		return 0, nil, errors.WithStack(derr)
	}
	if len(pending) == 0 {
		return 0, nil, nil
	}
	return run.AnchorMessageID, toPendingCalls(pending), nil
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

// AssistantSeqAtMessage 返回指定助手消息（id）在所属会话内的 seq，供手动压缩把折叠上界定位到该助手
// 回答；该消息不存在、不属于该会话与属主，或不是助手消息时返回 ErrMessageNotFound。按角色过滤而非按
// 「第 N 轮」定位：折叠边界以选中的助手回答为准，与前端选中的那条消息一一对应。
func (s *SessionStore) AssistantSeqAtMessage(ctx context.Context, sessionID, userID uint, messageID uint) (uint64, error) {
	var message ai.AgentMessage
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND id = ? AND role = ?", sessionID, userID, messageID, ai.MessageRoleAssistant).
		First(&message).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrMessageNotFound
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return message.Seq, nil
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
		return nil, errors.WithStack(err)
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
// summary_upto_seq，并追加一条 agent_session_compactions 压缩检查点（记录推进后的游标、摘要全文与
// 展示度量，供回滚逐级回退 start bound，以及刷新后回显历史压缩记录）。reason 取 rolling（静默滚动）/
// auto（自动压缩）/ manual（手动压缩）；metrics 为该次推进的真实展示度量（三种原因都必须填真值，
// 取值与是否展示无关——是否回显交由读取侧按 reason 决定）。
// 摘要经 crypto 序列化器加密落库，故会话更新必须以「结构体 + Select」写这两列、不触碰其它字段
// （避免并发改写会话标题/档位等；map 形式会绕过序列化器把明文直写库）；检查点同样是密文列，走
// Create 结构体路径加密。session 必须带有 ID / UserID / NamespaceName（调用点均持有已加载的会话）。
func (s *SessionStore) UpdateSummaryAndCheckpoint(ctx context.Context, session *ai.AgentSession, summary string, uptoSeq uint, reason string, metrics CompactionMetrics) error {
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
func (s *SessionStore) ListCompactions(ctx context.Context, sessionID, userID uint) ([]*ai.AgentSessionCompaction, error) {
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

// ContextTokens 返回会话的上下文占用度量：最近一条「有用量记录」的助手消息的输入 token 数
// （PromptTokens > 0）。被取消 / 失败 / 挂起而中断落库的助手消息从未收到供应商的用量帧，用量列
// 为 0——它不构成一次测量，故跳过并回落到更早的末次测量（与前端占用环「取最近一条带用量的助手
// 消息」的口径一致）；若无任何可用测量（无助手消息，或全部助手消息都未记下用量）返回 0，不视为
// 错误。注意 0 的含义是「尚无测量」，不是「占用为 0」。
func (s *SessionStore) ContextTokens(ctx context.Context, sessionID, userID uint) (int, error) {
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).
		Select("prompt_tokens").
		Where("session_id = ? AND user_id = ? AND role = ? AND prompt_tokens > 0", sessionID, userID, ai.MessageRoleAssistant).
		Order("seq DESC").
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return msg.PromptTokens, nil
}

// contextUsageRow 是展示占用取舍的一侧取值：Tokens 为该侧给出的上下文占用，Happened 为该值的落库
// 时间（真实测量取助手消息创建时间，估算取压缩检查点创建时间）；零值表示该侧无可用值。
type contextUsageRow struct {
	// Tokens 该侧给出的上下文占用 token
	Tokens int
	// Happened 该值的落库时间，用于与另一侧取更晚者
	Happened time.Time
}

// ContextUsage 返回会话用于展示的上下文占用：取「末次真实测量」与「最近一次带估算的压缩推进」中
// 更晚发生者的 token 数——压缩本身不产生新的真实测量（见 ContextTokens），若压缩晚于末次测量
// （即压缩后尚无生成落定），展示占用取该次压缩的即时估算（检查点 tokens_after）；此后任一生成落定
// （其助手消息行比该压缩更晚落库）即由真实测量接管。与 ContextTokens 的分工：本方法服务展示
// （会话列表 / 详情 / 前端的占用指示），ContextTokens 服务压缩前的度量基线——压缩基线必须始终取
// 真实测量，绝不能用上一次的估算去度量下一次压缩，否则估算偏差会逐次累积。
// 会话既无测量也无估算时返回 0，含义同 ContextTokens：表示「尚无测量」，不是「占用为 0」。
func (s *SessionStore) ContextUsage(ctx context.Context, sessionID, userID uint) (int, error) {
	usage, err := s.ContextUsageBySession(ctx, userID, []uint{sessionID})
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return usage[sessionID], nil
}

// ContextUsageBySession 批量返回每个会话用于展示的上下文占用（口径同 ContextUsage）：分别取每个
// 会话末次真实测量的助手消息与最近一条带估算的压缩检查点，逐会话取更晚发生者。两侧各以一条分组
// 查询批量取回（避免逐会话查询的 N+1），再在 Go 侧按落库时间合并——取舍规则只在此一处实现，单会话
// 口径即本方法的单元素特例。无可用值的会话不出现在结果中（调用方缺省为 0），sessionIDs 为空时返回
// 空 map。
func (s *SessionStore) ContextUsageBySession(ctx context.Context, userID uint, sessionIDs []uint) (map[uint]int, error) {
	if len(sessionIDs) == 0 {
		return map[uint]int{}, nil
	}
	measured, err := s.latestMeasuredUsage(ctx, userID, sessionIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	estimated, err := s.latestEstimatedUsage(ctx, userID, sessionIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint]int, len(measured)+len(estimated))
	for id, row := range measured {
		out[id] = row.Tokens
		if est, ok := estimated[id]; ok && laterUsage(row, est) {
			out[id] = est.Tokens
		}
	}
	for id, row := range estimated {
		if _, ok := measured[id]; !ok {
			out[id] = row.Tokens
		}
	}
	return out, nil
}

// laterUsage 判断估算侧是否晚于测量侧发生（更晚者用于展示）：同一时刻落库时取真实测量侧——测量是
// 精确值、估算是近似值。
func laterUsage(measured, estimated contextUsageRow) bool {
	return estimated.Happened.After(measured.Happened)
}

// latestMeasuredUsage 批量取每个会话的末次真实测量：最近一条「有用量记录」的助手消息（prompt_tokens
// > 0，按 seq 取最新）的输入 token 数与其落库时间，口径同 ContextTokens；无可用测量的会话不出现在
// 结果中。
func (s *SessionStore) latestMeasuredUsage(ctx context.Context, userID uint, sessionIDs []uint) (map[uint]contextUsageRow, error) {
	var rows []struct {
		SessionID uint
		Tokens    int
		Happened  time.Time
	}
	err := s.db.WithContext(ctx).Raw(`
SELECT m.session_id, m.prompt_tokens AS tokens, m.created_at AS happened
FROM agent_messages m
JOIN (
    SELECT session_id, MAX(seq) AS max_seq
    FROM agent_messages
    WHERE user_id = ? AND role = ? AND prompt_tokens > 0 AND session_id IN ?
    GROUP BY session_id
) latest ON m.session_id = latest.session_id AND m.seq = latest.max_seq
WHERE m.user_id = ? AND m.role = ?
`, userID, ai.MessageRoleAssistant, sessionIDs, userID, ai.MessageRoleAssistant).Scan(&rows).Error
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint]contextUsageRow, len(rows))
	for _, row := range rows {
		out[row.SessionID] = contextUsageRow{Tokens: row.Tokens, Happened: row.Happened}
	}
	return out, nil
}

// latestEstimatedUsage 批量取每个会话最近一次压缩推进的即时估算：最近一条 tokens_after > 0 的压缩
// 检查点（按 id 取最新）的估算 token 数与其落库时间；无估算的会话（含只有静默滚动推进的会话）不
// 出现在结果中。
func (s *SessionStore) latestEstimatedUsage(ctx context.Context, userID uint, sessionIDs []uint) (map[uint]contextUsageRow, error) {
	var rows []struct {
		SessionID uint
		Tokens    int
		Happened  time.Time
	}
	err := s.db.WithContext(ctx).Raw(`
SELECT c.session_id, c.tokens_after AS tokens, c.created_at AS happened
FROM agent_session_compactions c
JOIN (
    SELECT session_id, MAX(id) AS max_id
    FROM agent_session_compactions
    WHERE user_id = ? AND tokens_after > 0 AND session_id IN ?
    GROUP BY session_id
) latest ON c.session_id = latest.session_id AND c.id = latest.max_id
WHERE c.user_id = ?
`, userID, sessionIDs, userID).Scan(&rows).Error
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint]contextUsageRow, len(rows))
	for _, row := range rows {
		out[row.SessionID] = contextUsageRow{Tokens: row.Tokens, Happened: row.Happened}
	}
	return out, nil
}

// CountUserTurns 返回会话内当前存在的用户提问条数（有效轮次）：只计 role=user 且归属匹配的现存消息行。
// agent_messages 无 deleted_at 列（见 DeleteSession），回滚是物理删除，故按现存行计数即天然排除已回滚
// 的提问——「发送前判定」与「每轮终局判定」共用同一口径，不会出现「已满仍可发」或「发完不锁」的偏差。
//
// 会话锁定不需要写任何状态列：它是 StateOf 的全函数推导结果（UserTurns >= MaxUserTurns 且无在途活动 /
// 挂起现场时即为 locked），因此本方法只如实读数，调用方（视图层事实装配）据此推导，回滚删除若干条提问
// 后有效轮次自然下降，状态即自然回到 paused——「派生即解锁」，不存在额外的解锁写路径。
func (s *SessionStore) CountUserTurns(ctx context.Context, sessionID, userID uint) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&ai.AgentMessage{}).
		Where("session_id = ? AND user_id = ? AND role = ?", sessionID, userID, ai.MessageRoleUser).
		Count(&count).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	return int(count), nil
}

// PendingRunSubstate 返回会话当前挂起现场（agent_runs 中该会话最近一条未过期的 status=pending 行）
// 对应的「等待中」子状态：现场中存在问卷类 ask_user 调用时为 answer，其余（工具授权等）为 approval。
// 无可用现场（不存在 / 已消费 / 已过期）时返回空串，表示「无挂起现场」，由调用方据此判定是否落入
// 等待中。工具种类判定复用 isQuestionnaireTool，ask_user 字面量仍只在 client_tools.go 一处声明。
func (s *SessionStore) PendingRunSubstate(ctx context.Context, sessionID, userID uint) (ai.AgentInteractionSubstate, error) {
	var run ai.AgentRun
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND status = ? AND expires_at > ?",
			sessionID, userID, ai.AgentRunStatusPending, time.Now()).
		Order("id DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", errors.WithStack(err)
	}
	var pending []pendingToolCall
	if run.PendingCallsJSON != nil {
		if derr := json.Unmarshal(run.PendingCallsJSON.Value(), &pending); derr != nil {
			return "", errors.WithStack(derr)
		}
	}
	for _, call := range pending {
		if isQuestionnaireTool(call.ToolName) {
			return ai.AgentInteractionSubstateAnswer, nil
		}
	}
	return ai.AgentInteractionSubstateApproval, nil
}

// AppendBlock 新建一个内容块（agent_message_blocks）并按传入的 BlockSeq 落库：调用方已按块构建器的
// 分配结果填好 MessageID / BlockSeq / Kind，并在 Payload 上以 crypto 序列化器写入加密载荷。
// message_id 为真外键（ON DELETE CASCADE），块随所属助手消息级联删除，故不残留孤儿块。
// block 为空或 MessageID / BlockSeq 为 0 时返回英文错误。
func (s *SessionStore) AppendBlock(ctx context.Context, block *ai.AgentMessageBlock) error {
	if block == nil || block.MessageID == 0 || block.BlockSeq == 0 {
		return errors.New("session store: nil or unidentified block")
	}
	if err := s.db.WithContext(ctx).Create(block).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// UpdateBlock 更新同一内容块（开放块增量累积后的落库与收敛时的终态写回）：按 message_id + block_seq
// 定位，并限定租户归属（namespace_name + user_id）。只更新载荷列——BlockSeq / Kind 由创建时定死、
// 不可变。Payload 是密文列，必须以「结构体 + Select」写（map 形式会绕过 crypto 序列化器把明文直写库）。
// block 为空或 MessageID / BlockSeq 为 0 时返回英文错误；目标块不存在（含归属不符）时返回
// ErrMessageNotFound。
func (s *SessionStore) UpdateBlock(ctx context.Context, block *ai.AgentMessageBlock) error {
	if block == nil || block.MessageID == 0 || block.BlockSeq == 0 {
		return errors.New("session store: nil or unidentified block")
	}
	update := &ai.AgentMessageBlock{Payload: block.Payload}
	res := s.db.WithContext(ctx).Model(update).
		Where("message_id = ? AND block_seq = ? AND namespace_name = ? AND user_id = ?",
			block.MessageID, block.BlockSeq, block.NamespaceName, block.UserID).
		Select("payload").
		Updates(update)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	return nil
}

// updateBlockPayloadTx 在事务内按 message_id + block_seq 更新某内容块的载荷（限定租户归属），只写密文
// 载荷列：载荷经 crypto 序列化器加密后以「结构体 + Select」写（map 形式会绕过序列化器把明文直写库）。
// 供回退重开把锚点末轮 questionnaire / approval 块就地重置为待输入时复用，避免各处重复拼 Update 语句。
func updateBlockPayloadTx(tx *gorm.DB, namespaceName string, userID, messageID uint, blockSeq uint64, payload any) error {
	data, err := encodeBlockPayload(payload)
	if err != nil {
		return errors.WithStack(err)
	}
	update := &ai.AgentMessageBlock{Payload: serializer.NewCrypto(data)}
	return errors.WithStack(tx.Model(update).
		Where("message_id = ? AND block_seq = ? AND namespace_name = ? AND user_id = ?", messageID, blockSeq, namespaceName, userID).
		Select("payload").
		Updates(update).Error)
}

// ListBlocksByMessage 按 block_seq 升序返回某条助手消息的全部内容块，并限定租户归属
// （namespace_name + user_id）；无块时返回空切片。
func (s *SessionStore) ListBlocksByMessage(ctx context.Context, messageID uint, namespaceName string, userID uint) ([]*ai.AgentMessageBlock, error) {
	var blocks []*ai.AgentMessageBlock
	if err := s.db.WithContext(ctx).
		Where("message_id = ? AND namespace_name = ? AND user_id = ?", messageID, namespaceName, userID).
		Order("block_seq ASC").
		Find(&blocks).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return blocks, nil
}

// ListBlocksByMessages 批量返回多条助手消息的内容块（按 message_id、block_seq 升序），并限定租户归属
// （namespace_name + user_id），以单条 IN 查询避免逐消息查询的 N+1。返回 message_id → 该消息块列表
// 的映射；messageIDs 为空时返回空 map。无块的消息不出现在结果中。
func (s *SessionStore) ListBlocksByMessages(ctx context.Context, messageIDs []uint, namespaceName string, userID uint) (map[uint][]*ai.AgentMessageBlock, error) {
	if len(messageIDs) == 0 {
		return map[uint][]*ai.AgentMessageBlock{}, nil
	}
	var blocks []*ai.AgentMessageBlock
	if err := s.db.WithContext(ctx).
		Where("message_id IN ? AND namespace_name = ? AND user_id = ?", messageIDs, namespaceName, userID).
		Order("message_id ASC, block_seq ASC").
		Find(&blocks).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint][]*ai.AgentMessageBlock, len(messageIDs))
	for _, block := range blocks {
		out[block.MessageID] = append(out[block.MessageID], block)
	}
	return out, nil
}

// loadQuestionnaireRoundsFromBlocks 读取某助手消息上的 questionnaire 块并按 block_seq 先后还原为「按轮次」
// 的问卷载荷列表（各轮的题目 + 作答）。问卷题目与作答由 questionnaire 块承载（旧 questionnaire_json 列已
// 删除），故续跑（PrepareResume）与回退重答（RerunAssistantInput）一律据此取回，共用同一口径。无问卷块或
// 块载荷无法解码时返回 nil——调用方据此按「无问卷载荷」处理。可在事务内调用（db 传 tx 即可）。
func loadQuestionnaireRoundsFromBlocks(ctx context.Context, db *gorm.DB, messageID uint, namespaceName string, userID uint) []QuestionnairePayload {
	if messageID == 0 {
		return nil
	}
	var blocks []*ai.AgentMessageBlock
	if err := db.WithContext(ctx).
		Where("message_id = ? AND namespace_name = ? AND user_id = ?", messageID, namespaceName, userID).
		Order("block_seq ASC").
		Find(&blocks).Error; err != nil {
		return nil
	}
	rounds := make([]QuestionnairePayload, 0, len(blocks))
	for _, block := range blocks {
		if block.Kind != ai.AgentMessageBlockKindQuestionnaire || block.Payload == nil {
			continue
		}
		var payload QuestionnaireBlockPayload
		if derr := decodeBlockPayload(block.Payload.Value(), &payload); derr != nil {
			continue
		}
		if len(payload.Questions) == 0 {
			continue
		}
		rounds = append(rounds, QuestionnairePayload{Questions: payload.Questions, Answers: payload.Answers})
	}
	if len(rounds) == 0 {
		return nil
	}
	return rounds
}
