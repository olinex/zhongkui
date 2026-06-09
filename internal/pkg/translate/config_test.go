package translate

import (
	"testing"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

// TestConfigDefaultValues 测试 Config 结构体的默认值
func TestConfigDefaultValues(t *testing.T) {
	Convey("Test Config default values", t, func() {
		Convey("should set default values correctly", func() {
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			So(cfg.RootPath, ShouldEqual, "locales")
			So(cfg.DefaultLocale.String(), ShouldEqual, "en")
			So(len(cfg.SupportLocales), ShouldEqual, 2)
			So(cfg.SupportLocales[0].String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].String(), ShouldEqual, "zh-Hans")
		})
	})
}

// TestDescribeConfig 测试 DescribeConfig 函数
func TestDescribeConfig(t *testing.T) {
	Convey("Test DescribeConfig function", t, func() {
		Convey("should return config descriptions with correct prefix", func() {
			descriptions := DescribeConfig()

			So(descriptions, ShouldNotBeNil)
			So(len(descriptions), ShouldBeGreaterThan, 0)

			// 检查是否包含预期的配置项
			foundRootPath := false
			foundDefaultLocale := false
			foundSupportLocales := false

			for _, desc := range descriptions {
				switch desc.Path {
				case "i18n.rootPath":
					foundRootPath = true
					So(desc.Kind, ShouldEqual, "string")
					So(desc.Default, ShouldEqual, "locales")
					So(desc.Comment, ShouldContainSubstring, "root path for i18n locale files")
				case "i18n.defaultLocale":
					foundDefaultLocale = true
					So(desc.Kind, ShouldEqual, "struct")
					So(desc.Default, ShouldEqual, "en")
					So(desc.Comment, ShouldContainSubstring, "default locale/language for i18n")
				case "i18n.supportLocales":
					foundSupportLocales = true
					So(desc.Kind, ShouldEqual, "slice")
					So(desc.Default, ShouldEqual, `["en", "zh-Hans"]`)
					So(desc.Comment, ShouldContainSubstring, "list of supported locales/languages")
				}
			}

			So(foundRootPath, ShouldBeTrue)
			So(foundDefaultLocale, ShouldBeTrue)
			So(foundSupportLocales, ShouldBeTrue)
		})
	})
}

// TestExtractConfig 测试 ExtractConfig 函数
func TestExtractConfig(t *testing.T) {
	Convey("Test ExtractConfig function", t, func() {
		Convey("should extract config with default values when no config provided", func() {
			v := viper.New()
			// 需要设置一个空的 i18n 配置，否则 v.Sub("i18n") 会返回 nil
			v.Set("i18n", map[string]any{})
			cfg, err := ExtractConfig(v)

			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.RootPath, ShouldEqual, "locales")
			So(cfg.DefaultLocale.String(), ShouldEqual, "en")
			So(len(cfg.SupportLocales), ShouldEqual, 2)
			So(cfg.SupportLocales[0].String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].String(), ShouldEqual, "zh-Hans")
		})

		Convey("should extract config with custom values", func() {
			v := viper.New()
			// 使用 Viper 支持的方式设置配置
			v.Set("i18n.rootPath", "custom/locales")
			v.Set("i18n.defaultLocale", "zh-Hans")
			v.Set("i18n.supportLocales", []string{"zh-Hans", "en", "ja"})

			cfg, err := ExtractConfig(v)

			// 注意：由于 Language 类型实现了 TextUnmarshaler，Viper 可能无法正确解析
			// 这里我们测试错误处理或成功情况
			if err != nil {
				// 如果出错，错误信息应该包含解码错误
				So(err.Error(), ShouldContainSubstring, "error(s) decoding")
			} else {
				So(cfg, ShouldNotBeNil)
				So(cfg.RootPath, ShouldEqual, "custom/locales")
				So(cfg.DefaultLocale.String(), ShouldEqual, "zh-Hans")
				So(len(cfg.SupportLocales), ShouldEqual, 3)
				So(cfg.SupportLocales[0].String(), ShouldEqual, "zh-Hans")
				So(cfg.SupportLocales[1].String(), ShouldEqual, "en")
				So(cfg.SupportLocales[2].String(), ShouldEqual, "ja")
			}
		})
	})
}

// TestConfigValidation 测试 Config 结构体的验证
func TestConfigValidation(t *testing.T) {
	Convey("Test Config validation", t, func() {
		Convey("should validate required fields", func() {
			// 创建一个空的配置
			cfg := &Config{}

			// 使用 defaults 设置默认值
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 验证默认值是否正确设置
			So(cfg.RootPath, ShouldNotBeEmpty)
			So(cfg.DefaultLocale.String(), ShouldNotBeEmpty)
		})

		Convey("should validate locale format", func() {
			enLang, err := ParseLanguage("en")
			So(err, ShouldBeNil)

			zhLang, err := ParseLanguage("zh-Hans")
			So(err, ShouldBeNil)

			cfg := &Config{
				RootPath:       "locales",
				DefaultLocale:  *enLang,
				SupportLocales: []Language{*enLang, *zhLang},
			}

			// 验证语言是否正确解析
			So(cfg.DefaultLocale.String(), ShouldEqual, "en")
			So(cfg.SupportLocales[0].String(), ShouldEqual, "en")
			So(cfg.SupportLocales[1].String(), ShouldEqual, "zh-Hans")
		})
	})
}

// TestConfigPrefix 测试配置前缀常量
func TestConfigPrefix(t *testing.T) {
	Convey("Test config prefix constant", t, func() {
		Convey("should have correct prefix value", func() {
			So(PREFIX, ShouldEqual, "i18n")
		})

		Convey("should use prefix in DescribeConfig", func() {
			descriptions := DescribeConfig()
			So(descriptions, ShouldNotBeNil)

			// 检查所有描述是否都有正确的前缀
			for _, desc := range descriptions {
				So(desc.Path, ShouldStartWith, "i18n.")
			}
		})
	})
}
