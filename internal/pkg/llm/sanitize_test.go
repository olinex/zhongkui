package llm

import (
	// stdlib
	"encoding/json"
	"strings"
	"testing"
)

// sanitizeBudgetParams 构造覆盖 sanitize.maxBytes 的参数解析器，便于测试按字节预算清洗
// （sanitize.maxItems 置为 0，即不限制条目数）。
// sanitizeBudgetParams builds a parameter resolver overriding sanitize.maxBytes so the
// tests can exercise byte-budget sanitization (sanitize.maxItems is set to 0, i.e. no
// item-count limit).
func sanitizeBudgetParams(maxBytes string) *Params {
	return NewParams(map[string]string{
		"sanitize.maxItems": "0",
		"sanitize.maxBytes": maxBytes,
	})
}

// TestSanitizeBusinessData_Recursive 测试嵌套结构递归清洗：内层空条目/空元素被剔除，
// 清洗后变空的嵌套结构被连带剔除，非空嵌套结构保留且不影响顶层结果。
// TestSanitizeBusinessData_Recursive verifies recursive cleaning of nested structures:
// inner empty entries/elements are dropped, nested structures that turn empty are
// removed along the way, and non-empty nested structures survive with no impact on the
// top-level result.
func TestSanitizeBusinessData_Recursive(t *testing.T) {
	data := BusinessData{
		"nestedMap": map[string]any{
			"keep":    "v",
			"empty":   "",
			"dropped": nil,
			"innerMap": map[string]any{
				"x": 1,
				"y": "",
			},
		},
		"nestedSlice": []any{
			"a",
			"",
			map[string]any{"ok": true, "no": ""},
			0,
		},
		"becomesEmptyMap":   map[string]any{"a": "", "b": nil},
		"becomesEmptySlice": []any{"", nil},
		"kept":              "top",
	}
	out, err := SanitizeBusinessData(data, sanitizeBudgetParams("0"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	nested, ok := out["nestedMap"].(map[string]any)
	if !ok {
		t.Fatalf("nestedMap = %T, want map[string]any", out["nestedMap"])
	}
	if _, ok := nested["empty"]; ok {
		t.Errorf("nestedMap kept inner empty entry %q", "empty")
	}
	if _, ok := nested["dropped"]; ok {
		t.Errorf("nestedMap kept inner nil entry %q", "dropped")
	}
	inner, ok := nested["innerMap"].(map[string]any)
	if !ok {
		t.Fatalf("innerMap = %T, want map[string]any", nested["innerMap"])
	}
	if _, ok := inner["y"]; ok {
		t.Errorf("innerMap kept empty entry %q", "y")
	}
	if inner["x"] != 1 {
		t.Errorf("innerMap[x] = %v, want 1", inner["x"])
	}
	sl, ok := out["nestedSlice"].([]any)
	if !ok {
		t.Fatalf("nestedSlice = %T, want []any", out["nestedSlice"])
	}
	if len(sl) != 3 {
		t.Fatalf("nestedSlice len = %d, want 3 (empty string dropped)", len(sl))
	}
	if sl[0] != "a" {
		t.Errorf("nestedSlice[0] = %v, want a", sl[0])
	}
	if _, ok := out["becomesEmptyMap"]; ok {
		t.Errorf("out kept %q which becomes empty", "becomesEmptyMap")
	}
	if _, ok := out["becomesEmptySlice"]; ok {
		t.Errorf("out kept %q which becomes empty", "becomesEmptySlice")
	}
	if out["kept"] != "top" {
		t.Errorf("kept = %v, want top", out["kept"])
	}
	// 入参不被修改
	if _, ok := data["becomesEmptyMap"]; !ok {
		t.Errorf("SanitizeBusinessData() mutated the input")
	}
}

// TestSanitizeBusinessData_CleansEmptyValues 测试空值条目（nil/空字符串/空切片/空 map）
// 被剔除，数值 0 与布尔 false 保留；sanitize.maxBytes 非正数时仅执行清洗；入参不被修改。
// TestSanitizeBusinessData_CleansEmptyValues verifies empty entries (nil/empty string/
// empty slice/empty map) are dropped while numeric zero and boolean false are kept; a
// non-positive sanitize.maxBytes only sanitizes; the input is never mutated.
func TestSanitizeBusinessData_CleansEmptyValues(t *testing.T) {
	data := BusinessData{
		"nil":   nil,
		"empty": "",
		"list":  []string{},
		"map":   map[string]any{},
		"zero":  0,
		"false": false,
		"keep":  "v",
	}
	out, err := SanitizeBusinessData(data, sanitizeBudgetParams("0"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	for _, k := range []string{"nil", "empty", "list", "map"} {
		if _, ok := out[k]; ok {
			t.Errorf("SanitizeBusinessData() kept empty key %q", k)
		}
	}
	for _, k := range []string{"zero", "false", "keep"} {
		if _, ok := out[k]; !ok {
			t.Errorf("SanitizeBusinessData() dropped valid key %q", k)
		}
	}
	if _, ok := data["nil"]; !ok {
		t.Errorf("SanitizeBusinessData() mutated the input map")
	}
}

// TestSanitizeBusinessData_WithinBudgetUnchanged 测试未超预算时数据原样保留（仅清洗）。
// TestSanitizeBusinessData_WithinBudgetUnchanged verifies data within the budget is kept
// as-is (sanitization only).
func TestSanitizeBusinessData_WithinBudgetUnchanged(t *testing.T) {
	data := BusinessData{"a": "short", "b": 42, "drop": ""}
	out, err := SanitizeBusinessData(data, sanitizeBudgetParams("4096"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	if out["a"] != "short" || out["b"] != 42 {
		t.Errorf("SanitizeBusinessData() = %v, want a=short b=42", out)
	}
	if _, ok := out["drop"]; ok {
		t.Errorf("SanitizeBusinessData() kept the empty key %q", "drop")
	}
}

// TestSanitizeBusinessData_DropsLargestUntilFits 测试超预算时按字节数从大到小丢弃条目，
// 短条目优先保留，结果序列化后不超过预算。
// TestSanitizeBusinessData_DropsLargestUntilFits verifies over-budget data drops the
// largest entries first, keeps the short ones, and the serialized result fits the budget.
func TestSanitizeBusinessData_DropsLargestUntilFits(t *testing.T) {
	big := strings.Repeat("x", 1000)
	data := BusinessData{"big1": big, "big2": big, "small": "s"}
	out, err := SanitizeBusinessData(data, sanitizeBudgetParams("1100"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal(out) error = %v", err)
	}
	if len(b) > 1100 {
		t.Errorf("sanitized size = %d, want <= 1100", len(b))
	}
	if _, ok := out["small"]; !ok {
		t.Errorf("SanitizeBusinessData() dropped the short key %q that fits", "small")
	}
	keptBig := 0
	if _, ok := out["big1"]; ok {
		keptBig++
	}
	if _, ok := out["big2"]; ok {
		keptBig++
	}
	if keptBig != 1 {
		t.Errorf("kept big entries = %d, want exactly 1", keptBig)
	}
	if len(data) != 3 {
		t.Errorf("SanitizeBusinessData() mutated the input map (len = %d, want 3)", len(data))
	}
}

// TestSanitizeBusinessData_Deterministic 测试相同输入两次清洗结果一致。
// TestSanitizeBusinessData_Deterministic verifies the same input sanitized twice yields
// identical results.
func TestSanitizeBusinessData_Deterministic(t *testing.T) {
	data := BusinessData{
		"a": strings.Repeat("y", 300),
		"b": strings.Repeat("z", 300),
		"c": "tiny",
	}
	first, err := SanitizeBusinessData(data, sanitizeBudgetParams("350"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	second, err := SanitizeBusinessData(data, sanitizeBudgetParams("350"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("results differ in length: %d vs %d", len(first), len(second))
	}
	for k := range first {
		if _, ok := second[k]; !ok {
			t.Errorf("second result missing key %q kept by the first", k)
		}
	}
}

// TestSanitizeBusinessData_TinyBudget 测试预算小于等于空对象大小时返回空对象且仍为合法 JSON。
// TestSanitizeBusinessData_TinyBudget verifies a budget no larger than the empty object
// yields an empty object that is still valid JSON.
func TestSanitizeBusinessData_TinyBudget(t *testing.T) {
	data := BusinessData{"a": strings.Repeat("x", 100)}
	out, err := SanitizeBusinessData(data, sanitizeBudgetParams("2"))
	if err != nil {
		t.Fatalf("SanitizeBusinessData() error = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("SanitizeBusinessData() = %v, want empty map", out)
	}
	if b, err := json.Marshal(out); err != nil || string(b) != "{}" {
		t.Errorf("json.Marshal(out) = %q, %v; want %q", b, err, "{}")
	}
}
