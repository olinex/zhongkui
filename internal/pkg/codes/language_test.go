package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// 测试 ISO 639-2 语言的 Alpha2/Alpha3/名称查询。
func TestISO6392Codes(t *testing.T) {
	Convey("根据 Alpha-2 代码查询", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha2(""), ShouldBeNil)
			So(checker.ConvertFromAlpha2("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha2("xx"), ShouldBeNil)
		})
		Convey("2. 查询 zh（中文）", func(c C) {
			result := checker.ConvertFromAlpha2("zh")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "zh")
			So(result.Alpha3, ShouldEqual, "zho")
			So(result.Name, ShouldEqual, "Chinese")
		})
		Convey("3. 查询 en（英语）", func(c C) {
			result := checker.ConvertFromAlpha2("en")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "en")
			So(result.Alpha3, ShouldEqual, "eng")
			So(result.Name, ShouldEqual, "English")
		})
		Convey("4. 查询 ja（日语）", func(c C) {
			result := checker.ConvertFromAlpha2("ja")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ja")
			So(result.Alpha3, ShouldEqual, "jpn")
			So(result.Name, ShouldEqual, "Japanese")
		})
		Convey("5. 查询 ko（韩语）", func(c C) {
			result := checker.ConvertFromAlpha2("ko")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ko")
			So(result.Alpha3, ShouldEqual, "kor")
			So(result.Name, ShouldEqual, "Korean")
		})
		Convey("6. 查询 fr（法语）", func(c C) {
			result := checker.ConvertFromAlpha2("fr")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "fr")
			So(result.Alpha3, ShouldEqual, "fra")
			So(result.Name, ShouldEqual, "French")
		})
		Convey("7. 查询 de（德语）", func(c C) {
			result := checker.ConvertFromAlpha2("de")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "de")
			So(result.Alpha3, ShouldEqual, "deu")
			So(result.Name, ShouldEqual, "German")
		})
		Convey("8. 查询 es（西班牙语）", func(c C) {
			result := checker.ConvertFromAlpha2("es")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "es")
			So(result.Alpha3, ShouldEqual, "spa")
			So(result.Name, ShouldEqual, "Spanish; Castilian")
		})
		Convey("9. 查询 ru（俄语）", func(c C) {
			result := checker.ConvertFromAlpha2("ru")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ru")
			So(result.Alpha3, ShouldEqual, "rus")
			So(result.Name, ShouldEqual, "Russian")
		})
		Convey("10. 查询 ar（阿拉伯语）", func(c C) {
			result := checker.ConvertFromAlpha2("ar")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ar")
			So(result.Alpha3, ShouldEqual, "ara")
			So(result.Name, ShouldEqual, "Arabic")
		})
	})

	Convey("根据 Alpha-3 代码查询", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3(""), ShouldBeNil)
			So(checker.ConvertFromAlpha3("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("xxx"), ShouldBeNil)
		})
		Convey("2. 查询 zho（中文）", func(c C) {
			result := checker.ConvertFromAlpha3("zho")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "zh")
			So(result.Alpha3, ShouldEqual, "zho")
			So(result.Name, ShouldEqual, "Chinese")
		})
		Convey("3. 查询 eng（英语）", func(c C) {
			result := checker.ConvertFromAlpha3("eng")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "en")
			So(result.Alpha3, ShouldEqual, "eng")
			So(result.Name, ShouldEqual, "English")
		})
		Convey("4. 查询 jpn（日语）", func(c C) {
			result := checker.ConvertFromAlpha3("jpn")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ja")
			So(result.Alpha3, ShouldEqual, "jpn")
			So(result.Name, ShouldEqual, "Japanese")
		})
		Convey("5. 查询 kor（韩语）", func(c C) {
			result := checker.ConvertFromAlpha3("kor")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ko")
			So(result.Alpha3, ShouldEqual, "kor")
			So(result.Name, ShouldEqual, "Korean")
		})
		Convey("6. 查询 spa（西班牙语）", func(c C) {
			result := checker.ConvertFromAlpha3("spa")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "es")
			So(result.Alpha3, ShouldEqual, "spa")
			So(result.Name, ShouldEqual, "Spanish; Castilian")
		})
	})

	Convey("根据语言名称查询", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 查询不存在的名称应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("Unknown Language"), ShouldBeNil)
		})
		Convey("2. 查询 Chinese", func(c C) {
			result := checker.ConvertFromName("Chinese")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "zh")
			So(result.Alpha3, ShouldEqual, "zho")
			So(result.Name, ShouldEqual, "Chinese")
		})
		Convey("3. 查询 English", func(c C) {
			result := checker.ConvertFromName("English")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "en")
			So(result.Alpha3, ShouldEqual, "eng")
			So(result.Name, ShouldEqual, "English")
		})
		Convey("4. 查询 Japanese", func(c C) {
			result := checker.ConvertFromName("Japanese")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ja")
			So(result.Alpha3, ShouldEqual, "jpn")
			So(result.Name, ShouldEqual, "Japanese")
		})
		Convey("5. 查询 Korean", func(c C) {
			result := checker.ConvertFromName("Korean")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "ko")
			So(result.Alpha3, ShouldEqual, "kor")
			So(result.Name, ShouldEqual, "Korean")
		})
		Convey("6. 查询 French", func(c C) {
			result := checker.ConvertFromName("French")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "fr")
			So(result.Alpha3, ShouldEqual, "fra")
			So(result.Name, ShouldEqual, "French")
		})
		Convey("7. 查询 German", func(c C) {
			result := checker.ConvertFromName("German")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "de")
			So(result.Alpha3, ShouldEqual, "deu")
			So(result.Name, ShouldEqual, "German")
		})
		Convey("8. 查询 Spanish; Castilian", func(c C) {
			result := checker.ConvertFromName("Spanish; Castilian")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "es")
			So(result.Alpha3, ShouldEqual, "spa")
			So(result.Name, ShouldEqual, "Spanish; Castilian")
		})
	})

	Convey("验证数据完整性", t, func() {
		checker := GetISO6392Codes()
		Convey("1. Alpha2 索引应包含常见语言", func(c C) {
			So(checker.ConvertFromAlpha2("zh"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("en"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("ja"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("ko"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("fr"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("de"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("es"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("ru"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("ar"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("pt"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("it"), ShouldNotBeNil)
		})
		Convey("2. Alpha3 索引应包含常见语言", func(c C) {
			So(checker.ConvertFromAlpha3("zho"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("eng"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("jpn"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("kor"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("fra"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("deu"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("spa"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("rus"), ShouldNotBeNil)
		})
		Convey("3. 无 Alpha-2 代码的语言应仅可通过 Alpha-3 和名称查询", func(c C) {
			result := checker.ConvertFromAlpha3("grc")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "")
			So(result.Alpha3, ShouldEqual, "grc")
		})
	})
}

// 测试 GetISO6392Codes 的单例行为。
func TestISO6392CodesSingleton(t *testing.T) {
	Convey("验证 GetISO6392Codes 单例行为", t, func() {
		Convey("多次调用应返回相同的实例", func(c C) {
			instance1 := GetISO6392Codes()
			instance2 := GetISO6392Codes()
			So(instance1, ShouldPointTo, instance2)
		})
	})
}

// 测试不同索引查询同一语言返回同一条目。
func TestISO6392CodesCrossIndexConsistency(t *testing.T) {
	Convey("验证不同索引之间的交叉一致性", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 通过 Alpha2 查询的结果与 Alpha3 查询应返回同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("zh")
			a3Result := checker.ConvertFromAlpha3("zho")
			So(a2Result, ShouldPointTo, a3Result)
		})
		Convey("2. 通过 Alpha2 查询的结果与 Name 查询应返回同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("en")
			nameResult := checker.ConvertFromName("English")
			So(a2Result, ShouldPointTo, nameResult)
		})
		Convey("3. 通过 Alpha3 查询的结果与 Name 查询应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("jpn")
			nameResult := checker.ConvertFromName("Japanese")
			So(a3Result, ShouldPointTo, nameResult)
		})
		Convey("4. 无 Alpha-2 代码的语言通过 Alpha3 与 Name 查询应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("grc")
			nameResult := checker.ConvertFromName("Greek, Ancient (to 1453)")
			So(a3Result, ShouldPointTo, nameResult)
		})
	})
}

// 测试查询区分大小写。
func TestISO6392CodesCaseSensitive(t *testing.T) {
	Convey("验证查询区分大小写", t, func() {
		checker := GetISO6392Codes()
		Convey("1. Alpha2 查询区分大小写，大写应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha2("ZH"), ShouldBeNil)
			So(checker.ConvertFromAlpha2("En"), ShouldBeNil)
		})
		Convey("2. Alpha3 查询区分大小写，大写应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3("ZHO"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("Eng"), ShouldBeNil)
		})
		Convey("3. Name 查询区分大小写", func(c C) {
			So(checker.ConvertFromName("chinese"), ShouldBeNil)
			So(checker.ConvertFromName("CHINESE"), ShouldBeNil)
		})
	})
}

// 测试更多语言数据。
func TestISO6392CodesMoreLanguages(t *testing.T) {
	Convey("验证更多语言数据", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 葡萄牙语（pt/por）", func(c C) {
			result := checker.ConvertFromAlpha2("pt")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "pt")
			So(result.Alpha3, ShouldEqual, "por")
			So(result.Name, ShouldEqual, "Portuguese")
		})
		Convey("2. 意大利语（it/ita）", func(c C) {
			result := checker.ConvertFromAlpha2("it")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "it")
			So(result.Alpha3, ShouldEqual, "ita")
			So(result.Name, ShouldEqual, "Italian")
		})
		Convey("3. 印地语（hi/hin）", func(c C) {
			result := checker.ConvertFromAlpha2("hi")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "hi")
			So(result.Alpha3, ShouldEqual, "hin")
			So(result.Name, ShouldEqual, "Hindi")
		})
		Convey("4. 泰语（th/tha）", func(c C) {
			result := checker.ConvertFromAlpha2("th")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "th")
			So(result.Alpha3, ShouldEqual, "tha")
			So(result.Name, ShouldEqual, "Thai")
		})
		Convey("5. 越南语（vi/vie）", func(c C) {
			result := checker.ConvertFromAlpha2("vi")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "vi")
			So(result.Alpha3, ShouldEqual, "vie")
			So(result.Name, ShouldEqual, "Vietnamese")
		})
		Convey("6. 荷兰语（nl/nld）", func(c C) {
			result := checker.ConvertFromAlpha2("nl")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "nl")
			So(result.Alpha3, ShouldEqual, "nld")
			So(result.Name, ShouldEqual, "Dutch; Flemish")
		})
	})
}

