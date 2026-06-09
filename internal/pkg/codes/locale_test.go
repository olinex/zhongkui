package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRFC1766Codes(t *testing.T) {
	Convey("获取 RFC 1766 代码查询器实例", t, func() {
		checker := GetRFC1766Codes()
		Convey("1. 实例不应为 nil", func(c C) {
			So(checker, ShouldNotBeNil)
		})
		Convey("2. 多次调用应返回相同实例", func(c C) {
			So(GetRFC1766Codes(), ShouldEqual, checker)
		})
	})
}

func TestRFC1766Codes(t *testing.T) {
	Convey("根据区域设置名称查询", t, func() {
		checker := GetRFC1766Codes()
		Convey("1. 查询不存在的区域设置应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("xx_XX"), ShouldBeNil)
		})
		Convey("2. 语言存在但国家/地区不存在应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_XX"), ShouldBeNil)
		})
		Convey("3. 国家/地区存在但语言不存在应返回 nil", func(c C) {
			So(checker.ConvertFromName("xx_CN"), ShouldBeNil)
		})
		Convey("4. 查询 zh_CN（下划线分隔）", func(c C) {
			result := checker.ConvertFromName("zh_CN")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "zh")
			So(result.Language.Alpha3, ShouldEqual, "zho")
			So(result.Language.Name, ShouldEqual, "Chinese")
			So(result.Country.Alpha2, ShouldEqual, "CN")
			So(result.Country.Alpha3, ShouldEqual, "CHN")
			So(result.Country.Name, ShouldEqual, "China")
		})
		Convey("5. 查询 zh-CN（连字符分隔）", func(c C) {
			result := checker.ConvertFromName("zh-CN")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "zh")
			So(result.Country.Alpha2, ShouldEqual, "CN")
			So(result.Language.Name, ShouldEqual, "Chinese")
			So(result.Country.Name, ShouldEqual, "China")
		})
		Convey("6. 查询 en_US", func(c C) {
			result := checker.ConvertFromName("en_US")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "en")
			So(result.Country.Alpha2, ShouldEqual, "US")
			So(result.Language.Name, ShouldEqual, "English")
			So(result.Country.Name, ShouldEqual, "United States")
		})
		Convey("7. 查询 en-GB", func(c C) {
			result := checker.ConvertFromName("en-GB")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "en")
			So(result.Country.Alpha2, ShouldEqual, "GB")
			So(result.Language.Name, ShouldEqual, "English")
			So(result.Country.Name, ShouldEqual, "United Kingdom")
		})
		Convey("8. 查询 ja_JP", func(c C) {
			result := checker.ConvertFromName("ja_JP")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ja")
			So(result.Country.Alpha2, ShouldEqual, "JP")
			So(result.Language.Name, ShouldEqual, "Japanese")
			So(result.Country.Name, ShouldEqual, "Japan")
		})
		Convey("9. 查询 ko_KR", func(c C) {
			result := checker.ConvertFromName("ko_KR")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ko")
			So(result.Country.Alpha2, ShouldEqual, "KR")
			So(result.Language.Name, ShouldEqual, "Korean")
			So(result.Country.Name, ShouldEqual, "Korea, Republic of")
		})
		Convey("10. 查询 fr_FR", func(c C) {
			result := checker.ConvertFromName("fr_FR")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "fr")
			So(result.Country.Alpha2, ShouldEqual, "FR")
			So(result.Language.Name, ShouldEqual, "French")
			So(result.Country.Name, ShouldEqual, "France")
		})
		Convey("11. 查询 de_DE", func(c C) {
			result := checker.ConvertFromName("de_DE")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "de")
			So(result.Country.Alpha2, ShouldEqual, "DE")
			So(result.Language.Name, ShouldEqual, "German")
			So(result.Country.Name, ShouldEqual, "Germany")
		})
		Convey("12. 查询 ru_RU", func(c C) {
			result := checker.ConvertFromName("ru_RU")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ru")
			So(result.Country.Alpha2, ShouldEqual, "RU")
			So(result.Language.Name, ShouldEqual, "Russian")
			So(result.Country.Name, ShouldEqual, "Russian Federation")
		})
		Convey("13. 查询 es_ES", func(c C) {
			result := checker.ConvertFromName("es_ES")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "es")
			So(result.Country.Alpha2, ShouldEqual, "ES")
			So(result.Language.Name, ShouldEqual, "Spanish; Castilian")
			So(result.Country.Name, ShouldEqual, "Spain")
		})
		Convey("14. 查询 ar_SA", func(c C) {
			result := checker.ConvertFromName("ar_SA")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ar")
			So(result.Country.Alpha2, ShouldEqual, "SA")
			So(result.Language.Name, ShouldEqual, "Arabic")
			So(result.Country.Name, ShouldEqual, "Saudi Arabia")
		})
		Convey("15. 查询 pt_BR", func(c C) {
			result := checker.ConvertFromName("pt_BR")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "pt")
			So(result.Country.Alpha2, ShouldEqual, "BR")
			So(result.Language.Name, ShouldEqual, "Portuguese")
			So(result.Country.Name, ShouldEqual, "Brazil")
		})
	})

	Convey("验证无效格式", t, func() {
		checker := GetRFC1766Codes()
		Convey("1. 单一组成部分应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh"), ShouldBeNil)
		})
		Convey("2. 三个组成部分应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_CN_UTF8"), ShouldBeNil)
		})
		Convey("3. 语言为空应返回 nil", func(c C) {
			So(checker.ConvertFromName("_CN"), ShouldBeNil)
		})
		Convey("4. 国家/地区为空应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_"), ShouldBeNil)
		})
	})

	Convey("验证大小写敏感性", t, func() {
		checker := GetRFC1766Codes()
		Convey("1. 国家代码使用小写应返回 nil（ISO 3166-1 Alpha-2 为大写）", func(c C) {
			So(checker.ConvertFromName("zh_cn"), ShouldBeNil)
		})
		Convey("2. 语言代码使用大写应返回 nil（ISO 639-2 Alpha-2 为小写）", func(c C) {
			So(checker.ConvertFromName("ZH_CN"), ShouldBeNil)
		})
	})

	Convey("验证下划线与连字符结果一致性", t, func() {
		checker := GetRFC1766Codes()
		Convey("1. zh_CN 和 zh-CN 应返回相同的语言和国家代码", func(c C) {
			usResult := checker.ConvertFromName("zh_CN")
			hyphenResult := checker.ConvertFromName("zh-CN")
			So(usResult.Language.Alpha2, ShouldEqual, hyphenResult.Language.Alpha2)
			So(usResult.Country.Alpha2, ShouldEqual, hyphenResult.Country.Alpha2)
		})
		Convey("2. en_US 和 en-US 应返回相同的语言和国家代码", func(c C) {
			usResult := checker.ConvertFromName("en_US")
			hyphenResult := checker.ConvertFromName("en-US")
			So(usResult.Language.Alpha2, ShouldEqual, hyphenResult.Language.Alpha2)
			So(usResult.Country.Alpha2, ShouldEqual, hyphenResult.Country.Alpha2)
		})
	})
}

