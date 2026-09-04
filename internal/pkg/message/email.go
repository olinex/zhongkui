package message

import (
	// stdlib
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"reflect"
	"strconv"
	"strings"
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// smtpTLSPort 使用隐式 TLS 的 SMTP 端口（SSL 直连）。
// smtpTLSPort is the SMTP port that uses implicit TLS (direct SSL).
const smtpTLSPort = 465

// smtpValidateTimeout 单次 SMTP 连接探活（Noop）的超时上限，避免脏连接拖垮借出。
// smtpValidateTimeout is the timeout for a single SMTP connection liveness probe
// (Noop) so a stale connection cannot hold up the borrow.
const smtpValidateTimeout = 3 * time.Second

// emailChannel 邮件渠道实现：经 SMTP 服务器发送邮件，并由统一资源池托管其连接生命周期。
// 渠道实例通过实现 resourcepool.Lifespan 暴露 Open/Validate/Close 钩子：
//   - Open：惰性建立并认证一条可复用 SMTP 连接（465 隐式 TLS、587/25 STARTTLS）；
//   - Send：在该连接上以 MAIL/RCPT/DATA 逐次发送，不再重连；
//   - Validate：以 Noop 探活，识别被服务端断开的连接（借出时销毁并重建）；
//   - Close：关闭连接，回收文件句柄。
//
// 由于池保证单个实例在同一时刻仅被单一借出方独占，Send 无需内部加锁。
// 渠道级配置（notification_channels.config）结构：
//
//	{"host": "smtp.example.com", "port": 465, "username": "user", "password": "secret", "from": "alert@example.com"}
//
// 目标级配置（notification_targets.config）结构：
//
//	{"to": "ops@example.com"}
//
// emailChannel sends emails via an SMTP server while the unified resource pool
// manages its connection lifecycle. It implements resourcepool.Lifespan to expose
// the Open/Validate/Close hooks: Open lazily establishes and authenticates a
// reusable SMTP connection (implicit TLS on 465, STARTTLS on 587/25); Send
// delivers each message over that connection via MAIL/RCPT/DATA without reconnecting;
// Validate probes with Noop to detect a connection dropped by the server (destroyed
// and rebuilt on borrow); Close releases the connection.
//
// Since the pool guarantees an instance is exclusively held by one borrower at a
// time, Send needs no internal locking. The channel-level config schema is:
//
//	{"host": "smtp.example.com", "port": 465, "username": "user", "password": "secret", "from": "alert@example.com"}
//
// and the target-level config schema is:
//
//	{"to": "ops@example.com"}
type emailChannel struct {
	host     string       // SMTP 服务器地址
	port     int          // SMTP 端口
	username string       // 认证用户名
	password string       // 认证密码
	from     string       // 发件人地址
	client   *smtp.Client // 可复用 SMTP 连接（仅在其被借出期间访问）
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

// Send 在可复用的 SMTP 连接上经 MAIL/RCPT/DATA 向目标收件人发送邮件。
// 复用由 Open 建立的连接，不重复握手/认证；连接在归还后被池复用。
// 目标配置缺失收件人、收件人格式非法或连接未建立时返回错误。SMTP 协议不返回消息 ID。
// Send delivers the email to the target recipient over the reusable SMTP connection
// via MAIL/RCPT/DATA, without re-dialing or re-authenticating; the connection is
// reused after the object is returned to the pool. A missing recipient, an invalid
// recipient address, or a missing connection returns an error. SMTP provides no
// message ID.
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
	if c.client == nil {
		return "", errors.New("email: smtp connection not open, call Open first")
	}
	if err := c.client.Mail(c.from); err != nil {
		return "", errors.WithStack(err)
	}
	if err := c.client.Rcpt(cfg.To); err != nil {
		return "", errors.WithStack(err)
	}
	w, err := c.client.Data()
	if err != nil {
		return "", errors.WithStack(err)
	}
	if _, err := w.Write(buildMailBody(c.from, cfg.To, msg)); err != nil {
		_ = w.Close()
		return "", errors.WithStack(err)
	}
	if err := w.Close(); err != nil {
		return "", errors.WithStack(err)
	}
	return "", nil
}

// Open 惰性建立并认证一条可复用 SMTP 连接：
// 465 端口先做隐式 TLS；其余端口在服务器通过 EHLO 声明支持 STARTTLS 时升级，
// 否则按明文连接继续（老式服务器）。配置了用户名时才执行 AUTH PLAIN。
// 仅供资源池在建连时调用，同实例连接建立后由后续 Send 复用。
// Open lazily establishes and authenticates a reusable SMTP connection: implicit
// TLS first on port 465; on other ports it upgrades via STARTTLS when the server
// advertises it through EHLO, otherwise it proceeds on a plaintext connection
// (legacy servers). AUTH PLAIN only runs when a username is configured. It is meant
// to be called by the pool; the established connection is reused by later Sends.
func (c *emailChannel) Open(ctx context.Context) error {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errors.WithStack(err)
	}

	tlsConfig := &tls.Config{ServerName: c.host}
	if c.port == smtpTLSPort {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return errors.WithStack(err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		_ = conn.Close()
		return errors.WithStack(err)
	}
	if c.port != smtpTLSPort {
		// 非隐式 TLS 端口：服务器支持 STARTTLS 则立即升级（StartTLS 内部二次 EHLO）
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				_ = client.Close()
				return errors.WithStack(err)
			}
		}
	}
	if c.username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.username, c.password, c.host)); err != nil {
			_ = client.Close()
			return errors.WithStack(err)
		}
	}
	c.client = client
	return nil
}

// Validate 以 SMTP Noop 对可复用连接探活：连接未建立、Noop 失败或超时均视为失效，
// 由资源池在借出时销毁并重建。仅供资源池在校验时调用。
// Validate probes the reusable connection with SMTP Noop: a missing connection, a
// failed Noop, or a timeout marks it stale for the pool to destroy and rebuild on
// borrow. It is meant to be called by the pool.
func (c *emailChannel) Validate(ctx context.Context) bool {
	if c.client == nil {
		return false
	}
	done := make(chan error, 1)
	go func() { done <- c.client.Noop() }()
	select {
	case err := <-done:
		return err == nil
	case <-ctx.Done():
		return false
	case <-time.After(smtpValidateTimeout):
		return false
	}
}

// Close 关闭可复用 SMTP 连接并回收文件句柄（尽力作业，忽略关闭细节错误）。
// 仅供资源池在驱逐/关闭时调用。
// Close closes the reusable SMTP connection and releases its handle (best-effort).
// It is meant to be called by the pool on eviction/shutdown.
func (c *emailChannel) Close() error {
	if c.client == nil {
		return nil
	}
	client := c.client
	c.client = nil
	// 先发 QUIT 优雅退出（服务器已断开时可能失败），再强制 Close 兜底，确保连接被释放
	_ = client.Quit()
	return errors.WithStack(client.Close())
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
	// 按消息内容类型生成 MIME 头；未显式指定时回退 text/plain（与既有行为一致）
	ct := msg.ContentType
	if ct == "" {
		ct = "text/plain"
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: %s; charset=UTF-8\r\n", ct)
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
