// Package tag_event 提供标签事件相关的 asynq 任务处理器。
// EventDetectHandler 消费游标调度器（CursorScheduler）按触发间隔投递的 tag:event:detect 任务：
// 从 core 库加载启用条件，以任务载荷携带的游标时间点（TriggerAt）为检测时间，
// 调用 tagengine 的标签事件识别能力（Detect）完成检测，并将命中事件直接批量入库（status=pending）；
// 抑制评估由 EventSuppressHandler 消费 tag:event:suppress 定时任务完成。
package tag_event

import (
	"context"
	"time"

	"github.com/samber/do/v2"

	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/provider"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

const (
	// TagEventDetectTask 标签事件检测定时任务类型，由游标调度器（CursorScheduler）
	// 按任务配置的触发间隔（interval）投递，每个触发点携带独立的游标时间点。
	TagEventDetectTask = "tag:event:detect"
)

// 编译期断言：确保 EventDetectHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventDetectHandler)(nil)

// EventDetectHandler 标签事件检测处理器，消费游标调度器投递的 tag:event:detect 任务。
// 流程：解析任务载荷中的游标时间点 -> 加载启用条件 -> 过滤非法表达式
// -> tagengine.Detect 单条 UNION ALL SQL 查询 tag_summings -> 聚合事件 -> 将命中事件批量入库（status=pending）。
type EventDetectHandler struct {
	coreDB *database.DB    // core 库，读取标签事件规则与条件并写入检测结果
	rawDB  *database.DB    // raw 库（ClickHouse），查询 tag_summings 聚合统计
	logger *logging.Logger // 日志记录器
}

// NewEventDetectHandler 从 do.Injector 解析依赖并创建标签事件检测处理器。
// 供 worker 注册处直接构造，与 pcap.NewPcapBatchHandler 的注册模式一致。
func NewEventDetectHandler(i do.Injector) *EventDetectHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	rawDB := provider.MustInvokeRawDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	return &EventDetectHandler{coreDB: coreDB, rawDB: rawDB, logger: logger}
}

// Do 执行一次标签事件检测。
// 检测时间取自任务载荷中的游标时间点（CursorTaskPayload.TriggerAt），由 CursorScheduler
// 按游标推进投递：即使积压补投或多个触发点并行执行，每个触发点也使用自身时间窗口计算，
// 不会因实际执行时刻漂移而重复或遗漏。载荷解析失败时回退到当前时间并记录告警。
// 表达式非法仅记录告警日志并跳过对应条件，入库失败返回错误触发 asynq 重试；
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
	if len(events) > 0 {
		if err := h.persistEvents(ctx, events); err != nil {
			return errors.WithStack(err)
		}
		h.logger.Ctx(ctx).Infof("tag event detect: persisted %d events", len(events))
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
			RuleID:         row.TagEventRule.ID,
			RuleName:       row.TagEventRule.Name,
			MatchAll:       row.TagEventRule.MatchAll,
			SuppressWindow: row.TagEventRule.SuppressWindow,
			ConditionID:    row.ID,
			ConditionName:  row.Name,
			// 命名空间由所属规则承载（条件与规则多对一，条件自身不冗余存储）
			Namespace:  row.TagEventRule.NamespaceName,
			Model:      row.Model,
			Dimension:  row.Dimension,
			Item:       row.Item,
			Window:     row.Window,
			Threshold:  row.Threshold,
			Expression: row.Expression,
		})
	}
	return conds, nil
}

// persistEvents 将检测命中的标签事件记录批量写入 core 库：记录与其命中明细在同一事务内提交，状态固定为 pending。
func (h *EventDetectHandler) persistEvents(ctx context.Context, events []*tagengine.TagEventRecord) error {
	records := make([]*tag.TagEventRecord, 0, len(events))
	for _, ev := range events {
		records = append(records, toEventModel(ev))
	}
	if len(records) == 0 {
		return nil
	}
	return h.coreDB.WithContext(ctx).Transaction(func(tx *database.DB) error {
		return tx.Create(&records).Error
	})
}

// toEventModel 将检测结果 TagEventRecord 映射为持久化模型：计算 DedupKey、状态置 pending、DetectedAt 透传，命中明细映射为 TagEventRecordHit。
func toEventModel(ev *tagengine.TagEventRecord) *tag.TagEventRecord {
	hits := make([]tag.TagEventRecordHit, 0, len(ev.Hits))
	for _, hit := range ev.Hits {
		hits = append(hits, tag.TagEventRecordHit{
			ConditionID:   hit.ConditionID,
			ConditionName: hit.ConditionName,
			Model:         hit.Model,
			Dimension:     hit.Dimension,
			Item:          hit.Item,
			Count:         hit.Count,
			Threshold:     hit.Threshold,
			Window:        hit.Window,
			IDs:           hit.IDs,
		})
	}
	return &tag.TagEventRecord{
		NamespaceName: ev.Namespace,
		RuleID:        ev.RuleID,
		RuleName:      ev.RuleName,
		MatchAll:      ev.MatchAll,
		DedupKey:      DedupKey(ev),
		Status:        tag.EventStatusPending,
		DetectedAt:    ev.DetectedAt,
		Hits:          hits,
	}
}
