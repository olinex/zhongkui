// 本文件覆盖 internal/pkg/utils 中三个文本类纯计算算法的算法与域级用例：RunRegex（正则测试与替换）、
// DiffJSON（JSON 差异比较）与 StatsText（文本统计）。全部用例直接调用 utils 导出函数，不经过任何
// 适配器或运行期上下文；错误断言针对 utils 抛出的无工具名前缀的错误。

package utils

import (
	// 标准库
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================================
// RunRegex
// ============================================================================

// textUtilRegexTest 以 test 操作运行 RunRegex 并解出 RegexTestResult，供各用例共享。
func textUtilRegexTest(t *testing.T, pattern, text string) RegexTestResult {
	t.Helper()
	result, err := RunRegex("test", pattern, text, "", false)
	if err != nil {
		t.Fatalf("RunRegex(test, %q, %q) error = %v", pattern, text, err)
	}
	decoded, ok := result.(RegexTestResult)
	if !ok {
		t.Fatalf("result type = %T, want RegexTestResult", result)
	}
	return decoded
}

// textUtilRegexReplace 以 replace 操作运行 RunRegex 并解出 RegexReplaceResult，供各用例共享。
func textUtilRegexReplace(t *testing.T, pattern, text, replacement string, global bool) RegexReplaceResult {
	t.Helper()
	result, err := RunRegex("replace", pattern, text, replacement, global)
	if err != nil {
		t.Fatalf("RunRegex(replace, %q, %q, %q, %v) error = %v", pattern, text, replacement, global, err)
	}
	decoded, ok := result.(RegexReplaceResult)
	if !ok {
		t.Fatalf("result type = %T, want RegexReplaceResult", result)
	}
	return decoded
}

// TestRunRegex_Test 覆盖 spec 示例：pattern "\d+" / text "a12b345" 命中两处且位置正确。
func TestRunRegex_Test(t *testing.T) {
	got := textUtilRegexTest(t, `\d+`, "a12b345")
	if got.Operation != "test" {
		t.Errorf("operation = %q, want test", got.Operation)
	}
	if !got.Matched || got.Count != 2 {
		t.Fatalf("matched/count = %v/%d, want true/2", got.Matched, got.Count)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("matches length = %d, want 2", len(got.Matches))
	}
	if got.Matches[0].Text != "12" || got.Matches[0].Index != 1 {
		t.Errorf("matches[0] = %+v, want text 12 at index 1", got.Matches[0])
	}
	if got.Matches[1].Text != "345" || got.Matches[1].Index != 4 {
		t.Errorf("matches[1] = %+v, want text 345 at index 4", got.Matches[1])
	}
	if len(got.Matches[0].Groups) != 0 {
		t.Errorf("matches[0].groups = %v, want empty (no capture groups)", got.Matches[0].Groups)
	}
}

// TestRunRegex_TestNoMatch 覆盖未命中：matched=false、count=0 且 matches 为非 nil 的空数组（序列化后
// 为 [] 而非 null）。
func TestRunRegex_TestNoMatch(t *testing.T) {
	got := textUtilRegexTest(t, `\d+`, "abc")
	if got.Matched || got.Count != 0 || len(got.Matches) != 0 {
		t.Errorf("no-match result = %+v, want matched=false count=0 matches empty", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(encoded), `"matches":[]`) {
		t.Errorf("encoded = %q, want an empty JSON array (not null)", encoded)
	}
}

// TestRunRegex_TestGroups 覆盖捕获组与命名捕获组：groups 为子匹配文本，namedGroups 按名索引。
func TestRunRegex_TestGroups(t *testing.T) {
	got := textUtilRegexTest(t, `(?P<year>\d{4})-(?P<month>\d{2})`, "2024-05")
	if got.Count != 1 {
		t.Fatalf("count = %d, want 1", got.Count)
	}
	m := got.Matches[0]
	if m.Text != "2024-05" || m.Index != 0 {
		t.Errorf("match = %+v, want text 2024-05 at index 0", m)
	}
	if len(m.Groups) != 2 || m.Groups[0] != "2024" || m.Groups[1] != "05" {
		t.Errorf("groups = %v, want [2024 05]", m.Groups)
	}
	if m.NamedGroups["year"] != "2024" || m.NamedGroups["month"] != "05" {
		t.Errorf("namedGroups = %v, want year=2024 month=05", m.NamedGroups)
	}
}

// TestRunRegex_Replace 覆盖替换：global=false 仅替换首个，global=true 全部替换，count 与结果一致。
func TestRunRegex_Replace(t *testing.T) {
	first := textUtilRegexReplace(t, `\d+`, "a12b345", "-", false)
	if first.Operation != "replace" || first.Result != "a-b345" || first.Count != 1 {
		t.Errorf("replace(first) = %+v, want result a-b345 count 1", first)
	}

	all := textUtilRegexReplace(t, `\d+`, "a12b345", "-", true)
	if all.Result != "a-b-" || all.Count != 2 {
		t.Errorf("replace(global) = %+v, want result a-b- count 2", all)
	}
}

// TestRunRegex_ReplaceCapture 覆盖替换表达式的 $1 等捕获组引用。
func TestRunRegex_ReplaceCapture(t *testing.T) {
	got := textUtilRegexReplace(t, `(\w+)@(\w+)`, "a@b", "$2.$1", false)
	if got.Result != "b.a" || got.Count != 1 {
		t.Errorf("replace = %+v, want result b.a count 1", got)
	}
}

// TestRunRegex_Errors 覆盖错误路径：非法 pattern、非法 operation、replace 缺 replacement 与空 pattern
// 均返回无工具名前缀的英文错误且不产出结果。
func TestRunRegex_Errors(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		global   bool
		wantHint string
	}{
		{"invalid-pattern", []string{"test", "(", "x", ""}, false, "error parsing regexp"},
		{"invalid-operation", []string{"extract", `\d+`, "x", ""}, false, "invalid operation"},
		{"missing-operation", []string{"", `\d+`, "x", ""}, false, "invalid operation"},
		{"empty-pattern", []string{"test", "", "x", ""}, false, "pattern must be non-empty"},
		{"missing-replacement", []string{"replace", `\d+`, "x", ""}, false, "replacement must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunRegex(tc.args[0], tc.args[1], tc.args[2], tc.args[3], tc.global)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
			if strings.Contains(err.Error(), "regex:") {
				t.Errorf("error = %q, want no tool-name prefix", err.Error())
			}
		})
	}
}

