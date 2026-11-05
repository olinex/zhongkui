package periodic

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"time"

	// 第三方
	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
)

// NextTrigger 计算严格晚于 after 的下一个 cron 触发时间点。
// cronSpec 按标准五字段 cron 解析（与计划校验口径一致）；loc 非 nil 时以其时区解析与计算，
// 并把结果所在时区随触发点一并返回。非法表达式原样返回解析错误。
func NextTrigger(cronSpec string, after time.Time, loc *time.Location) (time.Time, error) {
	schedule, err := cron.ParseStandard(strings.TrimSpace(cronSpec))
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	if spec, ok := schedule.(*cron.SpecSchedule); ok && loc != nil {
		spec.Location = loc
	}
	if loc != nil {
		after = after.In(loc)
	}
	return schedule.Next(after), nil
}

// EnqueuePlanRun 把一条报告计划投递为 periodic-report:plan:run 任务，触发时间点为 triggerAt。
// 队列、超时、重试与保留时长取报告领域常量；重复投递的载荷完全一致，由处理器侧的
// (plan_id, trigger_at) 唯一约束保证幂等。
func EnqueuePlanRun(ctx context.Context, enq *pipeline.Client, planID uint, triggerAt time.Time) error {
	payload, err := json.Marshal(pipeline.CursorTaskPayload{
		TriggerAt: triggerAt,
		Payload:   map[string]any{"planId": planID},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	task := asynq.NewTask(
		TaskHandler,
		payload,
		asynq.Queue(RunQueue),
		asynq.Timeout(RunTimeout),
		asynq.MaxRetry(RunMaxRetry),
		asynq.Retention(RunRetention),
	)
	if _, err := enq.EnqueueContext(ctx, task); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
