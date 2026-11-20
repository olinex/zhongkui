package tagengine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// inputTestModel 类型键测试用的简单结构体
type inputTestModel struct {
	Method string `json:"method"`
}

// inputerModel 实现 Inputer 接口的测试模型
type inputerModel struct{}

// TagRecogInput 返回固定的自定义输入文档
func (inputerModel) TagRecogInput() (map[string]any, error) {
	return map[string]any{"custom": "doc"}, nil
}

// TestTypeKey 验证 TypeKey 对值/指针实例返回一致类型键，非法目标返回 ErrUnsupportedTarget。
func TestTypeKey(t *testing.T) {
	Convey("Test TypeKey", t, func() {
		Convey("传值实例与指针实例应返回相同的类型键", func() {
			const expected = "zhongkuisec.com/zhongkui/internal/tagengine.inputTestModel"

			valueKey, err := TypeKey(inputTestModel{})
			So(err, ShouldBeNil)
			So(valueKey, ShouldEqual, expected)

			ptrKey, err := TypeKey(&inputTestModel{})
			So(err, ShouldBeNil)
			So(ptrKey, ShouldEqual, expected)
		})

		Convey("非结构体目标应返回 ErrUnsupportedTarget", func() {
			_, err := TypeKey("string")
			So(errors.Is(err, ErrUnsupportedTarget), ShouldBeTrue)

			_, err = TypeKey(42)
			So(errors.Is(err, ErrUnsupportedTarget), ShouldBeTrue)

			_, err = TypeKey(nil)
			So(errors.Is(err, ErrUnsupportedTarget), ShouldBeTrue)
		})
	})
}

// TestBuildInput 验证 BuildInput 生成规则输入文档且不暴露敏感字段明文。
func TestBuildInput(t *testing.T) {
	Convey("Test BuildInput", t, func() {
		Convey("简单结构体应使用 json tag 作为 map 键", func() {
			input, err := BuildInput(inputTestModel{Method: "POST"})
			So(err, ShouldBeNil)
			So(input["method"], ShouldEqual, "POST")
		})

		Convey("实现 Inputer 接口的类型应使用自定义输入文档", func() {
			input, err := BuildInput(inputerModel{})
			So(err, ShouldBeNil)
			So(input, ShouldResemble, map[string]any{"custom": "doc"})
		})

		Convey("HttpTransport 应序列化普通字段且隐藏敏感字段", func() {
			transport := &network.HttpTransport{}
			transport.Method = "GET"
			transport.ReqHdrUserAgent = "curl/8.0"
			transport.ReqHdrAuthorization = serializer.NewCrypto([]byte("secret"))
			transport.ReqBody = serializer.NewS3Crypto("body-key")

			input, err := BuildInput(transport)
			So(err, ShouldBeNil)
			So(input["method"], ShouldEqual, "GET")
			So(input["reqHdrUserAgent"], ShouldEqual, "curl/8.0")
			// 加密字段不出现在规则输入文档中，只能经 crypto.reveal 获取明文
			So(input, ShouldNotContainKey, "reqHdrAuthorization")
			So(input, ShouldNotContainKey, "reqHdrCookie")
			So(input, ShouldNotContainKey, "reqBody")
			So(input, ShouldNotContainKey, "rspBody")
		})

		Convey("敏感字段为空时同样不出现在输入文档中", func() {
			transport := &network.HttpTransport{}
			input, err := BuildInput(transport)
			So(err, ShouldBeNil)
			So(input, ShouldNotContainKey, "reqHdrAuthorization")
			So(input, ShouldNotContainKey, "reqBody")
		})
	})
}
