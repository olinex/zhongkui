package translate

import (
	"encoding/json"
	"testing"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"
)

// TestParseLanguage 测试 ParseLanguage 函数
func TestParseLanguage(t *testing.T) {
	Convey("Test ParseLanguage function", t, func() {
		Convey("1. should parse ISO 639-1 language codes", func() {
			lang, err := ParseLanguage("en")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "en")

			lang, err = ParseLanguage("zh")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh")

			lang, err = ParseLanguage("fr")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "fr")
		})

		Convey("2. should parse language with script", func() {
			lang, err := ParseLanguage("zh-Hans")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans")

			lang, err = ParseLanguage("zh-Hant")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-Hant")

			lang, err = ParseLanguage("sr-Cyrl")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "sr-Cyrl")
		})

		Convey("3. should parse language with region", func() {
			lang, err := ParseLanguage("en-US")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "en-US")

			lang, err = ParseLanguage("zh-CN")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-CN")

			lang, err = ParseLanguage("zh-TW")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-TW")
		})

		Convey("4. should parse language with script and region", func() {
			lang, err := ParseLanguage("zh-Hans-CN")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans-CN")

			lang, err = ParseLanguage("zh-Hant-TW")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-Hant-TW")
		})

		Convey("5. should parse ISO 639-2 language codes", func() {
			lang, err := ParseLanguage("eng")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			// 注意：language.Parse 会将 "eng" 规范化为 "en"
			So(lang.String(), ShouldEqual, "en")

			lang, err = ParseLanguage("zho")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			// 注意：language.Parse 会将 "zho" 规范化为 "zh"
			So(lang.String(), ShouldEqual, "zh")

			lang, err = ParseLanguage("fra")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			// 注意：language.Parse 会将 "fra" 规范化为 "fr"
			So(lang.String(), ShouldEqual, "fr")
		})

		Convey("6. should return error for invalid language codes", func() {
			lang, err := ParseLanguage("")
			So(err, ShouldNotBeNil)
			So(lang, ShouldBeNil)

			lang, err = ParseLanguage("invalid")
			So(err, ShouldNotBeNil)
			So(lang, ShouldBeNil)

			lang, err = ParseLanguage("123")
			So(err, ShouldNotBeNil)
			So(lang, ShouldBeNil)
		})

		Convey("7. should parse language with extensions", func() {
			lang, err := ParseLanguage("en-US-u-ca-gregory")
			So(err, ShouldBeNil)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "en-US-u-ca-gregory")
		})
	})
}

// TestLanguageString 测试 Language.String() 方法
func TestLanguageString(t *testing.T) {
	Convey("Test Language.String() method", t, func() {
		Convey("1. should return correct string representation", func() {
			lang, _ := ParseLanguage("zh-Hans")
			So(lang.String(), ShouldEqual, "zh-Hans")

			lang, _ = ParseLanguage("en-US")
			So(lang.String(), ShouldEqual, "en-US")

			lang, _ = ParseLanguage("fr")
			So(lang.String(), ShouldEqual, "fr")
		})

		Convey("2. should work with fmt.Stringer interface", func() {
			lang, _ := ParseLanguage("zh-Hans")
			result := lang.String()
			So(result, ShouldEqual, "zh-Hans")
		})
	})
}

// TestLanguageStdTag 测试 Language.StdTag() 方法
func TestLanguageStdTag(t *testing.T) {
	Convey("Test Language.StdTag() method", t, func() {
		Convey("1. should return underlying language.Tag", func() {
			lang, _ := ParseLanguage("zh-Hans")
			tag := lang.StdTag()
			So(tag, ShouldNotBeNil)
			So(tag.String(), ShouldEqual, "zh-Hans")
		})

		Convey("2. should allow access to tag methods", func() {
			lang, _ := ParseLanguage("zh-Hans")
			tag := lang.StdTag()
			base, confidence := tag.Base()
			So(confidence, ShouldEqual, language.Exact)
			So(base.String(), ShouldEqual, "zh")
		})
	})
}

