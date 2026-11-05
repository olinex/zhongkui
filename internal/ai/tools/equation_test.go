// 本文件是四个高阶方程计算内置工具（poly_solve / linear_solve / root_find / nonlinear_solve）的
// 适配器级测试：只覆盖 Info schema、经 InvokableRun 的错误路径（断言「工具名: 子串」）与每个工具
// 1~2 个 JSON 线形状 smoke。算法与域级用例已随实现下沉到 internal/pkg/utils/equation_test.go。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"
)

// ============================================================================
// 多项式求解工具
// ============================================================================

// TestPolySolve_Info 覆盖 Info：工具名与入参 schema（coefficients 为 number 数组且必填）。
func TestPolySolve_Info(t *testing.T) {
	info, err := polySolveTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "poly_solve" {
		t.Errorf("Info name = %q, want poly_solve", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if v, ok := js.Properties.Get("coefficients"); !ok || v.Type != string(schema.Array) {
		t.Errorf("coefficients property = %+v (present=%v), want an array", v, ok)
	}
	if !mathTestRequiredContains(js.Required, "coefficients") {
		t.Errorf("schema required = %v, want coefficients required", js.Required)
	}
	if v, ok := js.Properties.Get("maxIterations"); !ok || v.Type != string(schema.Integer) {
		t.Errorf("maxIterations property = %+v (present=%v), want an integer", v, ok)
	}
}

// TestPolySolve_InvokableErrors 覆盖经 InvokableRun 的错误路径：空数组、全零系数、常数多项式、非正
// 容差与过小的迭代上限，断言错误消息带工具名前缀。
func TestPolySolve_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"empty", `{"coefficients":[]}`, "poly_solve: coefficients must be non-empty"},
		{"missing", `{}`, "poly_solve: coefficients must be non-empty"},
		{"all zero", `{"coefficients":[0,0,0]}`, "poly_solve: coefficients must not be all zero"},
		{"constant polynomial", `{"coefficients":[5]}`, "poly_solve: polynomial degree"},
		{"tolerance not positive", `{"coefficients":[1,0,-1],"tolerance":-1}`, "poly_solve: tolerance"},
		{"max iterations too small", `{"coefficients":[1,0,-1],"maxIterations":0}`, "poly_solve: maxIterations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := polySolveTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestPolySolve_Smoke 锁定 poly_solve 的出参 JSON 线形状与字段名。
func TestPolySolve_Smoke(t *testing.T) {
	out, err := polySolveTool.InvokableRun(context.Background(), `{"coefficients":[1,0,-1]}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"degree":2,"roots":[` +
		`{"real":-1,"imag":0,"exact":"-1","multiplicity":1},` +
		`{"real":1,"imag":0,"exact":"1","multiplicity":1}]}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// 线性方程求解工具
// ============================================================================

// TestLinearSolve_Info 覆盖 Info：工具名与入参 schema（a 为二维 number 数组、b 为 number 数组且均必填）。
func TestLinearSolve_Info(t *testing.T) {
	info, err := linearSolveTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "linear_solve" {
		t.Errorf("Info name = %q, want linear_solve", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if v, ok := js.Properties.Get("a"); !ok || v.Type != string(schema.Array) {
		t.Errorf("a property = %+v (present=%v), want an array", v, ok)
	}
	if v, ok := js.Properties.Get("b"); !ok || v.Type != string(schema.Array) {
		t.Errorf("b property = %+v (present=%v), want an array", v, ok)
	}
	for _, name := range []string{"a", "b"} {
		if !mathTestRequiredContains(js.Required, name) {
			t.Errorf("schema required = %v, want %q required", js.Required, name)
		}
	}
}

// TestLinearSolve_InvokableErrors 覆盖经 InvokableRun 的错误路径：空矩阵、非方阵、行列长度不一致、
// b 长度不符与非正容差，断言错误消息带工具名前缀。
func TestLinearSolve_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"empty a", `{"a":[],"b":[]}`, "linear_solve: matrix a must be non-empty"},
		{"non-square", `{"a":[[1,2,3],[4,5,6]],"b":[1,2]}`, "linear_solve: matrix a must be square"},
		{"row length mismatch", `{"a":[[1,2],[3]],"b":[1,2]}`, "linear_solve: matrix a must be square"},
		{"b length mismatch", `{"a":[[1,2],[3,4]],"b":[1]}`, "linear_solve: b length"},
		{"tolerance not positive", `{"a":[[1,2],[3,4]],"b":[1,2],"tolerance":0}`, "linear_solve: tolerance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := linearSolveTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestLinearSolve_Smoke 锁定 linear_solve 的出参 JSON 线形状与字段名。
func TestLinearSolve_Smoke(t *testing.T) {
	out, err := linearSolveTool.InvokableRun(context.Background(), `{"a":[[2,1],[1,-1]],"b":[1,2]}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"solutionType":"unique","rank":2,"solution":[1,-1],"particular":[],"nullspace":[]}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// 求根工具
// ============================================================================

// TestRootFind_Info 覆盖 Info：工具名与入参 schema（expression 为 string 且必填，interval 为 number 数组）。
func TestRootFind_Info(t *testing.T) {
	info, err := rootFindTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "root_find" {
		t.Errorf("Info name = %q, want root_find", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if v, ok := js.Properties.Get("expression"); !ok || v.Type != string(schema.String) {
		t.Errorf("expression property = %+v (present=%v), want a string", v, ok)
	}
	if !mathTestRequiredContains(js.Required, "expression") {
		t.Errorf("schema required = %v, want expression required", js.Required)
	}
	if v, ok := js.Properties.Get("interval"); !ok || v.Type != string(schema.Array) {
		t.Errorf("interval property = %+v (present=%v), want an array", v, ok)
	}
}

// TestRootFind_InvokableErrors 覆盖经 InvokableRun 的错误路径：既无区间也无初值、空/非法表达式、
// 逆序区间与非正容差，断言错误消息带工具名前缀。
func TestRootFind_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"no interval or initial", `{"expression":"x^2-4"}`, "root_find: at least one of interval or initial"},
		{"empty expression", `{"expression":""}`, "root_find: expression must be non-empty"},
		{"invalid expression", `{"expression":"x^2 -","interval":[-1,1]}`, "root_find: invalid expression"},
		{"reversed interval", `{"expression":"x^2-4","interval":[5,-5]}`, "root_find: interval"},
		{"zero length interval", `{"expression":"x^2-4","interval":[1,1]}`, "must satisfy lower < upper"},
		{"tolerance not positive", `{"expression":"x^2-4","interval":[-5,5],"tolerance":-1}`, "root_find: tolerance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rootFindTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestRootFind_Smoke 锁定 root_find 的出参 JSON 线形状与字段名（root/fValue/method）。
func TestRootFind_Smoke(t *testing.T) {
	out, err := rootFindTool.InvokableRun(context.Background(), `{"expression":"x^2-4","interval":[-5,5]}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	roots, ok := payload["roots"].([]any)
	if !ok || len(roots) != 2 {
		t.Fatalf("roots = %#v, want an array of 2 entries", payload["roots"])
	}
	for i, raw := range roots {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("roots[%d] = %#v, want an object", i, raw)
		}
		for _, key := range []string{"root", "fValue", "method"} {
			if _, present := entry[key]; !present {
				t.Errorf("roots[%d] missing key %q in %+v", i, key, entry)
			}
		}
	}
}

// ============================================================================
// 非线性方程求解工具
// ============================================================================

// TestNonlinearSolve_Info 覆盖 Info：工具名与入参 schema（equations / variables / initial 均必填）。
func TestNonlinearSolve_Info(t *testing.T) {
	info, err := nonlinearSolveTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "nonlinear_solve" {
		t.Errorf("Info name = %q, want nonlinear_solve", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	for _, name := range []string{"equations", "variables", "initial"} {
		if v, ok := js.Properties.Get(name); !ok || v.Type != string(schema.Array) {
			t.Errorf("%s property = %+v (present=%v), want an array", name, v, ok)
		}
		if !mathTestRequiredContains(js.Required, name) {
			t.Errorf("schema required = %v, want %q required", js.Required, name)
		}
	}
}

// TestNonlinearSolve_InvokableErrors 覆盖经 InvokableRun 的错误路径：长度不一致、空方程组与变量重名，
// 断言错误消息带工具名前缀。
func TestNonlinearSolve_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"length mismatch", `{"equations":["x^2-1","y-1"],"variables":["x"],"initial":[1]}`, "nonlinear_solve: equations (2) and variables (1) must have the same length"},
		{"empty equations", `{"equations":[],"variables":[],"initial":[]}`, "nonlinear_solve: equations must be non-empty"},
		{"duplicate variables", `{"equations":["x+y","x-y"],"variables":["x","x"],"initial":[1,1]}`, "nonlinear_solve: duplicate variable name"},
		{"initial length mismatch", `{"equations":["x+y"],"variables":["x"],"initial":[]}`, "nonlinear_solve: initial must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := nonlinearSolveTool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestNonlinearSolve_Smoke 锁定 nonlinear_solve 的出参 JSON 线形状与字段名。
func TestNonlinearSolve_Smoke(t *testing.T) {
	out, err := nonlinearSolveTool.InvokableRun(context.Background(),
		`{"equations":["x^2+y^2-1","x-y"],"variables":["x","y"],"initial":[1,1]}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	for _, key := range []string{"solution", "residual", "iterations", "converged"} {
		if _, present := payload[key]; !present {
			t.Errorf("missing key %q in %+v", key, payload)
		}
	}
	if _, ok := payload["solution"].(map[string]any); !ok {
		t.Errorf("solution = %#v, want an object", payload["solution"])
	}
}

// TestEquationTools_BuildResolves 覆盖 init 期登记生效：Build 可按名解析四个高阶方程计算工具。
func TestEquationTools_BuildResolves(t *testing.T) {
	for _, name := range []string{"poly_solve", "linear_solve", "root_find", "nonlinear_solve"} {
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