func TestGetRFC4646Codes(t *testing.T) {
	Convey("获取 RFC 4646 代码查询器实例", t, func() {
		checker := GetRFC4646Codes()
		Convey("1. 实例不应为 nil", func(c C) {
			So(checker, ShouldNotBeNil)
		})
		Convey("2. 多次调用应返回相同实例", func(c C) {
			So(GetRFC4646Codes(), ShouldEqual, checker)
		})
	})
}

func TestRFC4646Codes(t *testing.T) {
	Convey("根据语言标签名称查询", t, func() {
		checker := GetRFC4646Codes()
		Convey("1. 查询不存在的语言标签应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("xx_XXXX"), ShouldBeNil)
		})
		Convey("2. 语言存在但脚本不存在应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_XXXX"), ShouldBeNil)
		})
		Convey("3. 脚本存在但语言不存在应返回 nil", func(c C) {
			So(checker.ConvertFromName("xx_Hans"), ShouldBeNil)
		})
		Convey("4. 查询 zh_Hans（下划线分隔）", func(c C) {
			result := checker.ConvertFromName("zh_Hans")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "zh")
			So(result.Language.Alpha3, ShouldEqual, "zho")
			So(result.Language.Name, ShouldEqual, "Chinese")
			So(result.Script.Alpha4, ShouldEqual, "Hans")
			So(result.Script.Name, ShouldEqual, "Han (Simplified variant)")
			So(result.Script.Numeric, ShouldEqual, "501")
		})
		Convey("5. 查询 zh-Hans（连字符分隔）", func(c C) {
			result := checker.ConvertFromName("zh-Hans")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "zh")
			So(result.Script.Alpha4, ShouldEqual, "Hans")
			So(result.Language.Name, ShouldEqual, "Chinese")
			So(result.Script.Name, ShouldEqual, "Han (Simplified variant)")
		})
		Convey("6. 查询 zh_Hant", func(c C) {
			result := checker.ConvertFromName("zh_Hant")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "zh")
			So(result.Script.Alpha4, ShouldEqual, "Hant")
			So(result.Language.Name, ShouldEqual, "Chinese")
			So(result.Script.Name, ShouldEqual, "Han (Traditional variant)")
		})
		Convey("7. 查询 en_Latn", func(c C) {
			result := checker.ConvertFromName("en_Latn")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "en")
			So(result.Script.Alpha4, ShouldEqual, "Latn")
			So(result.Language.Name, ShouldEqual, "English")
			So(result.Script.Name, ShouldEqual, "Latin")
		})
		Convey("8. 查询 ja_Jpan", func(c C) {
			result := checker.ConvertFromName("ja_Jpan")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ja")
			So(result.Script.Alpha4, ShouldEqual, "Jpan")
			So(result.Language.Name, ShouldEqual, "Japanese")
			So(result.Script.Name, ShouldEqual, "Japanese (alias for Han + Hiragana + Katakana)")
		})
		Convey("9. 查询 ko_Kore", func(c C) {
			result := checker.ConvertFromName("ko_Kore")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ko")
			So(result.Script.Alpha4, ShouldEqual, "Kore")
			So(result.Language.Name, ShouldEqual, "Korean")
			So(result.Script.Name, ShouldEqual, "Korean (alias for Hangul + Han)")
		})
		Convey("10. 查询 ar_Arab", func(c C) {
			result := checker.ConvertFromName("ar_Arab")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ar")
			So(result.Script.Alpha4, ShouldEqual, "Arab")
			So(result.Language.Name, ShouldEqual, "Arabic")
			So(result.Script.Name, ShouldEqual, "Arabic")
		})
		Convey("11. 查询 ru_Cyrl", func(c C) {
			result := checker.ConvertFromName("ru_Cyrl")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "ru")
			So(result.Script.Alpha4, ShouldEqual, "Cyrl")
			So(result.Language.Name, ShouldEqual, "Russian")
			So(result.Script.Name, ShouldEqual, "Cyrillic")
		})
		Convey("12. 查询 el_Grek", func(c C) {
			result := checker.ConvertFromName("el_Grek")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "el")
			So(result.Script.Alpha4, ShouldEqual, "Grek")
			So(result.Language.Name, ShouldEqual, "Greek, Modern (1453-)")
			So(result.Script.Name, ShouldEqual, "Greek")
		})
		Convey("13. 查询 de_Latn", func(c C) {
			result := checker.ConvertFromName("de_Latn")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "de")
			So(result.Script.Alpha4, ShouldEqual, "Latn")
			So(result.Language.Name, ShouldEqual, "German")
			So(result.Script.Name, ShouldEqual, "Latin")
		})
		Convey("14. 查询 fr_Latn", func(c C) {
			result := checker.ConvertFromName("fr_Latn")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "fr")
			So(result.Script.Alpha4, ShouldEqual, "Latn")
			So(result.Language.Name, ShouldEqual, "French")
			So(result.Script.Name, ShouldEqual, "Latin")
		})
		Convey("15. 查询 he_Hebr", func(c C) {
			result := checker.ConvertFromName("he_Hebr")
			So(result, ShouldNotBeNil)
			So(result.Language.Alpha2, ShouldEqual, "he")
			So(result.Script.Alpha4, ShouldEqual, "Hebr")
			So(result.Language.Name, ShouldEqual, "Hebrew")
			So(result.Script.Name, ShouldEqual, "Hebrew")
		})
	})

	Convey("验证无效格式", t, func() {
		checker := GetRFC4646Codes()
		Convey("1. 单一组成部分应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh"), ShouldBeNil)
		})
		Convey("2. 三个组成部分应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_Hans_CN"), ShouldBeNil)
		})
		Convey("3. 语言为空应返回 nil", func(c C) {
			So(checker.ConvertFromName("_Hans"), ShouldBeNil)
		})
		Convey("4. 脚本为空应返回 nil", func(c C) {
			So(checker.ConvertFromName("zh_"), ShouldBeNil)
		})
	})

	Convey("验证下划线与连字符结果一致性", t, func() {
		checker := GetRFC4646Codes()
		Convey("1. zh_Hans 和 zh-Hans 应返回相同的语言和脚本代码", func(c C) {
			usResult := checker.ConvertFromName("zh_Hans")
			hyphenResult := checker.ConvertFromName("zh-Hans")
			So(usResult.Language.Alpha2, ShouldEqual, hyphenResult.Language.Alpha2)
			So(usResult.Script.Alpha4, ShouldEqual, hyphenResult.Script.Alpha4)
		})
		Convey("2. en_Latn 和 en-Latn 应返回相同的语言和脚本代码", func(c C) {
			usResult := checker.ConvertFromName("en_Latn")
			hyphenResult := checker.ConvertFromName("en-Latn")
			So(usResult.Language.Alpha2, ShouldEqual, hyphenResult.Language.Alpha2)
			So(usResult.Script.Alpha4, ShouldEqual, hyphenResult.Script.Alpha4)
		})
	})

	Convey("验证数据完整性", t, func() {
		checker := GetRFC4646Codes()
		Convey("1. 常见语言脚本组合应可查询", func(c C) {
			So(checker.ConvertFromName("zh_Hans"), ShouldNotBeNil)
			So(checker.ConvertFromName("zh_Hant"), ShouldNotBeNil)
			So(checker.ConvertFromName("en_Latn"), ShouldNotBeNil)
			So(checker.ConvertFromName("ja_Jpan"), ShouldNotBeNil)
			So(checker.ConvertFromName("ko_Kore"), ShouldNotBeNil)
			So(checker.ConvertFromName("ar_Arab"), ShouldNotBeNil)
			So(checker.ConvertFromName("ru_Cyrl"), ShouldNotBeNil)
			So(checker.ConvertFromName("el_Grek"), ShouldNotBeNil)
			So(checker.ConvertFromName("hi_Deva"), ShouldNotBeNil)
			So(checker.ConvertFromName("th_Thai"), ShouldNotBeNil)
		})
	})
}
