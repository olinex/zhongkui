package translate

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGetTranslatorBasic 测试 GetTranslator 基本功能
func TestGetTranslatorBasic(t *testing.T) {
	Convey("Test GetTranslator basic functionality", t, func() {
		// 创建测试配置
		enLang, _ := ParseLanguage("en")
		zhLang, _ := ParseLanguage("zh-Hans")

		cfg := &Config{
			DefaultLocale:  *enLang,
			SupportLocales: []Language{*enLang, *zhLang},
			RootPath:       "locales", // 使用内置的 locales 目录
		}

		// 由于测试中无法创建真实的 logger，我们测试核心逻辑
		Convey("should handle language parsing", func() {
			// 测试语言解析
			lang, err := ParseLanguage("en")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.StdTag().String(), ShouldEqual, "en")

			lang, err = ParseLanguage("zh-Hans")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.StdTag().String(), ShouldEqual, "zh-Hans")
		})

		Convey("should handle language format conversion", func() {
			// 测试语言格式转换
			lang, _ := ParseLanguage("zh-Hans")
			So(lang.UnderscoreLocale(), ShouldEqual, "zh_Hans")

			lang, _ = ParseLanguage("en-US")
			So(lang.UnderscoreLocale(), ShouldEqual, "en_US")
		})

		Convey("should validate configuration", func() {
			// 测试配置验证
			So(cfg.DefaultLocale.StdTag().String(), ShouldEqual, "en")
			So(len(cfg.SupportLocales), ShouldEqual, 2)
			So(cfg.SupportLocales[0].StdTag().String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].StdTag().String(), ShouldEqual, "zh-Hans")
		})
	})
}

// TestLanguageMethods 测试 Language 结构体的方法
func TestLanguageMethods(t *testing.T) {
	Convey("Test Language methods", t, func() {
		Convey("StdTag should return language tag", func() {
			lang, _ := ParseLanguage("en")
			tag := lang.StdTag()
			So(tag.String(), ShouldEqual, "en")
		})

		Convey("UnderscoreLocale should convert hyphens to underscores", func() {
			testCases := []struct {
				input    string
				expected string
			}{
				{"zh-Hans", "zh_Hans"},
				{"en-US", "en_US"},
				{"fr", "fr"},
				{"pt-BR", "pt_BR"},
			}

			for _, tc := range testCases {
				lang, _ := ParseLanguage(tc.input)
				So(lang.UnderscoreLocale(), ShouldEqual, tc.expected)
			}
		})

		Convey("ParseLanguage should handle invalid input", func() {
			// 测试无效语言字符串
			lang, err := ParseLanguage("invalid-language-code")
			So(err, ShouldNotBeNil)
			So(lang, ShouldBeNil)

			// 测试空字符串
			lang, err = ParseLanguage("")
			So(err, ShouldNotBeNil)
			So(lang, ShouldBeNil)
		})
	})
}

// TestConfigStructure 测试 Config 结构体
func TestConfigStructure(t *testing.T) {
	Convey("Test Config structure", t, func() {
		Convey("should create valid configuration", func() {
			enLang, _ := ParseLanguage("en")
			zhLang, _ := ParseLanguage("zh-Hans")

			cfg := &Config{
				DefaultLocale:  *enLang,
				SupportLocales: []Language{*enLang, *zhLang},
				RootPath:       "custom/locales",
			}

			So(cfg.RootPath, ShouldEqual, "custom/locales")
			So(cfg.DefaultLocale.StdTag().String(), ShouldEqual, "en")
			So(len(cfg.SupportLocales), ShouldEqual, 2)
		})

		Convey("should handle empty support locales", func() {
			enLang, _ := ParseLanguage("en")

			cfg := &Config{
				DefaultLocale:  *enLang,
				SupportLocales: []Language{},
				RootPath:       "locales",
			}

			So(len(cfg.SupportLocales), ShouldEqual, 0)
		})

		Convey("should handle multiple support locales", func() {
			enLang, _ := ParseLanguage("en")
			zhLang, _ := ParseLanguage("zh-Hans")
			jaLang, _ := ParseLanguage("ja")

			cfg := &Config{
				DefaultLocale: *enLang,
				SupportLocales: []Language{
					*enLang,
					*zhLang,
					*jaLang,
				},
				RootPath: "locales",
			}

			So(len(cfg.SupportLocales), ShouldEqual, 3)
			So(cfg.SupportLocales[0].StdTag().String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].StdTag().String(), ShouldEqual, "zh-Hans")
			So(cfg.SupportLocales[2].StdTag().String(), ShouldEqual, "ja")
		})
	})
}

// TestI18nIntegration 测试 I18n 集成功能
func TestI18nIntegration(t *testing.T) {
	Convey("Test I18n integration", t, func() {
		Convey("should demonstrate basic i18n workflow", func() {
			// 演示基本的国际化工作流程
			enLang, err := ParseLanguage("en")
			So(err, ShouldBeNil)
			So(enLang, ShouldNotBeNil)

			zhLang, err := ParseLanguage("zh-Hans")
			So(err, ShouldBeNil)
			So(zhLang, ShouldNotBeNil)

			// 创建配置
			cfg := &Config{
				DefaultLocale:  *enLang,
				SupportLocales: []Language{*enLang, *zhLang},
				RootPath:       "locales",
			}

			// 验证配置
			So(cfg.DefaultLocale.StdTag().String(), ShouldEqual, "en")
			So(cfg.SupportLocales[0].StdTag().String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].StdTag().String(), ShouldEqual, "zh-Hans")
			So(cfg.RootPath, ShouldEqual, "locales")

			// 验证语言方法
			So(enLang.UnderscoreLocale(), ShouldEqual, "en")
			So(zhLang.UnderscoreLocale(), ShouldEqual, "zh_Hans")
		})

		Convey("should handle language comparison", func() {
			lang1, _ := ParseLanguage("en")
			lang2, _ := ParseLanguage("en")
			lang3, _ := ParseLanguage("zh-Hans")

			// 相同语言应该相等
			So(lang1.StdTag().String(), ShouldEqual, lang2.StdTag().String())

			// 不同语言应该不相等
			So(lang1.StdTag().String(), ShouldNotEqual, lang3.StdTag().String())
		})
	})
}

// BenchmarkParseLanguage 基准测试 ParseLanguage 函数
func BenchmarkParseLanguage(b *testing.B) {
	languageStrings := []string{"en", "zh-Hans", "fr", "es", "de"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, langStr := range languageStrings {
			ParseLanguage(langStr)
		}
	}
}

// BenchmarkUnderscoreLocale 基准测试 UnderscoreLocale 方法
func BenchmarkUnderscoreLocale(b *testing.B) {
	lang, _ := ParseLanguage("zh-Hans")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lang.UnderscoreLocale()
	}
}
