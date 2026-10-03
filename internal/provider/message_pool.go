package provider

import (
	"github.com/samber/do/v2"

	"zhongkuisec.com/zhongkui/internal/pkg/message"
)

// newMessagePoolProvider 创建进程级消息渠道连接池单例。
func newMessagePoolProvider() do.Provider[*message.Pool] {
	return func(_ do.Injector) (*message.Pool, error) {
		return message.NewPool(), nil
	}
}

// InjectMessagePool 注册消息渠道连接池 provider。
// *message.Pool 自身实现 do.ShutdownerWithContextAndError（Shutdown 转发
// CloseAll），故启动期经 MustInvokeMessagePool 预解析后，优雅关闭阶段由
// samber/do 直接驱动池自身关闭，无需额外的关闭器 provider。
func InjectMessagePool(i do.Injector) {
	do.Provide(i, newMessagePoolProvider())
}

// InvokeMessagePool 取得消息渠道连接池（供通知投递 / 邮件直发 / 测试发送使用）。
func InvokeMessagePool(i do.Injector) (*message.Pool, error) {
	return do.Invoke[*message.Pool](i)
}

// MustInvokeMessagePool panic 版本取得消息渠道连接池
// （用于启动期预解析，确保池被实例化后其 Shutdown 在优雅关闭阶段被 samber/do 驱动）。
func MustInvokeMessagePool(i do.Injector) *message.Pool {
	return do.MustInvoke[*message.Pool](i)
}
