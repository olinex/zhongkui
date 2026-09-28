package tagengine_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/datatypes"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// httpTransportType 已注册目标模型 *network.HttpTransport 的类型键，作为样例与规则的目标模型。
var httpTransportType = func() string {
	t, err := tagengine.TypeKey(&network.HttpTransport{})
	if err != nil {
		panic("failed to compute http transport type key: " + err.Error())
	}
	return t
}()

// sampleTestNamespace 本测试专用命名空间，用于与迁移基线种子样例隔离。
// 基线样例（browser-post / curl-get / scanner-crawler / sparse-minimal）以已提交
// 的持久行归置于 default 命名空间，测试若插入同名样例将撞唯一约束
// uq_tag_rule_samples_namespace_name_model_name 而失败（且仅在数据库被重新迁移时
// 才暴露，具有环境/时序相关性）。故本测试改用独立命名空间操作自身的数据。
const sampleTestNamespace = "sample-unit-test"

// setupSampleDB 创建共享已迁移的 PostgreSQL 测试库（testdb.Core）。
// tag_rule_samples 表已由 testdb 统一迁移。
func setupSampleDB(t *testing.T) *database.DB {
	t.Helper()
	return testdb.Core(t)
}

// insertSample 为目标模型插入一条验证样例（归属 sampleTestNamespace，与 sampleRules 规则一致）。
func insertSample(t *testing.T, db *database.DB, name, input string) {
	t.Helper()
	sample := &tag.TagRuleSample{
		NamespaceName: sampleTestNamespace,
		Model:         httpTransportType,
		Name:          name,
		Input:         datatypes.JSON(input),
		Description:   "test sample",
	}
	if err := db.Create(sample).Error; err != nil {
		t.Fatal(err)
	}
}

// sampleRules 构造目标模型下的一组候选规则。
func sampleRules(rego string) []*tag.TagRule {
	return []*tag.TagRule{
		{
			NamespaceName: sampleTestNamespace,
			Name:          "candidate",
			Model:         httpTransportType,
			Rego:          rego,
			Enabled:       true,
		},
	}
}

// validSampleRego 语法与语义合法且可正常评估的规则源码。
const validSampleRego = `package zki

tags contains "browser" if {
	contains(input.reqHdrUserAgent, "Mozilla")
}`

// runtimeErrorRego 编译通过但评估时必然报错的规则源码：
// count 仅接受容器/字符串，样例中 reqHdrContentLength 为数字，评估时抛类型错误。
const runtimeErrorRego = `package zki

tags contains "oops" if {
	count(input.reqHdrContentLength) > 10
}`

// TestValidateGroupRuntime 验证运行期样例校验对合法规则、求值报错与不可反序列化样例的处理。
func TestValidateGroupRuntime(t *testing.T) {
	Convey("Test ValidateGroupRuntime", t, func() {
		db := setupSampleDB(t)
		insertSample(t, db, "browser-post", `{
			"namespace": "default",
			"method": "POST",
			"url": "https://api.example.com/v1/orders",
			"path": "/v1/orders",
			"host": "api.example.com",
			"reqHdrUserAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			"reqHdrContentLength": 42,
			"clientIp": "10.0.0.55",
			"serverIp": "142.250.72.14",
			"sessionId": "c81d4e2e-bcf2-11e6-869b-7df92533d2db",
			"traceId": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"timestamp": "2026-08-15T03:12:34Z"
		}`)
		// 该样例不含 reqHdrContentLength，反序列化后字段为零值 0，
		// count(0) 同样触发类型错误（与 browser-post 一样评估报错）
		insertSample(t, db, "curl-get", `{
			"namespace": "default",
			"method": "GET",
			"url": "https://example.com/api/v1/health",
			"path": "/api/v1/health",
			"reqHdrUserAgent": "curl/8.0.1",
			"clientIp": "192.168.1.10",
			"serverIp": "93.184.216.34",
			"sessionId": "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
		}`)

		Convey("合法规则运行期校验通过", func() {
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), db, sampleRules(validSampleRego))
			So(err, ShouldBeNil)
			So(issues, ShouldBeEmpty)
		})

		Convey("运行期报错规则返回对应样例的错误列表", func() {
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), db, sampleRules(runtimeErrorRego))
			So(err, ShouldBeNil)
			// 两条样例的 reqHdrContentLength 均为数字（curl-get 反序列化后为零值 0），
			// count 不接受数字，规则对两条样例评估均报类型错误
			So(len(issues), ShouldEqual, 2)
			So(issues[0].SampleName, ShouldEqual, "browser-post")
			So(issues[0].Error, ShouldNotBeEmpty)
			So(issues[1].SampleName, ShouldEqual, "curl-get")
			So(issues[1].Error, ShouldNotBeEmpty)
		})

		Convey("样例无法反序列化为目标模型时返回错误", func() {
			insertSample(t, db, "bad-session", `{
				"sessionId": "not-a-uuid",
				"method": "GET"
			}`)
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), db, sampleRules(validSampleRego))
			So(err, ShouldBeNil)
			So(len(issues), ShouldEqual, 1)
			So(issues[0].SampleName, ShouldEqual, "bad-session")
			So(issues[0].Error, ShouldContainSubstring, "cannot be deserialized")
		})

		Convey("空规则组直接通过", func() {
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), db, nil)
			So(err, ShouldBeNil)
			So(issues, ShouldBeEmpty)
		})

		Convey("未注册的目标模型返回 ErrModelNotRegistered", func() {
			rules := []*tag.TagRule{{
				NamespaceName: sampleTestNamespace,
				Name:          "unregistered",
				Model:         "unregistered.model.Type",
				Rego:          validSampleRego,
				Enabled:       true,
			}}
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), db, rules)
			So(issues, ShouldBeEmpty)
			So(errors.Is(err, tagengine.ErrModelNotRegistered), ShouldBeTrue)
		})

		Convey("无样例数据时返回 ErrSampleNotProvisioned（降级放行信号）", func() {
			emptyDB := setupSampleDB(t)
			issues, err := tagengine.ValidateGroupRuntime(context.Background(), emptyDB, sampleRules(validSampleRego))
			So(issues, ShouldBeEmpty)
			So(errors.Is(err, tagengine.ErrSampleNotProvisioned), ShouldBeTrue)
		})
	})
}
