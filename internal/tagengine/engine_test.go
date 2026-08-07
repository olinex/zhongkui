package tagengine_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// recogModel 标签识别测试用的本地结构体，其类型键经 TypeKey 计算作为规则 TargetType。
type recogModel struct {
	Method    string `json:"method"`
	UserAgent string `json:"user_agent"`
}

// setupEngine 创建内存 sqlite 数据库与标签识别引擎。
// :memory: sqlite 每个连接独立，限制单连接保证读写指向同一数据库。
func setupEngine(t *testing.T) (*tagengine.Engine, *database.DB) {
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
	if err := db.AutoMigrate(&tag.TagRule{}); err != nil {
		t.Fatal(err)
	}
	return tagengine.NewEngine(db, logger), db
}

// insertRule 插入一条启用的识别规则。
func insertRule(t *testing.T, db *database.DB, name, tagType, rego string) {
	t.Helper()
	targetType, err := tagengine.TypeKey(&recogModel{})
	if err != nil {
		t.Fatal(err)
	}
	rule := &tag.TagRule{
		Name:       name,
		TargetType: targetType,
		TagType:    tagType,
		Rego:       rego,
		Enabled:    true,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
}

func TestEngine_Recognize(t *testing.T) {
	Convey("Test Engine Recognize", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "ua-curl", "risk", `package zki

tags contains "automation" if {
    contains(input.user_agent, "curl")
}`)
		insertRule(t, db, "method-get", "risk", `package zki

tags contains "web_request" if {
    input.method == "GET"
}`)

		Convey("命中多条规则时返回去重排序后的标签集合", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Method: "GET", UserAgent: "curl/8.0"}, "risk")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"automation", "web_request"})
		})

		Convey("无规则命中时返回空标签集合与 nil 错误", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Method: "POST", UserAgent: "firefox/120"}, "risk")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldBeEmpty)
		})
	})
}

func TestEngine_NoRules(t *testing.T) {
	Convey("Test Engine NoRules", t, func() {
		engine, _ := setupEngine(t)

		Convey("空库应返回空标签集合与 nil 错误", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldBeEmpty)
		})
	})
}

func TestEngine_CacheAndInvalidate(t *testing.T) {
	Convey("Test Engine CacheAndInvalidate", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "r1", "risk", `package zki

tags contains "old" if {
    input.method == "GET"
}`)

		Convey("规则更新后未失效缓存仍返回旧结果，失效后返回新结果", func() {
			// 预热缓存
			first, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(first.Tags, ShouldResemble, []string{"old"})

			newRego := `package zki

tags contains "new" if {
    input.method == "GET"
}`
			if err := db.Model(&tag.TagRule{}).Where("name = ?", "r1").Update("rego", newRego).Error; err != nil {
				t.Fatal(err)
			}
			// 未失效缓存：仍命中旧规则
			cached, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(cached.Tags, ShouldResemble, []string{"old"})

			// 失效后重新加载：命中新规则
			targetType, err := tagengine.TypeKey(&recogModel{})
			So(err, ShouldBeNil)
			engine.Invalidate(targetType, "risk")
			fresh, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(fresh.Tags, ShouldResemble, []string{"new"})
		})
	})
}

func TestEngine_RecognizeWithDetail(t *testing.T) {
	Convey("Test Engine RecognizeWithDetail", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "r-hit", "risk", `package zki

tags contains "hit_tag" if {
    input.method == "GET"
}`)
		insertRule(t, db, "r-miss", "risk", `package zki

tags contains "miss_tag" if {
    input.method == "DELETE"
}`)

		Convey("返回命中的标签与规则名，未命中规则不计入", func() {
			result, err := engine.RecognizeWithDetail(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"hit_tag"})
			So(result.Matched, ShouldResemble, []string{"r-hit"})
		})
	})
}

func TestEngine_UnsupportedTarget(t *testing.T) {
	Convey("Test Engine UnsupportedTarget", t, func() {
		engine, _ := setupEngine(t)

		Convey("非结构体目标应返回 ErrUnsupportedTarget", func() {
			_, err := engine.Recognize(context.Background(), "not a struct", "risk")
			So(errors.Is(err, tagengine.ErrUnsupportedTarget), ShouldBeTrue)
		})
	})
}

func TestEngine_SeparateGroups(t *testing.T) {
	Convey("Test Engine SeparateGroups", t, func() {
		engine, db := setupEngine(t)
		insertRule(t, db, "risk-rule", "risk", `package zki

tags contains "risk_tag" if {
    input.method == "GET"
}`)
		insertRule(t, db, "asset-rule", "asset", `package zki

tags contains "asset_tag" if {
    input.method == "GET"
}`)

		Convey("不同标签类型分组互不影响", func() {
			result, err := engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "risk")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"risk_tag"})

			result, err = engine.Recognize(context.Background(), &recogModel{Method: "GET"}, "asset")
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"asset_tag"})
		})
	})
}
