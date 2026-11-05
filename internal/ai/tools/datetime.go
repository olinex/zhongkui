// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是内置工具 datetime_calc 的薄适配器：Agent 在工具循环中做日期与时区运算——取当前时间、在
// 时区间转换、对时间做加减、计算两个时间的间隔。工具完全无状态：不消费 Deps、不依赖运行期上下文。
// 纯算法实现已下沉到 internal/pkg/utils（RunDatetimeCalc），工具契约由 eino 的
// tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；本文件只保留
// 入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

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

// datetimeCalcArgs 是 datetime_calc 的 JSON 入参形态：operation 必填，其余按操作各取所需。
type datetimeCalcArgs struct {
	// Operation 要执行的操作：now / convert / add / diff
	Operation string `json:"operation" jsonschema:"enum=now,enum=convert,enum=add,enum=diff" jsonschema_description:"要执行的操作，必填：now 取当前时间、convert 时区转换、add 时间加减、diff 计算间隔。"`
	// Timezone 目标/解释时区（now 输出、convert 输出目标、add 运算时区），缺省 UTC
	Timezone string `json:"timezone,omitempty" jsonschema_description:"目标/解释时区（IANA 名称），缺省 UTC；now 的输出时区、convert 的输出目标时区、add 运算所基于的时区。"`
	// Time 待处理时间，除 now 外使用
	Time string `json:"time,omitempty" jsonschema_description:"待处理时间，除 now 外使用；支持 RFC3339（如 2024-01-01T00:00:00Z）与 2006-01-02 15:04:05 两种格式。"`
	// FromTimezone convert 的源时区，缺省 UTC
	FromTimezone string `json:"from_timezone,omitempty" jsonschema_description:"convert 的源时区（IANA 名称），缺省 UTC。"`
	// ToTimezone convert 的目标时区，缺省取 timezone，再缺省 UTC
	ToTimezone string `json:"to_timezone,omitempty" jsonschema_description:"convert 的目标时区（IANA 名称），缺省取 timezone，再缺省 UTC。"`
	// Duration add 的增量（支持 h/m/s/ms/us/ns 与 d）
	Duration string `json:"duration,omitempty" jsonschema_description:"add 的增量，支持 Go 时长单位 h/m/s/ms/us/ns 以及 d（天，按 24 小时计），如 1d2h。"`
	// Start diff 的起始时间
	Start string `json:"start,omitempty" jsonschema_description:"diff 的起始时间，支持 RFC3339 与 2006-01-02 15:04:05 两种格式。"`
	// End diff 的结束时间
	End string `json:"end,omitempty" jsonschema_description:"diff 的结束时间，支持 RFC3339 与 2006-01-02 15:04:05 两种格式。"`
}

// datetimeCalcTool 是内置工具 datetime_calc 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var datetimeCalcTool = mustInferTool(
	"datetime_calc",
	"进行日期与时间计算：获取当前时间、在时区间转换、对时间做加减、计算两个时间的间隔。"+
		"时区用 IANA 名称（如 Asia/Shanghai），缺省 UTC；时间支持 RFC3339 与 2006-01-02 15:04:05 两种格式。",
	func(_ context.Context, in datetimeCalcArgs) (any, error) {
		result, err := utils.RunDatetimeCalc(utils.DatetimeCalcInput{
			Operation:    in.Operation,
			Timezone:     in.Timezone,
			Time:         in.Time,
			FromTimezone: in.FromTimezone,
			ToTimezone:   in.ToTimezone,
			Duration:     in.Duration,
			Start:        in.Start,
			End:          in.End,
		})
		if err != nil {
			return nil, errors.Newf("datetime_calc: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 datetime_calc；工具无状态，不消费 Deps。
func init() {
	Register("datetime_calc", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return datetimeCalcTool, nil
	})
}