// ============================================================================
// DiffJSON
// ============================================================================

// TestDiffJSON_Differences 覆盖 spec 示例：left {"a":1,"b":2} / right {"a":1,"b":3,"c":4} 得到一条
// changed（b）与一条 added（c），且按路径确定性排列。
func TestDiffJSON_Differences(t *testing.T) {
	got, err := DiffJSON(`{"a":1,"b":2}`, `{"a":1,"b":3,"c":4}`)
	if err != nil {
		t.Fatalf("DiffJSON error = %v", err)
	}
	if got.Equal || got.Count != 2 || len(got.Differences) != 2 {
		t.Fatalf("result = %+v, want equal=false count=2", got)
	}
	if got.Differences[0].Path != "b" || got.Differences[0].Kind != jsonDiffChanged {
		t.Errorf("differences[0] = %+v, want b changed", got.Differences[0])
	}
	if got.Differences[0].Left != float64(2) || got.Differences[0].Right != float64(3) {
		t.Errorf("differences[0] values = %v/%v, want 2/3", got.Differences[0].Left, got.Differences[0].Right)
	}
	if got.Differences[1].Path != "c" || got.Differences[1].Kind != jsonDiffAdded {
		t.Errorf("differences[1] = %+v, want c added", got.Differences[1])
	}
}

