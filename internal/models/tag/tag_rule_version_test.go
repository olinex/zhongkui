package tag

import (
	"reflect"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// TestTagRuleVersion_Struct 测试 TagRuleVersion 结构体的基本属性
func TestTagRuleVersion_Struct(t *testing.T) {
	convey.Convey("测试 TagRuleVersion 结构体", t, func() {
		convey.Convey("所有字段应正确初始化", func() {
			v := &TagRuleVersion{
				NamespaceName: "default",
				RuleID:        7,
				Version:       3,
				Name:          "admin-panel",
				Description:   "识别管理后台登录",
				TargetType:    "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:       "risk",
				Rego:          "package zki\n\ntags contains \"admin_panel_login\"",
				Enabled:       true,
			}
			convey.So(v.NamespaceName, convey.ShouldEqual, "default")
			convey.So(v.RuleID, convey.ShouldEqual, 7)
			convey.So(v.Version, convey.ShouldEqual, 3)
			convey.So(v.Name, convey.ShouldEqual, "admin-panel")
			convey.So(v.TargetType, convey.ShouldEqual, "zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			convey.So(v.TagType, convey.ShouldEqual, "risk")
			convey.So(v.Rego, convey.ShouldContainSubstring, "package zki")
			convey.So(v.Enabled, convey.ShouldBeTrue)
		})

		convey.Convey("MaxTagRuleVersions 常量应为 30", func() {
			convey.So(MaxTagRuleVersions, convey.ShouldEqual, 30)
		})

		convey.Convey("gorm tag 应包含唯一索引 uq_tag_rule_version 与级联删除约束", func() {
			typ := reflect.TypeOf(TagRuleVersion{})
			ruleIDField, ok := typ.FieldByName("RuleID")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(ruleIDField.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_tag_rule_version")
			convey.So(ruleIDField.Tag.Get("gorm"), convey.ShouldContainSubstring, "OnDelete:CASCADE")

			versionField, ok := typ.FieldByName("Version")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(versionField.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_tag_rule_version")
		})
	})
}

// TestTagRuleVersion_TableName 测试 GORM 默认表名解析
func TestTagRuleVersion_TableName(t *testing.T) {
	convey.Convey("测试 TagRuleVersion 表名", t, func() {
		convey.Convey("GORM 默认表名应为 tag_rule_versions", func() {
			db := newTagRuleTestDB(t)
			if err := db.AutoMigrate(&TagRuleVersion{}); err != nil {
				t.Fatal("failed to migrate tag_rule_versions: " + err.Error())
			}
			stmt := db.Model(&TagRuleVersion{})
			convey.So(stmt.Statement.Parse(&TagRuleVersion{}), convey.ShouldBeNil)
			convey.So(stmt.Statement.Table, convey.ShouldEqual, "tag_rule_versions")
		})
	})
}

// TestTagRuleVersion_UniqueConstraint 测试 tag_rule_versions 表的 (rule_id, version) 唯一约束
func TestTagRuleVersion_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 TagRuleVersion 唯一约束", t, func() {
		convey.Convey("插入 (rule_id, version) 完全相同的版本应失败", func() {
			db := newTagRuleTestDB(t)
			if err := db.AutoMigrate(&TagRuleVersion{}); err != nil {
				t.Fatal("failed to migrate tag_rule_versions: " + err.Error())
			}
			base := TagRuleVersion{
				NamespaceName: "default",
				RuleID:        1,
				Version:       1,
				Name:          "r1",
				TargetType:    "zhongkui.com/zhongkui/internal/models/network.HttpTransport",
				TagType:       "risk",
				Rego:          "package zki\n\ntags contains \"a\"",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "UNIQUE constraint failed"), convey.ShouldBeTrue)
		})

		convey.Convey("相同 rule_id 不同 version 应插入成功", func() {
			db := newTagRuleTestDB(t)
			if err := db.AutoMigrate(&TagRuleVersion{}); err != nil {
				t.Fatal("failed to migrate tag_rule_versions: " + err.Error())
			}
			v1 := TagRuleVersion{NamespaceName: "default", RuleID: 1, Version: 1, Name: "r1", TargetType: "t", TagType: "risk", Rego: "package zki\n\ntags contains \"a\"", Enabled: true}
			convey.So(db.Create(&v1).Error, convey.ShouldBeNil)
			v2 := v1
			v2.ID = 0
			v2.Version = 2
			convey.So(db.Create(&v2).Error, convey.ShouldBeNil)
		})
	})
}
