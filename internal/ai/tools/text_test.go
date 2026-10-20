// 本文件是三个文本类内置工具（regex / json_diff / text_stats）的适配器级测试：只覆盖 Info schema、
// 经 InvokableRun 的错误路径（断言「工具名: 子串」）与每个工具 1~2 个 JSON 线形状 smoke。算法与域级
// 用例已随实现下沉到 internal/pkg/utils/text_test.go。

package tools

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ============================================================================
// regex
// ============================================================================

// TestRegex_Info 覆盖 Info：工具名为 regex；operation 为带枚举的 string 且必填；pattern/text 必填；
// global 为 boolean。
func TestRegex_Info(t *testing.T) {
	info, err := (&regexTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "regex" {
		t.Errorf("Info name = %q, want regex", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	op, ok := js.Properties.Get("operation")
	if !ok || op.Type != string(schema.String) {
		t.Fatalf("operation property = %+v (present=%v), want a string", op, ok)
	}
	foundTest := false
	for _, v := range op.Enum {
		if v == "test" {
			foundTest = true
		}
	}
	if !foundTest {
		t.Errorf("operation enum = %v, want it to contain test", op.Enum)
	}
	for _, name := range []string{"operation", "pattern", "text"} {
		if !textTestRequiredContains(js.Required, name) {
			t.Errorf("required = %v, want %s required", js.Required, name)
		}
	}
	if global, ok := js.Properties.Get("global"); !ok || global.Type != string(schema.Boolean) {
		t.Errorf("global property = %+v (present=%v), want a boolean", global, ok)
	}
}

// TestRegex_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法 pattern、非法 operation、空 pattern 与
// replace 缺 replacement，断言错误消息带工具名前缀；非法入参 JSON 的错误不经工具名前缀。
func TestRegex_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"invalid-pattern", `{"operation":"test","pattern":"(","text":"x"}`, "regex: error parsing regexp"},
		{"invalid-operation", `{"operation":"extract","pattern":"\\d+","text":"x"}`, "regex: invalid operation"},
		{"missing-operation", `{"pattern":"\\d+","text":"x"}`, "regex: invalid operation"},
		{"empty-pattern", `{"operation":"test","pattern":"","text":"x"}`, "regex: pattern must be non-empty"},
		{"missing-replacement", `{"operation":"replace","pattern":"\\d+","text":"x"}`, "regex: replacement must be non-empty"},
		{"invalid-arguments-in", `not-json`, "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&regexTool{}).InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestRegex_Smoke 锁定 regex 的出参 JSON 线形状与字段名（test 与 replace 各一例）。
func TestRegex_Smoke(t *testing.T) {
	test, err := (&regexTool{}).InvokableRun(context.Background(), `{"operation":"test","pattern":"\\d+","text":"a12b345"}`)
	if err != nil {
		t.Fatalf("InvokableRun(test) error = %v", err)
	}
	wantTest := `{"operation":"test","matched":true,"count":2,"matches":[` +
		`{"text":"12","index":1,"groups":[],"namedGroups":{}},` +
		`{"text":"345","index":4,"groups":[],"namedGroups":{}}]}`
	if test != wantTest {
		t.Errorf("test = %q, want %q", test, wantTest)
	}

	replace, err := (&regexTool{}).InvokableRun(context.Background(),
		`{"operation":"replace","pattern":"\\d+","text":"a12b345","replacement":"-","global":true}`)
	if err != nil {
		t.Fatalf("InvokableRun(replace) error = %v", err)
	}
	if want := `{"operation":"replace","result":"a-b-","count":2}`; replace != want {
		t.Errorf("replace = %q, want %q", replace, want)
	}
}

// ============================================================================
// json_diff
// ============================================================================

// TestJSONDiff_Info 覆盖 Info：工具名为 json_diff；left / right 均为必填 string。
func TestJSONDiff_Info(t *testing.T) {
	info, err := (&jsonDiffTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "json_diff" {
		t.Errorf("Info name = %q, want json_diff", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	for _, name := range []string{"left", "right"} {
		prop, ok := js.Properties.Get(name)
		if !ok || prop.Type != string(schema.String) {
			t.Errorf("property %q = %+v (present=%v), want a string", name, prop, ok)
		}
		if !textTestRequiredContains(js.Required, name) {
			t.Errorf("required = %v, want %s required", js.Required, name)
		}
	}
}

// TestJSONDiff_InvokableErrors 覆盖经 InvokableRun 的错误路径：任一侧非法 JSON，断言错误消息带工具名
// 前缀。
func TestJSONDiff_InvokableErrors(t *testing.T) {
	cases := map[string]string{
		"invalid-left":  `{"left":"{","right":"{}"}`,
		"invalid-right": `{"left":"{}","right":"[1,"}`,
		"both-invalid":  `{"left":"oops","right":"nope"}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := (&jsonDiffTool{}).InvokableRun(context.Background(), args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), "json_diff:") || !strings.Contains(err.Error(), "not valid JSON") {
				t.Errorf("error = %q, want a json_diff-prefixed invalid-JSON message", err.Error())
			}
		})
	}
}

// TestJSONDiff_Smoke 锁定 json_diff 的出参 JSON 线形状与字段名。
func TestJSONDiff_Smoke(t *testing.T) {
	out, err := (&jsonDiffTool{}).InvokableRun(context.Background(),
		`{"left":"{\"a\":1,\"b\":2}","right":"{\"a\":1,\"b\":3,\"c\":4}"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"equal":false,"count":2,"differences":[` +
		`{"path":"b","kind":"changed","left":2,"right":3},` +
		`{"path":"c","kind":"added","left":null,"right":4}]}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// text_stats
// ============================================================================

// TestTextStats_Info 覆盖 Info：工具名为 text_stats；text 为必填 string。
func TestTextStats_Info(t *testing.T) {
	info, err := (&textStatsTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "text_stats" {
		t.Errorf("Info name = %q, want text_stats", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	prop, ok := js.Properties.Get("text")
	if !ok || prop.Type != string(schema.String) {
		t.Errorf("text property = %+v (present=%v), want a string", prop, ok)
	}
	if !textTestRequiredContains(js.Required, "text") {
		t.Errorf("schema required = %v, want text required", js.Required)
	}
}

// TestTextStats_Smoke 锁定 text_stats 的出参 JSON 线形状与字段名（spec 示例）。
func TestTextStats_Smoke(t *testing.T) {
	out, err := (&textStatsTool{}).InvokableRun(context.Background(), `{"text":"你好\nworld OK"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"characters":11,"charactersNoWhitespace":9,"bytes":15,"words":3,"lines":2,"paragraphs":1}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// TestTextTools_BuildResolves 覆盖 init 期登记生效：Build 可按名解析三个工具并返回可用的 eino 工具。
func TestTextTools_BuildResolves(t *testing.T) {
	for _, name := range []string{"regex", "json_diff", "text_stats"} {
		t.Run(name, func(t *testing.T) {
			built, err := Build(context.Background(), name, Deps{})
			if err != nil {
				t.Fatalf("Build(%s) error = %v", name, err)
			}
			info, err := built.Info(context.Background())
			if err != nil {
				t.Fatalf("built tool Info error = %v", err)
			}
			if info.Name != name {
				t.Errorf("built tool name = %q, want %s", info.Name, name)
			}
		})
	}
}

// textTestRequiredContains 判断 JSON schema 的必填列表中是否含给定属性名，供各 Info 用例复用。
func textTestRequiredContains(required []string, name string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}

// 仅用于确认三个工具满足 eino 的可调用工具接口（编译期断言）。
var _ tool.InvokableTool = (*regexTool)(nil)
var _ tool.InvokableTool = (*jsonDiffTool)(nil)
var _ tool.InvokableTool = (*textStatsTool)(nil)
