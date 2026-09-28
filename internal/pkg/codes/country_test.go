package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// 测试 ISO 3166-1 国家/地区的 Alpha2/Alpha3/名称/数字查询。
func TestISO31661Codes(t *testing.T) {
	Convey("根据 Alpha-2 代码查询", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha2(""), ShouldBeNil)
			So(checker.ConvertFromAlpha2("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha2("XX"), ShouldBeNil)
		})
		Convey("2. 查询 CN（中国）", func(c C) {
			result := checker.ConvertFromAlpha2("CN")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "CN")
			So(result.Alpha3, ShouldEqual, "CHN")
			So(result.Name, ShouldEqual, "China")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 US（美国）", func(c C) {
			result := checker.ConvertFromAlpha2("US")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "US")
			So(result.Alpha3, ShouldEqual, "USA")
			So(result.Name, ShouldEqual, "United States")
			So(result.Flag, ShouldEqual, "🇺🇸")
		})
		Convey("4. 查询 JP（日本）", func(c C) {
			result := checker.ConvertFromAlpha2("JP")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "JP")
			So(result.Alpha3, ShouldEqual, "JPN")
			So(result.Name, ShouldEqual, "Japan")
			So(result.Numeric, ShouldEqual, "392")
		})
	})

	Convey("根据 Alpha-3 代码查询", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3(""), ShouldBeNil)
			So(checker.ConvertFromAlpha3("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("XXX"), ShouldBeNil)
		})
		Convey("2. 查询 CHN（中国）", func(c C) {
			result := checker.ConvertFromAlpha3("CHN")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "CN")
			So(result.Alpha3, ShouldEqual, "CHN")
			So(result.Name, ShouldEqual, "China")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 USA（美国）", func(c C) {
			result := checker.ConvertFromAlpha3("USA")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "US")
			So(result.Alpha3, ShouldEqual, "USA")
			So(result.Name, ShouldEqual, "United States")
		})
		Convey("4. 查询 GBR（英国）", func(c C) {
			result := checker.ConvertFromAlpha3("GBR")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "GB")
			So(result.Alpha3, ShouldEqual, "GBR")
			So(result.Name, ShouldEqual, "United Kingdom")
		})
	})

	Convey("根据国家/地区名称查询", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 查询不存在的名称应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("Not a Country"), ShouldBeNil)
		})
		Convey("2. 查询 China", func(c C) {
			result := checker.ConvertFromName("China")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "CN")
			So(result.Alpha3, ShouldEqual, "CHN")
			So(result.Name, ShouldEqual, "China")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 United States", func(c C) {
			result := checker.ConvertFromName("United States")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "US")
			So(result.Alpha3, ShouldEqual, "USA")
			So(result.Name, ShouldEqual, "United States")
		})
		Convey("4. 查询 Japan", func(c C) {
			result := checker.ConvertFromName("Japan")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "JP")
			So(result.Alpha3, ShouldEqual, "JPN")
			So(result.Name, ShouldEqual, "Japan")
		})
	})

	Convey("根据数字代码查询", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromNumeric(""), ShouldBeNil)
			So(checker.ConvertFromNumeric("unknown"), ShouldBeNil)
			So(checker.ConvertFromNumeric("999"), ShouldBeNil)
		})
		Convey("2. 查询 156（中国）", func(c C) {
			result := checker.ConvertFromNumeric("156")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "CN")
			So(result.Alpha3, ShouldEqual, "CHN")
			So(result.Name, ShouldEqual, "China")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 840（美国）", func(c C) {
			result := checker.ConvertFromNumeric("840")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "US")
			So(result.Alpha3, ShouldEqual, "USA")
			So(result.Name, ShouldEqual, "United States")
			So(result.Numeric, ShouldEqual, "840")
		})
		Convey("4. 查询 392（日本）", func(c C) {
			result := checker.ConvertFromNumeric("392")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "JP")
			So(result.Alpha3, ShouldEqual, "JPN")
			So(result.Name, ShouldEqual, "Japan")
			So(result.Numeric, ShouldEqual, "392")
		})
	})

	Convey("验证数据完整性", t, func() {
		checker := GetISO31661Codes()
		Convey("1. Alpha2 索引应包含常见国家", func(c C) {
			So(checker.ConvertFromAlpha2("CN"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("US"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("GB"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("DE"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("FR"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("JP"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha2("RU"), ShouldNotBeNil)
		})
		Convey("2. Alpha3 索引应包含常见国家", func(c C) {
			So(checker.ConvertFromAlpha3("CHN"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("USA"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("GBR"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("DEU"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("FRA"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("JPN"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("RUS"), ShouldNotBeNil)
		})
		Convey("3. 旗帜符号应正确", func(c C) {
			usResult := checker.ConvertFromAlpha2("US")
			So(usResult.Flag, ShouldEqual, "🇺🇸")
			cnResult := checker.ConvertFromAlpha2("CN")
			So(cnResult.Flag, ShouldEqual, "🇨🇳")
			jpResult := checker.ConvertFromAlpha2("JP")
			So(jpResult.Flag, ShouldEqual, "🇯🇵")
		})
	})
}

// 测试 GetISO31661Codes 的单例行为。
func TestISO31661CodesSingleton(t *testing.T) {
	Convey("验证 GetISO31661Codes 单例行为", t, func() {
		Convey("多次调用应返回相同的实例", func(c C) {
			instance1 := GetISO31661Codes()
			instance2 := GetISO31661Codes()
			So(instance1, ShouldPointTo, instance2)
		})
	})
}

// 测试不同索引查询同一国家/地区返回同一条目。
func TestISO31661CodesCrossIndexConsistency(t *testing.T) {
	Convey("验证不同索引之间的交叉一致性", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 通过 Alpha2 查询的结果与 Alpha3 查询应返回同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("CN")
			a3Result := checker.ConvertFromAlpha3("CHN")
			So(a2Result, ShouldPointTo, a3Result)
		})
		Convey("2. 通过 Alpha2 查询的结果与 Name 查询应返回同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("US")
			nameResult := checker.ConvertFromName("United States")
			So(a2Result, ShouldPointTo, nameResult)
		})
		Convey("3. 通过 Alpha2 查询的结果与 Numeric 查询应返回同一条目", func(c C) {
			a2Result := checker.ConvertFromAlpha2("JP")
			numericResult := checker.ConvertFromNumeric("392")
			So(a2Result, ShouldPointTo, numericResult)
		})
		Convey("4. 通过 Alpha3 查询的结果与 Numeric 查询应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("GBR")
			numericResult := checker.ConvertFromNumeric("826")
			So(a3Result, ShouldPointTo, numericResult)
		})
	})
}

// 测试 OfficialName 字段。
func TestISO31661CodesOfficialName(t *testing.T) {
	Convey("验证 OfficialName 字段", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 有官方名称的国家应正确返回", func(c C) {
			cnResult := checker.ConvertFromAlpha2("CN")
			So(cnResult.OfficialName, ShouldEqual, "People's Republic of China")
		})
		Convey("2. 部分国家/地区没有官方名称，OfficialName 应为空", func(c C) {
			asResult := checker.ConvertFromAlpha2("AS")
			So(asResult.OfficialName, ShouldEqual, "")
		})
	})
}

// 测试查询区分大小写。
func TestISO31661CodesCaseSensitive(t *testing.T) {
	Convey("验证查询区分大小写", t, func() {
		checker := GetISO31661Codes()
		Convey("1. Alpha2 查询区分大小写，小写应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha2("cn"), ShouldBeNil)
			So(checker.ConvertFromAlpha2("Us"), ShouldBeNil)
		})
		Convey("2. Alpha3 查询区分大小写，小写应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3("chn"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("Usa"), ShouldBeNil)
		})
		Convey("3. Name 查询区分大小写", func(c C) {
			So(checker.ConvertFromName("china"), ShouldBeNil)
			So(checker.ConvertFromName("CHINA"), ShouldBeNil)
		})
	})
}

// 测试更多国家/地区数据。
func TestISO31661CodesMoreCountries(t *testing.T) {
	Convey("验证更多国家/地区数据", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 德国（DE/DEU/276）", func(c C) {
			result := checker.ConvertFromAlpha2("DE")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "DE")
			So(result.Alpha3, ShouldEqual, "DEU")
			So(result.Numeric, ShouldEqual, "276")
			So(result.Name, ShouldEqual, "Germany")
		})
		Convey("2. 法国（FR/FRA/250）", func(c C) {
			result := checker.ConvertFromAlpha2("FR")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "FR")
			So(result.Alpha3, ShouldEqual, "FRA")
			So(result.Numeric, ShouldEqual, "250")
			So(result.Name, ShouldEqual, "France")
		})
		Convey("3. 韩国（KR/KOR/410）", func(c C) {
			result := checker.ConvertFromAlpha2("KR")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "KR")
			So(result.Alpha3, ShouldEqual, "KOR")
			So(result.Numeric, ShouldEqual, "410")
			So(result.Name, ShouldEqual, "Korea, Republic of")
		})
		Convey("4. 澳大利亚（AU/AUS/036）", func(c C) {
			result := checker.ConvertFromAlpha2("AU")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "AU")
			So(result.Alpha3, ShouldEqual, "AUS")
			So(result.Numeric, ShouldEqual, "036")
			So(result.Name, ShouldEqual, "Australia")
		})
		Convey("5. 巴西（BR/BRA/076）", func(c C) {
			result := checker.ConvertFromAlpha2("BR")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "BR")
			So(result.Alpha3, ShouldEqual, "BRA")
			So(result.Numeric, ShouldEqual, "076")
			So(result.Name, ShouldEqual, "Brazil")
		})
		Convey("6. 印度（IN/IND/356）", func(c C) {
			result := checker.ConvertFromAlpha2("IN")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "IN")
			So(result.Alpha3, ShouldEqual, "IND")
			So(result.Numeric, ShouldEqual, "356")
			So(result.Name, ShouldEqual, "India")
		})
	})
}

