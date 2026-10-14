// 本文件承载驻留运行（RunHub 管理的 liveRun）的持久化：运行登记（agent_live_runs）与事件日志
// （agent_run_events）。事件日志是断线续播（?after=）的回放来源，也是内存缓冲之外的落库兜底；
// 运行登记记录运行的生命周期阶段与已发布的最大事件序号，供刷新后发现在途运行。方法挂在
// SessionStore 上，与运行现场（agent_run.go）同模式，避免改动 Agent 的装配参数。

package ai

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AppendRunEvent 追加一条运行事件到事件日志（按 (run_id, seq) 唯一）；ev 为空时返回英文错误。
func (s *SessionStore) AppendRunEvent(ctx context.Context, ev *ai.AgentRunEvent) error {
	if ev == nil {
		return errors.New("session store: nil run event")
	}
	if err := s.db.WithContext(ctx).Create(ev).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListRunEventsAfter 返回某运行中序号大于 after 的事件（按 seq 升序），并限定属主用户；
// limit 非正数时不限定。
func (s *SessionStore) ListRunEventsAfter(ctx context.Context, runID serializer.HexUUID, userID uint, after uint64, limit int) ([]*ai.AgentRunEvent, error) {
	q := s.db.WithContext(ctx).
		Where("run_id = ? AND user_id = ? AND seq > ?", runID, userID, after).
		Order("seq ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var events []*ai.AgentRunEvent
	if err := q.Find(&events).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return events, nil
}

// CreateLiveRun 登记一条驻留运行；lr 为空时返回英文错误。同一 (session_id, user_id) 已有
// running 登记时（部分唯一索引 uq_agent_live_runs_running），以 ON CONFLICT (session_id, user_id)
// WHERE status='running' DO NOTHING 交由数据库原子裁决：不写入且返回 ErrAgentLiveRunAlreadyRunning，
// 免去解析驱动错误文案。
func (s *SessionStore) CreateLiveRun(ctx context.Context, lr *ai.AgentLiveRun) error {
	if lr == nil {
		return errors.New("session store: nil live run")
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}, {Name: "user_id"}},
		// ON CONFLICT 的 WHERE 须与部分唯一索引 uq_agent_live_runs_running 的谓词逐字一致，
		// 故内联字面量（绑定参数无法与索引谓词的文本匹配，SQLite 会判为无匹配约束）。
		TargetWhere: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "status = '" + string(ai.AgentRunPhaseRunning) + "'"},
		}},
		DoNothing: true,
	}).Create(lr)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAgentLiveRunAlreadyRunning
	}
	return nil
}

// UpdateLiveRunSeq 更新驻留运行的已发布最大事件序号（续播起点据此推进）；只写 last_seq 一列。
func (s *SessionStore) UpdateLiveRunSeq(ctx context.Context, runID serializer.HexUUID, lastSeq uint64) error {
	if err := s.db.WithContext(ctx).Model(&ai.AgentLiveRun{}).
		Where("run_id = ?", runID).
		Update("last_seq", lastSeq).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// FinishLiveRun 把驻留运行收敛到终态阶段并写入最终事件序号（限定属主用户）；只写 status 与
// last_seq 两列。
func (s *SessionStore) FinishLiveRun(ctx context.Context, runID serializer.HexUUID, userID uint, phase ai.AgentRunPhase, lastSeq uint64) error {
	if err := s.db.WithContext(ctx).Model(&ai.AgentLiveRun{}).
		Where("run_id = ? AND user_id = ?", runID, userID).
		Updates(map[string]any{"status": phase, "last_seq": lastSeq}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// LatestLiveRun 返回某会话最近一条驻留运行登记（按 id 倒序），并限定属主用户；
// 不存在时返回 ErrAgentRunNotFound。
func (s *SessionStore) LatestLiveRun(ctx context.Context, sessionID, userID uint) (*ai.AgentLiveRun, error) {
	var lr ai.AgentLiveRun
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("id DESC").
		First(&lr).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentRunNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &lr, nil
}
