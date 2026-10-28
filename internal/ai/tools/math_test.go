// 本文件是内置数学类工具（math_eval / unit_convert / base_convert）的适配器级测试：只覆盖 Info
// schema、经 InvokableRun 的错误路径（断言「工具名: 子串」）与每个工具 1~2 个 JSON 线形状 smoke。
// 算法与域级用例已随实现下沉到 internal/pkg/utils/math_test.go。

package tools

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"
)

// ============================================================================
// 数学求值工具
// ============================================================================

// TestMathEval_Info 覆盖 Info：工具名与入参 schema（expression 为 string 且必填）。
func TestMathEval_Info(t *testing.T) {
	info, err := mathEvalTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "math_eval" {
		t.Errorf("Info name = %q, want math_eval", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	expr, ok := js.Properties.Get("expression")
	if !ok || expr.Type != string(schema.String) {
		t.Errorf("expression property = %+v (present=%v), want a string", expr, ok)
	}
	if !mathTestRequiredContains(js.Required, "expression") {
		t.Errorf("schema required = %v, want expression required", js.Required)
	}
}

// TestMathEval_InvokableErrors 覆盖经 InvokableRun 的错误路径：空表达式、缺失表达式与非法语法，断言
// 错误消息带工具名前缀。
func TestMathEval_InvokableErrors(t *testing.T) {
	cases := map[string]string{
		"empty":       `{"expression":""}`,
		"whitespace":  `{"expression":"   "}`,
		"missing":     `{}`,
		"unbalanced":  `{"expression":"(1+2"}`,
		"trailing op": `{"expression":"1+"}`,
	}
	want := map[string]string{
		"empty":       "math_eval: expression must be non-empty",
		"whitespace":  "math_eval: expression must be non-empty",
		"missing":     "math_eval: expression must be non-empty",
		"unbalanced":  "math_eval: invalid expression",
		"trailing op": "math_eval: invalid expression",
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := mathEvalTool.InvokableRun(context.Background(), args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), want[name]) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), want[name])
			}
		})
	}
}

