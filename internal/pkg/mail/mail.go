package mail

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// Sender 系统层邮件直发器：持有 [smtp] 配置、日志器与消息渠道连接池，经 message
// 邮件渠道直接发送。与 notify.Service 不同，Sender 不落库、不解析 notification_targets，
// 仅按系统配置直接投递。
type Sender struct {
	cfg    *Config
	logger *logging.Logger
	pool   *message.Pool
}

// NewSender 构造一个系统层邮件直发器。
func NewSender(cfg *Config, logger *logging.Logger, pool *message.Pool) *Sender {
	return &Sender{cfg: cfg, logger: logger, pool: pool}
}

// ready 返回系统邮件直发的前置检查结果：未启用或必备配置（host/from）不完整时
// 返回带上下文的错误。Enabled 与 Send 共用同一守卫，保证能力判断与发送路径一致。
func (s *Sender) ready() error {
	if !s.cfg.Enabled {
		return errors.New("mail: system email is disabled")
	}
	if s.cfg.Host == "" || s.cfg.From == "" {
		return errors.New("mail: system email is not configured (host/from required)")
	}
	return nil
}

// Enabled 报告系统邮件直发能力是否可用：仅当 [smtp] 启用且必备配置（host/from）齐全时可用。
// 该能力判断供全局功能开关（features）使用，使底层 SMTP 配置细节仅在本包内可见，
// features 层无需直接读取 mail.Config。Send 也在投递前做同一守卫。
func (s *Sender) Enabled() bool { return s.ready() == nil }

// Send 向指定收件人发送一条结构化通知消息（message.Message）。
//
// 流程：先做启用 / 配置完整性守卫；再解析目标语言（非法时回退默认并告警）；
// 经 locale i18n 模板（notify.<template>.title/content）渲染标题与正文；
// 组装邮件渠道级与目标级 JSON 配置，最终调用注入的消息渠道连接池（Pool.Send）投递。
func (s *Sender) Send(ctx context.Context, to string, msg message.Message, lang string) error {
	if err := s.ready(); err != nil {
		return err
	}

	// 解析目标语言：空字符串使用平台默认；非法语言回退默认并告警。
	parsed := base.DefaultLanguage
	if lang != "" {
		if p, err := base.ParseLanguage(lang); err != nil {
			s.logger.Ctx(ctx).Warnf("mail: invalid language %q, fallback to default: %v", lang, err)
		} else {
			parsed = p
		}
	}
	parsed = parsed.Normalize()

	// 经 locale i18n 模板渲染标题与正文（不经 DB 持久化模板）。
	title := translate.Localize(
		parsed,
		fmt.Sprintf(`notify.%s.title`, msg.Template),
		msg.Params)
	content := translate.Localize(
		parsed,
		fmt.Sprintf(`notify.%s.content`, msg.Template),
		msg.Params)

	channelJSON, err := json.Marshal(message.EmailChannelConfig{
		Host:     s.cfg.Host,
		Port:     s.cfg.Port,
		Username: s.cfg.Username,
		Password: s.cfg.Password,
		From:     s.cfg.From,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	targetJSON, err := json.Marshal(message.EmailTarget{To: to})
	if err != nil {
		return errors.WithStack(err)
	}

	// 正文 MIME 类型由消息定义层（msg.ContentType）声明，明确"实际发送什么格式"；
	// 未声明（空值）时按系统邮件直发默认回退 text/html。
	contentType := msg.ContentType
	if contentType == "" {
		contentType = "text/html"
	}

	// 最终调用注入的消息渠道连接池 Send 投递，返回的已是项目 errors 链，直接透传。
	_, err = s.pool.Send(ctx, message.TypeEmail, channelJSON, &message.Target{Name: to, Config: targetJSON}, &message.RenderedMessage{
		Title:       title,
		Content:     content,
		ContentType: contentType,
	})
	return errors.WithStack(err)
}
