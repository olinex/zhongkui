// Package views 提供 HTTP 管理能力。
// 本文件包含标签规则样例 S3 body 副本逻辑的单元测试：
// 覆盖副本对象键格式与 body 对象键提取（缺失/空值跳过、非字符串报错）。
package views

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// TestSampleObjectKey 样例 S3 body 副本对象键格式。
func TestSampleObjectKey(t *testing.T) {
	Convey("测试样例 body 副本对象键格式", t, func() {
		Convey("按 sample/{model}/{uid}/{field}.zkcn 拼接", func() {
			key := sampleObjectKey("http-transport", "abc-123", "reqBody")
			So(key, ShouldEqual, "sample/http-transport/abc-123/reqBody."+combine.FileExt)
		})
	})
}

// TestSampleBodyObjectKeys 样例 body 对象键提取逻辑。
func TestSampleBodyObjectKeys(t *testing.T) {
	// httpTransportKey 为 HttpTransport 模型的 tagengine 类型键（样例接口 model 入参）
	httpTransportKey := tagengine.MustTypeKey(&network.HttpTransport{})
	Convey("测试样例 body 对象键提取", t, func() {
		Convey("http-transport 提取 reqBody/rspBody 两个对象键", func() {
			keys, err := SampleBodyObjectKeys(httpTransportKey, map[string]any{
				"reqBody": "body/request/aaa.zkcn",
				"rspBody": "body/response/bbb.zkcn",
			})
			So(err, ShouldBeNil)
			So(keys, ShouldResemble, map[string]string{
				"reqBody": "body/request/aaa.zkcn",
				"rspBody": "body/response/bbb.zkcn",
			})
		})

		Convey("字段缺失时跳过，返回空集合", func() {
			keys, err := SampleBodyObjectKeys(httpTransportKey, map[string]any{
				"method": "POST",
			})
			So(err, ShouldBeNil)
			So(keys, ShouldBeEmpty)
		})

		Convey("空字符串对象键（空 body）跳过", func() {
			keys, err := SampleBodyObjectKeys(httpTransportKey, map[string]any{
				"reqBody": "",
				"rspBody": "body/response/bbb.zkcn",
			})
			So(err, ShouldBeNil)
			So(keys, ShouldResemble, map[string]string{
				"rspBody": "body/response/bbb.zkcn",
			})
		})

		Convey("字段值非字符串返回错误", func() {
			_, err := SampleBodyObjectKeys(httpTransportKey, map[string]any{
				"reqBody": 123,
			})
			So(err, ShouldNotBeNil)
		})

		Convey("未注册 S3 字段的模型返回空集合", func() {
			keys, err := SampleBodyObjectKeys("some-other-model", map[string]any{
				"reqBody": "body/request/aaa.zkcn",
			})
			So(err, ShouldBeNil)
			So(keys, ShouldBeEmpty)
		})
	})
}
