// 本文件覆盖 internal/pkg/utils 中三个数据类纯计算能力的算法与域级用例：ProcessJSON（JSON 校验 /
// 格式化 / 压缩 / 路径查询）、ParseCSV（CSV 文本解析）与 ParseURL（URL 拆解）。全部用例直接调用 utils
// 导出函数，不经过任何适配器或运行期上下文；错误断言针对 utils 抛出的无工具名前缀的错误。

package utils

import (
	// 标准库
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// dataUtilValidate 调用 ProcessJSON 的 validate 操作并断言结果为 JSONValidateResult。
func dataUtilValidate(t *testing.T, text string) JSONValidateResult {
	t.Helper()
	out, err := ProcessJSON("validate", text, "", 0)
	if err != nil {
		t.Fatalf("ProcessJSON(validate, %q) error = %v", text, err)
	}
	result, ok := out.(JSONValidateResult)
	if !ok {
		t.Fatalf("result = %#v, want a JSONValidateResult", out)
	}
	return result
}

// ============================================================================
// ProcessJSON：validate
// ============================================================================

// TestProcessJSON_Validate 覆盖 validate 操作：合法文本返回 Valid=true 且 Error 为空；非法文本正常
// 返回 Valid=false 并附错误文本（不视为失败）。
func TestProcessJSON_Validate(t *testing.T) {
	valid := dataUtilValidate(t, `{"a":1}`)
	if valid.Operation != "validate" || !valid.Valid {
		t.Errorf("valid result = %+v, want operation validate and valid true", valid)
	}
	if valid.Error != "" {
		t.Errorf("valid result error = %q, want it omitted", valid.Error)
	}

	invalid := dataUtilValidate(t, `{`)
	if invalid.Valid {
		t.Errorf("invalid result = %+v, want valid false", invalid)
	}
	if invalid.Error == "" {
		t.Errorf("invalid result error is empty, want the parse error text")
	}
}

// ============================================================================
// ProcessJSON：format / minify
// ============================================================================

// TestProcessJSON_Format 覆盖 format 操作：indent 为 0 时取缺省 2、显式 4 时缩进匹配，结果可再次解析。
func TestProcessJSON_Format(t *testing.T) {
	cases := map[string]struct {
		indent int
		expect string
	}{
		"default": {0, "\n  \"a\": 1"},
		"custom":  {4, "\n    \"a\": 1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := ProcessJSON("format", `{"a":1,"b":[2,3]}`, "", tc.indent)
			if err != nil {
				t.Fatalf("ProcessJSON error = %v", err)
			}
			decoded, ok := out.(JSONTextResult)
			if !ok {
				t.Fatalf("result = %#v, want a JSONTextResult", out)
			}
			if decoded.Operation != "format" {
				t.Errorf("operation = %q, want format", decoded.Operation)
			}
			if !strings.Contains(decoded.Result, tc.expect) {
				t.Errorf("result = %q, want it to contain %q", decoded.Result, tc.expect)
			}
			var value any
			if err := json.Unmarshal([]byte(decoded.Result), &value); err != nil {
				t.Errorf("formatted result %q is not valid JSON: %v", decoded.Result, err)
			}
		})
	}
}

// TestProcessJSON_Minify 覆盖 minify 操作：含空白的 JSON 被压缩为紧凑文本。
func TestProcessJSON_Minify(t *testing.T) {
	out, err := ProcessJSON("minify", "{\n  \"a\": 1\n}", "", 0)
	if err != nil {
		t.Fatalf("ProcessJSON error = %v", err)
	}
	decoded, ok := out.(JSONTextResult)
	if !ok {
		t.Fatalf("result = %#v, want a JSONTextResult", out)
	}
	if decoded.Operation != "minify" {
		t.Errorf("operation = %q, want minify", decoded.Operation)
	}
	if decoded.Result != `{"a":1}` {
		t.Errorf("result = %q, want the compacted %q", decoded.Result, `{"a":1}`)
	}
}

// ============================================================================
// ProcessJSON：query
// ============================================================================