// TestMathEval_Smoke 锁定 math_eval 的出参 JSON 线形状与字段名。
func TestMathEval_Smoke(t *testing.T) {
	out, err := mathEvalTool.InvokableRun(context.Background(), `{"expression":"(1+2)*3-4/2"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if want := `{"result":7,"expression":"(1+2)*3-4/2"}`; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// 单位换算工具
// ============================================================================

// TestUnitConvert_Info 覆盖 Info：工具名与入参 schema（value 为 number、from/to 为 string 且均必填）。
func TestUnitConvert_Info(t *testing.T) {
	info, err := unitConvertTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "unit_convert" {
		t.Errorf("Info name = %q, want unit_convert", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if v, ok := js.Properties.Get("value"); !ok || v.Type != string(schema.Number) {
		t.Errorf("value property = %+v (present=%v), want a number", v, ok)
	}
	if v, ok := js.Properties.Get("from"); !ok || v.Type != string(schema.String) {
		t.Errorf("from property = %+v (present=%v), want a string", v, ok)
	}
	if v, ok := js.Properties.Get("to"); !ok || v.Type != string(schema.String) {
		t.Errorf("to property = %+v (present=%v), want a string", v, ok)
	}
	for _, name := range []string{"value", "from", "to"} {
		if !mathTestRequiredContains(js.Required, name) {
			t.Errorf("schema required = %v, want %q required", js.Required, name)
		}
	}
}

// TestUnitConvert_InvokableErrors 覆盖经 InvokableRun 的错误路径：未知单位、跨类别换算与空单位，断言
// 错误消息带工具名前缀。
func TestUnitConvert_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"unknown from", `{"value":1,"from":"lightyear","to":"m"}`, "unit_convert: unknown unit"},
		{"unknown to", `{"value":1,"from":"m","to":"parsec"}`, "unit_convert: unknown unit"},
		{"cross category", `{"value":1,"from":"km","to":"kg"}`, "unit_convert: cannot convert from"},
		{"empty from", `{"value":1,"from":"","to":"m"}`, "unit_convert: from must be non-empty"},
		{"empty to", `{"value":1,"from":"m","to":""}`, "unit_convert: to must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unitConvertTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestUnitConvert_Smoke 锁定 unit_convert 的出参 JSON 线形状与字段名。
func TestUnitConvert_Smoke(t *testing.T) {
	out, err := unitConvertTool.InvokableRun(context.Background(), `{"value":1,"from":"km","to":"m"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if want := `{"result":1000,"from":"km","to":"m","category":"length"}`; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// 进制转换工具
// ============================================================================

// TestBaseConvert_Info 覆盖 Info：工具名与入参 schema（value 为 string、from_base/to_base 为 integer
// 且均必填）。
func TestBaseConvert_Info(t *testing.T) {
	info, err := baseConvertTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "base_convert" {
		t.Errorf("Info name = %q, want base_convert", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if v, ok := js.Properties.Get("value"); !ok || v.Type != string(schema.String) {
		t.Errorf("value property = %+v (present=%v), want a string", v, ok)
	}
	for _, name := range []string{"from_base", "to_base"} {
		if v, ok := js.Properties.Get(name); !ok || v.Type != string(schema.Integer) {
			t.Errorf("%s property = %+v (present=%v), want an integer", name, v, ok)
		}
		if !mathTestRequiredContains(js.Required, name) {
			t.Errorf("schema required = %v, want %q required", js.Required, name)
		}
	}
}

// TestBaseConvert_InvokableErrors 覆盖经 InvokableRun 的错误路径：进制越界、value 对源进制非法与空
// value，断言错误消息带工具名前缀。
func TestBaseConvert_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"from_base too small", `{"value":"101","from_base":1,"to_base":10}`, "base_convert: from_base 1 is out of range"},
		{"from_base too big", `{"value":"101","from_base":37,"to_base":10}`, "base_convert: from_base 37 is out of range"},
		{"to_base too small", `{"value":"101","from_base":2,"to_base":1}`, "base_convert: to_base 1 is out of range"},
		{"to_base too big", `{"value":"101","from_base":2,"to_base":37}`, "base_convert: to_base 37 is out of range"},
		{"invalid digits", `{"value":"g","from_base":16,"to_base":10}`, `base_convert: value "g" is not a valid base-16 integer`},
		{"empty value", `{"value":"","from_base":10,"to_base":2}`, "base_convert: value must be non-empty"},
		{"missing value", `{"from_base":10,"to_base":2}`, "base_convert: value must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := baseConvertTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestBaseConvert_Smoke 锁定 base_convert 的出参 JSON 线形状与字段名。
func TestBaseConvert_Smoke(t *testing.T) {
	out, err := baseConvertTool.InvokableRun(context.Background(), `{"value":"ff","from_base":16,"to_base":2}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if want := `{"result":"11111111","decimal":"255"}`; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// TestMathTools_BuildResolves 覆盖 init 期登记生效：Build 可按名解析三个数学类工具。
func TestMathTools_BuildResolves(t *testing.T) {
	for _, name := range []string{"math_eval", "unit_convert", "base_convert"} {
		built, err := Build(context.Background(), name, Deps{})
		if err != nil {
			t.Fatalf("Build(%s) error = %v", name, err)
		}
		info, err := built.Info(context.Background())
		if err != nil {
			t.Fatalf("built tool %s Info error = %v", name, err)
		}
		if info.Name != name {
			t.Errorf("built tool name = %q, want %q", info.Name, name)
		}
	}
}

// mathTestRequiredContains 判断 JSON schema 的必填列表中是否含给定属性名，供各 Info 用例复用。
func mathTestRequiredContains(required []string, name string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}
