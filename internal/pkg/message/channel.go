// Package message 提供消息发送能力基座。
// 核心是渠道基座：Channel 抽象接口 + 工厂注册表，将「渠道类型」与「渠道实现」解耦，
// 支持未来持续添加新的渠道类型（如企业微信、钉钉、Slack、短信等）。
// 上层通过 Service 依据命名空间下的通知组解析目标，并经渠道基座将通用消息
// （Message，与事件类型/模板渲染解耦）投递到目标，同时维护消息目标记录。
// 本包与 GORM / asynq 解耦，仅依赖 stdlib 与 internal/pkg 内共享包（errors、validator）。
//
// 渠道类型键常量（Type）定义在本包而非 models/core，以避免 message 包与
// models/core 包之间的 import cycle（message 包不依赖 models/core）。
//
// 统一发送入口 Send 基于独立的 generic 资源池基座 internal/pkg/resourcepool 实现：
// 按「渠道类型 + 规范化渠道级配置」维护相互独立的连接池，复用 SMTP 等有状态连接、
// 失效自动重建、闲置定时回收，进程退出时经 ClosePools 释放全部存活连接。
//
// Package message provides the message-sending capability base. Its core is the
// channel base: the Channel interface plus a factory registry that decouples channel
// types from implementations, so new channel types can be added continuously. Upper
// layers resolve targets by the namespace-scoped notification group and deliver the
// generic Message (decoupled from event types and template rendering) via the channel
// base, while maintaining the message target records. This package is decoupled from
// GORM and asynq, depending only on the stdlib and shared internal/pkg packages
// (errors, validator).
package message

