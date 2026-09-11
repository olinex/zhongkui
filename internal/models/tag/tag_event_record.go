// Package tag 提供标签事件相关的持久化模型，包括标签事件规则（TagEventRule）、
// 标签事件记录（TagEventRecord）及其命中明细（TagEventRecordHit）等。
// 本文件定义 TagEventRecord 与 TagEventRecordHit 模型，承载标签事件规则触发后产生的事件数据持久化。
package tag

import (
	// stdlib
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// 标签事件状态常量，定义事件的四态状态机：
//   - EventStatusPending:    事件已创建，处于待处理状态，尚未进入告警或抑制判定
//   - EventStatusAlerting:   事件处于告警状态，已触发告警通知
//   - EventStatusSuppressed: 事件被抑制（从未发送通知），同一事件键在抑制窗口内不重复告警
//   - EventStatusClosed:     事件已关闭，不再参与告警与抑制流程；已通知过的告警
//
// 状态流转方向为
//   - pending -> alerting
//   - pending -> suppressed（从未通知，无效数据，可被定期清理）
//   - alerting -> closed（手动关闭或自动关闭，已通知过的告警历史，予以保留）
//
// Event status constants define the four-state state machine of a tag event:
// pending / alerting / suppressed / closed.
const (
	EventStatusPending    = "pending"
	EventStatusAlerting   = "alerting"
	EventStatusSuppressed = "suppressed"
	EventStatusClosed     = "closed"
)

// TagEventRecord 表示一条标签事件记录，由标签事件规则（TagEventRule）触发产生。
// 记录触发时的规则快照（规则名称、MatchAll）、抑制指纹（DedupKey）与
// 事件状态（Status），并关联一组命中明细（TagEventRecordHit）。
// 对应表为 tag_event_records。
// TagEventRecord represents a tag event record triggered by a TagEventRule, with rule
// snapshot, suppression fingerprint, status and associated hit details.
type TagEventRecord struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this tag event belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段
	// NamespaceName the namespace name of the tag event, for multi-tenant isolation
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32)"`
	// RuleID 所属标签事件规则 ID
	// RuleID the ID of the owning tag event rule
	RuleID uint `gorm:"column:rule_id;type:INT;index"`
	// TagEventRule 所属标签事件规则，规则删除时事件级联删除
	// TagEventRule the owning tag event rule, cascaded deleted with the rule
	TagEventRule TagEventRule `gorm:"foreignKey:RuleID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// RuleName 规则名称快照
	// RuleName the snapshot of the rule name
	RuleName string `gorm:"column:rule_name;type:VARCHAR(255)"`
	// MatchAll 规则 MatchAll 快照
	// MatchAll the snapshot of the rule MatchAll flag
	MatchAll bool `gorm:"column:match_all;type:BOOLEAN"`
	// DedupKey 抑制指纹（SHA-256 十六进制）
	// DedupKey the suppression fingerprint (SHA-256 hex)
	DedupKey string `gorm:"column:dedup_key;type:CHAR(64)"`
	// Status 事件状态
	// Status the status of the event
	Status string `gorm:"column:status;type:VARCHAR(16)"`
	// DetectedAt 检测时间
	// DetectedAt the time the event was detected
	DetectedAt time.Time `gorm:"column:detected_at;type:TIMESTAMPTZ"`
	// Hits 该事件记录下的全部命中明细，删除记录时级联删除
	// Hits the hit details of the event record, cascaded deleted with the record
	Hits []TagEventRecordHit `gorm:"foreignKey:TagEventRecordID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// TagEventRecordHit 表示一条标签事件记录的一条命中明细，记录某条事件条件
// （TagEventCondition）在时间窗口内对特定维度值的命中统计。
// 对应表为 tag_event_record_hits。
// TagEventRecordHit represents a hit detail of a tag event record, recording the hit
// statistics of a condition against a specific dimension item within a time window.
type TagEventRecordHit struct {
	database.BaseModel
	// TagEventRecordID 所属事件记录 ID
	// TagEventRecordID the ID of the owning event record
	TagEventRecordID uint `gorm:"column:tag_event_record_id;type:BIGINT;index"`
	// TagEventRecord 所属标签事件记录，记录删除时命中明细级联删除
	// TagEventRecord the owning tag event record, cascaded deleted with the record
	TagEventRecord TagEventRecord `gorm:"foreignKey:TagEventRecordID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// ConditionID 命中条件 ID
	// ConditionID the ID of the matched condition
	ConditionID uint `gorm:"column:condition_id;type:INT"`
	// TagEventCondition 命中条件（仅 ORM 关联，不建数据库外键：规则更新会删除并重建条件，
	// 级联或限制删除都会破坏既有事件命中数据或阻断规则更新）
	// TagEventCondition the matched condition (ORM association only, no DB foreign key since
	// rule updates delete and recreate conditions)
	TagEventCondition TagEventCondition `gorm:"foreignKey:ConditionID;references:ID"`
	// ConditionName 条件名称快照
	// ConditionName the snapshot of the condition name
	ConditionName string `gorm:"column:condition_name;type:VARCHAR(255)"`
	// Model 目标模型类型键
	// Model the target model type key
	Model string `gorm:"column:model;type:VARCHAR(255)"`
	// Dimension 标签维度类型
	// Dimension the tag dimension type
	Dimension string `gorm:"column:dimension;type:VARCHAR(128)"`
	// Item 维度具体值
	// Item the concrete value of the dimension
	Item string `gorm:"column:item;type:VARCHAR(255)"`
	// Count 分组命中次数
	// Count the hit count of the grouped items
	Count uint64 `gorm:"column:count;type:BIGINT"`
	// Threshold 条件阈值
	// Threshold the threshold of the condition
	Threshold int `gorm:"column:threshold;type:INT"`
	// Window 条件时间窗口
	// Window the time window of the condition
	Window serializer.DurationNS `gorm:"column:window;type:BIGINT"`
	// IDs 命中分组对应的原始记录 ID 列表（如 http_transports.id），
	// 由检测查询去重聚合而来，JSONB 序列化存储。
	// IDs the record ID list of the hit group, aggregated and deduplicated by the
	// detection query, persisted as JSONB via the json serializer.
	IDs serializer.HexUUIDs `gorm:"type:JSONB;serializer:json" json:"ids"`
}
