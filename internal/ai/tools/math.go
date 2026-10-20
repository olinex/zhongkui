// 本文件是三个无状态数学类内置工具的薄适配器：math_eval（算术/逻辑表达式求值）、unit_convert
// （单位换算）与 base_convert（进制转换）。三者均不持有任何依赖，故构造器忽略 Deps。纯算法实现
// 已下沉到 internal/pkg/utils（EvalMathExpression / ConvertUnit / ConvertBase），本文件只保留
// 入参结构体、Info（含中文 Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、调用
// utils 导出函数、出错时补上工具名前缀并以 JSON 序列化结果。工具元信息（internal/ai 的 ToolSpecs）
// 与出厂行（002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

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
	Expression string `json:"expression"`
}

// mathEvalTool 是内置工具 math_eval 的无状态实现：不持有依赖。
type mathEvalTool struct{}

// Info 返回 math_eval 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// 唯一键为 expression（必填）。
func (t *mathEvalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "math_eval",
		Desc: "对单个算术或逻辑表达式求值。支持四则运算（+ - * / % **，其中 ** 为幂）、" +
			"比较（> >= < <= == !=）与逻辑（&& || !）运算符，可按括号改变优先级。" +
			"不提供函数调用与变量，纯字面量表达式即可。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"expression": {
				Type:     schema.String,
				Desc:     "待求值的表达式，例如 (1+2)*3-4/2，必须非空。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 math_eval：解析 JSON 入参，调用 utils.EvalMathExpression 求值，返回形如
// {"result":4,"expression":"(1+2)*3-4/2"} 的 JSON 字符串。语法非法或结果为不可序列化类型时返回
// 英文错误（工具名前缀在此补齐）。
func (t *mathEvalTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args mathEvalArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.EvalMathExpression(args.Expression)
	if err != nil {
		return "", errors.Newf("math_eval: %s", err.Error())
	}
	return mathEncodeResult(result)
}

// ============================================================================
// unit_convert：单位换算
// ============================================================================

// unitConvertArgs 是 unit_convert 的 JSON 入参形态：value 必填，from / to 必填。
type unitConvertArgs struct {
	// Value 待换算的数值（必填）
	Value float64 `json:"value"`
	// From 源单位符号（必填，如 km）
	From string `json:"from"`
	// To 目标单位符号（必填，如 m）
	To string `json:"to"`
}

// unitConvertTool 是内置工具 unit_convert 的无状态实现：不持有依赖，换算表为 utils 包级常量。
type unitConvertTool struct{}

// Info 返回 unit_convert 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// value（number）与 from / to（string）均为必填。
func (t *unitConvertTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "unit_convert",
		Desc: "在同类别的常见单位之间做线性换算。支持类别：length（长度）、mass（质量）、" +
			"time（时间）、area（面积）、volume（体积）、data（数据量）、speed（速度）与" +
			"temperature（温度）。from 与 to 必须属于同一类别，单位以小写符号给出（如 km、lb、mb、mph、c）。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"value": {
				Type:     schema.Number,
				Desc:     "待换算的数值，必填。",
				Required: true,
			},
			"from": {
				Type:     schema.String,
				Desc:     "源单位符号，小写，例如 km、kg、gb、c。必填。",
				Required: true,
			},
			"to": {
				Type:     schema.String,
				Desc:     "目标单位符号，小写，例如 m、g、mb、f。必填。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 unit_convert：解析 JSON 入参，调用 utils.ConvertUnit，返回形如
// {"result":1000,"from":"km","to":"m","category":"length"} 的 JSON 字符串。未知单位或 from / to
// 跨类别时返回英文错误（工具名前缀在此补齐）。
func (t *unitConvertTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args unitConvertArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.ConvertUnit(args.Value, args.From, args.To)
	if err != nil {
		return "", errors.Newf("unit_convert: %s", err.Error())
	}
	return mathEncodeResult(result)
}

// ============================================================================
// base_convert：进制转换
// ============================================================================

// baseConvertArgs 是 base_convert 的 JSON 入参形态：value 必填（字符串），from_base / to_base 必填。
type baseConvertArgs struct {
	// Value 待转换的数值字符串（必填，须为 from_base 进制的合法表示，含可选负号）
	Value string `json:"value"`
	// FromBase 源进制（必填，取值 2..36）
	FromBase int `json:"from_base"`
	// ToBase 目标进制（必填，取值 2..36）
	ToBase int `json:"to_base"`
}

// baseConvertTool 是内置工具 base_convert 的无状态实现：不持有依赖。
type baseConvertTool struct{}

// Info 返回 base_convert 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// value（string）与 from_base / to_base（integer）均为必填。
func (t *baseConvertTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "base_convert",
		Desc: "把任意进制的整数在不同进制间转换（支持 2 到 36 进制，含十六进制与二进制等）。" +
			"value 以字符串给出，避免超出模型日志与 JSON 数字精度，例如 ff、1010、-1a。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"value": {
				Type:     schema.String,
				Desc:     "待转换的数值字符串，须为源进制下的合法表示，可带前导负号，例如 ff。必填。",
				Required: true,
			},
			"from_base": {
				Type:     schema.Integer,
				Desc:     "源进制，取值 2 到 36，例如 16。必填。",
				Required: true,
			},
			"to_base": {
				Type:     schema.Integer,
				Desc:     "目标进制，取值 2 到 36，例如 2。必填。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 base_convert：解析 JSON 入参，调用 utils.ConvertBase 在任意精度下转换，返回
// 形如 {"result":"11111111","decimal":"255"} 的 JSON 字符串。进制越界或 value 对源进制非法时返回
// 英文错误（工具名前缀在此补齐）。
func (t *baseConvertTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args baseConvertArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.ConvertBase(args.Value, args.FromBase, args.ToBase)
	if err != nil {
		return "", errors.Newf("base_convert: %s", err.Error())
	}
	return mathEncodeResult(result)
}

// mathEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func mathEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 math_eval / unit_convert / base_convert；三者均无状态，不消费 Deps。
func init() {
	Register("math_eval", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &mathEvalTool{}, nil
	})
	Register("unit_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &unitConvertTool{}, nil
	})
	Register("base_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &baseConvertTool{}, nil
	})
}
