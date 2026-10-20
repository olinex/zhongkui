// Package mail 提供系统级邮件直发（mail:send）的 asynq 任务处理器。
// MailSendHandler 消费 mail:send 任务：解析 MailSendInput 载荷后，调用 internal/pkg/mail
// 的 Sender 按系统 smtp 配置直接向指定收件人投递邮件，不经过数据库通知渠道/目标，
// 也不落任何发送记录。
package mail

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/message"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// MailSendTask 系统级邮件直发任务类型，载荷为 MailSendInput（收件人 + 结构化消息 + 语言）。
// 按系统 smtp 配置直接向指定收件人投递，不经过数据库通知渠道/目标，也不落发送记录。
const MailSendTask = "mail:send"

// MailSendInput 系统级邮件直发任务载荷。
type MailSendInput struct {
	// To 收件人邮箱地址（必填，需为合法邮箱）
	To string `json:"to" binding:"required,email"`
	// Message 结构化通知消息（必填），经 locale 模板渲染标题与正文后投递
	Message message.Message `json:"message" binding:"required"`
	// Language 渲染使用的目标语言（可空，回退平台默认语言）
	Language string `json:"language"`
}

// mailSender 邮件直发器接口，定义在消费侧并与 internal/pkg/mail 解耦，便于单元测试注入替代实现。
type mailSender interface {
	Send(ctx context.Context, to string, msg message.Message, lang string) error
}

// 编译期断言：确保 MailSendHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*MailSendHandler)(nil)

// MailSendHandler 系统级邮件直发处理器，消费 mail:send 任务。
// 解析邮件载荷后调用 Sender.Send 按系统 smtp 配置直接投递；
// 载荷非法返回错误触发重试。
type MailSendHandler struct {
	// sender 邮件直发器，按系统 smtp 配置直接投递邮件。
	sender mailSender
	// logger 结构化日志记录器。
	logger *logging.Logger
}

// NewMailSendHandler 从 do.Injector 解析依赖并创建系统级邮件直发处理器。
func NewMailSendHandler(i do.Injector) *MailSendHandler {
	return &MailSendHandler{sender: provider.MustInvokeMailSender(i), logger: provider.MustInvokeDatabaseLogger(i)}
}

// Do 消费一条 mail:send 任务：解析 MailSendInput 载荷并调用 Sender.Send 直接投递；
// 载荷非法或投递失败返回错误触发重试。
func (h *MailSendHandler) Do(ctx context.Context, task etl.Task) error {
	var input MailSendInput
	if err := json.Unmarshal(task.Payload(), &input); err != nil {
		return errors.WithStack(err)
	}
	if err := validateMailSendInput(input); err != nil {
		return errors.WithStack(err)
	}
	if err := h.sender.Send(ctx, input.To, input.Message, input.Language); err != nil {
		h.logger.Ctx(ctx).Errorf("mail: send failed (to=%s template=%s): %v", input.To, input.Message.Template, err)
		return errors.WithStack(err)
	}
	return nil
}

// NewMailSendTask 将系统级邮件直发请求编码为 mail:send 异步任务；必填字段缺失或邮箱非法返回错误。
func NewMailSendTask(in MailSendInput) (*asynq.Task, error) {
	if err := validateMailSendInput(in); err != nil {
		return nil, errors.WithStack(err)
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
func validateMailSendInput(in MailSendInput) error {
	return errors.WithStack(validator.Validate.Struct(&in))
}
