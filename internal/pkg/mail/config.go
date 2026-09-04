// Package mail 提供系统层邮件直发能力：基于 [smtp] 配置直接调用 message 邮件渠道
// 向指定收件人发送邮件，绕过 DB 背书的通知渠道 / 通知目标（不在 notification 表中
// 持久化任何记录）。本包与 GORM / models/core / notify / asynq 解耦，仅依赖 stdlib
// 与 internal/pkg 内共享包（config、errors、logging、message、translate、base、validator），
// 配置来源为 config.toml 的 [smtp] 段（见 Config）。
//
// Package mail provides system-level direct email sending: it sends emails to the
// given recipients through the message email channel based on the [smtp] config,
// bypassing the DB-backed notification channels / targets (it persists nothing in
// the notification tables). This package is decoupled from GORM / models/core /
// notify / asynq, depending only on the stdlib and the shared internal/pkg packages
// (config, errors, logging, message, translate, base, validator), with its config
// sourced from the [smtp] section of config.toml (see Config).
package mail

import (
	// third-party
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// Prefix 是系统层邮件直发配置在配置文件中的前缀（"smtp"）。
// 作为单一事实来源，供 provider / 远程配置提取引用。
// Prefix is the config-file prefix of the system-level email direct-send config
// ("smtp"); it is the single source of truth referenced by providers and config extraction.
const Prefix = "smtp"

// Config 是系统层邮件直发的配置结构体，对应 config.toml 的 [smtp] 段。
// 字段通过 mapstructure 标签映射、default 标签填充默认值、comment 标签提供中文说明（DescribeConfig 的渲染来源）。
// Config is the config struct of the system-level email direct-send, mapped to the
// [smtp] section of config.toml. Fields are mapped via the mapstructure tag, defaulted
// via the default tag, and documented in Chinese via the comment tag (rendered by DescribeConfig).
type Config struct {
	// Enabled 是否启用系统层邮件发送
	// Enabled whether the system-level email sending is enabled
	Enabled bool `mapstructure:"enabled" default:"false" comment:"是否启用系统层邮件发送"`
	// Host SMTP 服务器地址
	// Host the SMTP server address
	Host string `mapstructure:"host" default:"" comment:"SMTP 服务器地址"`
	// Port SMTP 端口（465 隐式 TLS，587/25 STARTTLS）
	// Port the SMTP port (465 implicit TLS, 587/25 STARTTLS)
	Port int `mapstructure:"port" default:"465" comment:"SMTP 端口（465 隐式 TLS，587/25 STARTTLS）"`
	// Username SMTP 认证用户名
	// Username the SMTP auth username
	Username string `mapstructure:"username" default:"" comment:"SMTP 认证用户名"`
	// Password SMTP 认证密码
	// Password the SMTP auth password
	Password string `mapstructure:"password" default:"" comment:"SMTP 认证密码"`
	// From 发件人邮箱地址
	// From the sender email address
	From string `mapstructure:"from" default:"" comment:"发件人邮箱地址"`
}

// DescribeConfig 返回系统层邮件直发配置的结构化描述信息（供 describe config 渲染）。
// DescribeConfig returns the structured description of the mail config (rendered by describe config).
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 Viper 配置管理器中提取 [smtp] 配置并校验。
// ExtractConfig extracts and validates the [smtp] config from the Viper instance.
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](v, validate, Prefix)
}
