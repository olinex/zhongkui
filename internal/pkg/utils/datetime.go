package utils

// 本文件承载 datetime_calc 的纯算法实现：取当前时间、在时区间转换、对时间做加减、计算两个时间的
// 间隔。它是与业务无关的纯计算，只依赖标准库与内嵌时区数据（import _ "time/tzdata" 兜底，容器 /
// 精简镜像缺系统 tzdata 时仍可用），不依赖 eino 等上层组件；internal/ai/tools 中的同名工具由此处
// 的导出函数驱动，仅负责入参 JSON 解析与结果序列化。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串与重构前
// 保持一致。

import (
	// 标准库
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// DefaultDatetimeTimezone 是时区类可选入参的缺省值：timezone / from_timezone / to_timezone 缺省时
// 一律按 UTC 解释。
const DefaultDatetimeTimezone = "UTC"

// datetimeSimpleLayout 是 datetime_calc 除 RFC3339 外接受的第二种时间格式（无时区偏移，按调用
// 指定的时区解释）。
const datetimeSimpleLayout = "2006-01-02 15:04:05"

// DatetimeOperations 是 operation 的取值空间（now / convert / add / diff），供适配器渲染参数 schema
// 的枚举与错误文案。
var DatetimeOperations = []string{"now", "convert", "add", "diff"}

// datetimeDayDurationRe 匹配时长串中的「天」分量（形如 1d 或 0.5d），用于把天归一化为小时后交给
// time.ParseDuration——后者不认识 d 单位。
var datetimeDayDurationRe = regexp.MustCompile(`([0-9]*\.?[0-9]+)d`)

// DatetimePointResult 是 now / convert / add 返回的载荷：一个时间点在目标时区下的各字段拆解，便于
// 模型直接引用。
type DatetimePointResult struct {
	// Operation 本次操作名
	Operation string `json:"operation"`
	// Time 目标时区下的 RFC3339 时间串
	Time string `json:"time"`
	// Timezone 结果所在时区名
	Timezone string `json:"timezone"`
	// EpochSeconds Unix 秒
	EpochSeconds int64 `json:"epochSeconds"`
	// Year 年
	Year int `json:"year"`
	// Month 月（1-12）
	Month int `json:"month"`
	// Day 日
	Day int `json:"day"`
	// Hour 时
	Hour int `json:"hour"`
	// Minute 分
	Minute int `json:"minute"`
	// Second 秒
	Second int `json:"second"`
	// Weekday 英文星期名
	Weekday string `json:"weekday"`
}

// DatetimeDiffResult 是 diff 返回的载荷：两个时间点之差。
type DatetimeDiffResult struct {
	// Operation 恒为 diff
	Operation string `json:"operation"`
	// Seconds 时间差（秒，含小数）
	Seconds float64 `json:"seconds"`
	// Duration 可读时长，如 60h0m0s
	Duration string `json:"duration"`
}

// DatetimeCalcInput 汇集 datetime_calc 的全部入参，供 RunDatetimeCalc 按 operation 分派。
type DatetimeCalcInput struct {
	// Operation 要执行的操作：now / convert / add / diff
	Operation string
	// Timezone 目标/解释时区（now 输出、convert 输出目标、add 运算时区），缺省 UTC
	Timezone string
	// Time 待处理时间，除 now 外使用
	Time string
	// FromTimezone convert 的源时区，缺省 UTC
	FromTimezone string
	// ToTimezone convert 的目标时区，缺省取 timezone，再缺省 UTC
	ToTimezone string
	// Duration add 的增量（支持 h/m/s/ms/us/ns 与 d）
	Duration string
	// Start diff 的起始时间
	Start string
	// End diff 的结束时间
	End string
}

// RunDatetimeCalc 按 operation 分派 datetime_calc 的各操作，返回 DatetimePointResult（now / convert /
// add）或 DatetimeDiffResult（diff）。operation 非法时返回英文错误（工具名前缀由适配器补齐）。
func RunDatetimeCalc(in DatetimeCalcInput) (any, error) {
	switch strings.TrimSpace(in.Operation) {
	case "now":
		return datetimeNow(in.Timezone)
	case "convert":
		return datetimeConvert(in.Time, in.FromTimezone, in.Timezone, in.ToTimezone)
	case "add":
		return datetimeAdd(in.Time, in.Timezone, in.Duration)
	case "diff":
		return datetimeDiff(in.Start, in.End)
	default:
		return nil, errors.Newf("invalid operation %q, expected one of %s",
			in.Operation, strings.Join(DatetimeOperations, ", "))
	}
}

// datetimeNow 取当前时间并按 timezone（缺省 UTC）输出。
func datetimeNow(timezone string) (DatetimePointResult, error) {
	loc, err := datetimeResolveLocation(timezone)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	return datetimePointFromTime("now", time.Now().In(loc)), nil
}

// datetimeConvert 把 timeStr 从 fromTimezone（缺省 UTC）转换到 toTimezone（缺省 timezone，再缺省 UTC）
// 并输出。
func datetimeConvert(timeStr, fromTimezone, timezone, toTimezone string) (DatetimePointResult, error) {
	fromLoc, err := datetimeResolveLocation(fromTimezone)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	toName := strings.TrimSpace(toTimezone)
	if toName == "" {
		toName = timezone
	}
	toLoc, err := datetimeResolveLocation(toName)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	ts, err := datetimeParse(timeStr, fromLoc)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	return datetimePointFromTime("convert", ts.In(toLoc)), nil
}

// datetimeAdd 在 timezone（缺省 UTC）下解释 timeStr，加上 duration 后原时区输出。
func datetimeAdd(timeStr, timezone, duration string) (DatetimePointResult, error) {
	loc, err := datetimeResolveLocation(timezone)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	ts, err := datetimeParse(timeStr, loc)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	delta, err := datetimeParseDuration(duration)
	if err != nil {
		return DatetimePointResult{}, errors.WithStack(err)
	}
	return datetimePointFromTime("add", ts.Add(delta).In(loc)), nil
}

// datetimeDiff 计算 end 与 start 之间的间隔。两个无偏移时间同处一个时区解释，其差值与时区无关，故
// 此处统一按 UTC 解释。
func datetimeDiff(start, end string) (DatetimeDiffResult, error) {
	s, err := datetimeParse(start, time.UTC)
	if err != nil {
		return DatetimeDiffResult{}, errors.WithStack(err)
	}
	e, err := datetimeParse(end, time.UTC)
	if err != nil {
		return DatetimeDiffResult{}, errors.WithStack(err)
	}
	delta := e.Sub(s)
	return DatetimeDiffResult{
		Operation: "diff",
		Seconds:   delta.Seconds(),
		Duration:  delta.String(),
	}, nil
}

// datetimeResolveLocation 把 IANA 时区名解析为 *time.Location：空串与 UTC 一律走 time.UTC，其余交给
// time.LoadLocation（失败返回英文错误）。内嵌的 time/tzdata 保证无系统时区数据的镜像同样可用。
func datetimeResolveLocation(name string) (*time.Location, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || strings.EqualFold(trimmed, DefaultDatetimeTimezone) {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, errors.Newf("invalid timezone %q", name)
	}
	return loc, nil
}

// datetimeParse 解析时间串：优先 RFC3339（自带偏移），否则按 datetimeSimpleLayout 在 naiveLoc 下解释；
// 空串或两种格式均不匹配时返回英文错误。
func datetimeParse(raw string, naiveLoc *time.Location) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, errors.New("time must be non-empty")
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts, nil
	}
	if ts, err := time.ParseInLocation(datetimeSimpleLayout, s, naiveLoc); err == nil {
		return ts, nil
	}
	return time.Time{}, errors.Newf("invalid time %q, expected RFC3339 or %s", raw, datetimeSimpleLayout)
}

