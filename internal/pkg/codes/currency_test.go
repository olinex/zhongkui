package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// 测试 ISO 4217 货币代码的 Alpha3/名称/数字代码查询。
func TestISO4217Codes(t *testing.T) {
	Convey("根据 Alpha-3 代码查询", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3(""), ShouldBeNil)
			So(checker.ConvertFromAlpha3("unknown"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("XXX"), ShouldNotBeNil)
		})
		Convey("2. 查询 CNY（人民币）", func(c C) {
			result := checker.ConvertFromAlpha3("CNY")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "CNY")
			So(result.Name, ShouldEqual, "Yuan Renminbi")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 USD（美元）", func(c C) {
			result := checker.ConvertFromAlpha3("USD")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "USD")
			So(result.Name, ShouldEqual, "US Dollar")
			So(result.Numeric, ShouldEqual, "840")
		})
		Convey("4. 查询 EUR（欧元）", func(c C) {
			result := checker.ConvertFromAlpha3("EUR")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "EUR")
			So(result.Name, ShouldEqual, "Euro")
			So(result.Numeric, ShouldEqual, "978")
		})
		Convey("5. 查询 GBP（英镑）", func(c C) {
			result := checker.ConvertFromAlpha3("GBP")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "GBP")
			So(result.Name, ShouldEqual, "Pound Sterling")
			So(result.Numeric, ShouldEqual, "826")
		})
		Convey("6. 查询 JPY（日元）", func(c C) {
			result := checker.ConvertFromAlpha3("JPY")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "JPY")
			So(result.Name, ShouldEqual, "Yen")
			So(result.Numeric, ShouldEqual, "392")
		})
	})

	Convey("根据货币名称查询", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 查询不存在的名称应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("Unknown Currency"), ShouldBeNil)
		})
		Convey("2. 查询 Yuan Renminbi（人民币）", func(c C) {
			result := checker.ConvertFromName("Yuan Renminbi")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "CNY")
			So(result.Name, ShouldEqual, "Yuan Renminbi")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 US Dollar（美元）", func(c C) {
			result := checker.ConvertFromName("US Dollar")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "USD")
			So(result.Name, ShouldEqual, "US Dollar")
			So(result.Numeric, ShouldEqual, "840")
		})
		Convey("4. 查询 Euro（欧元）", func(c C) {
			result := checker.ConvertFromName("Euro")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "EUR")
			So(result.Name, ShouldEqual, "Euro")
			So(result.Numeric, ShouldEqual, "978")
		})
		Convey("5. 查询 Yen（日元）", func(c C) {
			result := checker.ConvertFromName("Yen")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "JPY")
			So(result.Name, ShouldEqual, "Yen")
			So(result.Numeric, ShouldEqual, "392")
		})
	})

	Convey("根据数字代码查询", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 查询不存在的代码应返回 nil", func(c C) {
			So(checker.ConvertFromNumeric(""), ShouldBeNil)
			So(checker.ConvertFromNumeric("unknown"), ShouldBeNil)
			So(checker.ConvertFromNumeric("999"), ShouldNotBeNil)
		})
		Convey("2. 查询 156（人民币）", func(c C) {
			result := checker.ConvertFromNumeric("156")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "CNY")
			So(result.Name, ShouldEqual, "Yuan Renminbi")
			So(result.Numeric, ShouldEqual, "156")
		})
		Convey("3. 查询 840（美元）", func(c C) {
			result := checker.ConvertFromNumeric("840")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "USD")
			So(result.Name, ShouldEqual, "US Dollar")
			So(result.Numeric, ShouldEqual, "840")
		})
		Convey("4. 查询 978（欧元）", func(c C) {
			result := checker.ConvertFromNumeric("978")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "EUR")
			So(result.Name, ShouldEqual, "Euro")
			So(result.Numeric, ShouldEqual, "978")
		})
		Convey("5. 查询 826（英镑）", func(c C) {
			result := checker.ConvertFromNumeric("826")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "GBP")
			So(result.Name, ShouldEqual, "Pound Sterling")
			So(result.Numeric, ShouldEqual, "826")
		})
	})

	Convey("验证数据完整性", t, func() {
		checker := GetISO4217Codes()
		Convey("1. Alpha3 索引应包含常见货币", func(c C) {
			So(checker.ConvertFromAlpha3("CNY"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("USD"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("EUR"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("GBP"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("JPY"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("KRW"), ShouldNotBeNil)
			So(checker.ConvertFromAlpha3("RUB"), ShouldNotBeNil)
		})
		Convey("2. 货币数字代码应为 3 位", func(c C) {
			cnyResult := checker.ConvertFromAlpha3("CNY")
			So(len(cnyResult.Numeric), ShouldEqual, 3)
			usdResult := checker.ConvertFromAlpha3("USD")
			So(len(usdResult.Numeric), ShouldEqual, 3)
			eurResult := checker.ConvertFromAlpha3("EUR")
			So(len(eurResult.Numeric), ShouldEqual, 3)
		})
	})
}

// 测试 GetISO4217Codes 的单例行为。
func TestISO4217CodesSingleton(t *testing.T) {
	Convey("验证 GetISO4217Codes 单例行为", t, func() {
		Convey("多次调用应返回相同的实例", func(c C) {
			instance1 := GetISO4217Codes()
			instance2 := GetISO4217Codes()
			So(instance1, ShouldPointTo, instance2)
		})
	})
}

// 测试不同索引查询同一货币返回同一条目。
func TestISO4217CodesCrossIndexConsistency(t *testing.T) {
	Convey("验证不同索引之间的交叉一致性", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 通过 Alpha3 查询的结果与 Name 查询应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("CNY")
			nameResult := checker.ConvertFromName("Yuan Renminbi")
			So(a3Result, ShouldPointTo, nameResult)
		})
		Convey("2. 通过 Alpha3 查询的结果与 Numeric 查询应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("USD")
			numericResult := checker.ConvertFromNumeric("840")
			So(a3Result, ShouldPointTo, numericResult)
		})
		Convey("3. 通过 Name 查询的结果与 Numeric 查询应返回同一条目", func(c C) {
			nameResult := checker.ConvertFromName("Euro")
			numericResult := checker.ConvertFromNumeric("978")
			So(nameResult, ShouldPointTo, numericResult)
		})
		Convey("4. 通过 Alpha3 查询 GBP 与 Numeric 查询 826 应返回同一条目", func(c C) {
			a3Result := checker.ConvertFromAlpha3("GBP")
			numericResult := checker.ConvertFromNumeric("826")
			So(a3Result, ShouldPointTo, numericResult)
		})
	})
}

