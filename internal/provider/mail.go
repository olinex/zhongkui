package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/mail"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// newMailSenderProvider 提供进程级单例的系统邮件发送器：直接在此 provider 内从 Viper
// 提取并校验 [smtp] 配置，再构造发送器。 [smtp] 配置不作为独立注入项暴露，避免被
// 其他依赖直接读取而破坏配置封装（可用性判断已下沉到 mail.Sender.Enabled）。
// newMailSenderProvider provides a process-wide singleton of the system mail sender: it
// extracts and validates the [smtp] config from Viper directly inside this provider, then
// builds the sender. The [smtp] config is not exposed as a standalone injectable, so other
// dependencies cannot read it directly and break the config encapsulation (the availability
// check is already lowered into mail.Sender.Enabled).
func newMailSenderProvider() do.Provider[*mail.Sender] {
	return func(i do.Injector) (*mail.Sender, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := mail.ExtractConfig(viper, validator.Validate)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return mail.NewSender(cfg, MustInvokeDatabaseLogger(i)), nil
	}
}

// InjectMailSender 注册系统邮件发送器 provider。
// InjectMailSender registers the system mail sender provider.
func InjectMailSender(i do.Injector) {
	do.Provide(i, newMailSenderProvider())
}

// InvokeMailSender 取得系统邮件发送器（用于启动期解析）。
// InvokeMailSender resolves the system mail sender (for eager resolve at startup).
func InvokeMailSender(i do.Injector) (*mail.Sender, error) {
	return do.Invoke[*mail.Sender](i)
}

// MustInvokeMailSender panic 版本取得系统邮件发送器。
// MustInvokeMailSender is the panicking variant to resolve the system mail sender.
func MustInvokeMailSender(i do.Injector) *mail.Sender {
	return do.MustInvoke[*mail.Sender](i)
}

// DescribeMailConfig 返回系统邮件直发（[smtp]）配置的结构化描述信息（供 describe config 渲染）。
// DescribeMailConfig returns the structured description of the system mail ([smtp])
// config (rendered by describe config).
func DescribeMailConfig() config.ConfigDescriptions {
	return mail.DescribeConfig()
}
