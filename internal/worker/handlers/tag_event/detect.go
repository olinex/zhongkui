// Package tag_event 提供标签事件识别能力。
// 包含标签事件检测领域逻辑（表达式翻译、查询组装、事件聚合、任务载荷编码）与 asynq 任务处理器：
// EventDetectHandler 消费调度器每分钟投递的 tag:event:detect 任务，
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
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	// TagEventDetectTask 标签事件检测定时任务类型，由 asynq 调度器每分钟投递。
	TagEventDetectTask = "tag:event:detect"
)

// 编译期断言：确保 EventDetectHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventDetectHandler)(nil)

// EventDetectHandler 标签事件检测处理器，消费 asynq 每分钟投递的 tag:event:detect 任务。
// 流程：加载启用条件 -> BuildDetectionQuery 单条 UNION ALL SQL 查询 tag_summings
// -> GroupEvents 聚合事件 -> 以 tag:event:emit 任务异步投递。
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
// 表达式非法或投递失败仅记录告警日志，不中断整次检测；加载/查询错误返回错误触发 asynq 重试。
func (h *EventDetectHandler) Do(ctx context.Context, _ etl.Task) error {
	now := time.Now()
	conds, err := h.loadConditions(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	valid := make([]DetectionCondition, 0, len(conds))
	for _, cond := range conds {
		if _, err := TranslateExpression(cond.Expression); err != nil {
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
	query, args, err := BuildDetectionQuery(h.rawDB, valid, now)
	if err != nil {
		return errors.WithStack(err)
	}
	var hits []DetectionHit
	if err := h.rawDB.WithContext(ctx).Raw(query, args...).Scan(&hits).Error; err != nil {
		return errors.WithStack(err)
	}
	for _, ev := range GroupEvents(valid, hits, now) {
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
func (h *EventDetectHandler) loadConditions(ctx context.Context) ([]DetectionCondition, error) {
	var rows []*tag.TagEventCondition
	if err := h.coreDB.WithContext(ctx).
		Preload("TagEventRule").
		Where("enabled = ?", true).
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	conds := make([]DetectionCondition, 0, len(rows))
	for _, row := range rows {
		if row.TagEventRule.ID == 0 || !row.TagEventRule.Enabled {
			continue
		}
		conds = append(conds, DetectionCondition{
			RuleID:        row.TagEventRule.ID,
			RuleName:      row.TagEventRule.Name,
			MatchAll:      row.TagEventRule.MatchAll,
			ConditionID:   row.ID,
			ConditionName: row.Name,
			Namespace:     row.NamespaceName,
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
