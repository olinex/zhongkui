// Package mail 提供系统层邮件直发能力：基于 [smtp] 配置直接调用 message 邮件渠道
// 向指定收件人发送邮件，绕过 DB 背书的通知渠道 / 通知目标（不在 notification 表中
// 持久化任何记录）。本包与 GORM / models/core / notify / asynq 解耦，仅依赖 stdlib
// 与 internal/pkg 内共享包（config、errors、logging、message、translate、base、validator），
// 配置来源为 config.toml 的 [smtp] 段（见 Config）。
package mail

import (
	// 第三方
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/config"
)

// Prefix 是系统层邮件直发配置在配置文件中的前缀（"smtp"）。
// 作为单一事实来源，供 provider / 远程配置提取引用。
const Prefix = "smtp"

// Config 是系统层邮件直发的配置结构体，对应 config.toml 的 [smtp] 段。
// 字段通过 mapstructure 标签映射、default 标签填充默认值、comment 标签提供中文说明（DescribeConfig 的渲染来源）。
type Config struct {
	// Enabled 是否启用系统层邮件发送
	Enabled bool `mapstructure:"enabled" default:"false" comment:"是否启用系统层邮件发送"`
	// Host SMTP 服务器地址
	Host string `mapstructure:"host" default:"" comment:"SMTP 服务器地址"`
	// Port SMTP 端口（465 隐式 TLS，587/25 STARTTLS）
	Port int `mapstructure:"port" default:"465" comment:"SMTP 端口（465 隐式 TLS，587/25 STARTTLS）"`
	// Username SMTP 认证用户名
	Username string `mapstructure:"username" default:"" comment:"SMTP 认证用户名"`
	// Password SMTP 认证密码
	Password string `mapstructure:"password" default:"" comment:"SMTP 认证密码"`
	// From 发件人邮箱地址
	From string `mapstructure:"from" default:"" comment:"发件人邮箱地址"`
}

// DescribeConfig 返回系统层邮件直发配置的结构化描述信息（供 describe config 渲染）。
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 Viper 配置管理器中提取 [smtp] 配置并校验。
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](v, validate, Prefix)
}
