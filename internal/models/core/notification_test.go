package core

import (
	// stdlib
	"reflect"
	"strings"
	"testing"
	"time"

	// third-party
	"github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// newNotificationTestDB 创建 sqlite 内存库并迁移通知相关的五张表：
// notification_channels / notification_targets / notification_groups /
// notification_group_targets / notification_records
// 内存库必须限制为单连接，避免多个连接各自持有独立的内存数据库
func newNotificationTestDB(t *testing.T) *database.DB {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal("failed to create default logger: " + err.Error())
	}
	db, err := database.NewDatabase(&database.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
	if err != nil {
		t.Fatal("failed to create test database: " + err.Error())
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatal("failed to get underlying sql.DB: " + err.Error())
	} else {
		// sqlite 内存库强制单连接，避免多连接各自持有独立的内存数据库
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&NotificationChannel{}, &NotificationTarget{}, &NotificationGroup{}, &NotificationGroupTarget{}, &NotificationRecord{}); err != nil {
		t.Fatal("failed to migrate test database: " + err.Error())
	}
	return db
}

// TestNotification_TableName 测试 GORM 默认命名策略生成的表名与迁移 SQL 的
// notification_* 表名一致（模型不再显式声明 TableName，由默认命名策略保证）。
func TestNotification_TableName(t *testing.T) {
	convey.Convey("通知模型使用 GORM 默认命名，表名与迁移 SQL 一致", t, func() {
		db := newNotificationTestDB(t)
		for _, table := range []string{
			"notification_channels",
			"notification_targets",
			"notification_groups",
			"notification_group_targets",
			"notification_records",
		} {
			convey.So(db.Migrator().HasTable(table), convey.ShouldBeTrue)
		}
	})
}

// TestNotificationRecord_StatusConstants 测试通知记录状态常量取值
func TestNotificationRecord_StatusConstants(t *testing.T) {
	convey.Convey("测试通知记录状态常量", t, func() {
		convey.So(RecordStatusPending, convey.ShouldEqual, "pending")
		convey.So(RecordStatusSuccess, convey.ShouldEqual, "success")
		convey.So(RecordStatusFailed, convey.ShouldEqual, "failed")
	})
}

// TestNotificationChannel_Struct 测试 NotificationChannel 结构体的基本属性
func TestNotificationChannel_Struct(t *testing.T) {
	convey.Convey("测试 NotificationChannel 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			channel := &NotificationChannel{
				BaseModel: database.BaseModel{
					ID:        1,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				},
				NamespaceName: "default",
				Name:          "alert-email",
				Type:          "email",
				Enabled:       true,
				Config:        serializer.NewCrypto([]byte(`{"smtp":{"host":"smtp.example.com"}}`)),
				Description:   "发送告警邮件",
			}
			convey.So(channel.ID, convey.ShouldEqual, 1)
			convey.So(channel.NamespaceName, convey.ShouldEqual, "default")
			convey.So(channel.Name, convey.ShouldEqual, "alert-email")
			convey.So(channel.Type, convey.ShouldEqual, "email")
			convey.So(channel.Enabled, convey.ShouldBeTrue)
			convey.So(string(channel.ConfigData()), convey.ShouldContainSubstring, "smtp")
			convey.So(channel.Description, convey.ShouldEqual, "发送告警邮件")
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_notification_channels_namespace_name", func() {
			typ := reflect.TypeOf(NotificationChannel{})
			for _, name := range []string{"NamespaceName", "Name"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_notification_channels_namespace_name")
			}
		})

		convey.Convey("gorm tag 应包含命名空间普通索引 idx_notification_channels_namespace", func() {
			field, ok := reflect.TypeOf(NotificationChannel{}).FieldByName("NamespaceName")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_channels_namespace")
		})
	})
}

