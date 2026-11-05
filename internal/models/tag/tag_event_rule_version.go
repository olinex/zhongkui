package tag

import (
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// MaxTagEventRuleVersions 每条标签事件规则最多保留的历史版本数。
// 超出上限时自动裁剪最旧版本，保留最新版本。
const MaxTagEventRuleVersions = 30

// TagEventRuleVersion 表示标签事件规则的历史版本快照。
// 规则内容变更（更新/回滚）前，将当前内容以新版本号快照入库；
// 版本号按规则内 1..N 递增，同一规则最多保留 MaxTagEventRuleVersions 条历史版本。
// 除规则头字段外，全部条件业务字段以 JSON 文本（Conditions）快照，
// 供回滚时重建规则及其条件集合；快照同时记录所属命名空间，保证数据可溯源与隔离。
type TagEventRuleVersion struct {
	database.BaseModel
	// Namespace 所属命名空间的关联模型，用于多租户隔离
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 快照时所属命名空间名称
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// RuleID 所属标签事件规则的 ID，删除规则时级联删除其历史版本
	RuleID uint `gorm:"column:rule_id;type:INT;index;uniqueIndex:uq_tag_event_rule_versions_rule_id_version;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Version 规则内的递增版本号，从 1 开始
	Version uint `gorm:"column:version;type:INT;uniqueIndex:uq_tag_event_rule_versions_rule_id_version"`
	// Name 快照时的规则名称
	Name string `gorm:"type:VARCHAR(255)"`
	// MatchAll 快照时是否要求规则下所有条件均命中才触发事件；为 false 时任一条件命中即触发
	MatchAll bool `gorm:"column:match_all;type:BOOLEAN"`
	// SuppressWindow 快照时的规则级抑制窗口（纳秒），0 表示不抑制
	// 显式零值必须原样落库，故模型不设 default tag
	SuppressWindow serializer.DurationNS `gorm:"column:suppress_window;type:BIGINT"`
	// Description 快照时的规则描述
	Description string `gorm:"type:VARCHAR(256)"`
	// Conditions 快照时全部条件业务字段（model/dimension/item/name/window/threshold/expression/enabled/description）的 JSON 文本快照
	Conditions string `gorm:"type:TEXT"`
}