import (
	// stdlib
	"context"
	"encoding/json"
	"slices"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// 渠道类型键常量，用于 notification_channels.type 列、目标类型快照与通知记录。
// Channel type keys, used in the notification_channels.type column, target type
// snapshots and notification records.
const (
	// TypeEmail 邮件渠道，经 SMTP 发送
	// TypeEmail the email channel, sent via SMTP
	TypeEmail = "email"
	// TypeWebhook Webhook 渠道，以 HTTP POST 投递
	// TypeWebhook the webhook channel, delivered via HTTP POST
	TypeWebhook = "webhook"
)

// 模板键常量属业务层，由各个事件源在各自层（如 internal/notify）中定义，
// 本基座包不定义具体模板键，仅通过 Message.Template 承载并在渲染时按
// `notify.<template>.title` / `notify.<template>.content` 在 locales 翻译文件中
// 查找对应语言的文本模板（含参数占位符），实现文案自定义与 i18n。
// Message template keys are business-scoped and pinned by each event source in its own
// layer (e.g. internal/notify); this base package defines no concrete template keys. It
// only carries Message.Template and looks up `notify.<template>.title` /
// `notify.<template>.content` in the locale files for the target language (with
// parameter placeholders), enabling text customization and i18n.

// Message 通用结构化通知消息，与事件类型解耦：由事件源构造（模板键 + 参数），
// 在发送时由 Service 按目标语言经 locales 模板渲染为具体文本（RenderedMessage）。
// Params 的键与模板占位符（{{.Key}}）一一对应，采用开头大写驼峰命名。
// Message is the generic structured notification message, decoupled from event types:
// event sources build it (template key + params), and the Service renders it into the
// concrete text (RenderedMessage) via locale templates in the target language. The
// Params keys match the template placeholders ({{.Key}}) in UpperCamelCase.
type Message struct {
	// Template 消息模板键，取值由业务层定义（如 notify 下的模板键常量）（必填）
	// Template the message template key, defined at the business layer
	// (e.g. the template key constants under notify) (required)
	Template string `json:"template" binding:"required"`
	// Params 模板参数，供 locales 模板渲染
	// Params the template params used to render the locale template
	Params map[string]any `json:"params,omitempty"`
	// ContentType 消息正文 MIME 类型，由消息定义层声明要发送的正文格式；
	// 空值交由发送方按默认处理（系统邮件直发默认 text/html，渲染渠道默认 text/plain）
	// ContentType the MIME type of the message body, declared by the message definition
	// to state what body format will be sent; empty lets the sender use its default
	// (text/html for the system direct-send, text/plain for rendered channels)
	ContentType string `json:"contentType,omitempty"`
}

// RenderedMessage 渲染完成的具体通知消息（标题 + 正文），由 Service 依据目标语言
// 渲染 Message 后传入渠道；渠道负责将其投递，不再关心模板与参数。
// RenderedMessage is the rendered concrete notification message (title + content),
// produced by the Service from a Message in the target language and passed to the
// channel; the channel only delivers it and ignores templates and params.
type RenderedMessage struct {
	// Title 消息标题
	// Title the title of the message
	Title string `json:"title"`
	// Content 消息正文
	// Content the body of the message
	Content string `json:"content"`
	// ContentType 消息正文 MIME 类型，空值视为 text/plain（与既有行为一致）
	// ContentType the MIME type of the message body; empty means text/plain
	// (consistent with the existing behavior)
	ContentType string `json:"contentType,omitempty"`
}

// Result 渠道投递结果。
// Result is the delivery result of a channel.
type Result struct {
	// MessageID 渠道返回的消息 ID，渠道不提供时为空字符串
	// MessageID the message ID returned by the channel, empty if not provided
	MessageID string
}

// Target 目标信息，由 Service 从 notification_targets 行解析后传入渠道。
// Config 为目标级配置（JSON 原文），其结构由渠道类型定义并自行解析，
// 与渠道级配置相互独立：渠道级配置承载传输凭据（如 SMTP 账号密码），
// 目标级配置承载具体接收者（如收件邮箱、Webhook URL）。
// Language 为目标偏好的消息语言（如 en、zh-Hans），空字符串表示使用平台默认语言，
// 由 Service 在渲染消息时消费，与渠道类型无关。
// Target carries the target-level info resolved by the Service from a
// notification_targets row. Config is the raw JSON of the target-level config whose
// schema is defined and parsed by the channel type; it is independent of the
// channel-level config which carries transport credentials. Language is the
// target-preferred message language (e.g. en, zh-Hans); empty means the platform
// default, consumed by the Service when rendering messages, independent of channels.
type Target struct {
	// Name 目标名称
	// Name the name of the target
	Name string
	// Language 目标偏好的消息语言，空字符串使用平台默认
	// Language the target-preferred message language, empty for the platform default
	Language string
	// Config 目标级配置（JSON 原文）
	// Config the target-level config in raw JSON
	Config json.RawMessage
}

// Channel 通知渠道抽象接口，所有具体渠道实现（email、webhook 及后续新增）必须实现。
// 渠道实例承载渠道级配置（如 SMTP 账号密码）；Send 负责将消息投递到目标，
// 目标级配置由 Target 携带。新增渠道只需实现本接口并在工厂注册表中注册。
// Channel is the abstraction of a notification channel that every concrete
// implementation must satisfy. A channel instance carries the channel-level config
// (e.g. SMTP credentials); Send delivers the message to the target whose own config
// is carried by Target. Adding a channel means implementing this interface and
// registering its factory.
type Channel interface {
	// Type 返回渠道类型键
	// Type returns the channel type key
	Type() string
	// Send 向目标投递渲染后的消息，返回渠道消息 ID（渠道不提供时为 ""）；
	// 返回非 nil 错误表示投递失败。
	// Send delivers the rendered message to the target and returns the channel
	// message ID ("" if not provided); a non-nil error means the delivery failed.
	Send(ctx context.Context, target *Target, msg *RenderedMessage) (messageID string, err error)
}

// Factory 渠道工厂，依据渠道级配置（JSON 原文）构建渠道实例。
// 配置非法（缺失必填字段、JSON 语法错误）时返回错误。
// Factory builds a channel instance from the channel-level config in raw JSON;
// it returns an error when the config is invalid.
type Factory func(config json.RawMessage) (Channel, error)

// registry 渠道工厂注册表：渠道类型键 -> 工厂。
// registry maps channel type keys to their factories.
var registry = map[string]Factory{}

// Register 注册渠道类型工厂，供各渠道实现 init() 调用。
// 同一类型键重复注册视为编程错误（panic）。
// Register registers a channel factory; it is called by each channel's init().
// Registering the same type key twice is treated as a programming error (panic).
func Register(typ string, factory Factory) {
	if _, ok := registry[typ]; ok {
		panic(errors.Newf("message: channel type %q already registered", typ))
	}
	registry[typ] = factory
}

// New 依据渠道类型键与渠道级配置构建渠道实例；类型未注册时返回错误。
// New builds a channel instance by type key and channel-level config; it returns
// an error when the type is not registered.
func New(typ string, config json.RawMessage) (Channel, error) {
	factory, ok := registry[typ]
	if !ok {
		return nil, errors.Newf("message: channel type %q not registered", typ)
	}
	return factory(config)
}

// Types 返回已注册的渠道类型键（升序），供管理接口枚举可选类型。
// Types returns the registered channel type keys in ascending order, for management
// APIs to enumerate available types.
func Types() []string {
	types := make([]string, 0, len(registry))
	for typ := range registry {
		types = append(types, typ)
	}
	slices.Sort(types)
	return types
}
