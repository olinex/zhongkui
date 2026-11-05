package tag_event

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/notify"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/tagengine"
	notifyhandler "zhongkuisec.com/zhongkui/internal/worker/handlers/notify"
)

// failEnqueuer 实现 eventEnqueuer 接口，EnqueueContext 始终返回错误，用于投递失败场景。
type failEnqueuer struct{}

// EnqueueContext 固定返回错误，模拟 notify 任务投递失败。
func (failEnqueuer) EnqueueContext(_ context.Context, _ *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, errors.New("enqueue failed")
}

// createTagEventStoreTestDB 共享已迁移的 PostgreSQL 测试库（testdb.Core），
// tag_event_rules / tag_event_records / tag_event_record_hits 等全部业务表已迁移。
func createTagEventStoreTestDB(t *testing.T) *database.DB {
	t.Helper()
	return testdb.Core(t)
}

// insertSuppressRule 写入一条命名空间 default 的规则并返回其 ID。
func insertSuppressRule(db *database.DB, window time.Duration) uint {
	rule := &tag.TagEventRule{
		NamespaceName:  "default",
		Name:           "suppress-rule",
		Enabled:        true,
		SuppressWindow: serializer.DurationNS(window),
	}
	So(db.Create(rule).Error, ShouldBeNil)
	return rule.ID
}

// newEvent 构造一条标签事件（DetectedAt 默认当前时间，CreatedAt 由 GORM 自动填充）。
func newEvent(ruleID uint, dedupKey, status string) *tag.TagEventRecord {
	return &tag.TagEventRecord{
		NamespaceName: "default",
		RuleID:        ruleID,
		RuleName:      "suppress-rule",
		DedupKey:      dedupKey,
		Status:        status,
		DetectedAt:    time.Now(),
	}
}

// insertEvent 直接以 GORM 落库一条事件并返回其 ID。
func insertEvent(db *database.DB, ev *tag.TagEventRecord) uint {
	So(db.Create(ev).Error, ShouldBeNil)
	return ev.ID
}

// setEventDetectedAt 用原生 SQL 将事件检测时间调整为指定时刻，模拟抑制窗口外的旧事件。
func setEventDetectedAt(db *database.DB, id uint, detectedAt time.Time) {
	So(db.Exec("UPDATE tag_event_records SET detected_at = ? WHERE id = ?", detectedAt, id).Error, ShouldBeNil)
}

// eventStatus 查询事件当前状态。
func eventStatus(db *database.DB, id uint) string {
	var status string
	So(db.Model(&tag.TagEventRecord{}).Where("id = ?", id).Pluck("status", &status).Error, ShouldBeNil)
	return status
}

// notifyEventIDs 断言 fakeEnqueuer 投递的通知任务（可能多条）并聚合返回其事件 ID 列表。
func notifyEventIDs(enq *fakeEnqueuer) []uint {
	var ids []uint
	for _, task := range enq.tasks {
		So(task.Type(), ShouldEqual, notifyhandler.SendGroupTask)
		var payload notify.SendGroupInput
		So(json.Unmarshal(task.Payload(), &payload), ShouldBeNil)
		So(payload.EventType, ShouldEqual, core.EventTypeTagEventRecord)
		ids = append(ids, payload.EventID)
	}
	return ids
}

// TestDedupKey 验证抑制指纹的稳定性与区分度。
func TestDedupKey(t *testing.T) {
	Convey("Test DedupKey", t, func() {
		newEv := func(ns string, ruleID uint, hits ...tagengine.TagEventRecordHit) *tagengine.TagEventRecord {
			return &tagengine.TagEventRecord{Namespace: ns, RuleID: ruleID, Hits: hits}
		}

		Convey("单命中指纹稳定", func() {
			ev := newEv("default", 1, tagengine.TagEventRecordHit{Model: "m", Dimension: "d", Item: "i"})
			So(DedupKey(ev), ShouldEqual, DedupKey(ev))
		})

		Convey("多命中指纹与命中顺序无关", func() {
			hitsA := []tagengine.TagEventRecordHit{
				{Model: "m1", Dimension: "d1", Item: "i1"},
				{Model: "m2", Dimension: "d2", Item: "i2"},
			}
			hitsB := []tagengine.TagEventRecordHit{
				{Model: "m2", Dimension: "d2", Item: "i2"},
				{Model: "m1", Dimension: "d1", Item: "i1"},
			}
			So(DedupKey(newEv("default", 1, hitsA...)), ShouldEqual, DedupKey(newEv("default", 1, hitsB...)))
		})

		Convey("不同 namespace 或不同 ruleID 指纹不同", func() {
			base := newEv("ns1", 1, tagengine.TagEventRecordHit{Model: "m", Dimension: "d", Item: "i"})
			So(DedupKey(base), ShouldNotEqual, DedupKey(newEv("ns2", 1, tagengine.TagEventRecordHit{Model: "m", Dimension: "d", Item: "i"})))
			So(DedupKey(base), ShouldNotEqual, DedupKey(newEv("ns1", 2, tagengine.TagEventRecordHit{Model: "m", Dimension: "d", Item: "i"})))
		})
	})
}