// 测试 ISO6392Entry 字段完整性。
func TestISO6392EntryFields(t *testing.T) {
	Convey("验证 ISO6392Entry 字段完整性", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 查询到的条目核心字段（Alpha3 和 Name）不应为空", func(c C) {
			result := checker.ConvertFromAlpha2("zh")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldNotBeEmpty)
			So(result.Name, ShouldNotBeEmpty)
		})
		Convey("2. Alpha3 应为 3 个字符", func(c C) {
			result := checker.ConvertFromAlpha2("zh")
			So(len(result.Alpha3), ShouldEqual, 3)
		})
		Convey("3. 有 Alpha-2 代码的语言其 Alpha2 应为 2 个字符", func(c C) {
			result := checker.ConvertFromAlpha2("en")
			So(result.Alpha2, ShouldNotBeEmpty)
			So(len(result.Alpha2), ShouldEqual, 2)
		})
		Convey("4. 无 Alpha-2 代码的语言其 Alpha2 应为空", func(c C) {
			result := checker.ConvertFromAlpha3("grc")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "")
			So(result.Alpha3, ShouldNotBeEmpty)
			So(result.Name, ShouldNotBeEmpty)
		})
	})
}

// 测试 Alpha-2 与 Alpha-3 双向查询的一致性。
func TestISO6392CodesAlpha2Alpha3Bidirectional(t *testing.T) {
	Convey("验证 Alpha-2 与 Alpha-3 双向查询的一致性", t, func() {
		checker := GetISO6392Codes()
		Convey("1. 通过 Alpha2 查到 Alpha3，再用 Alpha3 反查应得到同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("zh")
			So(a2Result, ShouldNotBeNil)
			a3Result := checker.ConvertFromAlpha3(a2Result.Alpha3)
			So(a3Result, ShouldPointTo, a2Result)
		})
		Convey("2. 通过 Alpha3 查到 Alpha2（非空时），再用 Alpha2 反查应得到同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("eng")
			So(a3Result, ShouldNotBeNil)
			So(a3Result.Alpha2, ShouldNotBeEmpty)
			a2Result := checker.ConvertFromAlpha2(a3Result.Alpha2)
			So(a2Result, ShouldPointTo, a3Result)
		})
	})
}
