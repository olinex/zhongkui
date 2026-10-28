package session

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// SaveAgentRun 创建一行运行现场（状态为 pending），承载挂起时刻的运行现场。
func (s *Store) SaveAgentRun(ctx context.Context, run *modelsai.AgentRun) error {
	if run == nil {
		return errors.New("session store: nil agent run")
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ClaimAgentRun 认领一次恢复请求对应的运行现场：以（会话 ID、命名空间、属主用户）为查询条件，优先
// 取该会话最新的 pending 现场，无 pending 时再取最新一行用于区分「不存在 / 已消费」——回退（reanswer）
// 会把较早的问卷现场重置为 pending 并把更晚的现场置为已消费，故必须按状态优先而非单纯按 id 取最新，
// 否则会认领到已消费的现场。归属不符的行在查询阶段即被过滤，表现为「不存在」；随后校验状态，通过后
// 原子置为 consumed（并发双提交只有一方成功）并返回该现场。挂起现场不设过期：无大模型调用在跑即无
// 生命周期需要管理，只要会话未删除，任何时间都可恢复。失败类别见包级哨兵
// ErrAgentRunNotFound / ErrAgentRunConsumed，供上层映射业务码。
func (s *Store) ClaimAgentRun(ctx context.Context, sessionID uint, namespaceName string, userID uint) (*modelsai.AgentRun, error) {
	scope := "session_id = ? AND namespace_name = ? AND user_id = ?"
	var run modelsai.AgentRun
	err := s.db.WithContext(ctx).
		Where(scope+" AND status = ?", sessionID, namespaceName, userID, modelsai.AgentRunStatusPending).
		Order("id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 无 pending 现场：取最新一行只为区分「已消费 / 不存在」，不参与认领。
		err = s.db.WithContext(ctx).
			Where(scope, sessionID, namespaceName, userID).
			Order("id DESC").
			First(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if run.Status != modelsai.AgentRunStatusPending {
		return nil, ErrAgentRunConsumed
	}
	res := s.db.WithContext(ctx).Model(&modelsai.AgentRun{}).
		Where("id = ? AND status = ?", run.ID, modelsai.AgentRunStatusPending).
		Update("status", modelsai.AgentRunStatusConsumed)
	if res.Error != nil {
		return nil, errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrAgentRunConsumed
	}
	return &run, nil
}

// prepareClientToolCall 处理一次 client 来源工具调用：仅当本次运行有持久化会话且具备审批通道时，
// 原样登记待执行调用（工具名与入参文本，后端不做解释），待本轮结束统一挂起；否则返回回灌给模型的
// 英文错误文本，此时不产生任何 ToolRequest。api_invoke 是唯一例外：其目标接口必须由后端按能力
// SetAgentRunAnchor 把挂起锚点消息 ID 回写到运行现场：挂起时 wrapPersist 写好承载本回合最终回答（问卷
// 挂起还承载问卷载荷）的助手消息行后调用，使续跑能原地终态化该行、回退接口能据锚点截断其后消息。现场
// 不存在（含归属不符）时返回 ErrAgentRunNotFound。
func (s *Store) SetAgentRunAnchor(ctx context.Context, runID, anchorMessageID uint) error {
	if runID == 0 {
		return errors.New("session store: unidentified agent run")
	}
	res := s.db.WithContext(ctx).Model(&modelsai.AgentRun{}).
		Where("id = ?", runID).
		Update("anchor_message_id", anchorMessageID)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAgentRunNotFound
	}
	return nil
}

// LatestRunByAnchor 返回某会话内锚定到指定助手消息的最新运行现场（含已消费），供「重新审批」回退时
// 沿用原回合使用的档位；现场不存在（含归属不符）时返回 ErrAgentRunNotFound。
func (s *Store) LatestRunByAnchor(ctx context.Context, sessionID, userID, anchorMessageID uint) (*modelsai.AgentRun, error) {
	if anchorMessageID == 0 {
		return nil, ErrAgentRunNotFound
	}
	var run modelsai.AgentRun
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ? AND anchor_message_id = ?", sessionID, userID, anchorMessageID).
		Order("id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentRunNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &run, nil
}

// AbortPendingRun 中止一个「等待中」会话的挂起现场（用户点击停止）：作废该会话最近一条 pending 现场
// （置为 consumed），并把其锚点助手消息的终局阶段置为 cancelled，使状态推导回落 `暂停/用户中止`。
// 无 pending 现场（已消费 / 不存在）时幂等返回 nil——重复中止不报错。单事务执行，避免现场与
// 锚点消息中途不一致。不删除任何消息、长期记忆与压缩检查点。
func (s *Store) AbortPendingRun(ctx context.Context, sessionID, userID uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run modelsai.AgentRun
		err := tx.Where("session_id = ? AND user_id = ? AND status = ?", sessionID, userID, modelsai.AgentRunStatusPending).
			Order("id DESC").First(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Model(&modelsai.AgentRun{}).Where("id = ?", run.ID).
			Update("status", modelsai.AgentRunStatusConsumed).Error; err != nil {
			return errors.WithStack(err)
		}
		if run.AnchorMessageID != 0 {
			if err := tx.Model(&modelsai.AgentMessage{}).
				Where("id = ? AND user_id = ?", run.AnchorMessageID, userID).
				Update("phase", modelsai.AgentRunPhaseCancelled).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}
