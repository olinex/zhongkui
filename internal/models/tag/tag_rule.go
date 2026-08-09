package tag

import (
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// TagRule 表示标签识别规则。
// 每条规则绑定一个目标模型类型键（TargetType）与一个标签类型（TagType），
// 通过 Rego 源码声明在何种条件下为结构体实例打上标签。
// 规则归属命名空间（NamespaceName），不同租户的规则相互隔离。
type TagRule struct {
	database.BaseModel
	// Namespace 所属命名空间的关联模型，用于多租户隔离
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 所属命名空间名称，同一命名空间内的规则分组唯一
	NamespaceName string `gorm:"type:VARCHAR(32);uniqueIndex:uq_tag_rule_group"`
	// Name 规则名称，同一分组（namespace_name + target_type + tag_type）内唯一
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
