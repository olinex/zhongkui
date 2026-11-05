package tag

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// newTagRuleTestDB 创建共享已迁移的 PostgreSQL 测试库（testdb.Core）。
// 全部业务表已由 testdb 统一迁移，每次调用前对业务表做 TRUNCATE 保证测试隔离。
func newTagRuleTestDB(t *testing.T) *database.DB {
	t.Helper()
	return testdb.Core(t)
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
				NamespaceName: "default",
				Name:          "admin-panel",
				Description:   "识别管理后台登录",
				Model:         "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport",
				Rego:          "package zki\n\ntags contains \"admin_panel_login\" if {\n    input.method == \"POST\"\n}",
				Enabled:       true,
			}
			convey.So(rule.ID, convey.ShouldEqual, 42)
			convey.So(rule.NamespaceName, convey.ShouldEqual, "default")
			convey.So(rule.Name, convey.ShouldEqual, "admin-panel")
			convey.So(rule.Description, convey.ShouldEqual, "识别管理后台登录")
			convey.So(rule.Model, convey.ShouldEqual, "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport")
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

		convey.Convey("gorm tag 应包含唯一索引 uq_tag_rules_namespace_name_model_name 且带命名空间", func() {
			typ := reflect.TypeOf(TagRule{})
			for _, name := range []string{"NamespaceName", "Name", "Model"} {
				field, ok := typ.FieldByName(name)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(field.Tag.Get("gorm"), convey.ShouldContainSubstring, "uniqueIndex:uq_tag_rules_namespace_name_model_name")
			}
		})
	})
}

// TestTagRule_UniqueIndexColumnOrder 验证唯一索引列顺序与 SQL 约束一致
// 迁移文件（internal/migrations/core/001_init.sql）中的约束为 UNIQUE (namespace_name, model, name)，
// GORM 按结构体字段声明顺序构建索引列，因此字段顺序必须保持 NamespaceName、Model、Name。
// 说明：约束以内联表级 UNIQUE 创建，GORM 的 GetIndexes().Columns() 在 PostgreSQL 下
// 无法解析其列顺序（返回空），故直接查询 pg_get_indexdef 取回真实列顺序。
func TestTagRule_UniqueIndexColumnOrder(t *testing.T) {
	convey.Convey("测试 TagRule 唯一索引列顺序", t, func() {
		db := newTagRuleTestDB(t)
		var indexDef string
		err := db.Raw(`SELECT pg_get_indexdef(ic.oid)
			 FROM pg_class t
			 JOIN pg_index i ON i.indrelid = t.oid
			 JOIN pg_class ic ON ic.oid = i.indexrelid
			 WHERE t.relname = 'tag_rules'
			   AND ic.relname = 'uq_tag_rules_namespace_name_model_name'`).
			Scan(&indexDef).Error
		convey.So(err, convey.ShouldBeNil)
		// pg_get_indexdef 返回形如 "... USING btree (namespace_name, model, name)"，
		// 括号内列顺序即索引列顺序，须与 SQL 约束一致。
		convey.So(strings.Contains(indexDef, "(namespace_name, model, name)"), convey.ShouldBeTrue)
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
				NamespaceName: "default",
				Name:          "admin-panel",
				Description:   "识别管理后台登录",
				Model:         "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport",
				Rego:          "package zki\n\ntags contains \"admin_panel_login\" if {\n    input.method == \"POST\"\n}",
				Enabled:       true,
			}

			data, err := json.Marshal(rule)
			convey.So(err, convey.ShouldBeNil)
			// 字段未定义 json tag，Go 默认使用字段名作为 JSON 键名
			convey.So(string(data), convey.ShouldContainSubstring, `"Name"`)
			convey.So(string(data), convey.ShouldContainSubstring, `"Model"`)
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

// TestTagRule_UniqueConstraint 测试 tag_rules 表的分组唯一约束（含命名空间隔离）
func TestTagRule_UniqueConstraint(t *testing.T) {
	convey.Convey("测试 TagRule 分组唯一约束", t, func() {
		convey.Convey("同一命名空间下 (model, name) 完全相同的规则应失败", func() {
			db := newTagRuleTestDB(t)
			base := TagRule{
				NamespaceName: "default",
				Name:          "dup",
				Model:         "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport",
				Rego:          "package zki\n\ntags contains \"a\"",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			dup := base
			dup.ID = 0
			err := db.Create(&dup).Error
			convey.So(err, convey.ShouldNotBeNil)
			// 不同数据库方言错误文案不同（PostgreSQL: duplicate key，SQLite: UNIQUE constraint failed），
			// 统一以 "unique" 关键字判定唯一约束冲突。
			convey.So(strings.ToLower(err.Error()), convey.ShouldContainSubstring, "unique")
		})

		convey.Convey("不同命名空间下相同 name 的规则应插入成功", func() {
			db := newTagRuleTestDB(t)
			base := TagRule{
				NamespaceName: "default",
				Name:          "dup",
				Model:         "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport",
				Rego:          "package zki\n\ntags contains \"a\"",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.NamespaceName = "tenant-b"
			// tag_rules.namespace_name 为 namespaces(name) 外键，跨命名空间用例需先建命名空间。
			convey.So(db.Exec(
				`INSERT INTO namespaces (name) VALUES ('tenant-b') ON CONFLICT (name) DO NOTHING`,
			).Error, convey.ShouldBeNil)
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})

		convey.Convey("相同 name 但不同 model 的规则应插入成功", func() {
			db := newTagRuleTestDB(t)
			base := TagRule{
				NamespaceName: "default",
				Name:          "dup",
				Model:         "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport",
				Rego:          "package zki\n\ntags contains \"a\"",
				Enabled:       true,
			}
			convey.So(db.Create(&base).Error, convey.ShouldBeNil)

			other := base
			other.ID = 0
			other.Model = "zhongkuisec.com/zhongkui/internal/models/network.TcpStream"
			convey.So(db.Create(&other).Error, convey.ShouldBeNil)
		})
	})
}
