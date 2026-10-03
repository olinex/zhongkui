package notify

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	message "zhongkuisec.com/zhongkui/internal/pkg/message"
)

// ensureTestEnumClasses 确保类感知模板函数（TenumKey/TenumValue）所需的统一枚举类已注册。
// 测试二进制不链接 internal/manager/views（其 init() 注册 channelType / tagRuleModel 等类），
// 故此处按同一契约注册等价条目：channelType 的 key/value 一致，tagRuleModel 的 value 为
// 完整类型键（数据库存储语义）。已存在的类跳过注册，避免重复注册 panic。
func ensureTestEnumClasses() {
	register := func(name string, entries []enum.Entry) {
		if _, exists := enum.Get(name); !exists {
			enum.Register(enum.Type{Name: name, Entries: entries})
		}
	}
	register("channelType", []enum.Entry{
		{Key: "email", Value: "email"},
		{Key: "webhook", Value: "webhook"},
	})
	register("tagRuleModel", []enum.Entry{
		{Key: "httpTransport", Value: "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport"},
	})
	register("tagEventDimension", []enum.Entry{
		{Key: "fpTcpClientSig", Value: "fpTcpClientSig"},
		{Key: "fpTcpServerSig", Value: "fpTcpServerSig"},
	})
}

// TestRenderFuncsTemplateFuncs 验证 renderFuncs 注入的语义互斥模板函数：
//   - TenumValue "<类名>" <value>：先按 value 命中条目（Resolve 语义），经
//     enum.<类名>.<key> 翻译 label；未注册类或未命中 value 时回退原 value。
//   - TenumKey "<类名>" <key>：按枚举 key 直接翻译 label；未注册类/未命中 key 时回退 key。
//   - T "<i18n键>"：按完整 i18n 键直接翻译，缺失回退键本身。
//
// 三者均兼容 "enum." 前缀写法。
func TestRenderFuncsTemplateFuncs(t *testing.T) {
	convey.Convey("Test RenderFuncs template funcs", t, func() {
		ensureTestEnumClasses()
		zhLang, err := base.ParseLanguage("zh-Hans")
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("TenumValue resolves a DB value to its enum label", func() {
			fn := renderFuncs(zhLang)
			out, err := message.RenderTemplateWithFuncs(`{{ TenumValue "channelType" "email" }}`, nil, fn)
			convey.So(err, convey.ShouldBeNil)
			convey.So(out, convey.ShouldEqual, "邮件")
		})

		convey.Convey("TenumValue resolves a model value (full type key) to its label", func() {
			fn := renderFuncs(zhLang)
			out, err := message.RenderTemplateWithFuncs(`{{ TenumValue "tagRuleModel" "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport" }}`, nil, fn)
			convey.So(err, convey.ShouldBeNil)
			convey.So(out, convey.ShouldEqual, "HTTP 传输")
		})

		convey.Convey("TenumValue falls back to the original value for an unknown class", func() {
			fn := renderFuncs(zhLang)
			out, err := message.RenderTemplateWithFuncs(`{{ TenumValue "no.such.class" "someValue" }}`, nil, fn)
			convey.So(err, convey.ShouldBeNil)
			convey.So(out, convey.ShouldEqual, "someValue")
		})

		convey.Convey("TenumKey translates by enum key, honoring the enum. prefix", func() {
			fn := renderFuncs(zhLang)
			out, err := message.RenderTemplateWithFuncs(`{{ TenumKey "tagRuleModel" "httpTransport" }}`, nil, fn)
			convey.So(err, convey.ShouldBeNil)
			convey.So(out, convey.ShouldEqual, "HTTP 传输")

			out2, err2 := message.RenderTemplateWithFuncs(`{{ TenumKey "enum.channelType" "email" }}`, nil, fn)
			convey.So(err2, convey.ShouldBeNil)
			convey.So(out2, convey.ShouldEqual, "邮件")
		})

		convey.Convey("T falls back to the key itself for a missing i18n key", func() {
			fn := renderFuncs(zhLang)
			out, err := message.RenderTemplateWithFuncs(`{{ T "model.no.such.key" }}`, nil, fn)
			convey.So(err, convey.ShouldBeNil)
			convey.So(out, convey.ShouldEqual, "model.no.such.key")
		})
	})
}

// TestRenderI18nDefaultWithT 验证 renderI18nDefault 能经注册表内置默认模板 + 类感知模板函数
// 渲染出目标模型与标签维度的枚举 label（维度经 tagEventDimension 类翻译；不依赖持久化模板，
// 无需数据库）。
func TestRenderI18nDefaultWithT(t *testing.T) {
	convey.Convey("Test RenderI18nDefault with t", t, func() {
		ensureTestEnumClasses()
		spec, ok := EventSpecOf(core.EventTypeTagEventRecord)
		convey.So(ok, convey.ShouldBeTrue)
		zhLang, err := base.ParseLanguage("zh-Hans")
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("should resolve dimension and model labels in zh-Hans", func() {
			rendered, err := renderI18nDefault(zhLang, spec, spec.SampleParams, "text/plain")
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered, convey.ShouldNotBeNil)
			convey.So(rendered.Title, convey.ShouldNotEqual, "")
			convey.So(rendered.Content, convey.ShouldContainSubstring, "客户端指纹签名")
			convey.So(rendered.Content, convey.ShouldContainSubstring, "HTTP 传输")
			convey.So(rendered.ContentType, convey.ShouldEqual, "text/plain")
		})
	})
}