// 测试数字代码的前导零处理。
func TestISO31661CodesNumericPadding(t *testing.T) {
	Convey("验证数字代码的前导零处理", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 数字代码应保留前导零（如澳大利亚为 036）", func(c C) {
			result := checker.ConvertFromAlpha2("AU")
			So(result.Numeric, ShouldEqual, "036")
		})
		Convey("2. 去掉前导零后应无法匹配", func(c C) {
			So(checker.ConvertFromNumeric("36"), ShouldBeNil)
		})
		Convey("3. 使用完整三位数字代码应能正确查询", func(c C) {
			result := checker.ConvertFromNumeric("036")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldEqual, "AU")
		})
	})
}

// 测试 ISO31661Entry 字段完整性。
func TestISO31661EntryFields(t *testing.T) {
	Convey("验证 ISO31661Entry 字段完整性", t, func() {
		checker := GetISO31661Codes()
		Convey("1. 查询到的条目所有核心字段不应为空（除 OfficialName 外）", func(c C) {
			result := checker.ConvertFromAlpha2("CN")
			So(result, ShouldNotBeNil)
			So(result.Alpha2, ShouldNotBeEmpty)
			So(result.Alpha3, ShouldNotBeEmpty)
			So(result.Name, ShouldNotBeEmpty)
			So(result.Numeric, ShouldNotBeEmpty)
			So(result.Flag, ShouldNotBeEmpty)
		})
		Convey("2. Alpha2 应为 2 个字符", func(c C) {
			result := checker.ConvertFromAlpha2("CN")
			So(len(result.Alpha2), ShouldEqual, 2)
		})
		Convey("3. Alpha3 应为 3 个字符", func(c C) {
			result := checker.ConvertFromAlpha2("CN")
			So(len(result.Alpha3), ShouldEqual, 3)
		})
		Convey("4. Numeric 应为 3 个字符", func(c C) {
			result := checker.ConvertFromAlpha2("CN")
			So(len(result.Numeric), ShouldEqual, 3)
		})
	})
}
