package tag_event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tagengine"
	notifyhandler "zhongkui.com/zhongkui/internal/worker/handlers/notify"
)

const (
	// TagEventSuppressTask 标签事件抑制评估任务类型，由游标调度器按触发间隔（每分钟）投递。
	TagEventSuppressTask = "tag:event:suppress"
)

// eventEnqueuer 事件任务投递接口，定义在消费侧，便于单元测试注入替代实现。
type eventEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// 编译期断言：确保 EventSuppressHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventSuppressHandler)(nil)

// DedupKey 计算标签事件的抑制指纹：namespace + rule_id + 排序后的命中分组 (model|dimension|item) 串联后取 SHA-256 十六进制。
func DedupKey(ev *tagengine.TagEventRecord) string {
	keys := make([]string, 0, len(ev.Hits))
	for _, hit := range ev.Hits {
		keys = append(keys, hit.Model+":"+hit.Dimension+":"+hit.Item)
	}
	sort.Strings(keys)
	raw := ev.Namespace + "$" + strconv.FormatUint(uint64(ev.RuleID), 10) + "$" + strings.Join(keys, "#")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// keyAgg 抑制评估的单键聚合结果：按 dedup_key 分组后，本批最早 pending 与最早 alerting 锚的关键事实。
// 覆盖窗口终点 = 最早锚 detected_at + suppress_window，即"该键当前告警所覆盖的时间区间"。
// 时间统一以 epoch 秒（float64）表示，避免跨数据库驱动的时间序列化差异。
type keyAgg struct {
	// DedupKey 事件键（抑制指纹）
	DedupKey string
	// MinPendEpoch 本批该键最早 pending 的检测时间（detected_at 最小）对应的 epoch 秒
	MinPendEpoch float64
	// MinAlertEpoch 该键最早的 alerting 锚的检测时间对应的 epoch 秒，无锚时为 nil
	MinAlertEpoch *float64
	// Window 规则抑制窗口（纳秒），0 表示不抑制
	Window serializer.DurationNS
	// MinPendID 最早 pending 的 ID（detected_at 最小、ID 最小）
	MinPendID *uint
}

// EventSuppressHandler 标签事件抑制评估处理器，消费 tag:event:suppress 任务。
// 流程（全部在单事务内，先以一条 group-select 按 dedup_key 聚合，再依据聚合结果批量更新状态）：
//  1. 覆盖窗口抑制：落在 alerting 锚覆盖窗口（最早锚 detected_at + suppress_window）内的 pending -> suppressed
//  2. 新告警：无锚或首条 pending 已超出覆盖窗口的键，其首条 pending -> alerting（同键仅首条告警）
//  3. window=0 不抑制：该规则下全部 pending -> alerting
//  4. 其余抑制：新告警键中除首条外的其余 pending -> suppressed
//  5. 自动关闭：新告警取代同键前序 alerting -> closed（已通知过的告警转关闭态，保留作历史；
//     与手动关闭共用 closed 状态），而非 suppressed（从未通知的抑制态仅由步骤 1/4 产生）
//
// notify 任务在状态变更提交前投递：投递失败则事务回滚，事件保持 pending，下轮重估。
type EventSuppressHandler struct {
	coreDB *database.DB    // core 库，读取与更新标签事件
	client eventEnqueuer   // notify 任务投递客户端
	logger *logging.Logger // 日志记录器
}

// NewEventSuppressHandler 从 do.Injector 解析依赖并创建标签事件抑制评估处理器。
func NewEventSuppressHandler(i do.Injector) *EventSuppressHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	client := provider.MustInvokePipelineClient(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	return &EventSuppressHandler{coreDB: coreDB, client: client, logger: logger}
}

// Do 执行一次标签事件抑制评估。
// 单事务内处理全部 pending 事件，完成批量状态变更（覆盖抑制 / 新告警 / window0 全告警 / 其余抑制 / 自动关闭）；
// 无 pending 事件时直接返回；notify 任务投递失败返回错误触发重试（事务回滚，事件保持 pending，下轮重估）。
func (h *EventSuppressHandler) Do(ctx context.Context, task etl.Task) error {
	coverSQL := buildSuppressCoverSQL()

	var alertingIDs []uint
	if err := h.coreDB.WithContext(ctx).Transaction(func(tx *database.DB) error {
		// 单条 group-select 按 dedup_key 聚合（ROW_NUMBER 取每键最早 pending）：最早 pending / 最早 alerting 锚 / 规则窗口 / 首条 pending ID
		var aggs []keyAgg
		groupSQL := buildSuppressGroupSQL()

		if err := tx.
			Raw(groupSQL, tag.EventStatusAlerting, tag.EventStatusPending).
			Scan(&aggs).
			Error; err != nil {
			return errors.WithStack(err)
		}
		window0Keys, newAlertKeys, minPendIDs := classifyAggs(aggs)

		// 1) 覆盖窗口抑制：最早锚 detected_at + 窗口内的 pending -> suppressed
		if err := tx.
			Exec(coverSQL, tag.EventStatusSuppressed, tag.EventStatusAlerting, tag.EventStatusPending).
			Error; err != nil {
			return errors.WithStack(err)
		}
		// 2) 新告警：新发生键的首条 pending（无锚或已超出覆盖窗口）-> alerting，并收集新告警 ID
		if len(newAlertKeys) > 0 {
			if err := tx.
				Raw(
					`UPDATE tag_event_records SET status = ? WHERE status = ? AND id IN ? RETURNING id`,
					tag.EventStatusAlerting,
					tag.EventStatusPending,
					minPendIDs,
				).
				Scan(&alertingIDs).
				Error; err != nil {
				return errors.WithStack(err)
			}
		}
		// 3) window=0 不抑制：该键全部 pending -> alerting，并收集新告警 ID
		if len(window0Keys) > 0 {
			var window0IDs []uint
			if err := tx.
				Raw(
					`UPDATE tag_event_records SET status = ? WHERE status = ? AND dedup_key IN ? RETURNING id`,
					tag.EventStatusAlerting,
					tag.EventStatusPending,
					window0Keys,
				).
				Scan(&window0IDs).
				Error; err != nil {
				return errors.WithStack(err)
			}
			alertingIDs = append(alertingIDs, window0IDs...)
		}
		if len(alertingIDs) == 0 {
			return nil
		}
		// 通知 ID 按升序稳定，便于观察与断言
		slices.Sort(alertingIDs)
		// 4) 其余抑制：新告警键中除首条外的其余 pending（同键仅首条告警）-> suppressed
		if err := tx.
			Exec(
				`UPDATE tag_event_records SET status = ? WHERE status = ? AND dedup_key IN ?`,
				tag.EventStatusSuppressed,
				tag.EventStatusPending,
				newAlertKeys,
			).
			Error; err != nil {
			return errors.WithStack(err)
		}
		// 5) 自动关闭：新告警取代同键前序 alerting -> closed（已通知过的告警转关闭态，保留作历史）
		if err := tx.
			Exec(
				`UPDATE tag_event_records SET status = ? WHERE status = ? AND dedup_key IN ? AND id NOT IN ?`,
				tag.EventStatusClosed,
				tag.EventStatusAlerting,
				newAlertKeys,
				alertingIDs,
			).Error; err != nil {
			return errors.WithStack(err)
		}
		// 通知在状态变更提交前投递：失败则回滚，事件保持 pending
		sendTasks, err := buildSendTasks(tx, alertingIDs)
		if err != nil {
			return err
		}
		for _, sendTask := range sendTasks {
			if _, err := h.client.EnqueueContext(ctx, sendTask, asynq.MaxRetry(3)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return errors.WithStack(err)
	}
	if len(alertingIDs) > 0 {
		h.logger.Ctx(ctx).Infof("tag event suppress: %d alerting", len(alertingIDs))
	}
	return nil
}

// classifyAggs 依据单键聚合结果将键归类：
//   - window0Keys：suppress_window=0 的键，其全部 pending 均告警（不抑制）
//   - newAlertKeys：无 alerting 锚、或首条 pending 已超出锚覆盖窗口的键，其首条 pending 告警
//   - minPendIDs：newAlertKeys 中首条 pending 的 ID（即新告警事件）
//
// 锚覆盖窗口终点 = 最早锚 detected_at + suppress_window；首条 pending 落在窗口内（<= 终点）时
// 由覆盖窗口抑制统一处理，不产生新告警。
func classifyAggs(aggs []keyAgg) (window0Keys, newAlertKeys []string, minPendIDs []uint) {
	for _, agg := range aggs {
		if agg.Window <= 0 {
			window0Keys = append(window0Keys, agg.DedupKey)
			continue
		}
		if agg.MinAlertEpoch != nil {
			coverageEnd := *agg.MinAlertEpoch + float64(agg.Window)/1e9
			if agg.MinPendEpoch <= coverageEnd {
				continue // 首条 pending 仍在锚覆盖窗口内，全部交给覆盖窗口抑制
			}
		}
		if agg.MinPendID != nil {
			newAlertKeys = append(newAlertKeys, agg.DedupKey)
			minPendIDs = append(minPendIDs, *agg.MinPendID)
		}
	}
	return window0Keys, newAlertKeys, minPendIDs
}

// epochExpr 返回将 SQL 表达式转为 Unix 秒数的表达式：EXTRACT(EPOCH FROM …)。
func epochExpr(expr string) string {
	return "EXTRACT(EPOCH FROM " + expr + ")"
}

// buildSuppressCoverSQL 生成"覆盖窗口抑制"SQL：将落在 alerting 锚覆盖窗口（最早锚 detected_at + suppress_window）
// 内的 pending 置为 suppressed。
func buildSuppressCoverSQL() string {
	return fmt.Sprintf(`
UPDATE tag_event_records AS E
SET status = ?
FROM (
  SELECT
  	a.dedup_key,
    %s + r.suppress_window / 1e9 AS coverage_epoch
  FROM tag_event_records a
  JOIN tag_event_rules r ON r.id = a.rule_id AND r.suppress_window > 0
  WHERE a.status = ?
  GROUP BY a.dedup_key, r.suppress_window
) AS agg
WHERE E.status = ?
  AND E.dedup_key = agg.dedup_key
  AND %s <= agg.coverage_epoch`,
		epochExpr("MIN(a.detected_at)"),
		epochExpr("E.detected_at"),
	)
}

func buildSuppressGroupSQL() string {
	return fmt.Sprintf(`
WITH alrt AS (
  SELECT
  	a.dedup_key,
	a.rule_id,
	MIN(%s) AS min_alert_epoch
  FROM tag_event_records a
  WHERE a.status = ?
  GROUP BY a.dedup_key, a.rule_id
), pend AS (
  SELECT
  	p.dedup_key,
	p.rule_id,
	p.id,
    %s AS pend_epoch,
    ROW_NUMBER() OVER (PARTITION BY p.dedup_key, p.rule_id ORDER BY %s ASC, p.id ASC) AS rn
  FROM tag_event_records p
  WHERE p.status = ?
)
SELECT
	p.dedup_key,
    p.pend_epoch AS min_pend_epoch,
    a.min_alert_epoch,
    r.suppress_window AS window,
    p.id AS min_pend_id
FROM pend p
LEFT JOIN alrt a ON a.dedup_key = p.dedup_key
JOIN tag_event_rules r ON r.id = p.rule_id
WHERE p.rn = 1`,
		epochExpr("a.detected_at"),
		epochExpr("p.detected_at"),
		epochExpr("p.detected_at"),
	)
}

// buildSendTasks 加载告警事件（含规则绑定通知组/通知目标与命中明细），
// 为每个事件构造一条通知发送任务：规则直绑通知目标时构建单目标直接发送任务
// （notify:send:target，指向 NotificationTargetID）；否则构建通知组发送任务
// （notify:send:group）。事件规则既未绑定目标也未绑定通知组时 GroupID 传 nil，
// 由 notify 服务解析命名空间默认组。
// 返回空列表（如事件已全部被删除）时调用方应跳过投递。
// buildSendTasks loads the alerting events (with the rule-bound group/target and hit
// details) and builds one notification send task per event: a direct single-target
// task (notify:send:target, to NotificationTargetID) when the rule is directly bound to
// a target, otherwise a group-send task (notify:send:group). When the rule is bound to
// neither a target nor a group, GroupID is nil and the notify service resolves the
// namespace default group. An empty result (e.g. all events deleted) makes the caller
// skip delivery.
func buildSendTasks(db *database.DB, eventIDs []uint) ([]*asynq.Task, error) {
	var events []*tag.TagEventRecord
	if err := db.
		Preload("TagEventRule").
		Preload("Hits").
		Where("id IN ?", eventIDs).
		Find(&events).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	tasks := make([]*asynq.Task, 0, len(events))
	for _, ev := range events {
		var task *asynq.Task
		var err error
		if ev.TagEventRule.NotificationTargetID != nil {
			task, err = notifyhandler.NewSendTargetTask(notify.SendTargetInput{
				Namespace: ev.NamespaceName,
				TargetID:  *ev.TagEventRule.NotificationTargetID,
				EventType: core.EventTypeTagEventRecord,
				EventID:   ev.ID,
				Message:   buildEventMessage(ev),
			})
		} else {
			task, err = notifyhandler.NewSendGroupTask(notify.SendGroupInput{
				Namespace: ev.NamespaceName,
				GroupID:   ev.TagEventRule.NotificationGroupID,
				EventType: core.EventTypeTagEventRecord,
				EventID:   ev.ID,
				Message:   buildEventMessage(ev),
			})
		}
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// buildEventMessage 将标签事件构造为结构化通知消息：模板键 + 参数。
// 具体文案由 notify 服务按目标语言经 locales 模板（notify.tag_event.alert.title/content）
// 渲染，事件源不关心最终文本，仅固化模板键与参数（Params 键与模板占位符一一对应）。
// buildEventMessage maps a tag event to a structured notification message: a template
// key plus params. The concrete text is rendered by the notify service in the target
// language via the locale templates (notify.tag_event.alert.title/content); the event
// source only pins the template key and params (keys matching the placeholders).
func buildEventMessage(ev *tag.TagEventRecord) notify.Message {
	hits := make([]map[string]any, 0, len(ev.Hits))
	for _, hit := range ev.Hits {
		hits = append(hits, map[string]any{
			"ConditionName": hit.ConditionName,
			"Dimension":     hit.Dimension,
			"Item":          hit.Item,
			"Count":         hit.Count,
			"Threshold":     hit.Threshold,
		})
	}
	return notify.Message{
		Template: notify.TemplateTagEventAlert,
		Params: map[string]any{
			"Namespace":  ev.NamespaceName,
			"RuleName":   ev.RuleName,
			"DetectedAt": ev.DetectedAt.Format(time.RFC3339),
			"Hits":       hits,
		},
	}
}
