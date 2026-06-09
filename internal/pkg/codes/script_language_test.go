package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetISO15924Codes(t *testing.T) {
	Convey("验证单例行为", t, func() {
		instance1 := GetISO15924Codes()
		instance2 := GetISO15924Codes()
		Convey("1. 多次调用应返回同一实例", func(c C) {
			So(instance1, ShouldPointTo, instance2)
		})
		Convey("2. 单例实例不应为 nil", func(c C) {
			So(instance1, ShouldNotBeNil)
		})
	})
}

func TestISO15924Codes_ConvertFromAlpha4(t *testing.T) {
	Convey("根据 Alpha-4 代码查询", t, func() {
		checker := GetISO15924Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha4(""), ShouldBeNil)
			So(checker.ConvertFromAlpha4("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha4("XXXX"), ShouldBeNil)
		})
		Convey("2. 查询 Hans（简体汉字）", func(c C) {
			result := checker.ConvertFromAlpha4("Hans")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hans")
			So(result.Name, ShouldEqual, "Han (Simplified variant)")
			So(result.Numeric, ShouldEqual, "501")
		})
		Convey("3. 查询 Hant（繁体汉字）", func(c C) {
			result := checker.ConvertFromAlpha4("Hant")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hant")
			So(result.Name, ShouldEqual, "Han (Traditional variant)")
			So(result.Numeric, ShouldEqual, "502")
		})
		Convey("4. 查询 Latn（拉丁文）", func(c C) {
			result := checker.ConvertFromAlpha4("Latn")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Latn")
			So(result.Name, ShouldEqual, "Latin")
			So(result.Numeric, ShouldEqual, "215")
		})
		Convey("5. 查询 Cyrl（西里尔文）", func(c C) {
			result := checker.ConvertFromAlpha4("Cyrl")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Cyrl")
			So(result.Name, ShouldEqual, "Cyrillic")
			So(result.Numeric, ShouldEqual, "220")
		})
		Convey("6. 查询 Arab（阿拉伯文）", func(c C) {
			result := checker.ConvertFromAlpha4("Arab")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Arab")
			So(result.Name, ShouldEqual, "Arabic")
			So(result.Numeric, ShouldEqual, "160")
		})
		Convey("7. 查询 Jpan（日文）", func(c C) {
			result := checker.ConvertFromAlpha4("Jpan")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Jpan")
			So(result.Name, ShouldEqual, "Japanese (alias for Han + Hiragana + Katakana)")
			So(result.Numeric, ShouldEqual, "413")
		})
		Convey("8. 查询 Kore（韩文）", func(c C) {
			result := checker.ConvertFromAlpha4("Kore")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Kore")
			So(result.Name, ShouldEqual, "Korean (alias for Hangul + Han)")
			So(result.Numeric, ShouldEqual, "287")
		})
		Convey("9. 查询 Grek（希腊文）", func(c C) {
			result := checker.ConvertFromAlpha4("Grek")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Grek")
			So(result.Name, ShouldEqual, "Greek")
			So(result.Numeric, ShouldEqual, "200")
		})
		Convey("10. 查询 Hebr（希伯来文）", func(c C) {
			result := checker.ConvertFromAlpha4("Hebr")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hebr")
			So(result.Name, ShouldEqual, "Hebrew")
			So(result.Numeric, ShouldEqual, "125")
		})
		Convey("11. 查询 Thai（泰文）", func(c C) {
			result := checker.ConvertFromAlpha4("Thai")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Thai")
			So(result.Name, ShouldEqual, "Thai")
			So(result.Numeric, ShouldEqual, "352")
		})
		Convey("12. 查询 Deva（天城文）", func(c C) {
			result := checker.ConvertFromAlpha4("Deva")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Deva")
			So(result.Name, ShouldEqual, "Devanagari (Nagari)")
			So(result.Numeric, ShouldEqual, "315")
		})
	})
}

func TestISO15924Codes_ConvertFromName(t *testing.T) {
	Convey("根据书写系统名称查询", t, func() {
		checker := GetISO15924Codes()
		Convey("1. 查询不存在的名称应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("Unknown Script"), ShouldBeNil)
		})
		Convey("2. 查询 Han (Simplified variant)（简体汉字）", func(c C) {
			result := checker.ConvertFromName("Han (Simplified variant)")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hans")
			So(result.Name, ShouldEqual, "Han (Simplified variant)")
			So(result.Numeric, ShouldEqual, "501")
		})
		Convey("3. 查询 Han (Traditional variant)（繁体汉字）", func(c C) {
			result := checker.ConvertFromName("Han (Traditional variant)")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hant")
			So(result.Name, ShouldEqual, "Han (Traditional variant)")
			So(result.Numeric, ShouldEqual, "502")
		})
		Convey("4. 查询 Latin（拉丁文）", func(c C) {
			result := checker.ConvertFromName("Latin")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Latn")
			So(result.Name, ShouldEqual, "Latin")
			So(result.Numeric, ShouldEqual, "215")
		})
		Convey("5. 查询 Cyrillic（西里尔文）", func(c C) {
			result := checker.ConvertFromName("Cyrillic")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Cyrl")
			So(result.Name, ShouldEqual, "Cyrillic")
			So(result.Numeric, ShouldEqual, "220")
		})
		Convey("6. 查询 Arabic（阿拉伯文）", func(c C) {
			result := checker.ConvertFromName("Arabic")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Arab")
			So(result.Name, ShouldEqual, "Arabic")
			So(result.Numeric, ShouldEqual, "160")
		})
		Convey("7. 查询 Japanese 应返回 nil（完整名称为 Japanese (alias for Han + Hiragana + Katakana)）", func(c C) {
			result := checker.ConvertFromName("Japanese")
			So(result, ShouldBeNil)
		})
		Convey("8. 查询 Korean 应返回 nil（完整名称为 Korean (alias for Hangul + Han)）", func(c C) {
			result := checker.ConvertFromName("Korean")
			So(result, ShouldBeNil)
		})
	})
}

