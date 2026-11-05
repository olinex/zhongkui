// 本文件覆盖 internal/pkg/utils 中 datetime_calc 的算法与域级用例：now、convert（UTC→Asia/Shanghai）、
// add（含 d 单位）、diff，以及错误路径（非法时区、非法时间、非法 operation、缺 time）。全部用例直接
// 调用 utils 导出函数 RunDatetimeCalc，不经过任何适配器或运行期上下文；错误断言针对 utils 抛出的
// 无工具名前缀的错误。

package utils

import (
	// 标准库
	"strings"
	"testing"
)

// datetimeUtilPoint 调用 RunDatetimeCalc 并解出「时间点」形态的结果，供 now / convert / add 用例复用。
func datetimeUtilPoint(t *testing.T, in DatetimeCalcInput) DatetimePointResult {
	t.Helper()
	result, err := RunDatetimeCalc(in)
	if err != nil {
		t.Fatalf("RunDatetimeCalc(%+v) error = %v", in, err)
	}
	point, ok := result.(DatetimePointResult)
	if !ok {
		t.Fatalf("result type = %T, want DatetimePointResult", result)
	}
	return point
}

// datetimeUtilDiff 调用 RunDatetimeCalc 并解出「间隔」形态的结果，供 diff 用例复用。
func datetimeUtilDiff(t *testing.T, in DatetimeCalcInput) DatetimeDiffResult {
	t.Helper()
	result, err := RunDatetimeCalc(in)
	if err != nil {
		t.Fatalf("RunDatetimeCalc(%+v) error = %v", in, err)
	}
	diff, ok := result.(DatetimeDiffResult)
	if !ok {
		t.Fatalf("result type = %T, want DatetimeDiffResult", result)
	}
	return diff
}

// TestRunDatetimeCalc_Now 覆盖 now：返回结构非空、operation 与 timezone 正确；缺省 timezone 为 UTC。
func TestRunDatetimeCalc_Now(t *testing.T) {
	shanghai := datetimeUtilPoint(t, DatetimeCalcInput{Operation: "now", Timezone: "Asia/Shanghai"})
	if shanghai.Operation != "now" {
		t.Errorf("operation = %q, want now", shanghai.Operation)
	}
	if shanghai.Timezone != "Asia/Shanghai" {
		t.Errorf("timezone = %q, want Asia/Shanghai", shanghai.Timezone)
	}
	if shanghai.Time == "" || shanghai.Weekday == "" || shanghai.Year == 0 {
		t.Errorf("result = %+v, want a filled time point", shanghai)
	}

	// 缺省时区应为 UTC。
	if got := datetimeUtilPoint(t, DatetimeCalcInput{Operation: "now"}).Timezone; got != "UTC" {
		t.Errorf("default timezone = %q, want UTC", got)
	}
}

// TestRunDatetimeCalc_Convert 覆盖 convert：UTC 时间转到 Asia/Shanghai 得到 08:00:00+08:00，且字段与
// 目标时区一致；同时验证 RFC3339 入参亦可（自带偏移）。
func TestRunDatetimeCalc_Convert(t *testing.T) {
	cases := map[string]string{
		"simple":  "2024-01-01 00:00:00",
		"rfc3339": "2024-01-01T00:00:00Z",
	}
	for name, timeStr := range cases {
		t.Run(name, func(t *testing.T) {
			got := datetimeUtilPoint(t, DatetimeCalcInput{
				Operation:    "convert",
				Time:         timeStr,
				FromTimezone: "UTC",
				ToTimezone:   "Asia/Shanghai",
			})
			if got.Time != "2024-01-01T08:00:00+08:00" {
				t.Errorf("time = %q, want %q", got.Time, "2024-01-01T08:00:00+08:00")
			}
			if got.Timezone != "Asia/Shanghai" {
				t.Errorf("timezone = %q, want Asia/Shanghai", got.Timezone)
			}
			if got.Year != 2024 || got.Month != 1 || got.Day != 1 || got.Hour != 8 {
				t.Errorf("fields = %+v, want 2024-01-01 08:xx:xx", got)
			}
			if got.Weekday != "Monday" {
				t.Errorf("weekday = %q, want Monday", got.Weekday)
			}
		})
	}
}

// TestRunDatetimeCalc_Add 覆盖 add：支持 Go 时长单位与 d（天，按 24 小时计）单位，含复合时长。
func TestRunDatetimeCalc_Add(t *testing.T) {
	cases := map[string]struct {
		duration string
		want     string
	}{
		"day":   {"1d", "2024-01-02T00:00:00Z"},
		"mixed": {"1d2h", "2024-01-02T02:00:00Z"},
		"hours": {"90m", "2024-01-01T01:30:00Z"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := datetimeUtilPoint(t, DatetimeCalcInput{
				Operation: "add",
				Time:      "2024-01-01T00:00:00Z",
				Duration:  tc.duration,
				Timezone:  "UTC",
			})
			if got.Operation != "add" {
				t.Errorf("operation = %q, want add", got.Operation)
			}
			if got.Time != tc.want {
				t.Errorf("time = %q, want %q", got.Time, tc.want)
			}
			if got.Timezone != "UTC" {
				t.Errorf("timezone = %q, want UTC", got.Timezone)
			}
		})
	}
}

// TestRunDatetimeCalc_Diff 覆盖 diff：返回秒数与可读时长；反向（负）间隔同样应可计算。
func TestRunDatetimeCalc_Diff(t *testing.T) {
	got := datetimeUtilDiff(t, DatetimeCalcInput{
		Operation: "diff",
		Start:     "2024-01-01T00:00:00Z",
		End:       "2024-01-01T01:00:00Z",
	})
	if got.Operation != "diff" {
		t.Errorf("operation = %q, want diff", got.Operation)
	}
	if got.Seconds != 3600 {
		t.Errorf("seconds = %v, want 3600", got.Seconds)
	}
	if got.Duration != "1h0m0s" {
		t.Errorf("duration = %q, want 1h0m0s", got.Duration)
	}

	reversed := datetimeUtilDiff(t, DatetimeCalcInput{
		Operation: "diff",
		Start:     "2024-01-01 01:00:00",
		End:       "2024-01-01 00:00:00",
	})
	if reversed.Seconds != -3600 {
		t.Errorf("reversed seconds = %v, want -3600", reversed.Seconds)
	}
}

// TestRunDatetimeCalc_Errors 覆盖错误路径：非法 operation、非法时区、非法时间、add 缺 duration 与除
// now 外缺 time，均返回无工具名前缀的英文错误。
func TestRunDatetimeCalc_Errors(t *testing.T) {
	cases := []struct {
		name     string
		in       DatetimeCalcInput
		wantHint string
	}{
		{"invalid operation", DatetimeCalcInput{Operation: "frobnicate"}, "invalid operation"},
		{"missing operation", DatetimeCalcInput{}, "invalid operation"},
		{"invalid timezone", DatetimeCalcInput{Operation: "now", Timezone: "Not/AZone"}, `invalid timezone "Not/AZone"`},
		{"invalid time", DatetimeCalcInput{Operation: "add", Time: "not-a-time", Duration: "1h"}, "invalid time"},
		{"missing time", DatetimeCalcInput{Operation: "convert", FromTimezone: "UTC", ToTimezone: "UTC"}, "time must be non-empty"},
		{"missing duration", DatetimeCalcInput{Operation: "add", Time: "2024-01-01T00:00:00Z"}, "duration must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunDatetimeCalc(tc.in)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
			if strings.Contains(err.Error(), "datetime_calc:") {
				t.Errorf("error = %q, want no tool-name prefix", err.Error())
			}
		})
	}
}
