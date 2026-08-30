package tag_event

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/tagengine"
	notifyhandler "zhongkui.com/zhongkui/internal/worker/handlers/notify"
)

// failEnqueuer 实现 eventEnqueuer 接口，EnqueueContext 始终返回错误，用于投递失败场景。
type failEnqueuer struct{}

// EnqueueContext 固定返回错误，模拟 notify 任务投递失败。
func (failEnqueuer) EnqueueContext(_ context.Context, _ *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, errors.New("enqueue failed")
}

// createTagEventStoreTestDB 创建 sqlite 内存库并手写建表 tag_event_rules / tag_event_records / tag_event_record_hits。
// 与 detect_test 一致：sqlite 下 GORM AutoMigrate 处理多级关联会触发 nil 指针 panic，
// 因此按 core 迁移（001_init.sql）的列定义手写建表；内存库必须限制为单连接。
func createTagEventStoreTestDB(t *testing.T) *database.DB {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(1)
	}
	// 启用外键约束，使 tag_event_record_hits 对 records 的级联删除与生产 PG 迁移一致（superfluous in-memory PRAGMA）
	So(db.Exec("PRAGMA foreign_keys = ON").Error, ShouldBeNil)
	So(db.Exec(`CREATE TABLE tag_event_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		namespace_name varchar(32) NOT NULL,
		name varchar(255) NOT NULL,
		enabled boolean NOT NULL DEFAULT true,
		notification_group_id int,
		notification_target_id int,
		match_all boolean NOT NULL DEFAULT false,
		suppress_window bigint NOT NULL DEFAULT 0,
		description varchar(256) NOT NULL
	)`).Error, ShouldBeNil)
	So(db.Exec(`CREATE TABLE tag_event_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		namespace_name varchar(32) NOT NULL,
		rule_id int,
		rule_name varchar(255),
		match_all boolean,
		dedup_key char(64),
		status varchar(16),
		detected_at datetime
	)`).Error, ShouldBeNil)
	So(db.Exec(`CREATE TABLE tag_event_record_hits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at datetime NOT NULL,
		updated_at datetime NOT NULL,
		tag_event_record_id bigint REFERENCES tag_event_records(id) ON DELETE CASCADE,
		condition_id int,
		condition_name varchar(255),
		model varchar(255),
		dimension varchar(128),
		item varchar(255),
		count bigint,
		threshold int,
		"window" bigint
	)`).Error, ShouldBeNil)
	return db
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

// TestBuildSendTasks 验证 buildSendTasks 依据规则绑定关系分派发送任务：
// 直绑目标 -> notify:send:target（目标 ID 透传）；绑定通知组或未绑定 -> notify:send:group。
func TestBuildSendTasks(t *testing.T) {
	Convey("Test buildSendTasks", t, func() {
		db := createTagEventStoreTestDB(t)

		Convey("规则直绑通知目标：构建单目标直接发送任务", func() {
			targetID := uint(42)
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
			groupID := uint(7)
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