// TestEventSuppressHandler 验证抑制评估处理器：持续事件抑制、窗口过期重新告警、
// 关闭后重新告警、window=0 不抑制、同批同键去重与投递失败回留。
func TestEventSuppressHandler(t *testing.T) {
	Convey("Test EventSuppressHandler", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db := createTagEventStoreTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})

		Convey("持续事件仅首条告警：同键 alerting 锚点内 pending 被抑制且不投递", func() {
			ruleID := insertSuppressRule(db, time.Hour)
			key := "K"
			alertID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusAlerting))
			pendingID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))

			enq := &fakeEnqueuer{}
			handler := &EventSuppressHandler{coreDB: db, client: enq, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldBeNil)

			So(enq.tasks, ShouldHaveLength, 0)
			So(eventStatus(db, pendingID), ShouldEqual, tag.EventStatusSuppressed)
			So(eventStatus(db, alertID), ShouldEqual, tag.EventStatusAlerting)
		})

		Convey("超出窗口再次告警并自动关闭旧告警", func() {
			ruleID := insertSuppressRule(db, time.Hour)
			key := "K"
			oldID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusAlerting))
			setEventDetectedAt(db, oldID, time.Now().Add(-2*time.Hour))
			pendingID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))

			enq := &fakeEnqueuer{}
			handler := &EventSuppressHandler{coreDB: db, client: enq, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldBeNil)

			So(eventStatus(db, pendingID), ShouldEqual, tag.EventStatusAlerting)
			So(eventStatus(db, oldID), ShouldEqual, tag.EventStatusClosed)
			So(notifyEventIDs(enq), ShouldResemble, []uint{pendingID})
		})

		Convey("事件停止后再次发生重新告警：closed 事件不阻断告警", func() {
			ruleID := insertSuppressRule(db, time.Hour)
			key := "K"
			closedID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusClosed))
			setEventDetectedAt(db, closedID, time.Now().Add(-2*time.Hour))
			pendingID := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))

			enq := &fakeEnqueuer{}
			handler := &EventSuppressHandler{coreDB: db, client: enq, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldBeNil)

			So(eventStatus(db, pendingID), ShouldEqual, tag.EventStatusAlerting)
			So(eventStatus(db, closedID), ShouldEqual, tag.EventStatusClosed)
			So(notifyEventIDs(enq), ShouldResemble, []uint{pendingID})
		})

		Convey("window=0 不抑制：同键两条 pending 全部告警且逐事件投递", func() {
			ruleID := insertSuppressRule(db, 0)
			key := "K"
			id1 := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))
			id2 := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))

			enq := &fakeEnqueuer{}
			handler := &EventSuppressHandler{coreDB: db, client: enq, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldBeNil)

			So(eventStatus(db, id1), ShouldEqual, tag.EventStatusAlerting)
			So(eventStatus(db, id2), ShouldEqual, tag.EventStatusAlerting)
			So(notifyEventIDs(enq), ShouldResemble, []uint{id1, id2})
		})

		Convey("同批同键仅首条告警：第二条同键 pending 被抑制且仅投递首条", func() {
			ruleID := insertSuppressRule(db, time.Hour)
			key := "K"
			id1 := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))
			id2 := insertEvent(db, newEvent(ruleID, key, tag.EventStatusPending))

			enq := &fakeEnqueuer{}
			handler := &EventSuppressHandler{coreDB: db, client: enq, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldBeNil)

			So(eventStatus(db, id1), ShouldEqual, tag.EventStatusAlerting)
			So(eventStatus(db, id2), ShouldEqual, tag.EventStatusSuppressed)
			So(notifyEventIDs(enq), ShouldResemble, []uint{id1})
		})

		Convey("投递失败状态回留：事件保持 pending 且返回错误", func() {
			ruleID := insertSuppressRule(db, time.Hour)
			pendingID := insertEvent(db, newEvent(ruleID, "K", tag.EventStatusPending))

			handler := &EventSuppressHandler{coreDB: db, client: failEnqueuer{}, logger: logger}
			So(handler.Do(context.Background(), nil), ShouldNotBeNil)
			So(eventStatus(db, pendingID), ShouldEqual, tag.EventStatusPending)
		})
	})
}

