package utils

// 本文件承载三个与业务无关的纯计算能力的算法实现：MathEval（算术/逻辑表达式求值）、
// UnitConvert（单位换算）与 BaseConvert（进制转换）。它们不依赖 eino 等上层组件，只依赖
// 标准库与既有第三方 github.com/casbin/govaluate；internal/ai/tools 中的同名工具由此处的
// 导出函数驱动，仅负责入参 JSON 解析与结果序列化。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串
// 与重构前保持一致。

import (
	// 标准库
	"math/big"
	"sort"
	"strings"

	// 第三方
	"github.com/casbin/govaluate"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// unitCategoryTemperature 是温度类别名，换算不按比例系数而经摄氏度中转，故单独处理。
const unitCategoryTemperature = "temperature"

// unitRegistry 是单位换算注册表：类别 → 单位符号 → 相对该类别基准单位的换算系数。温度类别的
// 系数恒为 1，仅用于类别归属判定，实际换算由 unitTemperatureToCelsius / unitTemperatureFromCelsius 完成。
var unitRegistry = map[string]map[string]float64{
	"length": {
		"m":   1,
		"km":  1000,
		"cm":  0.01,
		"mm":  0.001,
		"um":  1e-6,
		"nm":  1e-9,
		"mi":  1609.344,
		"yd":  0.9144,
		"ft":  0.3048,
		"in":  0.0254,
		"nmi": 1852,
	},
	"mass": {
		"kg": 1,
		"g":  0.001,
		"mg": 1e-6,
		"t":  1000,
		"lb": 0.45359237,
		"oz": 0.028349523125,
	},
	"time": {
		"s":   1,
		"ms":  1e-3,
		"us":  1e-6,
		"ns":  1e-9,
		"min": 60,
		"h":   3600,
		"d":   86400,
	},
	"area": {
		"m2":   1,
		"km2":  1e6,
		"cm2":  1e-4,
		"ha":   10000,
		"acre": 4046.8564224,
		"ft2":  0.09290304,
	},
	"volume": {
		"l":    1,
		"ml":   1e-3,
		"m3":   1000,
		"gal":  3.785411784,
		"qt":   0.946352946,
		"pt":   0.473176473,
		"cup":  0.2365882365,
		"floz": 0.0295735295625,
	},
	"data": {
		"b":   1,
		"kb":  1000,
		"mb":  1e6,
		"gb":  1e9,
		"tb":  1e12,
		"kib": 1024,
		"mib": 1048576,
		"gib": 1073741824,
		"tib": 1099511627776,
	},
	"speed": {
		"mps": 1,
		"kmh": 1.0 / 3.6,
		"mph": 0.44704,
		"kn":  1852.0 / 3600,
	},
	unitCategoryTemperature: {
		"c": 1,
		"f": 1,
		"k": 1,
	},
}

// ============================================================================
// MathEval：算术/逻辑表达式求值
// ============================================================================

// MathEvalResult 是表达式求值成功时返回的载荷：result 为求值结果（数字统一为 float64，
// 布尔保持 bool），expression 为原表达式。
type MathEvalResult struct {
	// Result 求值结果：number（float64）或 bool
	Result any `json:"result"`
	// Expression 原始表达式
	Expression string `json:"expression"`
}

// EvalMathExpression 对单个算术/逻辑表达式求值并返回 MathEvalResult。表达式先去除首尾空白，
// 为空则返回错误；语法非法、求值失败或结果类型不可序列化时返回英文错误。结果中的 expression
// 为去空白后的表达式。
func EvalMathExpression(expression string) (MathEvalResult, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return MathEvalResult{}, errors.New("expression must be non-empty")
	}
	expr, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return MathEvalResult{}, errors.Newf("invalid expression %q: %s", expression, err.Error())
	}
	raw, err := expr.Evaluate(nil)
	if err != nil {
		return MathEvalResult{}, errors.Newf("failed to evaluate %q: %s", expression, err.Error())
	}
	result, ok := mathNormalizeEvalResult(raw)
	if !ok {
		return MathEvalResult{}, errors.Newf("expression %q produced an unsupported value of type %T", expression, raw)
	}
	return MathEvalResult{Result: result, Expression: expression}, nil
}

// mathNormalizeEvalResult 把 govaluate 的求值结果归一化为可直接 JSON 序列化的值：布尔保持
// bool，各类数字统一为 float64。非 number/bool 时返回 ok=false。
func mathNormalizeEvalResult(raw any) (result any, ok bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	default:
		return nil, false
	}
}

// ============================================================================
// UnitConvert：单位换算
// ============================================================================

