// Package mail 提供系统级邮件直发（mail:send）的 asynq 任务处理器。
// MailSendHandler 消费 mail:send 任务：解析 MailSendInput 载荷后，调用 internal/pkg/mail
// 的 Sender 按系统 smtp 配置直接向指定收件人投递邮件，不经过数据库通知渠道/目标，
// 也不落任何发送记录。
//
// Package mail provides the asynq handler for the system-level email direct-send
// (mail:send). MailSendHandler consumes mail:send tasks: after parsing the MailSendInput
// payload it calls the internal/pkg/mail Sender to deliver the email directly to the
// given recipient using the system SMTP config, bypassing the DB notification
// channels/targets and persisting no send records.
package mail

import (
	// stdlib
	"context"
	"encoding/json"

	// third-party
	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// MailSendTask 系统级邮件直发任务类型，载荷为 MailSendInput（收件人 + 结构化消息 + 语言）。
// 按系统 smtp 配置直接向指定收件人投递，不经过数据库通知渠道/目标，也不落发送记录。
// MailSendTask is the system-level email direct-send task type carrying a MailSendInput
// payload (recipient + structured message + language). It delivers directly to the given
// recipient via the system SMTP config, bypassing DB notification channels/targets and
// persisting no send records.
const MailSendTask = "mail:send"

// MailSendInput 系统级邮件直发任务载荷。
// MailSendInput is the payload of the system-level email direct-send task.
type MailSendInput struct {
	// To 收件人邮箱地址（必填，需为合法邮箱）
	// To the recipient email address (required, must be a valid email)
	To string `json:"to" binding:"required,email"`
	// Message 结构化通知消息（必填），经 locale 模板渲染标题与正文后投递
	// Message the structured notification message (required), rendered via locale templates
	Message message.Message `json:"message" binding:"required"`
	// Language 渲染使用的目标语言（可空，回退平台默认语言）
	// Language the target language for rendering (optional, falls back to platform default)
	Language string `json:"language"`
}

// mailSender 邮件直发器接口，定义在消费侧并与 internal/pkg/mail 解耦，便于单元测试注入替代实现。
// mailSender is the email sender interface, defined on the consumer side and kept
// decoupled from internal/pkg/mail, so tests can inject a substitute implementation.
type mailSender interface {
	Send(ctx context.Context, to string, msg message.Message, lang string) error
}

// 编译期断言：确保 MailSendHandler 实现了 etl.Handler 接口。
// Compile-time assertion that MailSendHandler implements etl.Handler.
var _ etl.Handler = (*MailSendHandler)(nil)

// MailSendHandler 系统级邮件直发处理器，消费 mail:send 任务。
// 解析邮件载荷后调用 Sender.Send 按系统 smtp 配置直接投递；
// 载荷非法返回错误触发重试。
// MailSendHandler is the system-level email direct-send handler consuming mail:send
// tasks. It parses the payload and calls Sender.Send to deliver directly via the system
// SMTP config; an invalid payload returns an error to trigger a retry.
type MailSendHandler struct {
	sender mailSender
	logger *logging.Logger
}

// NewMailSendHandler 从 do.Injector 解析依赖并创建系统级邮件直发处理器。
// NewMailSendHandler resolves dependencies from the do.Injector and creates the handler.
func NewMailSendHandler(i do.Injector) *MailSendHandler {
	return &MailSendHandler{sender: provider.MustInvokeMailSender(i), logger: provider.MustInvokeDatabaseLogger(i)}
}

// Do 消费一条 mail:send 任务：解析 MailSendInput 载荷并调用 Sender.Send 直接投递；
// 载荷非法或投递失败返回错误触发重试。
// Do consumes one mail:send task: it parses the MailSendInput payload and calls
// Sender.Send to deliver directly; an invalid payload or a delivery failure returns an
// error to trigger a retry.
func (h *MailSendHandler) Do(ctx context.Context, task etl.Task) error {
	var input MailSendInput
	if err := json.Unmarshal(task.Payload(), &input); err != nil {
		return errors.WithStack(err)
	}
	if err := validateMailSendInput(input); err != nil {
		return err
	}
	if err := h.sender.Send(ctx, input.To, input.Message, input.Language); err != nil {
		h.logger.Ctx(ctx).Errorf("mail: send failed (to=%s template=%s): %v", input.To, input.Message.Template, err)
		return err
	}
	return nil
}

// NewMailSendTask 将系统级邮件直发请求编码为 mail:send 异步任务；必填字段缺失或邮箱非法返回错误。
// NewMailSendTask encodes a system-level email direct-send request into a mail:send async
// task; missing required fields or an invalid email return an error.
func NewMailSendTask(in MailSendInput) (*asynq.Task, error) {
	if err := validateMailSendInput(in); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(&in)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(MailSendTask, payload), nil
}

// validateMailSendInput 校验系统级邮件直发载荷的必填字段：收件人邮箱（必需合法）与消息模板键。
// 校验规则声明在 MailSendInput / Message 结构体的 binding 标签上，经全局 validator 引擎
// 统一执行（validator.Validate.Struct），错误为 validator.ValidationErrors。
// validateMailSendInput validates the required fields of a direct-send payload: the
// recipient email (required and valid) and the message template key. The rules are
// declared as binding tags on the MailSendInput / Message structs and enforced by the
// global validator engine via validator.Validate.Struct, returning a
// validator.ValidationErrors on failure.
func validateMailSendInput(in MailSendInput) error {
	return errors.WithStack(validator.Validate.Struct(&in))
}