// TestProcessJSON_Query 覆盖 query 操作：对象键加数组下标的取值、根数组下标、命中零值仍返回 value、
// 未命中返回 Found=false 且省略 value。
func TestProcessJSON_Query(t *testing.T) {
	cases := map[string]struct {
		text  string
		path  string
		found bool
		value string
	}{
		"nested":  {`{"a":{"b":[10,20]}}`, "a.b[1]", true, "20"},
		"root":    {`[1,2,3]`, "[0]", true, "1"},
		"zero":    {`{"a":0}`, "a", true, "0"},
		"missKey": {`{"a":{"b":[10,20]}}`, "a.c", false, ""},
		"missIdx": {`{"a":{"b":[10,20]}}`, "a.b[9]", false, ""},
		"typeMis": {`{"a":1}`, "a.b", false, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := ProcessJSON("query", tc.text, tc.path, 0)
			if err != nil {
				t.Fatalf("ProcessJSON error = %v", err)
			}
			decoded, ok := out.(JSONQueryResult)
			if !ok {
				t.Fatalf("result = %#v, want a JSONQueryResult", out)
			}
			if decoded.Operation != "query" {
				t.Errorf("operation = %q, want query", decoded.Operation)
			}
			if decoded.Found != tc.found {
				t.Errorf("found = %v, want %v", decoded.Found, tc.found)
			}
			if tc.found {
				if got := string(decoded.Value); got != tc.value {
					t.Errorf("value = %q, want %q", got, tc.value)
				}
			} else if len(decoded.Value) != 0 {
				t.Errorf("value = %q, want it omitted when not found", decoded.Value)
			}
		})
	}
}

// TestProcessJSON_Errors 覆盖错误路径：非法 operation、format 缩进越界、format / minify / query 遇非法
// JSON、query 路径语法非法，均返回英文错误。
func TestProcessJSON_Errors(t *testing.T) {
	cases := map[string]struct {
		operation string
		text      string
		path      string
		indent    int
	}{
		"badOperation": {"bogus", "{}", "", 0},
		"missingOp":    {"", "{}", "", 0},
		"formatBad":    {"format", "{", "", 0},
		"minifyBad":    {"minify", "{", "", 0},
		"queryBadJSON": {"query", "{", "a", 0},
		"queryBadPath": {"query", "{}", "a[0", 0},
		"queryNoPath":  {"query", "{}", "", 0},
		"badIndent":    {"format", "{}", "", 99},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ProcessJSON(tc.operation, tc.text, tc.path, tc.indent); err == nil {
				t.Fatal("error = nil, want an English error")
			}
		})
	}
}

// ============================================================================
// ParseCSV
// ============================================================================

// TestParseCSV_Basic 覆盖正常路径：默认逗号分隔（delimiter 为空取缺省）、首行为表头，表头、数据行与
// 行列数均正确。
func TestParseCSV_Basic(t *testing.T) {
	got, err := ParseCSV("a,b\n1,2\n3,4", "", true)
	if err != nil {
		t.Fatalf("ParseCSV error = %v", err)
	}
	if got.Delimiter != DefaultCSVDelimiter || !got.HasHeader {
		t.Errorf("delimiter/hasHeader = %q/%v, want %q/true", got.Delimiter, got.HasHeader, DefaultCSVDelimiter)
	}
	if !reflect.DeepEqual(got.Header, []string{"a", "b"}) {
		t.Errorf("header = %v, want [a b]", got.Header)
	}
	if !reflect.DeepEqual(got.Rows, [][]string{{"1", "2"}, {"3", "4"}}) {
		t.Errorf("rows = %v, want [[1 2] [3 4]]", got.Rows)
	}
	if got.RowCount != 2 || got.ColumnCount != 2 {
		t.Errorf("rowCount/columnCount = %d/%d, want 2/2", got.RowCount, got.ColumnCount)
	}
}

// TestParseCSV_NoHeader 覆盖 hasHeader=false：首行也作为数据行，header 为空数组而非 null。
func TestParseCSV_NoHeader(t *testing.T) {
	got, err := ParseCSV("1,2\n3,4", "", false)
	if err != nil {
		t.Fatalf("ParseCSV error = %v", err)
	}
	if got.HasHeader {
		t.Errorf("hasHeader = true, want false")
	}
	if len(got.Header) != 0 {
		t.Errorf("header = %v, want empty", got.Header)
	}
	if !reflect.DeepEqual(got.Rows, [][]string{{"1", "2"}, {"3", "4"}}) {
		t.Errorf("rows = %v, want [[1 2] [3 4]]", got.Rows)
	}
	if got.RowCount != 2 || got.ColumnCount != 2 {
		t.Errorf("rowCount/columnCount = %d/%d, want 2/2", got.RowCount, got.ColumnCount)
	}
}