// TestDiffJSON_Equal 覆盖相等：键顺序不同但内容一致时 equal=true、count=0 且 differences 序列化为空
// 数组而非 null。
func TestDiffJSON_Equal(t *testing.T) {
	got, err := DiffJSON(`{"a":1,"b":[1,2]}`, `{"b":[1,2],"a":1}`)
	if err != nil {
		t.Fatalf("DiffJSON error = %v", err)
	}
	if !got.Equal || got.Count != 0 || len(got.Differences) != 0 {
		t.Errorf("result = %+v, want equal with no differences", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(encoded), `"differences":[]`) {
		t.Errorf("encoded = %q, want an empty JSON array (not null)", encoded)
	}
}

// TestDiffJSON_Nested 覆盖嵌套路径：对象键用 .key，数组下标用 [i]，长度差视为新增 / 删除。
func TestDiffJSON_Nested(t *testing.T) {
	got, err := DiffJSON(`{"a":{"b":[1,2]}}`, `{"a":{"b":[1,9,3]}}`)
	if err != nil {
		t.Fatalf("DiffJSON error = %v", err)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}
	if got.Differences[0].Path != "a.b[1]" || got.Differences[0].Kind != jsonDiffChanged {
		t.Errorf("differences[0] = %+v, want a.b[1] changed", got.Differences[0])
	}
	if got.Differences[1].Path != "a.b[2]" || got.Differences[1].Kind != jsonDiffAdded {
		t.Errorf("differences[1] = %+v, want a.b[2] added", got.Differences[1])
	}
}

// TestDiffJSON_Removed 覆盖仅左侧存在：left 有 right 无的键记为 removed。
func TestDiffJSON_Removed(t *testing.T) {
	got, err := DiffJSON(`{"a":1,"b":2}`, `{"a":1}`)
	if err != nil {
		t.Fatalf("DiffJSON error = %v", err)
	}
	if got.Count != 1 || got.Differences[0].Path != "b" || got.Differences[0].Kind != jsonDiffRemoved {
		t.Errorf("result = %+v, want single removed difference at b", got)
	}
}

// TestDiffJSON_InvalidJSON 覆盖任一侧非法 JSON：返回无工具名前缀的英文错误且不产出结果。
func TestDiffJSON_InvalidJSON(t *testing.T) {
	cases := []struct {
		name  string
		left  string
		right string
	}{
		{"invalid-left", "{", "{}"},
		{"invalid-right", "{}", "[1,"},
		{"both-invalid", "oops", "nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DiffJSON(tc.left, tc.right)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), "not valid JSON") {
				t.Errorf("error = %q, want it to mention invalid JSON", err.Error())
			}
			if strings.Contains(err.Error(), "json_diff:") {
				t.Errorf("error = %q, want no tool-name prefix", err.Error())
			}
		})
	}
}

// ============================================================================
// StatsText
// ============================================================================

// TestStatsText 覆盖 spec 示例：text "你好\nworld OK" 的六项统计。
func TestStatsText(t *testing.T) {
	got, err := StatsText("你好\nworld OK")
	if err != nil {
		t.Fatalf("StatsText error = %v", err)
	}
	want := TextStatsResult{Characters: 11, CharactersNoWhitespace: 9, Bytes: 15, Words: 3, Lines: 2, Paragraphs: 1}
	if got != want {
		t.Errorf("stats = %+v, want %+v", got, want)
	}
}

// TestStatsText_Empty 覆盖空文本：各项统计均为 0（lines 与 paragraphs 自洽为 0）。
func TestStatsText_Empty(t *testing.T) {
	got, err := StatsText("")
	if err != nil {
		t.Fatalf("StatsText error = %v", err)
	}
	if got != (TextStatsResult{}) {
		t.Errorf("stats = %+v, want all zeros", got)
	}
}

// TestStatsText_Paragraphs 覆盖段落与行的定义：连续空行分隔的非空块各计一段，行按 \n 计。
func TestStatsText_Paragraphs(t *testing.T) {
	got, err := StatsText("a\n\n\nb\nc")
	if err != nil {
		t.Fatalf("StatsText error = %v", err)
	}
	if got.Lines != 5 {
		t.Errorf("lines = %d, want 5", got.Lines)
	}
	if got.Paragraphs != 2 {
		t.Errorf("paragraphs = %d, want 2", got.Paragraphs)
	}
}
