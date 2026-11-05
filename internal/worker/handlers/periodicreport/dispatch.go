package periodicreport

import (
	// 标准库
	"context"
	"time"

	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

const (
	// TaskDispatch 报告计划分发任务类型，由 asynq 周期调度器每分钟投递；扫描到期计划并
	// 为每条计划投递一条 periodic-report:plan:run，不补投停机期间的历史触发点。
	TaskDispatch = "periodic-report:plan:dispatch"

	// dispatchBatchLimit 分发器单轮最多处理的计划条数。
	dispatchBatchLimit = 200
)

// DispatchHandler 消费 periodic-report:plan:dispatch 定时任务：扫描已启用、未到期且待计算或已到期的
// 报告计划，为待计算计划写入下次执行时间，为到期计划投递 periodic-report:plan:run 并推进下次执行时间。
type DispatchHandler struct {
	// store 报告领域存储（计划读写）。
	store *periodic.Store
	// enq 任务投递客户端。
	enq *pipeline.Client
	// loc cron 表达式解析时区。
	loc *time.Location
	// logger 日志记录器。
	logger *logging.Logger
	// now 当前时间来源，测试可替换。
	now func() time.Time
}

// 编译期断言：确保 DispatchHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*DispatchHandler)(nil)

// NewDispatchHandler 从 do.Injector 解析依赖并创建报告计划分发处理器。调度时区属于启动期
// 关键配置，提取或解析失败直接 panic。
func NewDispatchHandler(i do.Injector) *DispatchHandler {
	db := provider.MustInvokeCoreDatabase(i)
	enq := provider.MustInvokePipelineClient(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	v := provider.MustInvokeViper(i)
	cfg, err := pipeline.ExtractConfig(v, validator.Validate)
	if err != nil {
		panic(err)
	}
	loc, err := cfg.Scheduler.Location.ToSTD()
	if err != nil {
		panic(err)
	}
	store, err := periodic.NewStore(db)
	if err != nil {
		panic(err)
	}
	return &DispatchHandler{
		store:  store,
		enq:    enq,
		loc:    loc,
		logger: logger,
		now:    time.Now,
	}
}

// Do 消费一条 periodic-report:plan:dispatch 任务。任务载荷无需使用：每轮均以当前时刻扫描到期计划。
// 对 next_run_at 为 NULL 的计划只计算并写回下次执行时间，不投递；对已到期计划投递
// periodic-report:plan:run，再以 CAS 把 next_run_at 推进到当前时刻之后的第一个触发点（最多补一次，
// 不补历史积压）。每次运行使用独立会话，同一计划的多份报告可以并发生成，无需按计划串行拦截。
// 单计划失败只记录告警，不中断整轮。
func (h *DispatchHandler) Do(ctx context.Context, _ etl.Task) error {
	nowTime := h.now()
	plans, err := h.store.ListDispatchablePlans(ctx, nowTime, dispatchBatchLimit)
	if err != nil {
		return errors.WithStack(err)
	}
	var scheduled, dispatched, advanced, missed int
	for _, plan := range plans {
		if plan.NextRunAt == nil {
			next, err := periodic.NextTrigger(plan.CronSpec, nowTime, h.loc)
			if err != nil {
				h.logger.Ctx(ctx).Warnf("periodic report dispatch: plan %d has invalid cron %q: %v", plan.ID, plan.CronSpec, err)
				continue
			}
			if err := h.store.SetNextRunAtIfNull(ctx, plan.ID, next); err != nil {
				h.logger.Ctx(ctx).Warnf("periodic report dispatch: persist next run of plan %d failed: %v", plan.ID, err)
				continue
			}
			scheduled++
			continue
		}
		triggerAt := *plan.NextRunAt
		future, err := periodic.NextTrigger(plan.CronSpec, nowTime, h.loc)
		if err != nil {
			h.logger.Ctx(ctx).Warnf("periodic report dispatch: plan %d has invalid cron %q: %v", plan.ID, plan.CronSpec, err)
			continue
		}
		if err := periodic.EnqueuePlanRun(ctx, h.enq, plan.ID, triggerAt); err != nil {
			// 投递失败不推进 next_run_at，下一分钟重试
			h.logger.Ctx(ctx).Warnf("periodic report dispatch: enqueue plan %d at %s failed: %v", plan.ID, triggerAt.Format(time.RFC3339), err)
			continue
		}
		dispatched++
		ok, err := h.store.SetNextRunAtIfUnchanged(ctx, plan.ID, triggerAt, future)
		if err != nil {
			h.logger.Ctx(ctx).Warnf("periodic report dispatch: advance next run of plan %d failed: %v", plan.ID, err)
			continue
		}
		if !ok {
			// 已被其它实例推进：投递仍幂等（periodic_report_runs 以 (plan_id, trigger_at) 唯一约束兜底）
			h.logger.Ctx(ctx).Infof("periodic report dispatch: plan %d next run changed concurrently, skip advance", plan.ID)
			missed++
			continue
		}
		advanced++
	}
	h.logger.Ctx(ctx).Infof(
		"periodic report dispatch: candidates=%d scheduled=%d dispatched=%d advanced=%d casMissed=%d",
		len(plans), scheduled, dispatched, advanced, missed,
	)
	return nil
}
