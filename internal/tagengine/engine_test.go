package tagengine_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// recogModel 标签识别测试用的本地结构体，其类型键经 TypeKey 计算作为规则 Model。
type recogModel struct {
	Namespace string `json:"namespace"`
	Method    string `json:"method"`
	UserAgent string `json:"user_agent"`
}

// setupEngine 创建共享已迁移的 PostgreSQL 测试库与标签识别引擎（testdb.Core）。
// tag_rules 表已由 testdb 统一迁移。
func setupEngine(t *testing.T) (*tagengine.Engine, *database.DB) {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Core(t)
	return tagengine.NewEngine(db, logger, nil), db
}

// insertRule 插入一条启用的识别规则。
func insertRule(t *testing.T, db *database.DB, name, rego string) {
	t.Helper()
	targetType, err := tagengine.TypeKey(&recogModel{})
	if err != nil {
		t.Fatal(err)
	}
	rule := &tag.TagRule{
		NamespaceName: "default",
		Name:          name,
		Model:         targetType,
		Rego:          rego,
		Enabled:       true,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
}

// TestEngine_Recognize 验证引擎按目标模型识别并去重排序标签。
func TestEngine_Recognize(t *testing.T) {
	Convey("Test Engine Recognize", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "ua-curl", `package zki

tags contains "automation" if {
    contains(input.user_agent, "curl")
}`)
		insertRule(t, db, "method-get", `package zki

tags contains "web_request" if {
    input.method == "GET"
}`)

		Convey("命中多条规则时返回去重排序后的标签集合", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET", UserAgent: "curl/8.0"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"automation", "web_request"})
		})

		Convey("无规则命中时返回空标签集合与 nil 错误", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "POST", UserAgent: "firefox/120"})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})
	})
}

// TestEngine_NoRules 验证空规则库时返回空标签集合与 nil 错误。
func TestEngine_NoRules(t *testing.T) {
	Convey("Test Engine NoRules", t, func() {
		engine, _ := setupEngine(t)

		Convey("空库应返回空标签集合与 nil 错误", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})
	})
}

// TestEngine_CacheAndInvalidate 验证规则缓存加载与失效后识别结果的变化。
func TestEngine_CacheAndInvalidate(t *testing.T) {
	Convey("Test Engine CacheAndInvalidate", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "r1", `package zki

tags contains "old" if {
    input.method == "GET"
}`)

		Convey("规则更新后未失效缓存仍返回旧结果，失效后返回新结果", func() {
			// 预热缓存
			first, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(first, ShouldResemble, []string{"old"})

			newRego := `package zki

tags contains "new" if {
    input.method == "GET"
}`
			if err := db.Model(&tag.TagRule{}).Where("name = ?", "r1").Update("rego", newRego).Error; err != nil {
				t.Fatal(err)
			}
			// 未失效缓存：仍命中旧规则
			cached, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(cached, ShouldResemble, []string{"old"})

			// 失效后重新加载：命中新规则
			targetType, err := tagengine.TypeKey(&recogModel{})
			So(err, ShouldBeNil)
			engine.Invalidate("default", targetType)
			fresh, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(fresh, ShouldResemble, []string{"new"})
		})
	})
}

// TestEngine_RecognizeWithDetail 验证引擎返回命中的标签与逐规则识别详情。
func TestEngine_RecognizeWithDetail(t *testing.T) {
	Convey("Test Engine RecognizeWithDetail", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "r-hit", `package zki

tags contains "hit_tag" if {
    input.method == "GET"
}`)
		insertRule(t, db, "r-miss", `package zki

tags contains "miss_tag" if {
    input.method == "DELETE"
}`)

		Convey("返回命中的标签与规则详情，未命中规则标签为空", func() {
			result, err := engine.RecognizeWithDetail(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"hit_tag"})
			So(result.Rules, ShouldContainKey, "r-hit")
			So(result.Rules["r-hit"].Tags, ShouldResemble, []string{"hit_tag"})
			So(result.Rules["r-hit"].Error, ShouldBeNil)
			So(result.Rules, ShouldContainKey, "r-miss")
			So(result.Rules["r-miss"].Tags, ShouldBeEmpty)
		})
	})
}

