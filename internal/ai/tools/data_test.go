// 本文件是三个数据类内置工具（json / csv_parse / url_parse）的适配器级测试：只覆盖 Info schema、经
// InvokableRun 的错误路径（断言「工具名: 子串」）与每个工具的 JSON 线形状 smoke。算法与域级用例已随
// 实现下沉到 internal/pkg/utils/data_test.go。

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
// json
// ============================================================================

// TestJSON_Info 覆盖 Info：工具名为 json；operation 为带四种取值的必填 string；json 必填 string；
// indent 为可选 integer；path 为可选 string。
func TestJSON_Info(t *testing.T) {
	info, err := (&jsonTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "json" {
		t.Errorf("Info name = %q, want json", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	operation, ok := js.Properties.Get("operation")
	if !ok || operation.Type != string(schema.String) || len(operation.Enum) != 4 {
		t.Errorf("operation property = %+v (present=%v), want a string with the four operations", operation, ok)
	}
	if raw, ok := js.Properties.Get("json"); !ok || raw.Type != string(schema.String) {
		t.Errorf("json property = %+v (present=%v), want a string", raw, ok)
	}
	if indent, ok := js.Properties.Get("indent"); !ok || indent.Type != string(schema.Integer) {
		t.Errorf("indent property = %+v (present=%v), want an integer", indent, ok)
	}
	if _, ok := js.Properties.Get("path"); !ok {
		t.Errorf("path property missing from the json schema")
	}
	for _, name := range []string{"operation", "json"} {
		if !dataTestRequiredContains(js.Required, name) {
			t.Errorf("required = %v, want %s required", js.Required, name)
		}
	}
}

// TestJSON_InvokableErrors 覆盖经 InvokableRun 的错误路径：非法 operation、format 缩进越界、
// format / minify / query 遇非法 JSON、query 路径语法非法，断言错误消息带工具名前缀。
func TestJSON_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"bad-operation", `{"operation":"bogus","json":"{}"}`, "json: invalid operation"},
		{"missing-operation", `{"operation":"","json":"{}"}`, "json: invalid operation"},
		{"format-bad-json", `{"operation":"format","json":"{"}`, "json: invalid JSON"},
		{"minify-bad-json", `{"operation":"minify","json":"{"}`, "json: invalid JSON"},
		{"query-bad-json", `{"operation":"query","json":"{","path":"a"}`, "json: invalid JSON"},
		{"query-bad-path", `{"operation":"query","json":"{}","path":"a[0"}`, "json: invalid path"},
		{"query-no-path", `{"operation":"query","json":"{}","path":""}`, "json: path must be non-empty"},
		{"bad-indent", `{"operation":"format","json":"{}","indent":99}`, "json: indent 99 out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&jsonTool{}).InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestJSON_Smoke 锁定 json 的出参 JSON 线形状与字段名（validate / minify / query 各一例）。
func TestJSON_Smoke(t *testing.T) {
	cases := map[string]struct {
		args string
		want string
	}{
		"validate": {`{"operation":"validate","json":"{\"a\":1}"}`, `{"operation":"validate","valid":true}`},
		"minify":   {`{"operation":"minify","json":"{\n  \"a\": 1\n}"}`, `{"operation":"minify","result":"{\"a\":1}"}`},
		"query":    {`{"operation":"query","json":"{\"a\":{\"b\":[10,20]}}","path":"a.b[1]"}`, `{"operation":"query","found":true,"value":20,"path":"a.b[1]"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := (&jsonTool{}).InvokableRun(context.Background(), tc.args)
			if err != nil {
				t.Fatalf("InvokableRun error = %v", err)
			}
			if out != tc.want {
				t.Errorf("result = %q, want %q", out, tc.want)
			}
		})
	}
}

// ============================================================================
// csv_parse
// ============================================================================

// TestCSVParse_Info 覆盖 Info：工具名为 csv_parse；text 为必填 string；delimiter 为 string；
// has_header 为 boolean。
func TestCSVParse_Info(t *testing.T) {
	info, err := (&csvParseTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "csv_parse" {
		t.Errorf("Info name = %q, want csv_parse", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if text, ok := js.Properties.Get("text"); !ok || text.Type != string(schema.String) {
		t.Errorf("text property = %+v (present=%v), want a string", text, ok)
	}
	if header, ok := js.Properties.Get("has_header"); !ok || header.Type != string(schema.Boolean) {
		t.Errorf("has_header property = %+v (present=%v), want a boolean", header, ok)
	}
	if delimiter, ok := js.Properties.Get("delimiter"); !ok || delimiter.Type != string(schema.String) {
		t.Errorf("delimiter property = %+v (present=%v), want a string", delimiter, ok)
	}
	if !dataTestRequiredContains(js.Required, "text") {
		t.Errorf("required = %v, want text required", js.Required)
	}
}

// TestCSVParse_InvokableErrors 覆盖经 InvokableRun 的错误路径：多字符分隔符与语法非法的 CSV，断言
// 错误消息带工具名前缀。
func TestCSVParse_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"multi-delimiter", `{"text":"a,b","delimiter":";;"}`, "csv_parse: delimiter"},
		{"unclosed-quote", `{"text":"\"a,b","has_header":false}`, "csv_parse:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&csvParseTool{}).InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestCSVParse_Smoke 锁定 csv_parse 的出参 JSON 线形状与字段名。
func TestCSVParse_Smoke(t *testing.T) {
	out, err := (&csvParseTool{}).InvokableRun(context.Background(), `{"text":"a,b\n1,2\n3,4"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"delimiter":",","hasHeader":true,"header":["a","b"],"rows":[["1","2"],["3","4"]],"rowCount":2,"columnCount":2}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// ============================================================================
// url_parse
// ============================================================================

// TestURLParse_Info 覆盖 Info：工具名为 url_parse；url 为必填 string。
func TestURLParse_Info(t *testing.T) {
	info, err := (&urlParseTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "url_parse" {
		t.Errorf("Info name = %q, want url_parse", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	if target, ok := js.Properties.Get("url"); !ok || target.Type != string(schema.String) {
		t.Errorf("url property = %+v (present=%v), want a string", target, ok)
	}
	if !dataTestRequiredContains(js.Required, "url") {
		t.Errorf("required = %v, want url required", js.Required)
	}
}

// TestURLParse_InvokableErrors 覆盖经 InvokableRun 的错误路径：空 url、缺失 url 与非法 URL，断言错误
// 消息带工具名前缀。
func TestURLParse_InvokableErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		wantHint string
	}{
		{"empty", `{"url":""}`, "url_parse: url must be non-empty"},
		{"missing", `{}`, "url_parse: url must be non-empty"},
		{"invalid", `{"url":"http://[::1"}`, "url_parse: invalid URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&urlParseTool{}).InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// TestURLParse_Smoke 锁定 url_parse 的出参 JSON 线形状与字段名。
func TestURLParse_Smoke(t *testing.T) {
	out, err := (&urlParseTool{}).InvokableRun(context.Background(),
		`{"url":"https://u:p@example.com:8443/a/b?x=1&x=2#frag"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	want := `{"scheme":"https","user":"u","host":"example.com","port":"8443","path":"/a/b","fragment":"frag",` +
		`"query":{"x":["1","2"]},"queryString":"x=1\u0026x=2","isAbsolute":true}`
	if out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

// dataTestRequiredContains 判断 JSON schema 的必填列表中是否含给定属性名，供各 Info 用例复用。
func dataTestRequiredContains(required []string, name string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}

// TestDataTools_BuildResolves 覆盖 init 期登记生效：Build 可按名解析三个工具并返回可用的 eino 工具。
func TestDataTools_BuildResolves(t *testing.T) {
	for _, name := range []string{"json", "csv_parse", "url_parse"} {
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

// 仅用于确认工具满足 eino 的可调用工具接口（编译期断言）。
var _ tool.InvokableTool = (*jsonTool)(nil)
var _ tool.InvokableTool = (*csvParseTool)(nil)
var _ tool.InvokableTool = (*urlParseTool)(nil)
