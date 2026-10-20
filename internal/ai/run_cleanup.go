// 本文件承载过期运行数据的 TTL 清理：周期性硬删除超过 expires_at 的运行登记
// （agent_live_runs）、事件日志（agent_run_events）与挂起运行现场（agent_runs）。三张表均为
// 「写多读少、按时间自然过期」的数据，清理避免其无限增长而拖慢续播回放与挂起现场查询。
// 清理任务随 Runtime 的启动/关闭生命周期运行（见 runtime.go 的 Start/Shutdown）。

package ai

import (
	// 标准库
	"context"
	"time"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 清理任务的运行期常量。
const (
	// runCleanupInterval 清理任务的扫描间隔
	runCleanupInterval = 10 * time.Minute
	// runCleanupBatchSize 每批删除的行数上限（分批删除避免长事务与锁竞争）
	runCleanupBatchSize = 500
	// runCleanupMaxLoops 单张表单次清扫的最大批次数（异常情况下防止死循环）
	runCleanupMaxLoops = 100
)

// ExpiredRunStore 过期运行数据清理所需的持久化能力（由 *SessionStore 实现）。抽出接口便于
// 测试注入内存替身。
type ExpiredRunStore interface {
	// ConvergeExpiredPendingRuns 收敛一批已过期的挂起运行现场为失败：作废现场并写回锚点助手消息终局，
	// 使会话回落 暂停/失败（见 ConvergeExpiredPendingRuns 实现）。返回收敛行数。
	ConvergeExpiredPendingRuns(ctx context.Context, now time.Time, limit int) (int64, error)
	// ConvergeExpiredRunningLiveRuns 收敛一批已过期且仍登记为 running 的在途运行为失败：置失败并写回
	// 会话最近一条助手消息终局（见 ConvergeExpiredRunningLiveRuns 实现）。返回收敛行数。
	ConvergeExpiredRunningLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error)
	// DeleteExpiredLiveRuns 硬删除一批超过 now 的运行登记，返回删除行数
	DeleteExpiredLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error)
	// DeleteExpiredRunEvents 硬删除一批超过 now 的事件日志（兜底清理未被级联的事件），返回删除行数
	DeleteExpiredRunEvents(ctx context.Context, now time.Time, limit int) (int64, error)
	// DeleteExpiredAgentRuns 硬删除一批超过 now 的挂起运行现场（绕过 GORM 软删），返回删除行数
	DeleteExpiredAgentRuns(ctx context.Context, now time.Time, limit int) (int64, error)
}

// ConvergeOrphanLiveRun 把一个内存中已无对应活动的在途运行登记（running / suspended）收敛为失败：
// 置其阶段为 failed，并把该会话最近一条助手消息的终局阶段置为 failed，使会话派生回落 暂停/失败。
// 用于两处：状态推导读取路径发现孤儿（进程重启 / goroutine 丢失）时的即时兜底，以及过期清理任务对
// 超期 running 登记的收敛。登记已被收敛（或已被删除）时幂等返回 nil。不删除任何消息、长期记忆与
// 压缩检查点。
func (s *SessionStore) ConvergeOrphanLiveRun(ctx context.Context, runID serializer.HexUUID, sessionID, userID uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// WHERE 覆盖 running 与 suspended：孤儿既可能是进程重启前正在生成（running）的运行，也可能是
		// 挂起现场已作废（过期 / 被截断）后残留的 suspended 登记。
		res := tx.Model(&ai.AgentLiveRun{}).
			Where("run_id = ? AND user_id = ? AND status IN ?", runID, userID,
				[]ai.AgentRunPhase{ai.AgentRunPhaseRunning, ai.AgentRunPhaseSuspended}).
			Update("status", ai.AgentRunPhaseFailed)
		if res.Error != nil {
			return errors.WithStack(res.Error)
		}
		if res.RowsAffected == 0 {
			// 已被并发收敛 / 已删除：不重复写回锚点，幂等返回。
			return nil
		}
		var msg ai.AgentMessage
		err := tx.Where("session_id = ? AND user_id = ? AND role = ?", sessionID, userID, ai.MessageRoleAssistant).
			Order("seq DESC").First(&msg).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Model(&ai.AgentMessage{}).Where("id = ?", msg.ID).
			Update("phase", ai.AgentRunPhaseFailed).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}

