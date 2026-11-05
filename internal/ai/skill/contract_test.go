package skill

import (
	// 标准库
	"testing"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestValidateInputFields 测试输入字段声明校验：合法类型通过，空名称、已移除的 timeWindow
// 与未知类型被拒绝。
func TestValidateInputFields(t *testing.T) {
	valid := []modelsai.SkillInputField{
		{Name: modelsai.SkillInputFieldNameNowTime, Type: modelsai.SkillInputFieldTypeDate},
		{Name: "threshold", Type: "number"},
		{Name: "region", Type: "string"},
	}
	if err := ValidateInputFields(valid); err != nil {
		t.Errorf("ValidateInputFields(valid) error = %v, want nil", err)
	}
	for name, fields := range map[string][]modelsai.SkillInputField{
		"empty input name":   {{Name: "", Type: "string"}},
		"removed timeWindow": {{Name: "period", Type: "timeWindow"}},
		"unknown input type": {{Name: "period", Type: "unknown"}},
	} {
		if err := ValidateInputFields(fields); err == nil {
			t.Errorf("ValidateInputFields(%s) error = nil, want an error", name)
		}
	}
}

// TestValidateInputFieldsAcceptsDeclaredTypes 遍历模型层声明的全部输入字段类型，确保 oneof
// 校验集合与 SkillInputFieldType 取值空间不漂移：新增类型却漏改 oneof 时此测试失败。
func TestValidateInputFieldsAcceptsDeclaredTypes(t *testing.T) {
	for _, typ := range modelsai.AllSkillInputFieldTypes() {
		fields := []modelsai.SkillInputField{{Name: "field", Type: typ}}
		if err := ValidateInputFields(fields); err != nil {
			t.Errorf("ValidateInputFields(type %q) error = %v, want nil", typ, err)
		}
	}
}
