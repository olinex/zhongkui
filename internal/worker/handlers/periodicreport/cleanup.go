package periodicreport

import (
	// 标准库
	"context"
	"time"

	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// cleanupPerPlanLimit 是清理流程单次为每个计划处理的运行记录上限。
const cleanupPerPlanLimit = 200

// CleanupHandler 消费 periodic-report:run:cleanup 定时任务：把到期计划停用，
// 再按各计划保留天数清理已结束的运行记录及其报告对象与运行会话。
type CleanupHandler struct {
	// store 报告领域存储（计划与运行记录读写）。
	store *periodic.Store
	// objects 报告对象删除能力。
	objects periodic.ObjectRemover
	// sessions 运行会话删除能力，随运行记录一并清理每次运行的独立会话。
	sessions *session.Store
	// logger 日志记录器。
	logger *logging.Logger
	// now 当前时间来源，测试可替换。
	now func() time.Time
}

// 编译期断言：确保 CleanupHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*CleanupHandler)(nil)

// NewCleanupHandler 从 do.Injector 解析依赖并创建报告生命周期清理处理器。
func NewCleanupHandler(i do.Injector) *CleanupHandler {
	db := provider.MustInvokeCoreDatabase(i)
	objects := provider.MustInvokeObjectCache(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	store, err := periodic.NewStore(db)
	if err != nil {
		panic(err)
	}
	sessions, err := session.NewStore(db)
	if err != nil {
		panic(err)
	}
	return &CleanupHandler{
		store:    store,
		objects:  objects,
		sessions: sessions,
		logger:   logger,
		now:      time.Now,
	}
}

// Do 消费一条 periodic-report:run:cleanup 任务：解析游标载荷后以当前时间执行清理。清理以当前时刻而非
// 触发时间点计算截止时间，任务天然幂等；单项失败不中断整体流程，最终返回首个错误。
func (h *CleanupHandler) Do(ctx context.Context, task etl.Task) error {
	if _, _, err := pipeline.ParseCursorTaskPayload(task.Payload()); err != nil {
		return errors.WithStack(err)
	}
	nowTime := h.now()
	result, err := h.store.Cleanup(ctx, nowTime, h.objects, h.sessions, cleanupPerPlanLimit)
	h.logger.Ctx(ctx).Infof(
		"periodic report cleanup: expiredPlans=%d removedObjects=%d removedRuns=%d",
		result.ExpiredPlans, result.RemovedObjects, result.RemovedRuns,
	)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
