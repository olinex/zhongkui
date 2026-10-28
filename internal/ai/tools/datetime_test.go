// 本文件是内置工具 datetime_calc 的适配器级测试：只覆盖 Info schema、经 InvokableRun 的错误路径
// （断言「工具名: 子串」）与 1~2 个 JSON 线形状 smoke 用例。算法与域级用例已随实现下沉到
// internal/pkg/utils/datetime_test.go。

package tools

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"
)

// TestDatetimeCalc_Info 覆盖 Info：工具名为 datetime_calc；operation 存在、类型为 string、必填且带
// now/convert/add/diff 四个枚举；关键可选参数存在。
func TestDatetimeCalc_Info(t *testing.T) {
	info, err := datetimeCalcTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "datetime_calc" {
		t.Errorf("Info name = %q, want datetime_calc", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	operation, ok := js.Properties.Get("operation")
	if !ok || operation.Type != string(schema.String) {
		t.Fatalf("operation property = %+v (present=%v), want a string", operation, ok)
	}
	if !datetimeTestRequiredContains(js.Required, "operation") {
		t.Errorf("schema required = %v, want operation required", js.Required)
	}
	for _, name := range []string{"now", "convert", "add", "diff"} {
		found := false
		for _, v := range operation.Enum {
			if v == name {
				found = true
			}
		}
		if !found {
			t.Errorf("operation enum = %v, want it to contain %q", operation.Enum, name)
		}
	}
	for _, name := range []string{"timezone", "time", "from_timezone", "to_timezone", "duration", "start", "end"} {
		if _, ok := js.Properties.Get(name); !ok {
			t.Errorf("property %q is missing", name)
		}
	}
	if !strings.Contains(info.Desc, "时区") {
		t.Errorf("desc = %q, want a Chinese description", info.Desc)
	}
}

// TestDatetimeCalc_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法 operation、非法时区、非法时间、
// add 缺 duration 与除 now 外缺 time，断言错误消息带工具名前缀。
func TestDatetimeCalc_InvokableErrors(t *testing.T) {
	cases := map[string]struct {
		args     string
		wantHint string
	}{
		"invalid operation": {`{"operation":"frobnicate"}`, "datetime_calc: invalid operation"},
		"missing operation": {`{}`, "datetime_calc: invalid operation"},
		"invalid timezone":  {`{"operation":"now","timezone":"Not/AZone"}`, `datetime_calc: invalid timezone "Not/AZone"`},
		"invalid time":      {`{"operation":"add","time":"not-a-time","duration":"1h"}`, "datetime_calc: invalid time"},
		"missing time":      {`{"operation":"convert","from_timezone":"UTC","to_timezone":"UTC"}`, "datetime_calc: time must be non-empty"},
		"missing duration":  {`{"operation":"add","time":"2024-01-01T00:00:00Z"}`, "datetime_calc: duration must be non-empty"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := datetimeCalcTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestDatetimeCalc_Smoke 锁定 datetime_calc 的出参 JSON 线形状与字段名（convert 与 diff 各一例）。
func TestDatetimeCalc_Smoke(t *testing.T) {
	convert, err := datetimeCalcTool.InvokableRun(context.Background(),
		`{"operation":"convert","time":"2024-01-01 00:00:00","from_timezone":"UTC","to_timezone":"Asia/Shanghai"}`)
	if err != nil {
		t.Fatalf("InvokableRun(convert) error = %v", err)
	}
	wantConvert := `{"operation":"convert","time":"2024-01-01T08:00:00+08:00","timezone":"Asia/Shanghai",` +
		`"epochSeconds":1704067200,"year":2024,"month":1,"day":1,"hour":8,"minute":0,"second":0,"weekday":"Monday"}`
	if convert != wantConvert {
		t.Errorf("convert = %q, want %q", convert, wantConvert)
	}

	diff, err := datetimeCalcTool.InvokableRun(context.Background(),
		`{"operation":"diff","start":"2024-01-01T00:00:00Z","end":"2024-01-01T01:00:00Z"}`)
	if err != nil {
		t.Fatalf("InvokableRun(diff) error = %v", err)
	}
	if want := `{"operation":"diff","seconds":3600,"duration":"1h0m0s"}`; diff != want {
		t.Errorf("diff = %q, want %q", diff, want)
	}
}

// TestDatetimeCalc_BuildResolves 覆盖 init 期登记生效：Build 可按名解析 datetime_calc 并返回可用的
// eino 工具。
func TestDatetimeCalc_BuildResolves(t *testing.T) {
	built, err := Build(context.Background(), "datetime_calc", Deps{})
	if err != nil {
		t.Fatalf("Build(datetime_calc) error = %v", err)
	}
	info, err := built.Info(context.Background())
	if err != nil {
		t.Fatalf("built tool Info error = %v", err)
	}
	if info.Name != "datetime_calc" {
		t.Errorf("built tool name = %q, want datetime_calc", info.Name)
	}
}

// datetimeTestRequiredContains 判断 JSON schema 的必填列表中是否含给定属性名，供 Info 用例复用。
func datetimeTestRequiredContains(required []string, name string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}
