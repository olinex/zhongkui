// Package core 提供平台核心领域的持久化模型，包括命名空间（Namespace）、
// 用户（User）、采集器（Capturer）、分组（Group）、RBAC 策略与访问密钥等。
// 本文件定义通知渠道（NotificationChannel）、通知目标（NotificationTarget）、
// 通知组（NotificationGroup）、组目标关联（NotificationGroupTarget）、通知记录
// （NotificationRecord）与通知模板（NotificationTemplate）模型。通知基础设施是
// 命名空间级、与标签解耦的通用能力，
// 承载告警通知渠道配置、目标分组与通知发送记录（状态、消息 ID、错误信息等）
// 的持久化。
//
// Package core provides the persistence models of the platform core domain,
// including namespace, user, capturer, group, RBAC policy and access key. This file
// defines the notification channel, target, group, group-target association,
// record and template models. The notification infrastructure is namespace-scoped
// and decoupled
// from tags, persisting the alert channel configs, target groups and delivery
// records (status, message ID, error, etc.).
package core

import (
	// stdlib
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// 通知记录状态常量，定义通知发送的三态状态机：
//   - RecordStatusPending: 通知记录已创建，等待渠道发送
//   - RecordStatusSuccess: 通知已由渠道成功发送
//   - RecordStatusFailed:  通知发送失败，错误信息记录在 NotificationRecord.Error
//
// Notification record status constants define the three-state status of a
// notification record: pending / success / failed.
const (
	RecordStatusPending = "pending"
	RecordStatusSuccess = "success"
	RecordStatusFailed  = "failed"

	// EventTypeTagEventRecord 标签事件记录类型，用于 NotificationRecord.EventType 标记事件 ID 指向的事件类型
	// EventTypeTagEventRecord the tag event record type, used to mark what event type
	// the NotificationRecord's EventID refers to
	EventTypeTagEventRecord = "tag_event_record"
)

// 通知模板来源常量，标记 NotificationTemplate 行的来源：内置默认模板或用户覆盖模板。
// Notification template source constants mark whether a NotificationTemplate row is
// a built-in default template or a user-customized override template.
const (
	// TemplateSourceDefault 内置默认模板，由事件模板注册表（internal/notify/template.go）种子生成
	// TemplateSourceDefault the built-in default template, seeded from the event template
	// registry (internal/notify/template.go)
	TemplateSourceDefault = "default"
	// TemplateSourceOverride 用户覆盖模板，覆盖同一（event, channel_type, language）组合的默认模板
	// TemplateSourceOverride the user-customized override template, overriding the default
	// template of the same (event, channel_type, language) combination
	TemplateSourceOverride = "override"
)

// NotificationChannel 表示一个告警通知渠道，如邮件（email）、Webhook 等。
// 渠道类型键（Type）定义在 internal/notify/channel.go，不在本文件定义，
// 以避免 notify 包与 models/core 包之间的 import cycle（notify 包依赖 models/core）。
// 对应表为 notification_channels。
// NotificationChannel represents an alert notification channel such as email or webhook.
// The channel type keys are defined in internal/notify/channel.go to avoid an import
// cycle between the notify package and the models/core package.
type NotificationChannel struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this channel belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内渠道名称唯一
	// NamespaceName the namespace name, for multi-tenant isolation; channel names
	// are unique within the same namespace
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_channels_namespace_name;index:idx_notification_channels_namespace"`
	// Name 渠道名称，与 NamespaceName 组合唯一
	// Name the name of the channel, unique together with NamespaceName
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_channels_namespace_name"`
	// Type 渠道类型键（如 email、webhook），取值定义在 internal/notify/channel.go
	// Type the channel type key (e.g. email, webhook), defined in internal/notify/channel.go
	Type string `gorm:"column:type;type:VARCHAR(32)"`
	// Enabled 是否启用，禁用的渠道不参与通知发送
	// Enabled whether the channel is enabled, disabled channels are skipped when
	// sending notifications
	Enabled bool `gorm:"column:enabled;type:BOOLEAN;default:true"`
	// Config 渠道配置（JSON 文本），可能含传输凭据（如 SMTP 账号密码），
	// 以加密形式持久化（crypto serializer 密文入库，BYTEA 列）；读取明文经
	// ConfigData() 获取，详情接口仅输出密文，查看明文需经明文权限接口
	// Config the channel configuration in JSON text, possibly containing transport
	// credentials (e.g. SMTP account/password); persisted encrypted at rest via the
	// crypto serializer (BYTEA column). Plaintext is obtained via ConfigData(); the
	// detail endpoint outputs ciphertext only, and viewing plaintext requires the
	// plaintext permission endpoint
	Config *serializer.Crypto `json:"config" gorm:"column:config;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=21001,length=32"`
	// Description 渠道描述，说明渠道用途
	// Description the description of the channel
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// ConfigData 返回渠道配置的明文字节，nil 安全（Config 为空或 nil 时返回 nil）。
// ConfigData returns the plaintext bytes of the channel config, nil-safe (nil when
// Config is nil or empty).
func (c *NotificationChannel) ConfigData() []byte {
	if c == nil || c.Config == nil {
		return nil
	}
	return c.Config.Value()
}

// NotificationTarget 表示一个通知目标，是通知的实际接收者（如某个邮箱地址、
// Webhook URL、企业微信群机器人等），归属于某个通知渠道（ChannelID）。
// Type 为渠道类型键快照，取值定义在 internal/notify/channel.go。
// 对应表为 notification_targets。
// NotificationTarget represents a notification target, the actual receiver of a
// notification (e.g. an email address or a webhook URL), belonging to a
// notification channel. The channel type keys are defined in internal/notify/channel.go.
type NotificationTarget struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this target belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内目标名称唯一
	// NamespaceName the namespace name, for multi-tenant isolation; target names
	// are unique within the same namespace
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_targets_namespace_name;index:idx_notification_targets_namespace"`
	// Name 目标名称，与 NamespaceName 组合唯一
	// Name the name of the target, unique together with NamespaceName
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_targets_namespace_name"`
	// Channel 所属的通知渠道，渠道删除时目标级联删除
	// Channel the owning notification channel, cascaded deleted with the channel
	Channel NotificationChannel `gorm:"foreignKey:ChannelID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// ChannelID 目标所属的通知渠道 ID
	// ChannelID the ID of the notification channel the target belongs to
	ChannelID uint `gorm:"column:channel_id;type:INT;index:idx_notification_targets_channel_id"`
	// Type 渠道类型键快照（如 email、webhook），取值定义在 internal/notify/channel.go
	// Type the snapshot of the channel type key (e.g. email, webhook), defined in
	// internal/notify/channel.go
	Type string `gorm:"column:type;type:VARCHAR(32)"`
	// Enabled 是否启用，禁用的目标不参与通知发送
	// Enabled whether the target is enabled, disabled targets are skipped when
	// sending notifications
	Enabled bool `gorm:"column:enabled;type:BOOLEAN;default:true"`
	// Config 目标配置（JSON 文本），如收件人邮箱、Webhook URL，结构随渠道类型不同而不同；
	// 以加密形式持久化（crypto serializer 密文入库，BYTEA 列），读取明文经 ConfigData() 获取
	// Config the target configuration in JSON text, such as the recipient email or
	// webhook URL, the schema varies by channel type; persisted encrypted at rest via
	// the crypto serializer (BYTEA column), plaintext obtained via ConfigData()
	Config *serializer.Crypto `json:"config" gorm:"column:config;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=21069,length=32"`
	// Language 目标偏好的消息语言（如 en、zh-Hans），空字符串表示使用平台默认语言；
	// 通知消息按该语言经 locales 模板渲染（i18n）
	// Language the target-preferred message language (e.g. en, zh-Hans); empty means
	// the platform default; notification messages are rendered in this language via
	// the locale templates (i18n)
	Language string `gorm:"column:language;type:VARCHAR(32)"`
	// Description 目标描述，说明目标用途
	// Description the description of the target
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// ConfigData 返回目标配置的明文字节，nil 安全（Config 为空或 nil 时返回 nil）。
// ConfigData returns the plaintext bytes of the target config, nil-safe (nil when
// Config is nil or empty).
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
// NotificationGroup represents a notification target group, aggregating multiple
// notification targets; a tag event rule can bind a group to notify all targets in it.
// Each namespace has at most one default group, enforced by the partial unique index
// uq_notification_groups_namespace_default in the migration SQL; rules without a
// bound group use the default group.
type NotificationGroup struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this group belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内组名称唯一
	// NamespaceName the namespace name, for multi-tenant isolation; group names
	// are unique within the same namespace
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_groups_namespace_name"`
	// Name 组名称，与 NamespaceName 组合唯一
	// Name the name of the group, unique together with NamespaceName
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_notification_groups_namespace_name"`
	// IsDefault 是否为命名空间默认组，每命名空间至多一个默认组
	// IsDefault whether the group is the default group of the namespace, at most
	// one default group per namespace
	IsDefault bool `gorm:"column:is_default;type:BOOLEAN;default:false"`
	// Description 组描述，说明组用途
	// Description the description of the group
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// NotificationGroupTarget 表示通知组与通知目标的多对多关联记录。
// 同一目标可加入多个组，同一组内目标不重复（group_id + target_id 组合唯一）。
// 对应表为 notification_group_targets。
// NotificationGroupTarget represents the many-to-many association between a
// notification group and a notification target. A target can join multiple groups,
// and a group has no duplicate targets (group_id + target_id are unique together).
type NotificationGroupTarget struct {
	database.BaseModel

	// GroupID 通知组 ID
	// GroupID the ID of the notification group
	GroupID uint `gorm:"column:group_id;type:INT;uniqueIndex:uq_notification_group_targets_group_target"`
	// TargetID 通知目标 ID，与 GroupID 组合唯一
	// TargetID the ID of the notification target, unique together with GroupID
	TargetID uint `gorm:"column:target_id;type:INT;uniqueIndex:uq_notification_group_targets_group_target;index:idx_notification_group_targets_target_id"`
}

// NotificationRecord 表示一条通知发送记录，记录某事件（EventID）对某目标（TargetID）
// 的发送尝试与结果，包括所属通知组（GroupID）、发送渠道（ChannelID）与渠道类型快照
// （ChannelType）、发送状态（Status）、渠道返回的消息 ID（MessageID）、
// 通知负载快照（Payload）与实际发送成功时间（NotifiedAt）。
// 同一事件类型下的同一事件对同一目标至多一条记录（event_type + event_id + target_id 组合唯一）。
// 对应表为 notification_records。
// NotificationRecord represents a notification delivery record, recording the delivery
// attempt of an event to a target, including the notification group, the channel and
// its type snapshot, the delivery status, the channel message ID, the payload snapshot
// and the actual delivery time. An event has at most one record per target
// (event_type + event_id + target_id are unique together).
type NotificationRecord struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this record belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段
	// NamespaceName the namespace name, for multi-tenant isolation
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);index:idx_notification_records_namespace_created_at"`
	// EventType 事件类型，标识 EventID 指向的事件类型，当前仅标签事件 tag_event，
	// 与 EventID、TargetID 组合唯一（event_id 在不同事件类型间不全局唯一）
	// EventType the event type identifying what event EventID refers to, e.g. tag_event,
	// forming the composite unique key with EventID and TargetID (event_id is not
	// globally unique across event types)
	EventType string `gorm:"column:event_type;type:VARCHAR(32);uniqueIndex:uq_notification_records_event_target"`
	// EventID 触发通知的标签事件 ID
	// EventID the ID of the tag event that triggered the notification
	EventID uint `gorm:"column:event_id;type:BIGINT;uniqueIndex:uq_notification_records_event_target"`
	// GroupID 发送时所属的通知组 ID，0 表示未绑定组
	// GroupID the ID of the notification group at the sending time, 0 means not bound
	GroupID uint `gorm:"column:group_id;type:INT"`
	// TargetID 通知目标 ID，与 EventID 组合唯一，保证同一事件对同一目标至多一条记录
	// TargetID the ID of the notification target, unique together with EventID so that
	// an event has at most one record per target
	TargetID uint `gorm:"column:target_id;type:INT;uniqueIndex:uq_notification_records_event_target"`
	// ChannelID 发送通知的渠道 ID
	// ChannelID the ID of the channel that sent the notification
	ChannelID uint `gorm:"column:channel_id;type:INT;index:idx_notification_records_channel_id"`
	// ChannelType 渠道类型键快照，渠道删除或类型变更后仍可追溯
	// ChannelType the snapshot of the channel type key, kept for traceability after
	// the channel is deleted or its type changes
	ChannelType string `gorm:"column:channel_type;type:VARCHAR(32)"`
	// Status 通知发送状态：pending/success/failed
	// Status the delivery status of the notification: pending/success/failed
	Status string `gorm:"column:status;type:VARCHAR(16)"`
	// Error 发送失败时的错误信息，成功时为空字符串
	// Error the error message when the delivery failed, empty on success
	Error string `gorm:"column:error;type:TEXT"`
	// MessageID 渠道返回的消息 ID，可用于后续查询发送状态
	// MessageID the message ID returned by the channel, used to query the delivery
	// status later
	MessageID string `gorm:"column:message_id;type:VARCHAR(128)"`
	// Payload 通知负载（JSON 文本）快照
	// Payload the snapshot of the notification payload in JSON text
	Payload string `gorm:"column:payload;type:TEXT"`
	// NotifiedAt 通知实际发送成功的时间，nil 表示尚未发送成功
	// NotifiedAt the time the notification was actually delivered, nil means not
	// delivered yet
	NotifiedAt *time.Time `gorm:"column:notified_at;type:TIMESTAMPTZ"`
}

