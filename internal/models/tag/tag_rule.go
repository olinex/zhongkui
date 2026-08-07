package tag

import (
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// TagRule 表示标签识别规则。
// 每条规则绑定一个目标模型类型键（TargetType）与一个标签类型（TagType），
// 通过 Rego 源码声明在何种条件下为结构体实例打上标签。
type TagRule struct {
	database.BaseModel
	// Name 规则名称，同一分组（target_type + tag_type）内唯一
	Name string `gorm:"type:VARCHAR(128);uniqueIndex:uq_tag_rule_group"`
	// Description 规则描述，说明规则意图
	Description string `gorm:"type:VARCHAR(512)"`
	// TargetType 目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	TargetType string `gorm:"type:VARCHAR(255);uniqueIndex:uq_tag_rule_group"`
	// TagType 标签类型（维度，如 risk / asset / behavior）
	TagType string `gorm:"type:VARCHAR(64);uniqueIndex:uq_tag_rule_group"`
	// Rego Rego 规则源码，遵循 package zki + tags contains 契约
	Rego string `gorm:"type:TEXT"`
	// Enabled 是否启用，引擎只读取启用规则
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
}
