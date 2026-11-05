// 本文件覆盖技能固定参数的自由形状 JSON 解析/编码、周期时间窗口校验、偏置类型匹配与
// 参数合并优先级等纯函数，不涉及数据库与评审流程。
package ai

import (
	// 标准库
	"encoding/json"
	"testing"
	"time"

	// 第三方
	"gorm.io/datatypes"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// skillRaw 把 JSON 文本转换为 json.RawMessage，便于在测试中构造任意类型的固定参数字面量。
func skillRaw(text string) json.RawMessage {
	return json.RawMessage(text)
}

// TestParseSkillFixedFieldsEmpty 测试空值解析：nil、空串、纯空白与 JSON null 均返回非 nil 的空 map。
func TestParseSkillFixedFieldsEmpty(t *testing.T) {
	cases := []datatypes.JSON{nil, datatypes.JSON(""), datatypes.JSON("   "), datatypes.JSON("null"), datatypes.JSON("{}")}
	for _, raw := range cases {
		fields, err := ParseSkillFixedFields(raw)
		if err != nil {
			t.Errorf("ParseSkillFixedFields(%q) error = %v, want nil", string(raw), err)
			continue
		}
		if fields == nil || len(fields) != 0 {
			t.Errorf("ParseSkillFixedFields(%q) = %v, want an empty non-nil map", string(raw), fields)
		}
	}
}

// TestParseSkillFixedFieldsRejectsNonObject 测试非 JSON object 与非法 JSON 均返回错误。
func TestParseSkillFixedFieldsRejectsNonObject(t *testing.T) {
	cases := []datatypes.JSON{
		datatypes.JSON(`[]`),
		datatypes.JSON(`"text"`),
		datatypes.JSON(`123`),
		datatypes.JSON(`true`),
		datatypes.JSON(`{`),
	}
	for _, raw := range cases {
		if _, err := ParseSkillFixedFields(raw); err == nil {
			t.Errorf("ParseSkillFixedFields(%q) error = nil, want an error", string(raw))
		}
	}
}

// TestSkillFixedFieldsJSONEmpty 测试 nil 与空 map 均编码为空 JSON 对象。
func TestSkillFixedFieldsJSONEmpty(t *testing.T) {
	cases := []map[string]json.RawMessage{nil, {}}
	for _, fields := range cases {
		raw, err := SkillFixedFieldsJSON(fields)
		if err != nil {
			t.Errorf("SkillFixedFieldsJSON(%v) error = %v, want nil", fields, err)
			continue
		}
		if string(raw) != "{}" {
			t.Errorf("SkillFixedFieldsJSON(%v) = %s, want {}", fields, string(raw))
		}
	}
}

// TestSkillFixedFieldsRoundTripPreservesTypes 测试编码再解析后混合类型的原始 JSON 值保持不变：
// 数字、布尔、字符串、对象与数组不被字符串化。
func TestSkillFixedFieldsRoundTripPreservesTypes(t *testing.T) {
	fields := map[string]json.RawMessage{
		"threshold": skillRaw(`0.75`),
		"onlyHigh":  skillRaw(`true`),
		"region":    skillRaw(`"cn"`),
		"nested":    skillRaw(`{"min":1}`),
		"tags":      skillRaw(`["a","b"]`),
	}
	encoded, err := SkillFixedFieldsJSON(fields)
	if err != nil {
		t.Fatalf("SkillFixedFieldsJSON() error = %v, want nil", err)
	}
	decoded, err := ParseSkillFixedFields(encoded)
	if err != nil {
		t.Fatalf("ParseSkillFixedFields() error = %v, want nil", err)
	}
	if len(decoded) != len(fields) {
		t.Errorf("decoded field count = %d, want %d", len(decoded), len(fields))
	}
	for key, want := range fields {
		got, ok := decoded[key]
		if !ok {
			t.Errorf("decoded missing key %q", key)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("decoded[%q] = %s, want %s", key, string(got), string(want))
		}
	}
	for key, want := range map[string]string{"threshold": "number", "onlyHigh": "boolean", "region": "string", "nested": "object", "tags": "array"} {
		kind, err := validator.JSONValueKind(decoded[key])
		if err != nil || string(kind) != want {
			t.Errorf("JSONValueKind(decoded[%q]) = (%q, %v), want (%q, nil)", key, kind, err, want)
		}
	}
}

// TestParseSkillPeriodicWindow 测试周期时间窗口解析：合法时长为可读字符串或整数毫秒、偏移允许为负；
// 缺失、无法解析与 timeWindow <= 0 均拒绝。
func TestParseSkillPeriodicWindow(t *testing.T) {
	cases := []struct {
		name string
		raw  datatypes.JSON
		want SkillPeriodicWindow
		ok   bool
	}{
		{"negative offset", datatypes.JSON(`{"timeOffset":"-24h","timeWindow":"24h"}`), SkillPeriodicWindow{-24 * time.Hour, 24 * time.Hour}, true},
		{"positive offset", datatypes.JSON(`{"timeOffset":"168h","timeWindow":"720h"}`), SkillPeriodicWindow{168 * time.Hour, 720 * time.Hour}, true},
		{"zero offset", datatypes.JSON(`{"timeOffset":"0s","timeWindow":"1h"}`), SkillPeriodicWindow{0, time.Hour}, true},
		{"mixed fields", datatypes.JSON(`{"region":"cn","timeOffset":"-1h","timeWindow":"2h"}`), SkillPeriodicWindow{-time.Hour, 2 * time.Hour}, true},
		{"missing offset", datatypes.JSON(`{"timeWindow":"24h"}`), SkillPeriodicWindow{}, false},
		{"missing window", datatypes.JSON(`{"timeOffset":"0s"}`), SkillPeriodicWindow{}, false},
		{"offset integer milliseconds", datatypes.JSON(`{"timeOffset":24,"timeWindow":"24h"}`), SkillPeriodicWindow{24 * time.Millisecond, 24 * time.Hour}, true},
		{"window not duration", datatypes.JSON(`{"timeOffset":"0s","timeWindow":true}`), SkillPeriodicWindow{}, false},
		{"invalid offset", datatypes.JSON(`{"timeOffset":"1d","timeWindow":"24h"}`), SkillPeriodicWindow{}, false},
		{"invalid window", datatypes.JSON(`{"timeOffset":"0s","timeWindow":"abc"}`), SkillPeriodicWindow{}, false},
		{"zero window", datatypes.JSON(`{"timeOffset":"0s","timeWindow":"0s"}`), SkillPeriodicWindow{}, false},
		{"negative window", datatypes.JSON(`{"timeOffset":"0s","timeWindow":"-1h"}`), SkillPeriodicWindow{}, false},
		{"empty", nil, SkillPeriodicWindow{}, false},
	}
	for _, c := range cases {
		window, err := ParseSkillPeriodicWindow(c.raw)
		if c.ok {
			if err != nil || window != c.want {
				t.Errorf("%s: ParseSkillPeriodicWindow(%q) = (%+v, %v), want (%+v, nil)", c.name, string(c.raw), window, err, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: ParseSkillPeriodicWindow(%q) error = nil, want an error", c.name, string(c.raw))
		}
	}
}

// TestParsePeriodicWindowFromFields 测试从已解码 map 解析周期窗口与从 JSON 解析结果一致。
func TestParsePeriodicWindowFromFields(t *testing.T) {
	fields := map[string]json.RawMessage{
		SkillFixedFieldTimeOffset: skillRaw(`"-48h"`),
		SkillFixedFieldTimeWindow: skillRaw(`"2h"`),
	}
	window, err := ParsePeriodicWindowFromFields(fields)
	if err != nil || window.TimeOffset != -48*time.Hour || window.TimeWindow != 2*time.Hour {
		t.Errorf("ParsePeriodicWindowFromFields() = (%+v, %v), want offset -48h and window 2h", window, err)
	}
}

// TestValidateSkillFixedFields 测试固定参数校验：保留键（任意类型）与输入字段同名冲突被拒绝，
// 周期技能要求合法时长，非周期技能不要求时长键。
func TestValidateSkillFixedFields(t *testing.T) {
	validPeriodic := map[string]json.RawMessage{
		SkillFixedFieldTimeOffset: skillRaw(`"-24h"`),
		SkillFixedFieldTimeWindow: skillRaw(`"24h"`),
		"threshold":               skillRaw(`0.75`),
	}
	if err := ValidateSkillFixedFields(validPeriodic, true, []SkillInputField{{Name: SkillInputFieldNameNowTime, Type: SkillInputFieldTypeDate}}); err != nil {
		t.Errorf("ValidateSkillFixedFields(valid periodic) error = %v, want nil", err)
	}
	nonPeriodic := map[string]json.RawMessage{"region": skillRaw(`"cn"`), "onlyHigh": skillRaw(`true`)}
	if err := ValidateSkillFixedFields(nonPeriodic, false, []SkillInputField{{Name: "region", Type: "string"}}); err == nil {
		t.Error("ValidateSkillFixedFields(conflict) error = nil, want an error")
	}
	if err := ValidateSkillFixedFields(map[string]json.RawMessage{}, false, nil); err != nil {
		t.Errorf("ValidateSkillFixedFields(empty non-periodic) error = %v, want nil", err)
	}

	reservedValues := []string{`"text"`, `true`, `1`, `{}`, `[]`, `null`}
	for _, key := range []string{SkillInputFieldNameNowTime, "startTime", "endTime"} {
		for _, value := range reservedValues {
			if err := ValidateSkillFixedFields(map[string]json.RawMessage{key: skillRaw(value)}, false, nil); err == nil {
				t.Errorf("ValidateSkillFixedFields(reserved %q, value %s) error = nil, want an error", key, value)
			}
		}
	}

	for name, fields := range map[string]map[string]json.RawMessage{
		"missing durations": {"threshold": skillRaw(`0.5`)},
		"invalid window":    {SkillFixedFieldTimeOffset: skillRaw(`"0s"`), SkillFixedFieldTimeWindow: skillRaw(`"0s"`)},
	} {
		if err := ValidateSkillFixedFields(fields, true, nil); err == nil {
			t.Errorf("ValidateSkillFixedFields(%s, periodic) error = nil, want an error", name)
		}
	}
}

// TestInputFieldAcceptsJSON 测试输入字段类型与 JSON 值类型的匹配规则。
func TestInputFieldAcceptsJSON(t *testing.T) {
	cases := []struct {
		fieldType SkillInputFieldType
		raw       string
		want      bool
	}{
		{"nowTime", `"2026-11-02T00:00:00Z"`, false},
		{"number", `0.75`, true},
		{"number", `"0.75"`, false},
		{"boolean", `true`, true},
		{"boolean", `1`, false},
		{"string", `"cn"`, true},
		{"string", `1`, false},
		{"enum", `"high"`, true},
		{"enum", `false`, false},
		{"date", `"2026-11-02"`, true},
		{"date", `true`, false},
		{"unknown", `"value"`, false},
		{"number", `0.7.5`, false},
	}
	for _, c := range cases {
		if got := InputFieldAcceptsJSON(c.fieldType, json.RawMessage(c.raw)); got != c.want {
			t.Errorf("InputFieldAcceptsJSON(%q, %s) = %v, want %v", c.fieldType, c.raw, got, c.want)
		}
	}
}

// TestMergeParameters 测试合并优先级：后者覆盖前者，返回新 map 且不修改入参。
func TestMergeParameters(t *testing.T) {
	fixed := map[string]json.RawMessage{
		"threshold": skillRaw(`0.5`),
		"region":    skillRaw(`"cn"`),
		"onlyHigh":  skillRaw(`false`),
	}
	plan := map[string]json.RawMessage{"threshold": skillRaw(`0.75`)}
	runtime := map[string]json.RawMessage{"nowTime": skillRaw(`"2026-11-02T00:00:00Z"`)}

	merged := MergeParameters(fixed, plan, runtime)
	if string(merged["threshold"]) != `0.75` {
		t.Errorf("merged threshold = %s, want 0.75", string(merged["threshold"]))
	}
	if string(merged["region"]) != `"cn"` || string(merged["onlyHigh"]) != `false` {
		t.Errorf("merged fixed values = region %s, onlyHigh %s, want the original values", string(merged["region"]), string(merged["onlyHigh"]))
	}
	if string(merged["nowTime"]) != `"2026-11-02T00:00:00Z"` {
		t.Errorf("merged nowTime = %s, want the runtime value", string(merged["nowTime"]))
	}
	if len(fixed) != 3 || string(fixed["threshold"]) != `0.5` {
		t.Errorf("MergeParameters modified its fixed source: %v", fixed)
	}
	merged["region"] = skillRaw(`"us"`)
	if string(fixed["region"]) != `"cn"` {
		t.Errorf("MergeParameters result shares the source map: fixed region = %s", string(fixed["region"]))
	}

	empty := MergeParameters()
	if empty == nil || len(empty) != 0 {
		t.Errorf("MergeParameters() = %v, want an empty non-nil map", empty)
	}
}