// ConvergeExpiredRunningLiveRuns 分批收敛已过期且仍登记为 running 的在途运行：逐行经
// ConvergeOrphanLiveRun 置失败并写回会话最近助手消息终局。收敛后阶段变为 failed，故同一批不会重复命中
// （分批循环可自然抽干）。仅收敛 running；suspended / done / failed / cancelled 的过期登记留给删除步骤。
func (s *SessionStore) ConvergeExpiredRunningLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = runCleanupBatchSize
	}
	var rows []ai.AgentLiveRun
	if err := s.db.WithContext(ctx).
		Where("status = ? AND expires_at < ?", ai.AgentRunPhaseRunning, now).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	var converged int64
	for _, row := range rows {
		if err := s.ConvergeOrphanLiveRun(ctx, row.RunID, row.SessionID, row.UserID); err != nil {
			return converged, errors.WithStack(err)
		}
		converged++
	}
	return converged, nil
}

// ConvergeExpiredPendingRuns 分批收敛已过期的挂起运行现场：把其锚点助手消息终局置为 failed（使会话回落
// 暂停/失败），并把现场本身置为 expired（作废，不再可恢复）。单事务内对一批（≤ limit）执行，收敛后状态
// 变为 expired，故同一批不会重复命中。不删除任何消息、长期记忆与压缩检查点。
func (s *SessionStore) ConvergeExpiredPendingRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = runCleanupBatchSize
	}
	var converged int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runs []ai.AgentRun
		if err := tx.Where("status = ? AND expires_at < ?", ai.AgentRunStatusPending, now).
			Order("id ASC").Limit(limit).Find(&runs).Error; err != nil {
			return errors.WithStack(err)
		}
		if len(runs) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(runs))
		anchors := make([]uint, 0, len(runs))
		for _, run := range runs {
			ids = append(ids, run.ID)
			if run.AnchorMessageID != 0 {
				anchors = append(anchors, run.AnchorMessageID)
			}
		}
		if len(anchors) > 0 {
			if err := tx.Model(&ai.AgentMessage{}).Where("id IN ?", anchors).
				Update("phase", ai.AgentRunPhaseFailed).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		if err := tx.Model(&ai.AgentRun{}).Where("id IN ?", ids).
			Update("status", ai.AgentRunStatusExpired).Error; err != nil {
			return errors.WithStack(err)
		}
		converged = int64(len(runs))
		return nil
	})
	return converged, errors.WithStack(err)
}

// DeleteExpiredLiveRuns 分批硬删除超过 now 的运行登记；其事件经 run_id 外键级联删除。
// 用原生 SQL 的子查询 LIMIT 取一批 id 再删，参数用 time.Time 与 int 占位符（Postgres / SQLite 通用）。
func (s *SessionStore) DeleteExpiredLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = runCleanupBatchSize
	}
	res := s.db.WithContext(ctx).Exec(
		`DELETE FROM agent_live_runs WHERE id IN (SELECT id FROM agent_live_runs WHERE expires_at < ? ORDER BY id LIMIT ?)`,
		now, limit,
	)
	if res.Error != nil {
		return 0, errors.WithStack(res.Error)
	}
	return res.RowsAffected, nil
}

// DeleteExpiredRunEvents 分批硬删除超过 now 的事件日志（兜底清理未被外键级联的事件）。
func (s *SessionStore) DeleteExpiredRunEvents(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = runCleanupBatchSize
	}
	res := s.db.WithContext(ctx).Exec(
		`DELETE FROM agent_run_events WHERE id IN (SELECT id FROM agent_run_events WHERE expires_at < ? ORDER BY id LIMIT ?)`,
		now, limit,
	)
	if res.Error != nil {
		return 0, errors.WithStack(res.Error)
	}
	return res.RowsAffected, nil
}