// insertSuppressRuleWithBinding 写入一条命名空间 default 的规则并返回其 ID；groupID 与 targetID 用于
// 设置规则的通知组/直绑目标绑定关系（nil 表示不绑定）。
func insertSuppressRuleWithBinding(db *database.DB, groupID, targetID *uint) uint {
	rule := &tag.TagEventRule{
		NamespaceName:        "default",
		Name:                 "suppress-rule",
		Enabled:              true,
		SuppressWindow:       serializer.DurationNS(time.Hour),
		NotificationGroupID:  groupID,
		NotificationTargetID: targetID,
	}
	So(db.Create(rule).Error, ShouldBeNil)
	return rule.ID
}

// seedNotificationChannel 创建一条命名空间 default 的渠道并返回其 ID。
func seedNotificationChannel(db *database.DB) uint {
	ch := &core.NotificationChannel{NamespaceName: "default", Name: "cfg-channel", Type: "webhook", Enabled: true}
	So(db.Create(ch).Error, ShouldBeNil)
	return ch.ID
}

// seedNotificationTarget 创建归属于 channelID 的目标并返回其 ID。
func seedNotificationTarget(db *database.DB, channelID uint) uint {
	t := &core.NotificationTarget{NamespaceName: "default", Name: "cfg-target", ChannelID: channelID, Type: "webhook", Enabled: true}
	So(db.Create(t).Error, ShouldBeNil)
	return t.ID
}

// seedNotificationGroup 创建一条非默认的组并返回其 ID。
func seedNotificationGroup(db *database.DB) uint {
	g := &core.NotificationGroup{NamespaceName: "default", Name: "cfg-group", IsDefault: false}
	So(db.Create(g).Error, ShouldBeNil)
	return g.ID
}

// TestBuildSendTasks 验证 buildSendTasks 依据规则绑定关系分派发送任务：
// 直绑目标 -> notify:send:target（目标 ID 透传）；绑定通知组或未绑定 -> notify:send:group。
func TestBuildSendTasks(t *testing.T) {
	Convey("Test buildSendTasks", t, func() {
		db := createTagEventStoreTestDB(t)
		// 每个叶子外层体重建各自事务，叶子末尾回滚释放行锁，避免 PostgreSQL 下事务重叠挂死。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})

		Convey("规则直绑通知目标：构建单目标直接发送任务", func() {
			channelID := seedNotificationChannel(db)
			targetID := seedNotificationTarget(db, channelID)
			ruleID := insertSuppressRuleWithBinding(db, nil, &targetID)
			eventID := insertEvent(db, newEvent(ruleID, "K", tag.EventStatusAlerting))

			tasks, err := buildSendTasks(db, []uint{eventID})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 1)

			So(tasks[0].Type(), ShouldEqual, notifyhandler.SendTargetTask)
			var payload notify.SendTargetInput
			So(json.Unmarshal(tasks[0].Payload(), &payload), ShouldBeNil)
			So(payload.Namespace, ShouldEqual, "default")
			So(payload.TargetID, ShouldEqual, targetID)
			So(payload.EventID, ShouldEqual, eventID)
			So(payload.EventType, ShouldEqual, core.EventTypeTagEventRecord)
		})

		Convey("规则绑定通知组：仍构建通知组发送任务", func() {
			groupID := seedNotificationGroup(db)
			ruleID := insertSuppressRuleWithBinding(db, &groupID, nil)
			eventID := insertEvent(db, newEvent(ruleID, "K", tag.EventStatusAlerting))

			tasks, err := buildSendTasks(db, []uint{eventID})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 1)

			So(tasks[0].Type(), ShouldEqual, notifyhandler.SendGroupTask)
			var payload notify.SendGroupInput
			So(json.Unmarshal(tasks[0].Payload(), &payload), ShouldBeNil)
			So(payload.GroupID, ShouldNotBeNil)
			So(*payload.GroupID, ShouldEqual, groupID)
			So(payload.EventID, ShouldEqual, eventID)
		})

		Convey("规则未绑定目标与通知组：构建通知组发送任务且 GroupID 为空", func() {
			ruleID := insertSuppressRuleWithBinding(db, nil, nil)
			eventID := insertEvent(db, newEvent(ruleID, "K", tag.EventStatusAlerting))

			tasks, err := buildSendTasks(db, []uint{eventID})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 1)

			So(tasks[0].Type(), ShouldEqual, notifyhandler.SendGroupTask)
			var payload notify.SendGroupInput
			So(json.Unmarshal(tasks[0].Payload(), &payload), ShouldBeNil)
			So(payload.GroupID, ShouldBeNil)
		})
	})
}