// datetimeParseDuration 解析 duration：先把其中的「天」分量按 24 小时归一化为小时（time.ParseDuration
// 不认识 d），再交给 time.ParseDuration；空串或解析失败返回英文错误。
func datetimeParseDuration(raw string) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, errors.New("duration must be non-empty for add")
	}
	normalized := datetimeDayDurationRe.ReplaceAllStringFunc(s, func(match string) string {
		days, err := strconv.ParseFloat(strings.TrimSuffix(match, "d"), 64)
		if err != nil {
			return match
		}
		return strconv.FormatFloat(days*24, 'f', -1, 64) + "h"
	})
	delta, err := time.ParseDuration(normalized)
	if err != nil {
		return 0, errors.Newf("invalid duration %q", raw)
	}
	return delta, nil
}

// datetimePointFromTime 把一个时间点构造为 DatetimePointResult：时间按所在时区以 RFC3339 输出，并拆解
// 为各字段。
func datetimePointFromTime(operation string, ts time.Time) DatetimePointResult {
	return DatetimePointResult{
		Operation:    operation,
		Time:         ts.Format(time.RFC3339),
		Timezone:     ts.Location().String(),
		EpochSeconds: ts.Unix(),
		Year:         ts.Year(),
		Month:        int(ts.Month()),
		Day:          ts.Day(),
		Hour:         ts.Hour(),
		Minute:       ts.Minute(),
		Second:       ts.Second(),
		Weekday:      ts.Weekday().String(),
	}
}
