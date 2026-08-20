package tagengine_test

import (
	// stdlib
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	// third-party
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// testSampleModel 测试用的自定义目标模型（无预置样例，验证懒供给 default）。
type testSampleModel struct {
	Foo string `json:"foo"`
}

// customModelType 自定义模型 testSampleModel 的类型键。
var customModelType = func() string {
	t, err := tagengine.TypeKey(&testSampleModel{})
	if err != nil {
		panic("failed to compute custom model type key: " + err.Error())
	}
	return t
}()

// registerCustomModelOnce 保证自定义模型在测试进程中仅注册一次（重复注册会 panic）。
var registerCustomModelOnce sync.Once

// registerCustomModel 注册自定义目标模型到全局注册表。
func registerCustomModel() {
	registerCustomModelOnce.Do(func() {
		tagengine.Register(tagengine.Target{Type: &testSampleModel{}, DisplayName: "test"})
	})
}

func TestListModelSamples(t *testing.T) {
	Convey("Test ListModelSamples", t, func() {
		db := setupSampleDB(t)

		Convey("未注册模型返回 ErrModelNotRegistered", func() {
			_, err := tagengine.ListModelSamples(context.Background(), db, "unregistered.model.Type")
			So(errors.Is(err, tagengine.ErrModelNotRegistered), ShouldBeTrue)
		})

		Convey("供给后按 name 排序返回样例列表", func() {
			_, err := tagengine.EnsureModelSamples(context.Background(), db, httpTransportType)
			So(err, ShouldBeNil)

			samples, err := tagengine.ListModelSamples(context.Background(), db, httpTransportType)
			So(err, ShouldBeNil)
			So(len(samples), ShouldEqual, 2)
			So(samples[0].Name, ShouldEqual, "default")
			So(samples[1].Name, ShouldEqual, "http-example")
		})
	})
}

func TestEnsureModelSamples(t *testing.T) {
	Convey("Test EnsureModelSamples", t, func() {
		db := setupSampleDB(t)

		Convey("未注册模型返回 ErrModelNotRegistered", func() {
			_, err := tagengine.EnsureModelSamples(context.Background(), db, "unregistered.model.Type")
			So(errors.Is(err, tagengine.ErrModelNotRegistered), ShouldBeTrue)
		})

		Convey("HttpTransport 懒供给 default 与 http-example 两条样例", func() {
			samples, err := tagengine.EnsureModelSamples(context.Background(), db, httpTransportType)
			So(err, ShouldBeNil)
			So(len(samples), ShouldEqual, 2)

			names := []string{samples[0].Name, samples[1].Name}
			So(names, ShouldContain, "default")
			So(names, ShouldContain, "http-example")

			// 每条样例 Input 均可反序列化回 *network.HttpTransport
			for _, sample := range samples {
				var transport network.HttpTransport
				So(json.Unmarshal(sample.Input, &transport), ShouldBeNil)
			}

			// http-example 的 Input 含 method 与 reqHdrAuthorization（值为 {}）
			var example *tag.TagRuleSample
			for _, sample := range samples {
				if sample.Name == "http-example" {
					example = sample
				}
			}
			So(example, ShouldNotBeNil)
			var input map[string]any
			So(json.Unmarshal(example.Input, &input), ShouldBeNil)
			So(input["method"], ShouldEqual, "POST")
			So(input["reqHdrAuthorization"], ShouldResemble, map[string]any{})
		})

		Convey("重复调用 EnsureModelSamples 幂等（样例数量不变）", func() {
			_, err := tagengine.EnsureModelSamples(context.Background(), db, httpTransportType)
			So(err, ShouldBeNil)
			samples, err := tagengine.EnsureModelSamples(context.Background(), db, httpTransportType)
			So(err, ShouldBeNil)
			So(len(samples), ShouldEqual, 2)
		})

		Convey("无预置样例模型首次供给 default，再次调用不新增", func() {
			registerCustomModel()
			samples, err := tagengine.EnsureModelSamples(context.Background(), db, customModelType)
			So(err, ShouldBeNil)
			So(len(samples), ShouldEqual, 1)
			So(samples[0].Name, ShouldEqual, "default")
			So(string(samples[0].Input), ShouldEqual, `{"foo":""}`)

			samples, err = tagengine.EnsureModelSamples(context.Background(), db, customModelType)
			So(err, ShouldBeNil)
			So(len(samples), ShouldEqual, 1)
		})
	})
}
