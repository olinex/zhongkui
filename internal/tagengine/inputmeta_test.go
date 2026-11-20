// 本文件测试输入文档字段元信息推导（InputMetadata）。
package tagengine_test

import (
	// 标准库
	"reflect"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/network"
	pkgErrors "zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/tagengine"
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
		Convey("普通字段与 BuildInput 键集合一致，敏感字段仅出现在元信息", func() {
			input, err := tagengine.BuildInput(&network.HttpTransport{})
			So(err, ShouldBeNil)
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			for _, meta := range metas {
				_, present := input[meta.Field]
				if meta.Sensitive {
					// 敏感字段不出现在规则输入文档中，只作为 crypto.reveal 的字段名
					So(present, ShouldBeFalse)
					continue
				}
				So(present, ShouldBeTrue)
			}
			// BuildInput 键集合的每个键都有对应元信息（以反射为准，null 字段也在内）
			for key := range input {
				So(findMeta(metas, key), ShouldNotBeNil)
			}
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

		Convey("clientPort 字段：数字类型", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "clientPort")
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

		Convey("reqHdrAuthorization 字段：敏感字符串，仅元信息可见", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "reqHdrAuthorization")
			So(meta, ShouldNotBeNil)
			So(meta.Sensitive, ShouldBeTrue)
			So(meta.Type, ShouldEqual, "string")
			So(meta.GoType, ShouldEqual, "*serializer.Crypto")
		})

		Convey("reqBodyHash 字段：摘要不可解密，不标记为敏感", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "reqBodyHash")
			So(meta, ShouldNotBeNil)
			So(meta.Sensitive, ShouldBeFalse)
			So(meta.GoType, ShouldEqual, "serializer.Hash")
		})

		Convey("reqBody 字段：S3 对象键字符串且敏感", func() {
			metas, err := tagengine.InputMetadata(httpTransportType)
			So(err, ShouldBeNil)
			meta := findMeta(metas, "reqBody")
			So(meta, ShouldNotBeNil)
			So(meta.Sensitive, ShouldBeTrue)
			So(meta.Type, ShouldEqual, "string")
			So(meta.GoType, ShouldEqual, "serializer.S3Crypto")
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
		_, err := tagengine.InputMetadata("zhongkuisec.com/zhongkui/internal/models/network.NotExist")
		So(err, ShouldNotBeNil)
		So(pkgErrors.Is(err, tagengine.ErrModelNotRegistered), ShouldBeTrue)
	})
}
