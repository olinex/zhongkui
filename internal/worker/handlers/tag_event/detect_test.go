package tag_event

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// fakeEnqueuer 实现 eventEnqueuer 接口，追加记录投递的任务供断言使用。
type fakeEnqueuer struct {
	tasks []*asynq.Task
}

// EnqueueContext 记录投递的任务并返回固定 TaskInfo。
func (f *fakeEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{ID: "test"}, nil
}

// TestParseCursorTaskPayload 验证从任务载荷解析游标时间点与固定参数。
func TestParseCursorTaskPayload(t *testing.T) {
	Convey("Test ParseCursorTaskPayload", t, func() {
		trigger := time.Date(2026, 8, 11, 9, 5, 0, 0, time.UTC)
		raw, err := json.Marshal(pipeline.CursorTaskPayload{TriggerAt: trigger, Payload: map[string]any{"region": "cn"}})
		So(err, ShouldBeNil)

		Convey("游标载荷：返回 TriggerAt 与 Payload", func() {
			triggerAt, payload, err := pipeline.ParseCursorTaskPayload(raw)
			So(err, ShouldBeNil)
			So(triggerAt.Equal(trigger), ShouldBeTrue)
			So(payload["region"], ShouldEqual, "cn")
		})

		Convey("空载荷：返回错误", func() {
			_, _, err := pipeline.ParseCursorTaskPayload(nil)
			So(err, ShouldNotBeNil)
		})

		Convey("非法 JSON 载荷：返回错误", func() {
			_, _, err := pipeline.ParseCursorTaskPayload([]byte("not-json"))
			So(err, ShouldNotBeNil)
		})

		Convey("非游标载荷（旧版配置 JSON）：解析成功但 TriggerAt 为零值", func() {
			triggerAt, _, err := pipeline.ParseCursorTaskPayload([]byte(`{"cronSpec":"* * * * *","enabled":true}`))
			So(err, ShouldBeNil)
			So(triggerAt.IsZero(), ShouldBeTrue)
		})
	})
}

// TestEventDetectHandler_LoadConditions 验证仅加载启用规则下的启用条件。
func TestEventDetectHandler_LoadConditions(t *testing.T) {
	Convey("Test EventDetectHandler LoadConditions", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
		So(err, ShouldBeNil)
		if sqlDB, err := db.DB(); err != nil {
			t.Fatal(err)
		} else {
			sqlDB.SetMaxOpenConns(1)
		}
		// sqlite 下 GORM AutoMigrate 处理多级 belongs-to 关联（tag_event_rules -> tag_event_conditions）
		// 会触发 nil 指针 panic，因此按 core 迁移（001_init.sql）的列定义手写建表。
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
		So(db.Exec(`CREATE TABLE tag_event_conditions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			tag_event_rule_id integer NOT NULL,
			model varchar(255) NOT NULL DEFAULT '',
			dimension varchar(128) NOT NULL DEFAULT '',
			item varchar(255) NOT NULL DEFAULT '',
			name varchar(255) NOT NULL,
			"window" bigint NOT NULL,
			threshold int NOT NULL,
			expression varchar(1024) NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			description varchar(256) NOT NULL
		)`).Error, ShouldBeNil)

		// 启用规则：含 1 个启用条件 + 1 个禁用条件
		enabledRule := &tag.TagEventRule{
			NamespaceName:  "default",
			Name:           "enabled-rule",
			Enabled:        true,
			SuppressWindow: serializer.DurationNS(time.Hour),
		}
		So(db.Create(enabledRule).Error, ShouldBeNil)
		enabledCond := &tag.TagEventCondition{
			TagEventRuleID: enabledRule.ID,
			Name:           "enabled-cond",
			Window:         serializer.DurationNS(5 * time.Minute),
			Threshold:      3,
			Expression:     "A AND B",
			Enabled:        true,
		}
		So(db.Create(enabledCond).Error, ShouldBeNil)
		So(db.Create(&tag.TagEventCondition{
			TagEventRuleID: enabledRule.ID,
			Name:           "disabled-cond",
			Window:         serializer.DurationNS(5 * time.Minute),
			Threshold:      3,
			Expression:     "A",
			Enabled:        false,
		}).Error, ShouldBeNil)

		// 禁用规则：含 1 个启用条件（因规则禁用不应加载）
		disabledRule := &tag.TagEventRule{NamespaceName: "default", Name: "disabled-rule", Enabled: false}
		So(db.Create(disabledRule).Error, ShouldBeNil)
		So(db.Create(&tag.TagEventCondition{
			TagEventRuleID: disabledRule.ID,
			Name:           "orphan-enabled-cond",
			Window:         serializer.DurationNS(5 * time.Minute),
			Threshold:      3,
			Expression:     "B",
			Enabled:        true,
		}).Error, ShouldBeNil)

		handler := &EventDetectHandler{coreDB: db, logger: logger}
		conds, err := handler.loadConditions(context.Background())
		So(err, ShouldBeNil)
		So(conds, ShouldHaveLength, 1)
		So(conds[0].ConditionID, ShouldEqual, enabledCond.ID)
		So(conds[0].RuleID, ShouldEqual, enabledRule.ID)
		So(conds[0].RuleName, ShouldEqual, "enabled-rule")
		So(conds[0].Namespace, ShouldEqual, "default")
		So(conds[0].Expression, ShouldEqual, "A AND B")
		So(conds[0].SuppressWindow, ShouldEqual, serializer.DurationNS(time.Hour))
	})
}