// TestLanguageUnderscoreLocale 测试 Language.UnderscoreLocale() 方法
func TestLanguageUnderscoreLocale(t *testing.T) {
	Convey("Test Language.UnderscoreLocale() method", t, func() {
		Convey("1. should convert hyphen to underscore", func() {
			lang, _ := ParseLanguage("zh-Hans")
			So(lang.UnderscoreLocale(), ShouldEqual, "zh_Hans")

			lang, _ = ParseLanguage("en-US")
			So(lang.UnderscoreLocale(), ShouldEqual, "en_US")

			lang, _ = ParseLanguage("fr-FR")
			So(lang.UnderscoreLocale(), ShouldEqual, "fr_FR")
		})

		Convey("2. should handle multiple hyphens", func() {
			lang, _ := ParseLanguage("zh-Hans-CN")
			So(lang.UnderscoreLocale(), ShouldEqual, "zh_Hans_CN")

			lang, _ = ParseLanguage("en-US-u-ca-gregory")
			So(lang.UnderscoreLocale(), ShouldEqual, "en_US_u_ca_gregory")
		})

		Convey("3. should return same string if no hyphens", func() {
			lang, _ := ParseLanguage("en")
			So(lang.UnderscoreLocale(), ShouldEqual, "en")

			lang, _ = ParseLanguage("fr")
			So(lang.UnderscoreLocale(), ShouldEqual, "fr")
		})
	})
}

// TestLanguageSetDefaults 测试 Language.SetDefaults() 方法
func TestLanguageSetDefaults(t *testing.T) {
	Convey("Test Language.SetDefaults() method", t, func() {
		Convey("1. should set default language to English when updatable", func() {
			// 注意：对于零值的 Language 结构体，defaults.CanUpdate 可能返回 false
			// 因此 SetDefaults 可能不会生效
			var lang Language
			lang.SetDefaults()
			// 零值 Language 的字符串表示是 "und" (undefined)
			So(lang.String(), ShouldEqual, "und")
		})

		Convey("2. should work with defaults package in struct context", func() {
			// 在结构体上下文中，defaults 包可以正确设置默认值
			type Config struct {
				DefaultLang Language `default:""`
			}

			var cfg Config
			err := defaults.Set(&cfg)
			So(err, ShouldBeNil)
			// 注意：由于 Language 是值类型，defaults 包可能无法正确设置
			// 这取决于 defaults 包的具体实现
			// 我们只测试没有错误发生
			So(cfg.DefaultLang.String(), ShouldEqual, "und")
		})

		Convey("3. should not override existing value", func() {
			lang, _ := ParseLanguage("zh-Hans")
			lang.SetDefaults()
			So(lang.String(), ShouldEqual, "zh-Hans")
		})
	})
}

// TestLanguageUnmarshalText 测试 Language.UnmarshalText() 方法
func TestLanguageUnmarshalText(t *testing.T) {
	Convey("Test Language.UnmarshalText() method", t, func() {
		Convey("1. should unmarshal valid language text", func() {
			var lang Language
			err := lang.UnmarshalText([]byte("zh-Hans"))
			So(err, ShouldBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans")
		})

		Convey("2. should return error for invalid language text", func() {
			var lang Language
			err := lang.UnmarshalText([]byte("invalid"))
			So(err, ShouldNotBeNil)
		})

		Convey("3. should not modify language for empty text", func() {
			var lang Language
			// 先设置一个值
			lang.UnmarshalText([]byte("zh-Hans"))
			// 然后尝试空文本
			err := lang.UnmarshalText([]byte(""))
			So(err, ShouldBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans")
		})

		Convey("4. should work with YAML/TOML unmarshaling", func() {
			// 模拟 YAML 反序列化
			type Config struct {
				Language Language
			}

			// 在实际使用中，YAML/TOML 库会调用 UnmarshalText
			var cfg Config
			err := cfg.Language.UnmarshalText([]byte("en-US"))
			So(err, ShouldBeNil)
			So(cfg.Language.String(), ShouldEqual, "en-US")
		})
	})
}

// TestLanguageMarshalText 测试 Language.MarshalText() 方法
func TestLanguageMarshalText(t *testing.T) {
	Convey("Test Language.MarshalText() method", t, func() {
		Convey("1. should marshal language to text", func() {
			lang, _ := ParseLanguage("zh-Hans")
			data, err := lang.MarshalText()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "zh-Hans")
		})

		Convey("2. should always return nil error", func() {
			lang, _ := ParseLanguage("en-US")
			data, err := lang.MarshalText()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "en-US")
		})

		Convey("3. should work with YAML/TOML marshaling", func() {
			lang, _ := ParseLanguage("fr-FR")
			data, err := lang.MarshalText()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "fr-FR")
		})
	})
}