// NotificationTemplate 表示一条命名空间级的通知消息模板，描述某事件（Event）对某
// 渠道类型（ChannelType）在某种语言（Language）下的标题（Title）与正文（Content）；
// Title 与 Content 为 Go text/template 模板，按事件负载渲染。同一命名空间下
// （event + channel_type + language）组合唯一：该组合唯一确定一条可用的消息模板，
// ChannelType 与 Language 支持 "*" 通配。Source 标记模板行是内置默认模板（default）
// 还是用户覆盖模板（override）；内置默认模板由事件模板注册表
// （internal/notify/template.go）种子生成。对应表为 notification_templates。
// NotificationTemplate represents a namespace-scoped notification message template
// describing, for an event (Event) targeted at a channel type (ChannelType) in a
// language (Language), the title (Title) and content (Content); Title and Content are
// Go text/template templates rendered against the event payload. The
// (event + channel_type + language) combination is unique within a namespace and
// uniquely determines the usable template; ChannelType and Language support "*"
// wildcard. Source marks whether the row is a built-in default template (default) or
// a user-customized override (override); the built-in defaults are seeded from the
// event template registry (internal/notify/template.go).
type NotificationTemplate struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	// Namespace the namespace entity this template belongs to, for GORM association
	// (foreignKey:NamespaceName;references:Name;onDelete:CASCADE)
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段；与 Event、ChannelType、Language 组合唯一
	// NamespaceName the namespace name, for multi-tenant isolation; unique together
	// with Event, ChannelType and Language
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language;index:idx_notification_templates_namespace"`
	// Event 事件类型键（如 tag_event_record），与 ChannelType、Language 组合唯一
	// Event the event type key (e.g. tag_event_record), unique together with ChannelType and Language
	Event string `gorm:"column:event;type:VARCHAR(64);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 事件类型键，如 tag_event_record
	// ChannelType 渠道类型键，允许 * 通配（对所有渠道生效）
	// ChannelType the channel type key, "*" matches all channels
	ChannelType string `gorm:"column:channel_type;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 渠道类型键，允许 * 通配
	// Language 语言，允许 * 通配（对所有语言生效）
	// Language the language, "*" matches all languages
	Language string `gorm:"column:language;type:VARCHAR(32);uniqueIndex:uq_notification_templates_namespace_event_channel_language"` // 语言，允许 * 通配
	// Title 标题模板（Go text/template），按事件负载渲染
	// Title the title template (Go text/template), rendered against the event payload
	Title string `gorm:"column:title;type:TEXT"` // Go text/template 标题模板
	// Content 正文模板（Go text/template），按事件负载渲染
	// Content the content template (Go text/template), rendered against the event payload
	Content string `gorm:"column:content;type:TEXT"` // Go text/template 正文模板
	// Source 模板来源：default | override，标记内置默认模板或用户覆盖模板
	// Source the template source: default | override, marking a built-in default or a user override
	Source string `gorm:"column:source;type:VARCHAR(16);default:default"` // default | override
	// Description 模板描述（可选）
	// Description the optional description of the template
	Description string `gorm:"column:description;type:VARCHAR(256)"` // 可选描述
}
