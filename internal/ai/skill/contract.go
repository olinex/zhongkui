package skill

import (
	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// inputFieldRule 是技能输入字段的校验视图：字段名必填、类型限定在约定的取值集合。
// 校验规则只在此声明，模型层 SkillInputField 仅承载数据，不再携带 binding 标签。
type inputFieldRule struct {
	// Name 字段名称，作为报告计划 inputValues 中的键
	Name string `json:"name" binding:"notBlank"`
	// Type 字段类型，取值限制由 binding 的 oneof 规则声明
	Type modelsai.SkillInputFieldType `json:"type" binding:"required,oneof=date number string enum boolean"`
	// Required 该字段是否必填
	Required bool `json:"required"`
	// Description 字段用途说明，可为空
	Description string `json:"description"`
}

// inputFieldContract 是输入字段列表的结构化校验包装：dive 让每个字段按 inputFieldRule 的
// binding 规则校验。
type inputFieldContract struct {
	// Fields 待校验的技能输入字段
	Fields []inputFieldRule `binding:"dive"`
}

// ValidateInputFields 校验技能输入字段声明：字段名非空，类型只能是 date / number / string /
// enum / boolean。供技能创建 / 更新与周期性报告运行期命令边界统一调用。
func ValidateInputFields(fields []modelsai.SkillInputField) error {
	items := make([]inputFieldRule, 0, len(fields))
	for _, field := range fields {
		items = append(items, inputFieldRule{Name: field.Name, Type: field.Type})
	}
	if err := validator.Check(&inputFieldContract{Fields: items}); err != nil {
		return errors.Newf("agent skill input fields are invalid: %v", err)
	}
	return nil
}