// TestEventDetectHandler_PersistEvents 验证检测命中事件批量入库：事件状态固定 pending、
// dedup_key 等于 DedupKey(ev)、detected_at 透传，命中明细与所属事件正确关联。
func TestEventDetectHandler_PersistEvents(t *testing.T) {
	Convey("Test EventDetectHandler persistEvents", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db := createTagEventStoreTestDB(t)
		// 构造不含 client 的处理器：编译期即证明检测处理器不再有任何任务投递能力
		handler := &EventDetectHandler{coreDB: db, logger: logger}

		detectedAt := time.Date(2026, 8, 11, 9, 5, 0, 0, time.UTC)
		events := []*tagengine.TagEventRecord{
			{
				Namespace: "default", RuleID: 1, RuleName: "rule-a", MatchAll: false,
				DetectedAt: detectedAt,
				Hits: []tagengine.TagEventRecordHit{
					{ConditionID: 1, ConditionName: "cond-1", Model: "m1", Dimension: "d1", Item: "i1", Count: 5, Threshold: 3, Window: serializer.DurationNS(5 * time.Minute)},
				},
			},
			{
				Namespace: "default", RuleID: 2, RuleName: "rule-b", MatchAll: true,
				DetectedAt: detectedAt.Add(time.Minute),
				Hits: []tagengine.TagEventRecordHit{
					{ConditionID: 2, ConditionName: "cond-2", Model: "m2", Dimension: "d2", Item: "i2", Count: 8, Threshold: 4, Window: serializer.DurationNS(10 * time.Minute)},
					{ConditionID: 3, ConditionName: "cond-3", Model: "m3", Dimension: "d3", Item: "i3", Count: 1, Threshold: 1, Window: serializer.DurationNS(5 * time.Minute)},
				},
			},
		}

		So(handler.persistEvents(context.Background(), events), ShouldBeNil)

		Convey("tag_event_records 新增 2 行：pending、dedup_key 正确、detected_at 透传", func() {
			var rows []*tag.TagEventRecord
			So(db.Order("id ASC").Find(&rows).Error, ShouldBeNil)
			So(rows, ShouldHaveLength, 2)
			for i, ev := range events {
				So(rows[i].NamespaceName, ShouldEqual, ev.Namespace)
				So(rows[i].RuleID, ShouldEqual, ev.RuleID)
				So(rows[i].RuleName, ShouldEqual, ev.RuleName)
				So(rows[i].MatchAll, ShouldEqual, ev.MatchAll)
				So(rows[i].Status, ShouldEqual, tag.EventStatusPending)
				So(rows[i].DedupKey, ShouldNotBeEmpty)
				So(rows[i].DedupKey, ShouldEqual, DedupKey(ev))
				So(rows[i].DetectedAt.Equal(ev.DetectedAt), ShouldBeTrue)
			}
		})

		Convey("tag_event_record_hits 新增对应明细行且 TagEventRecordID 关联正确", func() {
			var hits []*tag.TagEventRecordHit
			So(db.Order("id ASC").Find(&hits).Error, ShouldBeNil)
			So(hits, ShouldHaveLength, 3)

			So(hits[0].TagEventRecordID, ShouldEqual, rowsID(db, 1))
			So(hits[0].ConditionID, ShouldEqual, uint(1))
			So(hits[0].ConditionName, ShouldEqual, "cond-1")
			So(hits[0].Model, ShouldEqual, "m1")
			So(hits[0].Count, ShouldEqual, uint64(5))
			So(hits[0].Threshold, ShouldEqual, 3)
			So(hits[0].Window, ShouldEqual, serializer.DurationNS(5*time.Minute))

			So(hits[1].TagEventRecordID, ShouldEqual, rowsID(db, 2))
			So(hits[1].ConditionID, ShouldEqual, uint(2))
			So(hits[1].Model, ShouldEqual, "m2")

			So(hits[2].TagEventRecordID, ShouldEqual, rowsID(db, 2))
			So(hits[2].ConditionID, ShouldEqual, uint(3))
			So(hits[2].Model, ShouldEqual, "m3")
		})
	})
}

// rowsID 查询 tag_event_records 第 n 行（按 id 升序）的主键。
func rowsID(db *database.DB, n int) uint {
	var id uint
	So(db.Model(&tag.TagEventRecord{}).Order("id ASC").Offset(n-1).Limit(1).Pluck("id", &id).Error, ShouldBeNil)
	return id
}
