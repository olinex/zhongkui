package tagengine

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// notifyModel 变更通知测试用的本地结构体。
type notifyModel struct {
	Namespace string `json:"namespace"`
	Method    string `json:"method"`
}

// setupNotifyEngine 创建共享已迁移的 PostgreSQL 测试库与标签识别引擎（testdb.Core）。
func setupNotifyEngine(t *testing.T) (*Engine, *database.DB) {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Core(t)
	return NewEngine(db, logger, nil), db
}

// TestRuleChangeKey 测试规则变更广播键的全局前缀。
func TestRuleChangeKey(t *testing.T) {
	Convey("Test RuleChangeKey", t, func() {
		Convey("规则变更键应落在全局前缀下", func() {
			So(ruleChangeKey, ShouldEqual, "zki/tagengine/rule-change")
		})
	})
}

// TestEngineBroadcastChange 测试引擎广播规则变更的行为。
func TestEngineBroadcastChange(t *testing.T) {
	Convey("Test Engine BroadcastChange", t, func() {
		Convey("未绑定 etcd 客户端时广播快速失败", func() {
			engine, _ := setupNotifyEngine(t)
			err := engine.BroadcastChange(context.Background(), "default", "t")
			So(err, ShouldNotBeNil)
			So(err, ShouldEqual, etcd.ErrChangeUnavailable)
		})
	})
}

// TestEngineStartNoEtcd 未绑定 etcd 客户端时 Start 直接返回（无监听能力）。
func TestEngineStartNoEtcd(t *testing.T) {
	Convey("Test Engine Start without etcd", t, func() {
		Convey("watcher 存在但未绑定 etcd 客户端时 Start 直接返回", func() {
			engine, _ := setupNotifyEngine(t)
			So(engine.watcher, ShouldNotBeNil)
			So(engine.Start(context.Background()), ShouldBeNil)
		})

		Convey("watcher 为 nil 时 Start 跳过并返回 nil", func() {
			engine, _ := setupNotifyEngine(t)
			engine.watcher = nil
			So(engine.Start(context.Background()), ShouldBeNil)
		})
	})
}

// TestRuleChangeHandler 测试规则变更事件处理器失效编译缓存的行为。
func TestRuleChangeHandler(t *testing.T) {
	Convey("Test RuleChangeHandler", t, func() {
		engine, db := setupNotifyEngine(t)
		handler := &ruleChangeHandler{e: engine}
		targetType, err := TypeKey(&notifyModel{})
		if err != nil {
			t.Fatal(err)
		}
		insertNotifyRule(t, db, "default", targetType, "r1", `tags contains "old" if { input.method == "GET" }`)
		insertNotifyRule(t, db, "tenant-b", targetType, "r1b", `tags contains "tenant-old" if { input.method == "GET" }`)

		Convey("收到精准事件后仅失效对应分组，其他分组缓存保持", func() {
			// 预热两个命名空间分组缓存
			defaultFirst, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(defaultFirst, ShouldResemble, []string{"old"})
			tenantFirst, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)
			So(tenantFirst, ShouldResemble, []string{"tenant-old"})
			So(cacheHas(engine, "default", targetType), ShouldBeTrue)
			So(cacheHas(engine, "tenant-b", targetType), ShouldBeTrue)

			// 直接修改 default 组规则，未失效缓存仍返回旧结果
			newRego := `package zki

tags contains "new" if {
    input.method == "GET"
}`
			if err := db.Model(&tag.TagRule{}).Where("name = ?", "r1").Update("rego", newRego).Error; err != nil {
				t.Fatal(err)
			}
			cached, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(cached, ShouldResemble, []string{"old"})

			// 处理器收到仅含 default 分组的事件
			So(handler.HandleChange(context.Background(), eventPayload(&RuleChangeEvent{Namespace: "default", Model: targetType})), ShouldBeNil)

			// 仅 default 组缓存被失效，tenant-b 组缓存保持（在重新识别前检查，识别会重新填充缓存）
			So(cacheHas(engine, "default", targetType), ShouldBeFalse)
			So(cacheHas(engine, "tenant-b", targetType), ShouldBeTrue)

			// default 组重新加载命中新规则
			fresh, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			So(fresh, ShouldResemble, []string{"new"})

			// tenant-b 组缓存未被失效，仍命中缓存中的旧规则
			tenantAgain, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)
			So(tenantAgain, ShouldResemble, []string{"tenant-old"})
		})

		Convey("负载无法解析时回退全量失效", func() {
			_, err := engine.Recognize(context.Background(), &notifyModel{Namespace: "default", Method: "GET"})
			So(err, ShouldBeNil)
			_, err = engine.Recognize(context.Background(), &notifyModel{Namespace: "tenant-b", Method: "GET"})
			So(err, ShouldBeNil)
			So(cacheHas(engine, "default", targetType), ShouldBeTrue)
			So(cacheHas(engine, "tenant-b", targetType), ShouldBeTrue)

			// 发送不可解析负载（非 JSON 且无分组信息），处理返回 nil 且全量失效
			So(handler.HandleChange(context.Background(), []byte("not-json")), ShouldBeNil)

			So(cacheHas(engine, "default", targetType), ShouldBeFalse)
			So(cacheHas(engine, "tenant-b", targetType), ShouldBeFalse)
		})
	})
}