// TestNotificationChannel_UniqueConstraint 测试 notification_channels 表的唯一约束
func TestNotificationChannel_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 NotificationChannel 唯一约束", t, func() {
		convey.Convey("同一命名空间下相同 name 的渠道应插入失败", func() {
			db := newNotificationTestDB(t)
			base := NotificationChannel{
				NamespaceName: "default",
				Name:          "alert-email",
				Type:          "email",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("不同命名空间下相同 name 的渠道应插入成功", func() {
			db := newNotificationTestDB(t)
			base := NotificationChannel{
				NamespaceName: "default",
				Name:          "alert-email",
				Type:          "email",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.NamespaceName = "tenant-b"
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}

// TestNotificationTarget_Struct 测试 NotificationTarget 结构体的基本属性
func TestNotificationTarget_Struct(t *testing.T) {
	convey.Convey("测试 NotificationTarget 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			target := &NotificationTarget{
				NamespaceName: "default",
				Name:          "ops-email",
				ChannelID:     3,
				Type:          "email",
				Enabled:       true,
				Config:        serializer.NewCrypto([]byte(`{"to":"ops@example.com"}`)),
				Description:   "运维告警邮箱",
			}
			convey.So(target.NamespaceName, convey.ShouldEqual, "default")
			convey.So(target.Name, convey.ShouldEqual, "ops-email")
			convey.So(target.ChannelID, convey.ShouldEqual, 3)
			convey.So(target.Type, convey.ShouldEqual, "email")
			convey.So(target.Enabled, convey.ShouldBeTrue)
			convey.So(string(target.ConfigData()), convey.ShouldContainSubstring, "ops@example.com")
			convey.So(target.Description, convey.ShouldEqual, "运维告警邮箱")
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_notification_targets_namespace_name", func() {
			typ := reflect.TypeOf(NotificationTarget{})
			for _, name := range []string{"NamespaceName", "Name"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_notification_targets_namespace_name")
			}
		})

		convey.Convey("gorm tag 应包含命名空间普通索引 idx_notification_targets_namespace", func() {
			field, ok := reflect.TypeOf(NotificationTarget{}).FieldByName("NamespaceName")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_targets_namespace")
		})

		convey.Convey("gorm tag 应包含 channel_id 普通索引 idx_notification_targets_channel_id", func() {
			field, ok := reflect.TypeOf(NotificationTarget{}).FieldByName("ChannelID")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_targets_channel_id")
		})
	})
}

// TestNotificationTarget_UniqueConstraint 测试 notification_targets 表的唯一约束
func TestNotificationTarget_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 NotificationTarget 唯一约束", t, func() {
		convey.Convey("同一命名空间下相同 name 的目标应插入失败", func() {
			db := newNotificationTestDB(t)
			base := NotificationTarget{
				NamespaceName: "default",
				Name:          "ops-email",
				ChannelID:     3,
				Type:          "email",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("不同命名空间下相同 name 的目标应插入成功", func() {
			db := newNotificationTestDB(t)
			base := NotificationTarget{
				NamespaceName: "default",
				Name:          "ops-email",
				ChannelID:     3,
				Type:          "email",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.NamespaceName = "tenant-b"
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}

// TestNotificationGroup_Struct 测试 NotificationGroup 结构体的基本属性
func TestNotificationGroup_Struct(t *testing.T) {
	convey.Convey("测试 NotificationGroup 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			group := &NotificationGroup{
				NamespaceName: "default",
				Name:          "default-group",
				IsDefault:     true,
				Description:   "默认通知组",
			}
			convey.So(group.NamespaceName, convey.ShouldEqual, "default")
			convey.So(group.Name, convey.ShouldEqual, "default-group")
			convey.So(group.IsDefault, convey.ShouldBeTrue)
			convey.So(group.Description, convey.ShouldEqual, "默认通知组")
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_notification_groups_namespace_name", func() {
			typ := reflect.TypeOf(NotificationGroup{})
			for _, name := range []string{"NamespaceName", "Name"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_notification_groups_namespace_name")
			}
		})

		convey.Convey("gorm tag 应包含 is_default 列标签", func() {
			field, ok := reflect.TypeOf(NotificationGroup{}).FieldByName("IsDefault")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "column:is_default")
		})
	})
}

// TestNotificationGroup_UniqueConstraint 测试 notification_groups 表的唯一约束
func TestNotificationGroup_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 NotificationGroup 唯一约束", t, func() {
		convey.Convey("同一命名空间下相同 name 的组应插入失败", func() {
			db := newNotificationTestDB(t)
			base := NotificationGroup{
				NamespaceName: "default",
				Name:          "group-a",
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("不同命名空间下相同 name 的组应插入成功", func() {
			db := newNotificationTestDB(t)
			base := NotificationGroup{
				NamespaceName: "default",
				Name:          "group-a",
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.NamespaceName = "tenant-b"
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}

// TestNotificationGroupTarget_Struct 测试 NotificationGroupTarget 结构体的基本属性
func TestNotificationGroupTarget_Struct(t *testing.T) {
	convey.Convey("测试 NotificationGroupTarget 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			rel := &NotificationGroupTarget{
				GroupID:  1,
				TargetID: 2,
			}
			convey.So(rel.GroupID, convey.ShouldEqual, 1)
			convey.So(rel.TargetID, convey.ShouldEqual, 2)
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_notification_group_targets_group_target", func() {
			typ := reflect.TypeOf(NotificationGroupTarget{})
			for _, name := range []string{"GroupID", "TargetID"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_notification_group_targets_group_target")
			}
		})

		convey.Convey("gorm tag 应包含 target_id 普通索引 idx_notification_group_targets_target_id", func() {
			field, ok := reflect.TypeOf(NotificationGroupTarget{}).FieldByName("TargetID")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_group_targets_target_id")
		})
	})
}

// TestNotificationGroupTarget_UniqueConstraint 测试 notification_group_targets 表的唯一约束
func TestNotificationGroupTarget_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 NotificationGroupTarget 唯一约束", t, func() {
		convey.Convey("同一 group_id + target_id 的重复关联应插入失败", func() {
			db := newNotificationTestDB(t)
			base := NotificationGroupTarget{
				GroupID:  1,
				TargetID: 2,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("同一目标加入不同组应插入成功", func() {
			db := newNotificationTestDB(t)
			base := NotificationGroupTarget{
				GroupID:  1,
				TargetID: 2,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.GroupID = 3
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}

// TestNotificationRecord_Struct 测试 NotificationRecord 结构体的基本属性
func TestNotificationRecord_Struct(t *testing.T) {
	convey.Convey("测试 NotificationRecord 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			record := &NotificationRecord{
				BaseModel: database.BaseModel{
					ID:        1,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				},
				NamespaceName: "default",
				EventID:       42,
				GroupID:       5,
				TargetID:      9,
				ChannelID:     7,
				ChannelType:   "email",
				Status:        RecordStatusPending,
				MessageID:     "msg-1",
				Payload:       `{"title":"alert"}`,
			}
			convey.So(record.ID, convey.ShouldEqual, 1)
			convey.So(record.NamespaceName, convey.ShouldEqual, "default")
			convey.So(record.EventID, convey.ShouldEqual, 42)
			convey.So(record.GroupID, convey.ShouldEqual, 5)
			convey.So(record.TargetID, convey.ShouldEqual, 9)
			convey.So(record.ChannelID, convey.ShouldEqual, 7)
			convey.So(record.ChannelType, convey.ShouldEqual, "email")
			convey.So(record.Status, convey.ShouldEqual, "pending")
			convey.So(record.MessageID, convey.ShouldEqual, "msg-1")
			convey.So(record.Payload, convey.ShouldContainSubstring, "title")
			convey.So(record.NotifiedAt, convey.ShouldBeNil)
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_notification_records_event_target", func() {
			typ := reflect.TypeOf(NotificationRecord{})
			for _, name := range []string{"EventID", "TargetID"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_notification_records_event_target")
			}
		})

		convey.Convey("gorm tag 应包含 group_id 列标签", func() {
			field, ok := reflect.TypeOf(NotificationRecord{}).FieldByName("GroupID")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "column:group_id")
		})

		convey.Convey("gorm tag 应包含 channel_id 普通索引 idx_notification_records_channel_id", func() {
			field, ok := reflect.TypeOf(NotificationRecord{}).FieldByName("ChannelID")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_records_channel_id")
		})

		convey.Convey("gorm tag 应包含 namespace+created_at 组合索引 idx_notification_records_namespace_created_at", func() {
			field, ok := reflect.TypeOf(NotificationRecord{}).FieldByName("NamespaceName")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "index:idx_notification_records_namespace_created_at")
		})
	})
}

// TestNotificationRecord_UniqueConstraint 测试 notification_records 表的唯一约束
func TestNotificationRecord_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 NotificationRecord 唯一约束", t, func() {
		convey.Convey("同一 event_id + target_id 的重复记录应插入失败", func() {
			db := newNotificationTestDB(t)
			base := NotificationRecord{
				NamespaceName: "default",
				EventID:       42,
				TargetID:      9,
				ChannelID:     7,
				ChannelType:   "email",
				Status:        RecordStatusSuccess,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("不同 target_id 的相同 event_id 记录应插入成功", func() {
			db := newNotificationTestDB(t)
			base := NotificationRecord{
				NamespaceName: "default",
				EventID:       42,
				TargetID:      9,
				ChannelID:     7,
				ChannelType:   "email",
				Status:        RecordStatusSuccess,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.TargetID = 10
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}