// UnitConvertResult 是单位换算成功时返回的载荷。
type UnitConvertResult struct {
	// Result 换算后的数值
	Result float64 `json:"result"`
	// From 源单位（已规范化为小写）
	From string `json:"from"`
	// To 目标单位（已规范化为小写）
	To string `json:"to"`
	// Category 所属类别
	Category string `json:"category"`
}

// ConvertUnit 把 value 从 from 单位换算为 to 单位并返回 UnitConvertResult。from / to 先规范化为
// 小写并去空白；空单位、未知单位或跨类别换算时返回英文错误。
func ConvertUnit(value float64, from, to string) (UnitConvertResult, error) {
	from = unitNormalize(from)
	to = unitNormalize(to)
	if from == "" {
		return UnitConvertResult{}, errors.New("from must be non-empty")
	}
	if to == "" {
		return UnitConvertResult{}, errors.New("to must be non-empty")
	}
	fromCategory, fromFactor, ok := unitLookup(from)
	if !ok {
		return UnitConvertResult{}, errors.Newf("unknown unit %q, expected one of %s", from, strings.Join(unitNames(), ", "))
	}
	toCategory, toFactor, ok := unitLookup(to)
	if !ok {
		return UnitConvertResult{}, errors.Newf("unknown unit %q, expected one of %s", to, strings.Join(unitNames(), ", "))
	}
	if fromCategory != toCategory {
		return UnitConvertResult{}, errors.Newf("cannot convert from %q (%s) to %q (%s), units belong to different categories",
			from, fromCategory, to, toCategory)
	}
	var result float64
	if fromCategory == unitCategoryTemperature {
		result = unitTemperatureFromCelsius(unitTemperatureToCelsius(value, from), to)
	} else {
		result = value * fromFactor / toFactor
	}
	return UnitConvertResult{Result: result, From: from, To: to, Category: fromCategory}, nil
}

// unitNormalize 规范化单位符号：去除首尾空白并转小写。
func unitNormalize(unit string) string {
	return strings.ToLower(strings.TrimSpace(unit))
}

// unitLookup 在注册表中查找单位，返回其所属类别与相对基准的换算系数；未登记时 ok=false。
func unitLookup(unit string) (category string, factor float64, ok bool) {
	for cat, units := range unitRegistry {
		if f, found := units[unit]; found {
			return cat, f, true
		}
	}
	return "", 0, false
}

// unitNames 返回全部已登记单位符号（字典序），供未知单位的错误提示渲染。
func unitNames() []string {
	names := make([]string, 0)
	for _, units := range unitRegistry {
		for name := range units {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// unitTemperatureToCelsius 把温度值从其单位换算为摄氏度：c 原值，f 按 C=(F-32)*5/9，k 按 C=K-273.15。
func unitTemperatureToCelsius(value float64, unit string) float64 {
	switch unit {
	case "f":
		return (value - 32) * 5 / 9
	case "k":
		return value - 273.15
	default: // c
		return value
	}
}

// unitTemperatureFromCelsius 把摄氏度换算为目标温度单位：c 原值，f 按 F=C*9/5+32，k 按 K=C+273.15。
func unitTemperatureFromCelsius(celsius float64, unit string) float64 {
	switch unit {
	case "f":
		return celsius*9/5 + 32
	case "k":
		return celsius + 273.15
	default: // c
		return celsius
	}
}

// ============================================================================
// BaseConvert：进制转换
// ============================================================================

// BaseConvertResult 是进制转换成功时返回的载荷。
type BaseConvertResult struct {
	// Result 目标进制下的数值字符串
	Result string `json:"result"`
	// Decimal 十进制下的数值字符串
	Decimal string `json:"decimal"`
}

// ConvertBase 把 value（fromBase 进制下的整数表示，可带前导负号）转换为 toBase 进制并返回
// BaseConvertResult。fromBase / toBase 须在 2..36 之间；value 去空白后为空或对源进制非法时
// 返回英文错误。
func ConvertBase(value string, fromBase, toBase int) (BaseConvertResult, error) {
	if fromBase < 2 || fromBase > 36 {
		return BaseConvertResult{}, errors.Newf("from_base %d is out of range, expected 2..36", fromBase)
	}
	if toBase < 2 || toBase > 36 {
		return BaseConvertResult{}, errors.Newf("to_base %d is out of range, expected 2..36", toBase)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return BaseConvertResult{}, errors.New("value must be non-empty")
	}
	n, ok := new(big.Int).SetString(value, fromBase)
	if !ok {
		return BaseConvertResult{}, errors.Newf("value %q is not a valid base-%d integer", value, fromBase)
	}
	return BaseConvertResult{Result: n.Text(toBase), Decimal: n.Text(10)}, nil
}
