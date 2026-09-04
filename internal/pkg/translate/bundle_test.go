package translate

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// TestSupportLanguages 测试应用支持的语言列表
func TestSupportLanguages(t *testing.T) {
	Convey("Test SupportLanguages", t, func() {
		Convey("should support zh-Hans and en", func() {
			So(len(base.SupportLanguages), ShouldEqual, 2)
			So(base.SupportLanguages[0].StdTag().String(), ShouldEqual, "zh-Hans")
			So(base.SupportLanguages[1].StdTag().String(), ShouldEqual, "en")
		})
	})
}

// TestBundle 测试消息包的初始化
func TestBundle(t *testing.T) {
	Convey("Test Bundle", t, func() {
		Convey("Bundle should be initialized at package init", func() {
			So(Bundle, ShouldNotBeNil)
		})

		Convey("newBundle should return a non-nil bundle", func() {
			So(newBundle(), ShouldNotBeNil)
		})
	})
}

// TestLocalize 测试本地化函数
func TestLocalize(t *testing.T) {
	Convey("Test Localize", t, func() {
		zhLang, err := base.ParseLanguage("zh-Hans")
		So(err, ShouldBeNil)

		Convey("should translate an existing key", func() {
			// locales/zh-Hans.toml 中 [code] 200 = "成功"
			So(Localize(zhLang, "code.200", nil), ShouldEqual, "成功")
		})

		Convey("should return the key itself for a missing key", func() {
			// 未命中 key 时回退为 key 本身，由调用方（如 cliT）处理中文 fallback
			So(Localize(zhLang, "missing.key", nil), ShouldEqual, "missing.key")
		})

		Convey("external @file reference should be expanded into the template", func() {
			// notify.password.reset.content 声明为 "@emails/password-reset-content.zh-Hans.html"，
			// 解析时应收敛为外部 HTML 模板内容，并通过 go-i18n 渲染 {{.ResetURL}} 占位符。
			rendered := Localize(zhLang, "notify.password.reset.content", map[string]any{"ResetURL": "https://example.com/reset?token=abc"})
			So(rendered, ShouldNotEqual, "@emails/password-reset-content.zh-Hans.html")
			So(rendered, ShouldContainSubstring, "https://example.com/reset?token=abc")
			So(rendered, ShouldContainSubstring, "<html")
		})
	})
}

// TestLocalizer 测试创建本地化器并翻译英文消息
func TestLocalizer(t *testing.T) {
	Convey("Test Localizer", t, func() {
		enLang, err := base.ParseLanguage("en")
		So(err, ShouldBeNil)

		Convey("should create a localizer for a supported language", func() {
			So(Localizer(enLang), ShouldNotBeNil)
		})

		Convey("should translate an existing English key", func() {
			// locales/en.toml 中 [code] 200 = "Ok"
			So(Localize(enLang, "code.200", nil), ShouldEqual, "Ok")
		})
	})
}
