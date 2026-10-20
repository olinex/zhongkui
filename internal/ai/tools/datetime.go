// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是内置工具 datetime_calc 的薄适配器：Agent 在工具循环中做日期与时区运算——取当前时间、在
// 时区间转换、对时间做加减、计算两个时间的间隔。工具完全无状态：不消费 Deps、不依赖运行期上下文。
// 纯算法实现已下沉到 internal/pkg/utils（RunDatetimeCalc 及各 helper），本文件只保留入参结构体、
// Info（含中文 Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、调用 utils 导出函数、
// 出错时补上工具名前缀并以 JSON 序列化结果。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

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

// datetimeCalcArgs 是 datetime_calc 的 JSON 入参形态：operation 必填，其余按操作各取所需。
type datetimeCalcArgs struct {
	// Operation 要执行的操作：now / convert / add / diff
	Operation string `json:"operation"`
	// Timezone 目标/解释时区（now 输出、convert 输出目标、add 运算时区），缺省 UTC
	Timezone string `json:"timezone"`
	// Time 待处理时间，除 now 外使用
	Time string `json:"time"`
	// FromTimezone convert 的源时区，缺省 UTC
	FromTimezone string `json:"from_timezone"`
	// ToTimezone convert 的目标时区，缺省取 timezone，再缺省 UTC
	ToTimezone string `json:"to_timezone"`
	// Duration add 的增量（支持 h/m/s/ms/us/ns 与 d）
	Duration string `json:"duration"`
	// Start diff 的起始时间
	Start string `json:"start"`
	// End diff 的结束时间
	End string `json:"end"`
}

// datetimeCalcTool 是内置工具 datetime_calc 的无状态实现：不持有依赖，不读取运行期上下文。
type datetimeCalcTool struct{}

// Info 返回 datetime_calc 的工具元信息与参数 JSON schema，供模型决定何时调用。所有参数说明为中文，
// operation 带枚举取值且必填；timezone / time / from_timezone / to_timezone / duration / start / end
// 均按需可选。
func (t *datetimeCalcTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "datetime_calc",
		Desc: "进行日期与时间计算：获取当前时间、在时区间转换、对时间做加减、计算两个时间的间隔。" +
			"时区用 IANA 名称（如 Asia/Shanghai），缺省 UTC；时间支持 RFC3339 与 2006-01-02 15:04:05 两种格式。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     "要执行的操作，必填：now 取当前时间、convert 时区转换、add 时间加减、diff 计算间隔。",
				Required: true,
				Enum:     utils.DatetimeOperations,
			},
			"timezone": {
				Type: schema.String,
				Desc: "目标/解释时区（IANA 名称），缺省 UTC；now 的输出时区、convert 的输出目标时区、add 运算所基于的时区。",
			},
			"time": {
				Type: schema.String,
				Desc: "待处理时间，除 now 外使用；支持 RFC3339（如 2024-01-01T00:00:00Z）与 2006-01-02 15:04:05 两种格式。",
			},
			"from_timezone": {
				Type: schema.String,
				Desc: "convert 的源时区（IANA 名称），缺省 UTC。",
			},
			"to_timezone": {
				Type: schema.String,
				Desc: "convert 的目标时区（IANA 名称），缺省取 timezone，再缺省 UTC。",
			},
			"duration": {
				Type: schema.String,
				Desc: "add 的增量，支持 Go 时长单位 h/m/s/ms/us/ns 以及 d（天，按 24 小时计），如 1d2h。",
			},
			"start": {
				Type: schema.String,
				Desc: "diff 的起始时间，支持 RFC3339 与 2006-01-02 15:04:05 两种格式。",
			},
			"end": {
				Type: schema.String,
				Desc: "diff 的结束时间，支持 RFC3339 与 2006-01-02 15:04:05 两种格式。",
			},
		}),
	}, nil
}

// InvokableRun 执行 datetime_calc：解析 JSON 入参，调用 utils.RunDatetimeCalc 按 operation 分派计算，
// 返回紧凑 JSON 字符串。operation 非法、缺 time、时区非法或时间不可解析均返回英文错误（工具名前缀
// 在此补齐）。
func (t *datetimeCalcTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args datetimeCalcArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.RunDatetimeCalc(utils.DatetimeCalcInput{
		Operation:    args.Operation,
		Timezone:     args.Timezone,
		Time:         args.Time,
		FromTimezone: args.FromTimezone,
		ToTimezone:   args.ToTimezone,
		Duration:     args.Duration,
		Start:        args.Start,
		End:          args.End,
	})
	if err != nil {
		return "", errors.Newf("datetime_calc: %s", err.Error())
	}
	return datetimeEncodeResult(result)
}

// datetimeEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func datetimeEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 datetime_calc；工具无状态，不消费 Deps。
func init() {
	Register("datetime_calc", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &datetimeCalcTool{}, nil
	})
}
