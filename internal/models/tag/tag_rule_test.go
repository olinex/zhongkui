package tag

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// newTagRuleTestDB 创建 sqlite 内存库并迁移 tag_rules 表
// 内存库必须限制为单连接，避免多个连接各自持有独立的内存数据库
func newTagRuleTestDB(t *testing.T) *database.DB {
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
	if err := db.AutoMigrate(&TagRule{}); err != nil {
		t.Fatal("failed to migrate test database: " + err.Error())
	}
	return db
}

// TestTagRule_Struct 测试 TagRule 结构体的基本属性
func TestTagRule_Struct(t *testing.T) {
	convey.Convey("测试 TagRule 结构体", t, func() {
		convey.Convey("应该正确初始化所有字段", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			rule := &TagRule{
				BaseModel: database.BaseModel{
					ID:        42,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				},
				Name:        "admin-panel",
				Description: "识别管理后台登录",
				TargetType:  "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:     "risk",
				Rego:        "package zki\n\ntags contains \"admin_panel_login\" if {\n    input.method == \"POST\"\n}",
				Enabled:     true,
			}
			convey.So(rule.ID, convey.ShouldEqual, 42)
			convey.So(rule.Name, convey.ShouldEqual, "admin-panel")
			convey.So(rule.Description, convey.ShouldEqual, "识别管理后台登录")
			convey.So(rule.TargetType, convey.ShouldEqual, "zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			convey.So(rule.TagType, convey.ShouldEqual, "risk")
			convey.So(rule.Rego, convey.ShouldContainSubstring, "package zki")
			convey.So(rule.Enabled, convey.ShouldBeTrue)
		})

		convey.Convey("Enabled 默认语义", func() {
			convey.Convey("Go 结构体零值为 false", func() {
				rule := &TagRule{}
				// GORM 的 default:true 标签只在数据库层面生效
				// 在 Go 结构体中，布尔字段的默认值是 false
				convey.So(rule.Enabled, convey.ShouldBeFalse)
			})

			convey.Convey("gorm tag 声明数据库层默认值为 true", func() {
				field, ok := reflect.TypeOf(TagRule{}).FieldByName("Enabled")
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "default:true")
			})
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_tag_rule_group", func() {
			typ := reflect.TypeOf(TagRule{})
			for _, name := range []string{"Name", "TargetType", "TagType"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_tag_rule_group")
			}
		})
	})
}

// TestTagRule_JSONSerialization 测试 TagRule 的 JSON 序列化与反序列化往返
func TestTagRule_JSONSerialization(t *testing.T) {
	convey.Convey("测试 TagRule JSON 序列化", t, func() {
		convey.Convey("序列化后再反序列化应保持字段一致", func() {
			now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
			rule := &TagRule{
				BaseModel: database.BaseModel{
					ID:        7,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Name:        "admin-panel",
				Description: "识别管理后台登录",
				TargetType:  "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:     "risk",
				Rego:        "package zki\n\ntags contains \"admin_panel_login\" if {\n    input.method == \"POST\"\n}",
				Enabled:     true,
			}

			data, err := json.Marshal(rule)
			convey.So(err, convey.ShouldBeNil)
			// 字段未定义 json tag，Go 默认使用字段名作为 JSON 键名
			convey.So(string(data), convey.ShouldContainSubstring, `"Name"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"TargetType"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"TagType"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"Rego"`)

			var decoded TagRule
			convey.So(json.Unmarshal(data, &decoded), convey.ShouldBeNil)
			convey.So(decoded, convey.ShouldResemble, *rule)
		})
	})
}

// TestTagRule_TableName 测试 GORM 默认表名解析
func TestTagRule_TableName(t *testing.T) {
	convey.Convey("测试 TagRule 表名", t, func() {
		convey.Convey("GORM 默认表名应为 tag_rules", func() {
			db := newTagRuleTestDB(t)
			stmt := db.Model(&TagRule{})
			convey.So(stmt.Statement.Parse(&TagRule{}), convey.ShouldBeNil)
			convey.So(stmt.Statement.Table, convey.ShouldEqual, "tag_rules")
		})
	})
}

// TestTagRule_UniqueConstraint 测试 tag_rules 表的分组唯一约束
func TestTagRule_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 TagRule 分组唯一约束", t, func() {
		convey.Convey("插入 (target_type, tag_type, name) 完全相同的规则应失败", func() {
			db := newTagRuleTestDB(t)
			base := TagRule{
				Name:       "dup",
				TargetType: "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:    "risk",
				Rego:       "package zki\n\ntags contains \"a\"",
				Enabled:    true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("相同 name 但不同 tag_type 的规则应插入成功", func() {
			db := newTagRuleTestDB(t)
			base := TagRule{
				Name:       "dup",
				TargetType: "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:    "risk",
				Rego:       "package zki\n\ntags contains \"a\"",
				Enabled:    true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.TagType = "asset"
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}
