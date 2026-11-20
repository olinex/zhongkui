// Package core 提供平台核心领域的持久化模型，包括命名空间（Namespace）、
// 用户（User）、采集器（Capturer）、分组（Group）、RBAC 策略与访问密钥等。
// 本文件定义通知渠道（NotificationChannel）、通知目标（NotificationTarget）、
// 通知组（NotificationGroup）、组目标关联（NotificationGroupTarget）、通知记录
// （NotificationRecord）与通知模板（NotificationTemplate）模型。通知基础设施是
// 命名空间级、与标签解耦的通用能力，
// 承载告警通知渠道配置、目标分组与通知发送记录（状态、消息 ID、错误信息等）
// 的持久化。
package core

import (
	// 标准库
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 通知记录状态常量，定义通知发送的三态状态机：
//   - RecordStatusPending: 通知记录已创建，等待渠道发送
//   - RecordStatusSuccess: 通知已由渠道成功发送
//   - RecordStatusFailed:  通知发送失败，错误信息记录在 NotificationRecord.Error
const (
	RecordStatusPending = "pending"
	RecordStatusSuccess = "success"
	RecordStatusFailed  = "failed"

	// EventTypeTagEventRecord 标签事件记录类型，用于 NotificationRecord.EventType 标记事件 ID 指向的事件类型
	EventTypeTagEventRecord = "tag_event_record"
)

// 通知模板来源常量，标记 NotificationTemplate 行的来源：内置默认模板或用户覆盖模板。
const (
	// TemplateSourceDefault 内置默认模板，由事件模板注册表（internal/notify/template.go）种子生成
	TemplateSourceDefault = "default"
	// TemplateSourceOverride 用户覆盖模板，覆盖同一（event, channel_type, language）组合的默认模板
	TemplateSourceOverride = "override"
)

// NotificationChannel 表示一个告警通知渠道，如邮件（email）、Webhook 等。
// 渠道类型键（Type）与渠道基座定义在 internal/pkg/message，不在本文件定义，
// 以避免 message 包与 models/core 包之间的 import cycle（message 包依赖内部 pkg，不依赖 models）。
// 对应表为 notification_channels。
type NotificationChannel struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内渠道名称唯一
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_channels_namespace_name;index:idx_notification_channels_namespace"`
	// Name 渠道名称，与 NamespaceName 组合唯一
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_channels_namespace_name"`
	// Type 渠道类型键（如 email、webhook），取值定义在 internal/pkg/message
	Type string `gorm:"column:type;type:VARCHAR(32)"`
	// Enabled 是否启用，禁用的渠道不参与通知发送
	Enabled bool `gorm:"column:enabled;type:BOOLEAN;default:true"`
	// Config 渠道配置（JSON 文本），可能含传输凭据（如 SMTP 账号密码），
	// 以加密形式持久化（crypto serializer 密文入库，BYTEA 列）；读取明文经
	// ConfigData() 获取，详情接口仅输出密文，查看明文需经明文权限接口
	Config *serializer.Crypto `json:"config" gorm:"column:config;type:BYTEA;serializer:crypto" crypto:"mode=\"aes-gcm\",offset=21001,length=32"`
	// Description 渠道描述，说明渠道用途
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// ConfigData 返回渠道配置的明文字节，nil 安全（Config 为空或 nil 时返回 nil）。
func (c *NotificationChannel) ConfigData() []byte {
	if c == nil || c.Config == nil {
		return nil
	}
	return c.Config.Value()
}

// NotificationTarget 表示一个通知目标，是通知的实际接收者（如某个邮箱地址、
// Webhook URL、企业微信群机器人等），归属于某个通知渠道（ChannelID）。
// Type 为渠道类型键快照，取值定义在 internal/pkg/message。
// 对应表为 notification_targets。
type NotificationTarget struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内目标名称唯一
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_targets_namespace_name;index:idx_notification_targets_namespace"`
	// Name 目标名称，与 NamespaceName 组合唯一
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_targets_namespace_name"`
	// Channel 所属的通知渠道，渠道删除时目标级联删除
	Channel NotificationChannel `gorm:"foreignKey:ChannelID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// ChannelID 目标所属的通知渠道 ID
	ChannelID uint `gorm:"column:channel_id;type:INT;index:idx_notification_targets_channel_id"`
	// Type 渠道类型键快照（如 email、webhook），取值定义在 internal/pkg/message
	Type string `gorm:"column:type;type:VARCHAR(32)"`
	// Enabled 是否启用，禁用的目标不参与通知发送
	Enabled bool `gorm:"column:enabled;type:BOOLEAN;default:true"`
	// Config 目标配置（JSON 文本），如收件人邮箱、Webhook URL，结构随渠道类型不同而不同；
	// 以加密形式持久化（crypto serializer 密文入库，BYTEA 列），读取明文经 ConfigData() 获取
	Config *serializer.Crypto `json:"config" gorm:"column:config;type:BYTEA;serializer:crypto" crypto:"mode=\"aes-gcm\",offset=21069,length=32"`
	// Language 目标偏好的消息语言（如 en、zh-Hans），空字符串表示使用平台默认语言；
	// 通知消息按该语言经 locales 模板渲染（i18n）
	Language string `gorm:"column:language;type:VARCHAR(32)"`
	// Description 目标描述，说明目标用途
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// ConfigData 返回目标配置的明文字节，nil 安全（Config 为空或 nil 时返回 nil）。
func (t *NotificationTarget) ConfigData() []byte {
	if t == nil || t.Config == nil {
		return nil
	}
	return t.Config.Value()
}

// NotificationGroup 表示一个通知目标组，将多个通知目标聚合为一个组，
// 事件规则（TagEventRule）可绑定通知组以批量通知组内目标。
// 每个命名空间至多一个默认组（IsDefault），由迁移 SQL 中的部分唯一索引
// uq_notification_groups_namespace_default 保证；未绑定通知组的规则使用默认组。
// 对应表为 notification_groups。
type NotificationGroup struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内组名称唯一
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_groups_namespace_name"`
	// Name 组名称，与 NamespaceName 组合唯一
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_groups_namespace_name"`
	// IsDefault 是否为命名空间默认组，每命名空间至多一个默认组
	IsDefault bool `gorm:"column:is_default;type:BOOLEAN;default:false"`
	// Description 组描述，说明组用途
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// NotificationGroupTarget 表示通知组与通知目标的多对多关联记录。
// 同一目标可加入多个组，同一组内目标不重复（group_id + target_id 组合唯一）。
// 对应表为 notification_group_targets。
type NotificationGroupTarget struct {
	database.BaseModel

	// GroupID 通知组 ID
	GroupID uint `gorm:"column:group_id;type:INT;uniqueIndex:uq_notification_group_targets_group_target"`
	// TargetID 通知目标 ID，与 GroupID 组合唯一
	TargetID uint `gorm:"column:target_id;type:INT;uniqueIndex:uq_notification_group_targets_group_target;index:idx_notification_group_targets_target_id"`
}

// NotificationRecord 表示一条通知发送记录，记录某事件（EventID）对某目标（TargetID）
// 的发送尝试与结果，包括所属通知组（GroupID）、发送渠道（ChannelID）与渠道类型快照
// （ChannelType）、发送状态（Status）、渠道返回的消息 ID（MessageID）、
// 通知负载快照（Payload）与实际发送成功时间（NotifiedAt）。
// 同一事件类型下的同一事件对同一目标至多一条记录（event_type + event_id + target_id 组合唯一）。
// 对应表为 notification_records。
type NotificationRecord struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);index:idx_notification_records_namespace_created_at"`
	// EventType 事件类型，标识 EventID 指向的事件类型，当前仅标签事件 tag_event，
	// 与 EventID、TargetID 组合唯一（event_id 在不同事件类型间不全局唯一）
	EventType string `gorm:"column:event_type;type:VARCHAR(32);uniqueIndex:uq_notification_records_event_target"`
	// EventID 触发通知的标签事件 ID
	EventID uint `gorm:"column:event_id;type:BIGINT;uniqueIndex:uq_notification_records_event_target"`
	// GroupID 发送时所属的通知组 ID，0 表示未绑定组
	GroupID uint `gorm:"column:group_id;type:INT"`
	// TargetID 通知目标 ID，与 EventID 组合唯一，保证同一事件对同一目标至多一条记录
	TargetID uint `gorm:"column:target_id;type:INT;uniqueIndex:uq_notification_records_event_target"`
	// ChannelID 发送通知的渠道 ID
	ChannelID uint `gorm:"column:channel_id;type:INT;index:idx_notification_records_channel_id"`
	// ChannelType 渠道类型键快照，渠道删除或类型变更后仍可追溯
	ChannelType string `gorm:"column:channel_type;type:VARCHAR(32)"`
	// Status 通知发送状态：pending/success/failed
	Status string `gorm:"column:status;type:VARCHAR(16)"`
	// Error 发送失败时的错误信息，成功时为空字符串
	Error string `gorm:"column:error;type:TEXT"`
	// MessageID 渠道返回的消息 ID，可用于后续查询发送状态
	MessageID string `gorm:"column:message_id;type:VARCHAR(128)"`
	// Payload 通知负载（JSON 文本）快照
	Payload string `gorm:"column:payload;type:TEXT"`
	// NotifiedAt 通知实际发送成功的时间，nil 表示尚未发送成功
	NotifiedAt *time.Time `gorm:"column:notified_at;type:TIMESTAMPTZ"`
}