// 测试查询区分大小写。
func TestISO4217CodesCaseSensitive(t *testing.T) {
	Convey("验证查询区分大小写", t, func() {
		checker := GetISO4217Codes()
		Convey("1. Alpha3 查询区分大小写，小写应返回 nil", func(c C) {
			So(checker.ConvertFromAlpha3("cny"), ShouldBeNil)
			So(checker.ConvertFromAlpha3("Usd"), ShouldBeNil)
		})
		Convey("2. Name 查询区分大小写", func(c C) {
			So(checker.ConvertFromName("yuan renminbi"), ShouldBeNil)
			So(checker.ConvertFromName("YUAN RENMINBI"), ShouldBeNil)
		})
	})
}

// 测试更多货币数据。
func TestISO4217CodesMoreCurrencies(t *testing.T) {
	Convey("验证更多货币数据", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 韩元（KRW/410）", func(c C) {
			result := checker.ConvertFromAlpha3("KRW")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "KRW")
			So(result.Numeric, ShouldEqual, "410")
		})
		Convey("2. 俄罗斯卢布（RUB/643）", func(c C) {
			result := checker.ConvertFromAlpha3("RUB")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "RUB")
			So(result.Numeric, ShouldEqual, "643")
		})
		Convey("3. 港币（HKD/344）", func(c C) {
			result := checker.ConvertFromAlpha3("HKD")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "HKD")
			So(result.Numeric, ShouldEqual, "344")
		})
		Convey("4. 澳大利亚元（AUD/036）", func(c C) {
			result := checker.ConvertFromAlpha3("AUD")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "AUD")
			So(result.Numeric, ShouldEqual, "036")
		})
		Convey("5. 加拿大元（CAD/124）", func(c C) {
			result := checker.ConvertFromAlpha3("CAD")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "CAD")
			So(result.Numeric, ShouldEqual, "124")
		})
		Convey("6. 瑞士法郎（CHF/756）", func(c C) {
			result := checker.ConvertFromAlpha3("CHF")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "CHF")
			So(result.Numeric, ShouldEqual, "756")
		})
	})
}

// 测试数字代码的前导零处理。
func TestISO4217CodesNumericPadding(t *testing.T) {
	Convey("验证数字代码的前导零处理", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 数字代码应保留前导零（如澳大利亚元为 036）", func(c C) {
			result := checker.ConvertFromAlpha3("AUD")
			So(result.Numeric, ShouldEqual, "036")
		})
		Convey("2. 去掉前导零后应无法匹配", func(c C) {
			So(checker.ConvertFromNumeric("36"), ShouldBeNil)
		})
		Convey("3. 使用完整三位数字代码应能正确查询", func(c C) {
			result := checker.ConvertFromNumeric("036")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldEqual, "AUD")
		})
	})
}

// 测试 ISO4217Entry 字段完整性。
func TestISO4217EntryFields(t *testing.T) {
	Convey("验证 ISO4217Entry 字段完整性", t, func() {
		checker := GetISO4217Codes()
		Convey("1. 查询到的条目所有核心字段不应为空", func(c C) {
			result := checker.ConvertFromAlpha3("CNY")
			So(result, ShouldNotBeNil)
			So(result.Alpha3, ShouldNotBeEmpty)
			So(result.Name, ShouldNotBeEmpty)
			So(result.Numeric, ShouldNotBeEmpty)
		})
		Convey("2. Alpha3 应为 3 个字符", func(c C) {
			result := checker.ConvertFromAlpha3("CNY")
			So(len(result.Alpha3), ShouldEqual, 3)
		})
		Convey("3. Numeric 应为 3 个字符", func(c C) {
			result := checker.ConvertFromAlpha3("CNY")
			So(len(result.Numeric), ShouldEqual, 3)
		})
	})
}
