// 本文件是三个无状态数学类内置工具的薄适配器：math_eval（算术/逻辑表达式求值）、unit_convert
// （单位换算）与 base_convert（进制转换）。三者均不持有任何依赖，故构造器忽略 Deps。纯算法实现
// 已下沉到 internal/pkg/utils（EvalMathExpression / ConvertUnit / ConvertBase），工具契约由 eino
// 的 tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；本文件
// 只保留入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与
// 出厂行（002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// ============================================================================
// math_eval：算术/逻辑表达式求值
// ============================================================================

// mathEvalArgs 是 math_eval 的 JSON 入参形态：expression 必填。
type mathEvalArgs struct {
	// Expression 待求值的单个算术/逻辑表达式（必填，去空白后非空）
	Expression string `json:"expression" jsonschema_description:"待求值的表达式，例如 (1+2)*3-4/2，必须非空。"`
}

// mathEvalTool 是内置工具 math_eval 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var mathEvalTool = mustInferTool(
	"math_eval",
	"对单个算术或逻辑表达式求值。支持四则运算（+ - * / % **，其中 ** 为幂）、"+
		"比较（> >= < <= == !=）与逻辑（&& || !）运算符，可按括号改变优先级。"+
		"不提供函数调用与变量，纯字面量表达式即可。",
	func(_ context.Context, in mathEvalArgs) (utils.MathEvalResult, error) {
		result, err := utils.EvalMathExpression(in.Expression)
		if err != nil {
			return utils.MathEvalResult{}, errors.Newf("math_eval: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// unit_convert：单位换算
// ============================================================================

// unitConvertArgs 是 unit_convert 的 JSON 入参形态：value / from / to 均必填。
type unitConvertArgs struct {
	// Value 待换算的数值（必填）
	Value float64 `json:"value" jsonschema_description:"待换算的数值，必填。"`
	// From 源单位符号（必填，如 km）
	From string `json:"from" jsonschema_description:"源单位符号，小写，例如 km、kg、gb、c。必填。"`
	// To 目标单位符号（必填，如 m）
	To string `json:"to" jsonschema_description:"目标单位符号，小写，例如 m、g、mb、f。必填。"`
}

// unitConvertTool 是内置工具 unit_convert 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var unitConvertTool = mustInferTool(
	"unit_convert",
	"在同类别的常见单位之间做线性换算。支持类别：length（长度）、mass（质量）、"+
		"time（时间）、area（面积）、volume（体积）、data（数据量）、speed（速度）与"+
		"temperature（温度）。from 与 to 必须属于同一类别，单位以小写符号给出（如 km、lb、mb、mph、c）。",
	func(_ context.Context, in unitConvertArgs) (utils.UnitConvertResult, error) {
		result, err := utils.ConvertUnit(in.Value, in.From, in.To)
		if err != nil {
			return utils.UnitConvertResult{}, errors.Newf("unit_convert: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// base_convert：进制转换
// ============================================================================

// baseConvertArgs 是 base_convert 的 JSON 入参形态：value / from_base / to_base 均必填。
type baseConvertArgs struct {
	// Value 待转换的数值字符串（必填，须为 from_base 进制的合法表示，含可选负号）
	Value string `json:"value" jsonschema_description:"待转换的数值字符串，须为源进制下的合法表示，可带前导负号，例如 ff。必填。"`
	// FromBase 源进制（必填，取值 2..36）
	FromBase int `json:"from_base" jsonschema_description:"源进制，取值 2 到 36，例如 16。必填。"`
	// ToBase 目标进制（必填，取值 2..36）
	ToBase int `json:"to_base" jsonschema_description:"目标进制，取值 2 到 36，例如 2。必填。"`
}

// baseConvertTool 是内置工具 base_convert 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var baseConvertTool = mustInferTool(
	"base_convert",
	"把任意进制的整数在不同进制间转换（支持 2 到 36 进制，含十六进制与二进制等）。"+
		"value 以字符串给出，避免超出模型日志与 JSON 数字精度，例如 ff、1010、-1a。",
	func(_ context.Context, in baseConvertArgs) (utils.BaseConvertResult, error) {
		result, err := utils.ConvertBase(in.Value, in.FromBase, in.ToBase)
		if err != nil {
			return utils.BaseConvertResult{}, errors.Newf("base_convert: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 math_eval / unit_convert / base_convert；三者均无状态，不消费 Deps。
func init() {
	Register("math_eval", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return mathEvalTool, nil
	})
	Register("unit_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return unitConvertTool, nil
	})
	Register("base_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return baseConvertTool, nil
	})
}