// TestParseCSV_CustomDelimiter 覆盖自定义分隔符：分号生效并回显在结果中。
func TestParseCSV_CustomDelimiter(t *testing.T) {
	got, err := ParseCSV("a;b\n1;2", ";", true)
	if err != nil {
		t.Fatalf("ParseCSV error = %v", err)
	}
	if got.Delimiter != ";" {
		t.Errorf("delimiter = %q, want \";\"", got.Delimiter)
	}
	if !reflect.DeepEqual(got.Header, []string{"a", "b"}) {
		t.Errorf("header = %v, want [a b]", got.Header)
	}
}

// TestParseCSV_Errors 覆盖错误路径：多字符分隔符与语法非法的 CSV 均返回英文错误。
func TestParseCSV_Errors(t *testing.T) {
	cases := []struct {
		text      string
		delimiter string
	}{
		{"a,b", ";;"},
		{"\"a,b", ","},
	}
	for _, tc := range cases {
		if _, err := ParseCSV(tc.text, tc.delimiter, false); err == nil {
			t.Errorf("text %q delimiter %q: error = nil, want an English error", tc.text, tc.delimiter)
		}
	}
}

// ============================================================================
// ParseURL
// ============================================================================

// TestParseURL_Basic 覆盖正常路径：spec 示例 URL 的各字段均正确拆解。
func TestParseURL_Basic(t *testing.T) {
	got, err := ParseURL("https://u:p@example.com:8443/a/b?x=1&x=2#frag")
	if err != nil {
		t.Fatalf("ParseURL error = %v", err)
	}
	if got.Scheme != "https" || got.User != "u" || got.Host != "example.com" {
		t.Errorf("scheme/user/host = %q/%q/%q, want https/u/example.com",
			got.Scheme, got.User, got.Host)
	}
	if got.Port != "8443" || got.Path != "/a/b" || got.Fragment != "frag" {
		t.Errorf("port/path/fragment = %q/%q/%q, want 8443//a/b/frag",
			got.Port, got.Path, got.Fragment)
	}
	if !reflect.DeepEqual(got.Query, map[string][]string{"x": {"1", "2"}}) {
		t.Errorf("query = %v, want map[x:[1 2]]", got.Query)
	}
	if got.QueryString != "x=1&x=2" {
		t.Errorf("queryString = %q, want x=1&x=2", got.QueryString)
	}
	if !got.IsAbsolute {
		t.Errorf("isAbsolute = false, want true")
	}
}

// TestParseURL_Minimal 覆盖无用户信息、无端口、无查询与片段时，字段为空串 / 空映射而非遗漏。
func TestParseURL_Minimal(t *testing.T) {
	got, err := ParseURL("/a/b")
	if err != nil {
		t.Fatalf("ParseURL error = %v", err)
	}
	if got.Scheme != "" || got.User != "" || got.Host != "" || got.Port != "" {
		t.Errorf("scheme/user/host/port = %q/%q/%q/%q, want empty",
			got.Scheme, got.User, got.Host, got.Port)
	}
	if got.Path != "/a/b" || got.IsAbsolute {
		t.Errorf("path/isAbsolute = %q/%v, want /a/b/false", got.Path, got.IsAbsolute)
	}
	if got.Query == nil || len(got.Query) != 0 {
		t.Errorf("query = %v, want an empty (non-nil) map", got.Query)
	}
}

// TestParseURL_Errors 覆盖错误路径：空 url 与非法 URL 均返回英文错误。
func TestParseURL_Errors(t *testing.T) {
	for _, raw := range []string{"", "   ", "http://[::1"} {
		if _, err := ParseURL(raw); err == nil {
			t.Errorf("raw %q: error = nil, want an English error", raw)
		}
	}
}
