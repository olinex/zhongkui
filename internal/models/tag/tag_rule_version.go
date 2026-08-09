package tag

import (
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// MaxTagRuleVersions 每条规则最多保留的历史版本数。
// 超出上限时自动裁剪最旧版本，保留最新版本。
const MaxTagRuleVersions = 30

// TagRuleVersion 表示标签识别规则的历史版本快照。
// 规则内容变更（更新/回滚）前，将当前内容以新版本号快照入库；
// 版本号按规则内 1..N 递增，同一规则最多保留 MaxTagRuleVersions 条历史版本。
// 快照同时记录所属命名空间，保证跨命名空间访问时数据可溯源与隔离。
type TagRuleVersion struct {
	database.BaseModel
	// Namespace 所属命名空间的关联模型，用于多租户隔离
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 快照时所属命名空间名称
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// RuleID 所属规则的 ID，删除规则时级联删除其历史版本
	RuleID uint `gorm:"column:rule_id;type:INT;index;uniqueIndex:uq_tag_rule_version;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Version 规则内的递增版本号，从 1 开始
	Version uint `gorm:"column:version;type:INT;uniqueIndex:uq_tag_rule_version"`
	// Name 快照时的规则名称
	Name string `gorm:"type:VARCHAR(128)"`
	// Description 快照时的规则描述
	Description string `gorm:"type:VARCHAR(512)"`
	// TargetType 快照时的目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	TargetType string `gorm:"type:VARCHAR(255)"`
	// TagType 快照时的标签类型（维度，如 risk / asset / behavior）
	TagType string `gorm:"type:VARCHAR(64)"`
	// Rego 快照时的 Rego 规则源码
	Rego string `gorm:"type:TEXT"`
	// Enabled 快照时的启用状态
	Enabled bool `gorm:"type:BOOLEAN"`
}