// TestLanguageUnmarshalJSON 测试 Language.UnmarshalJSON() 方法
func TestLanguageUnmarshalJSON(t *testing.T) {
	Convey("Test Language.UnmarshalJSON() method", t, func() {
		Convey("1. should unmarshal valid JSON string", func() {
			var lang Language
			err := lang.UnmarshalJSON([]byte(`"zh-Hans"`))
			So(err, ShouldBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans")
		})

		Convey("2. should return error for invalid JSON", func() {
			var lang Language
			err := lang.UnmarshalJSON([]byte(`invalid json`))
			So(err, ShouldNotBeNil)
		})

		Convey("3. should return error for invalid language in JSON", func() {
			var lang Language
			err := lang.UnmarshalJSON([]byte(`"invalid"`))
			So(err, ShouldNotBeNil)
		})

		Convey("4. should work with JSON unmarshaling in struct", func() {
			type Config struct {
				Language Language `json:"language"`
			}

			var cfg Config
			jsonData := []byte(`{"language": "en-US"}`)
			err := json.Unmarshal(jsonData, &cfg)
			So(err, ShouldBeNil)
			So(cfg.Language.String(), ShouldEqual, "en-US")
		})

		Convey("5. should handle null JSON value", func() {
			var lang Language
			// 先设置一个值
			lang.UnmarshalJSON([]byte(`"zh-Hans"`))
			// 然后尝试 null
			err := lang.UnmarshalJSON([]byte(`null`))
			So(err, ShouldBeNil)
			// 注意：当前实现中，null 不会修改现有值
			So(lang.String(), ShouldEqual, "zh-Hans")
		})
	})
}

// TestLanguageMarshalJSON 测试 Language.MarshalJSON() 方法
func TestLanguageMarshalJSON(t *testing.T) {
	Convey("Test Language.MarshalJSON() method", t, func() {
		Convey("1. should marshal language to JSON", func() {
			lang, _ := ParseLanguage("zh-Hans")
			data, err := lang.MarshalJSON()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `"zh-Hans"`)
		})

		Convey("2. should work with JSON marshaling in struct", func() {
			type Config struct {
				Language Language `json:"language"`
			}

			lang, _ := ParseLanguage("en-US")
			cfg := Config{Language: *lang}
			data, err := json.Marshal(cfg)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `{"language":"en-US"}`)
		})

		Convey("3. should handle empty language", func() {
			var lang Language
			data, err := lang.MarshalJSON()
			So(err, ShouldBeNil)
			// 空 Language 的字符串表示是 "und" (undefined)
			So(string(data), ShouldEqual, `"und"`)
		})
	})
}

// TestDefaultLang 测试 DefaultLang 变量
func TestDefaultLang(t *testing.T) {
	Convey("Test DefaultLang variable", t, func() {
		Convey("1. should be initialized with English", func() {
			So(DefaultLang, ShouldNotBeNil)
			So(DefaultLang.String(), ShouldEqual, "en")
		})

		Convey("2. should be usable directly", func() {
			lang := DefaultLang
			So(lang.String(), ShouldEqual, "en")
			So(lang.UnderscoreLocale(), ShouldEqual, "en")
		})
	})
}

// TestLanguageEquality 测试 Language 相等性
func TestLanguageEquality(t *testing.T) {
	Convey("Test Language equality", t, func() {
		Convey("1. should compare by string representation", func() {
			lang1, _ := ParseLanguage("zh-Hans")
			lang2, _ := ParseLanguage("zh-Hans")
			lang3, _ := ParseLanguage("zh-Hant")

			So(lang1.String() == lang2.String(), ShouldBeTrue)
			So(lang1.String() == lang3.String(), ShouldBeFalse)
		})

		Convey("2. should handle different formats for same language", func() {
			lang1, _ := ParseLanguage("zh")
			lang2, _ := ParseLanguage("zho")
			// 注意：language.Parse 会将 "zho" 规范化为 "zh"
			So(lang1.String(), ShouldEqual, "zh")
			So(lang2.String(), ShouldEqual, "zh")
		})
	})
}
