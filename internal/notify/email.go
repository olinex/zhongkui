package notify

import (
	// stdlib
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"reflect"
	"strconv"
	"strings"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// emailChannel 邮件渠道实现：经 SMTP 服务器发送邮件。
// 渠道级配置（notification_channels.config）结构：
//
//	{"host": "smtp.example.com", "port": 465, "username": "user", "password": "secret", "from": "alert@example.com"}
//
// 目标级配置（notification_targets.config）结构：
//
//	{"to": "ops@example.com"}
//
// emailChannel sends emails via an SMTP server. The channel-level config schema is:
//
//	{"host": "smtp.example.com", "port": 465, "username": "user", "password": "secret", "from": "alert@example.com"}
//
// and the target-level config schema is:
//
//	{"to": "ops@example.com"}
type emailChannel struct {
	host     string // SMTP 服务器地址
	port     int    // SMTP 端口
	username string // 认证用户名
	password string // 认证密码
	from     string // 发件人地址
}

// EmailChannelConfig 邮件渠道级配置，同时是前端配置表单的单一事实来源。
// 字段的 binding 标签驱动校验，form 标签（kind/default）经反射提取为前端表单 schema；
// 字段展示文案由前端依据字段名与渠道类型拼接 i18n key 翻译。
// EmailChannelConfig is the channel-level config of the email channel and the single
// source of truth for the frontend config form; the binding tag drives validation and
// the form tag (kind/default) is extracted into the frontend form schema via reflection.
// The field label is derived by the frontend from the field name and channel type.
type EmailChannelConfig struct {
	// Host SMTP 服务器地址，必填
	// Host the SMTP server address, required
	Host string `json:"host" binding:"required" form:"kind=string"`
	// Port SMTP 端口，默认 25
	// Port the SMTP port, defaults to 25
	Port int `json:"port" binding:"min=0,max=65535" form:"kind=number" default:"25"`
	// Username 认证用户名
	// Username the auth username
	Username string `json:"username" form:"kind=string"`
	// Password 认证密码
	// Password the auth password
	Password string `json:"password" form:"kind=password"`
	// From 发件人邮箱地址，必填
	// From the sender email address, required
	From string `json:"from" binding:"required" form:"kind=string"`
}

// EmailTarget 邮件目标级配置。
// EmailTarget is the target-level config of the email channel.
type EmailTarget struct {
	// To 收件人邮箱地址，必填
	// To the recipient email address, required
	To string `json:"to" binding:"required" form:"kind=string"`
}

// Type 返回渠道类型键。
// Type returns the channel type key.
func (c *emailChannel) Type() string { return TypeEmail }

// Send 经 SMTP 将邮件发送到目标收件人。
// 使用 net/smtp 的标准 SMTP 流程（AUTH PLAIN + 可选 STARTTLS），适用于 25/587 端口；
// 目标配置缺失收件人时返回错误。SMTP 协议不返回消息 ID。
// Send delivers the email to the target recipient via SMTP. It uses the standard
// net/smtp flow (AUTH PLAIN + optional STARTTLS) for ports 25/587; a missing
// recipient returns an error. SMTP provides no message ID.
func (c *emailChannel) Send(ctx context.Context, target *Target, msg *RenderedMessage) (string, error) {
	var cfg EmailTarget
	if err := json.Unmarshal(target.Config, &cfg); err != nil {
		return "", errors.WithStack(err)
	}
	if err := validator.Validate.Struct(&cfg); err != nil {
		return "", errors.WithStack(err)
	}
	// 校验收件人邮箱格式
	if !strings.Contains(cfg.To, "@") {
		return "", errors.New("email: invalid recipient address")
	}
	auth := smtp.PlainAuth("", c.username, c.password, c.host)
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	body := buildMailBody(c.from, cfg.To, msg)
	if err := smtp.SendMail(addr, auth, c.from, []string{cfg.To}, body); err != nil {
		return "", errors.WithStack(err)
	}
	return "", nil
}

// buildMailBody 组装 SMTP 邮件报文：标准邮件头（From/To/Subject/MIME）+ 正文。
// 注意：标题含非 ASCII 字符（如中文）时未做 RFC 2047 编码，基础实现仅适用于纯 ASCII 标题。
// buildMailBody assembles the SMTP message with standard headers (From/To/Subject/MIME)
// plus the body. Note: non-ASCII titles are not RFC 2047 encoded; this basic
// implementation targets ASCII-only titles.
func buildMailBody(from, to string, msg *RenderedMessage) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Title)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Content)
	return []byte(b.String())
}

// init 注册邮件渠道工厂，并注册其配置结构体类型供表单元信息抽取。
// init registers the email channel factory and its config struct types.
func init() {
	Register(TypeEmail, func(config json.RawMessage) (Channel, error) {
		var cfg EmailChannelConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := validator.Validate.Struct(&cfg); err != nil {
			return nil, errors.WithStack(err)
		}
		if cfg.Port == 0 {
			cfg.Port = 25
		}
		return &emailChannel{
			host:     cfg.Host,
			port:     cfg.Port,
			username: cfg.Username,
			password: cfg.Password,
			from:     cfg.From,
		}, nil
	})
	registerFormStructs(TypeEmail, reflect.TypeFor[EmailChannelConfig](), reflect.TypeFor[EmailTarget]())
}
