package tag_event

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
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
			match_all boolean NOT NULL DEFAULT false,
			description varchar(256) NOT NULL
		)`).Error, ShouldBeNil)
		So(db.Exec(`CREATE TABLE tag_event_conditions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			tag_event_rule_id integer NOT NULL,
			namespace_name varchar(32) NOT NULL,
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
		enabledRule := &tag.TagEventRule{NamespaceName: "default", Name: "enabled-rule", Enabled: true}
		So(db.Create(enabledRule).Error, ShouldBeNil)
		enabledCond := &tag.TagEventCondition{
			NamespaceName:  "default",
			TagEventRuleID: enabledRule.ID,
			Name:           "enabled-cond",
			Window:         5 * time.Minute,
			Threshold:      3,
			Expression:     "A AND B",
			Enabled:        true,
		}
		So(db.Create(enabledCond).Error, ShouldBeNil)
		So(db.Create(&tag.TagEventCondition{
			NamespaceName:  "default",
			TagEventRuleID: enabledRule.ID,
			Name:           "disabled-cond",
			Window:         5 * time.Minute,
			Threshold:      3,
			Expression:     "A",
			Enabled:        false,
		}).Error, ShouldBeNil)

		// 禁用规则：含 1 个启用条件（因规则禁用不应加载）
		disabledRule := &tag.TagEventRule{NamespaceName: "default", Name: "disabled-rule", Enabled: false}
		So(db.Create(disabledRule).Error, ShouldBeNil)
		So(db.Create(&tag.TagEventCondition{
			NamespaceName:  "default",
			TagEventRuleID: disabledRule.ID,
			Name:           "orphan-enabled-cond",
			Window:         5 * time.Minute,
			Threshold:      3,
			Expression:     "B",
			Enabled:        true,
		}).Error, ShouldBeNil)

		handler := &EventDetectHandler{coreDB: db, client: &fakeEnqueuer{}, logger: logger}
		conds, err := handler.loadConditions(context.Background())
		So(err, ShouldBeNil)
		So(conds, ShouldHaveLength, 1)
		So(conds[0].ConditionID, ShouldEqual, enabledCond.ID)
		So(conds[0].RuleID, ShouldEqual, enabledRule.ID)
		So(conds[0].RuleName, ShouldEqual, "enabled-rule")
		So(conds[0].Namespace, ShouldEqual, "default")
		So(conds[0].Expression, ShouldEqual, "A AND B")
	})
}
