// Package tag_event 提供标签事件相关的 asynq 任务处理器。
// EventDetectHandler 消费游标调度器（CursorScheduler）按触发间隔投递的 tag:event:detect 任务：
// 从 core 库加载启用条件，以任务载荷携带的游标时间点（TriggerAt）为检测时间，
// 调用 tagengine 的标签事件识别能力（Detect）完成检测，并将命中事件以 tag:event:emit 任务异步投递；
// EventEmitHandler 消费检测命中后投递的 tag:event:emit 任务。
package tag_event

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tagengine"
)

const (
	// TagEventDetectTask 标签事件检测定时任务类型，由游标调度器（CursorScheduler）
	// 按任务配置的触发间隔（interval）投递，每个触发点携带独立的游标时间点。
	TagEventDetectTask = "tag:event:detect"
)

// eventEnqueuer 事件任务投递接口，定义在消费侧，便于单元测试注入替代实现。
// *pipeline.Client 满足该接口。
type eventEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// 编译期断言：确保 EventDetectHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventDetectHandler)(nil)

// EventDetectHandler 标签事件检测处理器，消费游标调度器投递的 tag:event:detect 任务。
// 流程：解析任务载荷中的游标时间点 -> 加载启用条件 -> 过滤非法表达式
// -> tagengine.Detect 单条 UNION ALL SQL 查询 tag_summings -> 聚合事件 -> 以 tag:event:emit 任务异步投递。
type EventDetectHandler struct {
	coreDB *database.DB    // core 库，读取标签事件规则与条件
	rawDB  *database.DB    // raw 库（ClickHouse），查询 tag_summings 聚合统计
	client eventEnqueuer   // 事件任务投递客户端
	logger *logging.Logger // 日志记录器
}

// NewEventDetectHandler 从 do.Injector 解析依赖并创建标签事件检测处理器。
// 供 worker 注册处直接构造，与 pcap.NewPcapBatchHandler 的注册模式一致。
func NewEventDetectHandler(i do.Injector) *EventDetectHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	rawDB := provider.MustInvokeRawDatabase(i)
	client := provider.MustInvokePipelineClient(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	return &EventDetectHandler{coreDB: coreDB, rawDB: rawDB, client: client, logger: logger}
}

// Do 执行一次标签事件检测。
// 检测时间取自任务载荷中的游标时间点（CursorTaskPayload.TriggerAt），由 CursorScheduler
// 按游标推进投递：即使积压补投或多个触发点并行执行，每个触发点也使用自身时间窗口计算，
// 不会因实际执行时刻漂移而重复或遗漏。载荷解析失败时回退到当前时间并记录告警。
// 表达式非法仅记录告警日志并跳过对应条件，投递失败记录告警日志，不中断整次检测；
// 加载/查询错误返回错误触发 asynq 重试。
func (h *EventDetectHandler) Do(ctx context.Context, task etl.Task) error {
	now, _, err := pipeline.ParseCursorTaskPayload(task.Payload())
	if err != nil {
		// 载荷非游标格式（如旧版本 cron 调度投递的配置 JSON 载荷）时回退到当前时间
		h.logger.Ctx(ctx).Warnf("tag event detect: parse cursor payload failed, fallback to now: %v", err)
		now = time.Now()
	} else if now.IsZero() {
		now = time.Now()
	}
	conds, err := h.loadConditions(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	valid := make([]tagengine.DetectionCondition, 0, len(conds))
	for _, cond := range conds {
		if _, err := tagengine.TranslateExpression(cond.Expression); err != nil {
			h.logger.Ctx(ctx).Warnf("tag event detect: skip condition %d (rule %d), invalid expression %q: %v",
				cond.ConditionID, cond.RuleID, cond.Expression, err)
			continue
		}
		valid = append(valid, cond)
	}
	if len(valid) == 0 {
		h.logger.Ctx(ctx).Debug("tag event detect: no enabled valid conditions")
		return nil
	}
	events, err := tagengine.Detect(ctx, h.rawDB, valid, now)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, ev := range events {
		emitTask, err := NewTagEventEmitTask(ev)
		if err != nil {
			h.logger.Ctx(ctx).Warnf("tag event detect: build emit task failed: %v", err)
			continue
		}
		if _, err := h.client.EnqueueContext(ctx, emitTask, asynq.MaxRetry(3)); err != nil {
			h.logger.Ctx(ctx).Warnf("tag event detect: enqueue emit task failed: %v", err)
			continue
		}
		h.logger.Ctx(ctx).Infof("tag event detect: enqueued emit task rule=%s namespace=%s matchAll=%v hits=%d",
			ev.RuleName, ev.Namespace, ev.MatchAll, len(ev.Hits))
	}
	return nil
}

// loadConditions 从 core 库加载启用且所属规则启用的标签事件条件。
func (h *EventDetectHandler) loadConditions(ctx context.Context) ([]tagengine.DetectionCondition, error) {
	var rows []*tag.TagEventCondition
	if err := h.coreDB.WithContext(ctx).
		Preload("TagEventRule").
		Where("enabled = ?", true).
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	conds := make([]tagengine.DetectionCondition, 0, len(rows))
	for _, row := range rows {
		if row.TagEventRule.ID == 0 || !row.TagEventRule.Enabled {
			continue
		}
		conds = append(conds, tagengine.DetectionCondition{
			RuleID:        row.TagEventRule.ID,
			RuleName:      row.TagEventRule.Name,
			MatchAll:      row.TagEventRule.MatchAll,
			ConditionID:   row.ID,
			ConditionName: row.Name,
			// 命名空间由所属规则承载（条件与规则多对一，条件自身不冗余存储）
			Namespace:     row.TagEventRule.NamespaceName,
			Model:         row.Model,
			Dimension:     row.Dimension,
			Item:          row.Item,
			Window:        row.Window,
			Threshold:     row.Threshold,
			Expression:    row.Expression,
		})
	}
	return conds, nil
}