// DeleteExpiredAgentRuns 分批硬删除超过 now 的挂起运行现场。agent_runs 有 deleted_at 列，必须用
// 原生 SQL 硬删以绕过 GORM 软删，否则过期行只被标记而不被真正移除。
func (s *SessionStore) DeleteExpiredAgentRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = runCleanupBatchSize
	}
	res := s.db.WithContext(ctx).Exec(
		`DELETE FROM agent_runs WHERE id IN (SELECT id FROM agent_runs WHERE expires_at < ? ORDER BY id LIMIT ?)`,
		now, limit,
	)
	if res.Error != nil {
		return 0, errors.WithStack(res.Error)
	}
	return res.RowsAffected, nil
}

// runCleaner 是过期运行数据清理器：立即清扫一次后按 interval 周期清扫，直到 ctx 取消。
type runCleaner struct {
	// store 持久化删除能力
	store ExpiredRunStore
	// logger 日志记录器（可空）
	logger *logging.Logger
	// interval 扫描间隔（测试可覆盖）
	interval time.Duration
	// batch 每批删除行数上限（测试可覆盖）
	batch int
	// now 当前时间来源（测试可覆盖）
	now func() time.Time
}

// newRunCleaner 构造清理器，采用包内默认间隔与批量。
func newRunCleaner(store ExpiredRunStore, logger *logging.Logger) *runCleaner {
	return &runCleaner{
		store:    store,
		logger:   logger,
		interval: runCleanupInterval,
		batch:    runCleanupBatchSize,
		now:      time.Now,
	}
}

// run 启动清理循环：先立即清扫一次，随后周期性清扫，直到 ctx 取消（务必停掉 ticker）。
func (c *runCleaner) run(ctx context.Context) {
	_ = c.sweep(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.sweep(ctx)
		}
	}
}

// sweep 先收敛再删除：把过期的挂起现场与超期仍登记为 running 的在途运行收敛为失败（写回锚点 / 最近
// 助手消息终局），使会话派生回落 `暂停/失败`；随后再分批硬删除三张表的过期行。收敛与删除分开，是为了
// 避免「先删登记后会话无从判定为失败」——登记删除后便无从定位该写回哪条消息。任一步失败记告警并返回
// 错误（调用方在 run 中忽略，避免一次失败终止整个清理任务）。
func (c *runCleaner) sweep(ctx context.Context) error {
	now := c.now()
	// 收敛：过期挂起现场（→ failed 语义 + 作废）、超期 running 在途运行（→ failed + 写回终局）。
	if err := c.sweepTable(ctx, now, "converging expired pending runs", c.store.ConvergeExpiredPendingRuns); err != nil {
		return errors.WithStack(err)
	}
	if err := c.sweepTable(ctx, now, "converging expired running live runs", c.store.ConvergeExpiredRunningLiveRuns); err != nil {
		return errors.WithStack(err)
	}
	if err := c.sweepTable(ctx, now, "live runs", c.store.DeleteExpiredLiveRuns); err != nil {
		return errors.WithStack(err)
	}
	if err := c.sweepTable(ctx, now, "run events", c.store.DeleteExpiredRunEvents); err != nil {
		return errors.WithStack(err)
	}
	if err := c.sweepTable(ctx, now, "agent runs", c.store.DeleteExpiredAgentRuns); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// sweepTable 对单张表循环分批删除：某批删除数小于 batch 即视为抽干而停止；达到最大批次数兜底
// 停止，防止异常下死循环。失败时记告警并返回错误。
func (c *runCleaner) sweepTable(
	ctx context.Context,
	now time.Time,
	label string,
	del func(context.Context, time.Time, int) (int64, error),
) error {
	for i := 0; i < runCleanupMaxLoops; i++ {
		affected, err := del(ctx, now, c.batch)
		if err != nil {
			if c.logger != nil {
				c.logger.Ctx(ctx).Warnf("ai: run cleanup: deleting expired %s failed: %v", label, err)
			}
			return errors.WithStack(err)
		}
		if affected < int64(c.batch) {
			return nil
		}
	}
	if c.logger != nil {
		c.logger.Ctx(ctx).Warnf("ai: run cleanup: reached the max batch count for %s", label)
	}
	return nil
}