func TestISO15924Codes_ConvertFromNumeric(t *testing.T) {
	Convey("根据数字代码查询", t, func() {
		checker := GetISO15924Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromNumeric(""), ShouldBeNil)
			So(checker.ConvertFromNumeric("unknown"), ShouldBeNil)
			So(checker.ConvertFromNumeric("000"), ShouldBeNil)
		})
		Convey("2. 查询 501（简体汉字 Hans）", func(c C) {
			result := checker.ConvertFromNumeric("501")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hans")
			So(result.Name, ShouldEqual, "Han (Simplified variant)")
			So(result.Numeric, ShouldEqual, "501")
		})
		Convey("3. 查询 500（汉字 Hani）", func(c C) {
			result := checker.ConvertFromNumeric("500")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Hani")
			So(result.Name, ShouldEqual, "Han (Hanzi, Kanji, Hanja)")
			So(result.Numeric, ShouldEqual, "500")
		})
		Convey("4. 查询 215（拉丁文 Latn）", func(c C) {
			result := checker.ConvertFromNumeric("215")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Latn")
			So(result.Name, ShouldEqual, "Latin")
			So(result.Numeric, ShouldEqual, "215")
		})
		Convey("5. 查询 220（西里尔文 Cyrl）", func(c C) {
			result := checker.ConvertFromNumeric("220")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Cyrl")
			So(result.Name, ShouldEqual, "Cyrillic")
			So(result.Numeric, ShouldEqual, "220")
		})
		Convey("6. 查询 160（阿拉伯文 Arab）", func(c C) {
			result := checker.ConvertFromNumeric("160")
			So(result, ShouldNotBeNil)
			So(result.Alpha4, ShouldEqual, "Arab")
			So(result.Name, ShouldEqual, "Arabic")
			So(result.Numeric, ShouldEqual, "160")
		})
	})
}

func TestISO15924Codes_DataIntegrity(t *testing.T) {
	Convey("验证数据完整性", t, func() {
		checker := GetISO15924Codes()
		Convey("1. Alpha4 索引应包含常见书写系统", func(c C) {
			So(checker.ConvertFromAlpha4("Hans"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Hant"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Latn"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Cyrl"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Arab"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Jpan"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Kore"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Grek"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Hebr"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Thai"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("Deva"), ShouldNotBeNil)
		})
		Convey("2. 书写系统数字代码应为 3 位数字", func(c C) {
			hansResult := checker.ConvertFromAlpha4("Hans")
			So(len(hansResult.Numeric), ShouldEqual, 3)
			latnResult := checker.ConvertFromAlpha4("Latn")
			So(len(latnResult.Numeric), ShouldEqual, 3)
			cyrlResult := checker.ConvertFromAlpha4("Cyrl")
			So(len(cyrlResult.Numeric), ShouldEqual, 3)
			arabResult := checker.ConvertFromAlpha4("Arab")
			So(len(arabResult.Numeric), ShouldEqual, 3)
		})
		Convey("3. 同一条目通过不同查询方式应返回一致的结果", func(c C) {
			byAlpha4 := checker.ConvertFromAlpha4("Hans")
			byName := checker.ConvertFromName("Han (Simplified variant)")
			byNumeric := checker.ConvertFromNumeric("501")
			So(byAlpha4.Alpha4, ShouldEqual, byName.Alpha4)
			So(byAlpha4.Alpha4, ShouldEqual, byNumeric.Alpha4)
			So(byAlpha4.Name, ShouldEqual, byName.Name)
			So(byAlpha4.Name, ShouldEqual, byNumeric.Name)
			So(byAlpha4.Numeric, ShouldEqual, byName.Numeric)
			So(byAlpha4.Numeric, ShouldEqual, byNumeric.Numeric)
		})
		Convey("4. Alpha4 代码大小写敏感", func(c C) {
			So(checker.ConvertFromAlpha4("Hans"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha4("hans"), ShouldBeNil)
			So(checker.ConvertFromAlpha4("HANS"), ShouldBeNil)
		})
	})
}