// NotificationTemplate 表示一条命名空间级的通知消息模板，描述某事件（Event）对某
// 渠道类型（ChannelType）在某种语言（Language）下的标题（Title）与正文（Content）；
// Title 与 Content 为 Go text/template 模板，按事件负载渲染。同一命名空间下
// （event + channel_type + language）组合唯一：该组合唯一确定一条可用的消息模板，
// ChannelType 与 Language 支持 "*" 通配。Source 标记模板行是内置默认模板（default）
// 还是用户覆盖模板（override）；内置默认模板由事件模板注册表
// （internal/notify/template.go）种子生成。对应表为 notification_templates。
type NotificationTemplate struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段；与 Event、ChannelType、Language 组合唯一
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language;index:idx_notification_templates_namespace"`
	// Event 事件类型键（如 tag_event_record），与 ChannelType、Language 组合唯一
	Event string `gorm:"column:event;type:VARCHAR(64);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 事件类型键，如 tag_event_record
	// ChannelType 渠道类型键，允许 * 通配（对所有渠道生效）
	ChannelType string `gorm:"column:channel_type;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 渠道类型键，允许 * 通配
	// Language 语言，允许 * 通配（对所有语言生效）
	Language string `gorm:"column:language;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 语言，允许 * 通配
	// Title 标题模板（Go text/template），按事件负载渲染
	Title string `gorm:"column:title;type:TEXT"` // Go text/template 标题模板
	// Content 正文模板（Go text/template），按事件负载渲染
	Content string `gorm:"column:content;type:TEXT"` // Go text/template 正文模板
	// Source 模板来源：default | override，标记内置默认模板或用户覆盖模板
	Source string `gorm:"column:source;type:VARCHAR(16);default:default"` // default | override
	// Description 模板描述（可选）
	Description string `gorm:"column:description;type:VARCHAR(256)"` // 可选描述
}
