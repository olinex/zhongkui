// 本文件承载过期运行数据的 TTL 清理：周期性硬删除超过 expires_at 的运行登记
// （agent_live_runs）、事件日志（agent_run_events）与挂起运行现场（agent_runs）。三张表均为
// 「写多读少、按时间自然过期」的数据，清理避免其无限增长而拖慢续播回放与挂起现场查询。
// 清理任务随 Runtime 的启动/关闭生命周期运行（见 runtime.go 的 Start/Shutdown）。

package ai

import (
	// 标准库
	"context"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
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
	// DeleteExpiredLiveRuns 硬删除一批超过 now 的运行登记，返回删除行数
	DeleteExpiredLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error)
	// DeleteExpiredRunEvents 硬删除一批超过 now 的事件日志（兜底清理未被级联的事件），返回删除行数
	DeleteExpiredRunEvents(ctx context.Context, now time.Time, limit int) (int64, error)
	// DeleteExpiredAgentRuns 硬删除一批超过 now 的挂起运行现场（绕过 GORM 软删），返回删除行数
	DeleteExpiredAgentRuns(ctx context.Context, now time.Time, limit int) (int64, error)
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

// sweep 对三张表分别循环分批删除，直到某批删除数小于 batch（即已抽干）为止；任一步失败记告警
// 并返回错误（调用方在 run 中忽略，避免一次失败终止整个清理任务）。
func (c *runCleaner) sweep(ctx context.Context) error {
	now := c.now()
	if err := c.sweepTable(ctx, now, "live runs", c.store.DeleteExpiredLiveRuns); err != nil {
		return err
	}
	if err := c.sweepTable(ctx, now, "run events", c.store.DeleteExpiredRunEvents); err != nil {
		return err
	}
	if err := c.sweepTable(ctx, now, "agent runs", c.store.DeleteExpiredAgentRuns); err != nil {
		return err
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
			return err
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
