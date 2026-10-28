package session

import (
	// 标准库
	"context"
	"time"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ContextTokens 返回会话的上下文占用度量：最近一条「有用量记录」的助手消息的输入 token 数
// （PromptTokens > 0）。被取消 / 失败 / 挂起而中断落库的助手消息从未收到供应商的用量帧，用量列
// 为 0——它不构成一次测量，故跳过并回落到更早的末次测量（与前端占用环「取最近一条带用量的助手
// 消息」的口径一致）；若无任何可用测量（无助手消息，或全部助手消息都未记下用量）返回 0，不视为
// 错误。注意 0 的含义是「尚无测量」，不是「占用为 0」。
func (s *Store) ContextTokens(ctx context.Context, sessionID uint, namespaceName string, userID uint) (int, error) {
	var msg ai.AgentMessage
	err := s.db.WithContext(ctx).
		Select("prompt_tokens").
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND role = ? AND prompt_tokens > 0", sessionID, namespaceName, userID, ai.MessageRoleAssistant).
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
func (s *Store) ContextUsage(ctx context.Context, sessionID uint, namespaceName string, userID uint) (int, error) {
	usage, err := s.ContextUsageBySession(ctx, namespaceName, userID, []uint{sessionID})
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
func (s *Store) ContextUsageBySession(ctx context.Context, namespaceName string, userID uint, sessionIDs []uint) (map[uint]int, error) {
	if len(sessionIDs) == 0 {
		return map[uint]int{}, nil
	}
	measured, err := s.latestMeasuredUsage(ctx, namespaceName, userID, sessionIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	estimated, err := s.latestEstimatedUsage(ctx, namespaceName, userID, sessionIDs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[uint]int, len(measured)+len(estimated))
	for id, row := range measured {
		out[id] = row.Tokens
		// 估算侧更晚发生时取估算值；同一时刻落库时保留真实测量侧（精确值优先于近似值），故用严格 After。
		if est, ok := estimated[id]; ok && est.Happened.After(row.Happened) {
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

// latestMeasuredUsage 批量取每个会话的末次真实测量：最近一条「有用量记录」的助手消息（prompt_tokens
// > 0，按 seq 取最新）的输入 token 数与其落库时间，口径同 ContextTokens；无可用测量的会话不出现在
// 结果中。
func (s *Store) latestMeasuredUsage(ctx context.Context, namespaceName string, userID uint, sessionIDs []uint) (map[uint]contextUsageRow, error) {
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
    WHERE namespace_name = ? AND user_id = ? AND role = ? AND prompt_tokens > 0 AND session_id IN ?
    GROUP BY session_id
) latest ON m.session_id = latest.session_id AND m.seq = latest.max_seq
WHERE m.namespace_name = ? AND m.user_id = ? AND m.role = ?
`, namespaceName, userID, ai.MessageRoleAssistant, sessionIDs, namespaceName, userID, ai.MessageRoleAssistant).Scan(&rows).Error
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
func (s *Store) latestEstimatedUsage(ctx context.Context, namespaceName string, userID uint, sessionIDs []uint) (map[uint]contextUsageRow, error) {
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
    WHERE namespace_name = ? AND user_id = ? AND tokens_after > 0 AND session_id IN ?
    GROUP BY session_id
) latest ON c.session_id = latest.session_id AND c.id = latest.max_id
WHERE c.namespace_name = ? AND c.user_id = ?
`, namespaceName, userID, sessionIDs, namespaceName, userID).Scan(&rows).Error
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
func (s *Store) CountUserTurns(ctx context.Context, sessionID uint, namespaceName string, userID uint) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&ai.AgentMessage{}).
		Where("session_id = ? AND namespace_name = ? AND user_id = ? AND role = ?", sessionID, namespaceName, userID, ai.MessageRoleUser).
		Count(&count).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	return int(count), nil
}