// TestEngine_UnsupportedTarget 验证非结构体目标返回 ErrUnsupportedTarget。
func TestEngine_UnsupportedTarget(t *testing.T) {
	Convey("Test Engine UnsupportedTarget", t, func() {
		engine, _ := setupEngine(t)

		Convey("非结构体目标应返回 ErrUnsupportedTarget", func() {
			_, err := engine.Recognize(context.Background(), "not a struct")
			So(errors.Is(err, tagengine.ErrUnsupportedTarget), ShouldBeTrue)
		})
	})
}

// recogModelB 标签识别测试用的另一个目标结构体，其类型键与 recogModel 不同，
// 用于验证引擎按目标类型键分组隔离规则。
type recogModelB struct {
	Namespace string `json:"namespace"`
	Method    string `json:"method"`
}

// TestEngine_SeparateGroups 验证引擎按目标类型键分组隔离规则。
func TestEngine_SeparateGroups(t *testing.T) {
	Convey("Test Engine SeparateGroups", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "recog-rule", `package zki

tags contains "recog_tag" if {
    input.method == "GET"
}`)
		targetTypeB, err := tagengine.TypeKey(&recogModelB{})
		So(err, ShouldBeNil)
		So(db.Create(&tag.TagRule{
			NamespaceName: "default",
			Name:          "recogb-rule",
			Model:         targetTypeB,
			Rego:          "package zki\n\ntags contains \"recogb_tag\" if {\n    input.method == \"GET\"\n}",
			Enabled:       true,
		}).Error, ShouldBeNil)

		Convey("不同目标类型分组互不影响", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"recog_tag"})

			result, err = engine.Recognize(context.Background(), &recogModelB{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"recogb_tag"})
		})
	})
}

// TestEngine_NamespaceIsolation 验证识别引擎按命名空间隔离加载与失效规则：
// 同一目标类型下，不同命名空间的目标实例只命中本命名空间的规则，
// 缓存失效也只影响指定命名空间的分组。
func TestEngine_NamespaceIsolation(t *testing.T) {
	Convey("Test Engine NamespaceIsolation", t, func() {
		engine, db := setupEngine(t)
		targetType, err := tagengine.TypeKey(&recogModel{})
		So(err, ShouldBeNil)
		insertRule(t, db, "ns-default", `package zki

tags contains "default_tag" if {
    input.method == "GET"
}`)
		So(db.Create(&tag.TagRule{
			NamespaceName: "tenant-b",
			Name:          "ns-b",
			Model:         targetType,
			Rego:          "package zki\n\ntags contains \"tenant_b_tag\" if {\n    input.method == \"GET\"\n}",
			Enabled:       true,
		}).Error, ShouldBeNil)

		Convey("default 目标只命中 default 命名空间规则", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"default_tag"})
		})

		Convey("tenant-b 目标只命中 tenant-b 命名空间规则", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"tenant_b_tag"})
		})

		Convey("未显式设置命名空间的目标回退到 default 命名空间规则", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"default_tag"})
		})

		Convey("失效指定命名空间分组不影响其他命名空间缓存", func() {
			// 预热两个命名空间分组缓存
			_, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			_, err = engine.Recognize(context.Background(), &recogModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)

			// 同时修改两个命名空间的规则，但仅失效 tenant-b 分组
			if err := db.Model(&tag.TagRule{}).
				Where("namespace_name = ? AND name = ?", "tenant-b", "ns-b").
				Update("rego", "package zki\n\ntags contains \"tenant_b_new\" if {\n    input.method == \"GET\"\n}").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&tag.TagRule{}).
				Where("namespace_name = ? AND name = ?", "default", "ns-default").
				Update("rego", "package zki\n\ntags contains \"default_new\" if {\n    input.method == \"GET\"\n}").Error; err != nil {
				t.Fatal(err)
			}
			engine.Invalidate("tenant-b", targetType)

			// tenant-b 分组被失效，重新加载命中新规则
			fresh, err := engine.Recognize(context.Background(), &recogModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)
			So(fresh, ShouldResemble, []string{"tenant_b_new"})

			// default 分组缓存未被失效，仍命中旧规则（证明失效按命名空间隔离）
			cached, err := engine.Recognize(context.Background(), &recogModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(cached, ShouldResemble, []string{"default_tag"})
		})
	})
}
