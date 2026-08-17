// 本文件测试输入文档字段元信息推导（InputMetadata）。
package tagengine_test

import (
	// stdlib
	"reflect"
	"testing"

	// third-party
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/models/network"
	pkgErrors "zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// findMeta 在元信息列表中查找指定字段，未找到时返回 nil。
func findMeta(metas []tagengine.InputFieldMeta, field string) *tagengine.InputFieldMeta {
	for i := range metas {
		if metas[i].Field == field {
			return &metas[i]
		}
	}
	return nil
}

// TestInputMeta 验证 InputMetadata 对已注册目标模型的字段元信息推导。
func TestInputMeta(t *testing.T) {
	Convey("HttpTransport 输入文档元信息", t, func() {
		Convey("字段集合与 BuildInput 零值文档键集合一致", func() {
			input, err := tagengine.BuildInput(&network.HttpTransport{})
			So(err, ShouldBeNil)
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			// 每个元信息字段都出现在 BuildInput 键集合中
			for _, meta := range metas {
				_, ok := input[meta.Field]
				So(ok, ShouldBeTrue)
			}
			// BuildInput 键集合的每个键都有对应元信息（以反射为准，null 字段也在内）
			for key := range input {
				So(findMeta(metas, key), ShouldNotBeNil)
			}
			// 字段数量一致
			So(len(metas), ShouldEqual, len(input))
		})

		Convey("method 字段：字符串类型且标注描述翻译键", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "method")
			So(meta, ShouldNotBeNil)
			So(meta.Type, ShouldEqual, "string")
			So(meta.GoType, ShouldContainSubstring, "string")
			// 描述不再由源码注释提取：tagengine 层标注 i18n 消息键，Description 由接口层翻译后填充
			So(meta.DescriptionKey, ShouldEqual, "input."+httpTransportType+".method")
			So(meta.Description, ShouldBeEmpty)
		})

		Convey("每个字段均标注非空且格式正确的 DescriptionKey", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			So(metas, ShouldNotBeEmpty)
			for _, meta := range metas {
				So(meta.DescriptionKey, ShouldEqual, "input."+httpTransportType+"."+meta.Field)
			}
		})

		Convey("client_port 字段：数字类型", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "client_port")
			So(meta, ShouldNotBeNil)
			So(meta.Type, ShouldEqual, "number")
		})

		Convey("tags 字段：数组类型且元素为字符串", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "tags")
			So(meta, ShouldNotBeNil)
			So(meta.Type, ShouldEqual, "array")
			So(meta.Items, ShouldNotBeNil)
			So(meta.Items.Type, ShouldEqual, "string")
		})

		Convey("req_hdr_authorization 字段：敏感对象", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "req_hdr_authorization")
			So(meta, ShouldNotBeNil)
			So(meta.Sensitive, ShouldBeTrue)
			So(meta.Type, ShouldEqual, "object")
			So(meta.GoType, ShouldEqual, "*serializer.Crypto")
		})

		Convey("排序稳定：两次调用返回一致", func() {
			first, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			second, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			So(reflect.DeepEqual(first, second), ShouldBeTrue)
			// 且严格按字段名升序
			for i := 1; i < len(first); i++ {
				So(first[i-1].Field < first[i].Field, ShouldBeTrue)
			}
		})
	})

	Convey("未注册模型返回 ErrModelNotRegistered", t, func() {
		_, err := tagengine.InputMetadata("zhongkui.com/zhongkui/internal/models/network.NotExist")
		So(err, ShouldNotBeNil)
		So(pkgErrors.Is(err, tagengine.ErrModelNotRegistered), ShouldBeTrue)
	})
}