// cacheHas 检查引擎缓存中是否存在指定命名空间下的分组。
func cacheHas(engine *Engine, namespace, targetType string) bool {
	_, ok := engine.cache.Load(groupKey(namespace, targetType))
	return ok
}

// eventPayload 序列化规则变更事件为处理器负载。
func eventPayload(event *RuleChangeEvent) []byte {
	payload, err := json.Marshal(event)
	if err != nil {
		panic("failed to marshal rule change event: " + err.Error())
	}
	return payload
}

// insertNotifyRule 插入一条指定命名空间下启用的识别规则。
func insertNotifyRule(t *testing.T, db *database.DB, namespace, model, name, body string) {
	t.Helper()
	if err := db.Create(&tag.TagRule{
		NamespaceName: namespace,
		Name:          name,
		Model:         model,
		Rego:          "package zki\n\n" + body,
		Enabled:       true,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

// TestExtractNamespace 测试 extractNamespace 的命名空间提取与缺省回退行为。
func TestExtractNamespace(t *testing.T) {
	Convey("Test extractNamespace", t, func() {
		Convey("缺失 Namespace 字段的结构体回退到默认命名空间", func() {
			So(extractNamespace(&struct{ Method string }{Method: "GET"}), ShouldEqual, base.DefaultNamespace)
		})

		Convey("Namespace 字段为空的实例回退到默认命名空间", func() {
			So(extractNamespace(&notifyModel{Method: "GET"}), ShouldEqual, base.DefaultNamespace)
		})

		Convey("NamespaceName 字符串字段优先返回其值", func() {
			// GORM 模型以嵌入 Struct 的 Namespace 表达关联，真正的命名空间字符串在
			// NamespaceName 列上；本用例以 Namespace 为结构体、NamespaceName 为字符串模拟。
			type gormModel struct {
				Namespace     struct{}
				NamespaceName string
			}
			So(extractNamespace(&gormModel{NamespaceName: "tenant-c"}), ShouldEqual, "tenant-c")
		})

		Convey("Namespace 字段非空的实例返回其值", func() {
			So(extractNamespace(&notifyModel{Namespace: "tenant-b", Method: "GET"}), ShouldEqual, "tenant-b")
		})

		Convey("nil 指针回退到默认命名空间", func() {
			var m *notifyModel
			So(extractNamespace(m), ShouldEqual, base.DefaultNamespace)
		})

		Convey("非结构体目标回退到默认命名空间", func() {
			So(extractNamespace("not a struct"), ShouldEqual, base.DefaultNamespace)
		})
	})
}
