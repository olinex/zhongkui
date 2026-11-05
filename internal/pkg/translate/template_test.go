package translate

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// TestNotifyDefaultRaw 验证 notify 默认模板经 Localize 在 [[ / ]] 数据定界符下
// “原样透出原始模板串”（含 {{...}}），缺失键回退返回键本身，nil 语言回退默认语言。
// 这是 renderI18nDefault 取原始模板串的实现依据——不再存在整表复制的 rawByLang 冗余映射。
func TestNotifyDefaultRaw(t *testing.T) {
	convey.Convey("Test Notify Default Raw", t, func() {
		zhLang, err := base.ParseLanguage("zh-Hans")
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("should pass through the raw template string via the [[ ]] data delimiter", func() {
			raw := Localize(zhLang, "notify.tag_event.alert.content", nil)
			convey.So(raw, convey.ShouldNotEqual, "notify.tag_event.alert.content")
			convey.So(raw, convey.ShouldContainSubstring, "{{")
			convey.So(raw, convey.ShouldContainSubstring, `TenumValue "tagRuleModel"`)
		})

		convey.Convey("should return the key itself for a missing key", func() {
			raw := Localize(zhLang, "missing.key", nil)
			convey.So(raw, convey.ShouldEqual, "missing.key")
		})

		convey.Convey("nil language falls back to the default language", func() {
			raw := Localize(nil, "notify.tag_event.alert.content", nil)
			convey.So(raw, convey.ShouldNotEqual, "notify.tag_event.alert.content")
			convey.So(raw, convey.ShouldNotEqual, "")
		})
	})
}

// TestEnumLabel 验证 EnumLabel 单键解析：命中返回 label、未命中回退传入的 key。
// 枚举 label 键（enum.<类名>.<key>）由枚举注册表直接推导，无需启动期收集键清单。
func TestEnumLabel(t *testing.T) {
	convey.Convey("Test EnumLabel", t, func() {
		zhLang, err := base.ParseLanguage("zh-Hans")
		convey.So(err, convey.ShouldBeNil)
		enLang, err := base.ParseLanguage("en")
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("EnumLabel resolves a single key and falls back to the key on a miss", func() {
			convey.So(EnumLabel(zhLang, "channelType", "email"), convey.ShouldEqual, "邮件")
			convey.So(EnumLabel(enLang, "tagRuleModel", "httpTransport"), convey.ShouldEqual, "HTTP Transport")
			convey.So(EnumLabel(zhLang, "channelType", "missing"), convey.ShouldEqual, "missing")
			convey.So(EnumLabel(nil, "language", "zh-Hans"), convey.ShouldEqual, "简体中文")
		})
	})
}
