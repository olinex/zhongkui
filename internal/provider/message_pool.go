package provider

import (
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/pkg/message"
)

// newMessagePoolProvider 创建进程级消息渠道连接池单例。
// newMessagePoolProvider creates the process-wide message channel pool singleton.
func newMessagePoolProvider() do.Provider[*message.Pool] {
	return func(_ do.Injector) (*message.Pool, error) {
		return message.NewPool(), nil
	}
}

// InjectMessagePool 注册消息渠道连接池 provider。
// *message.Pool 自身实现 do.ShutdownerWithContextAndError（Shutdown 转发
// CloseAll），故启动期经 MustInvokeMessagePool 预解析后，优雅关闭阶段由
// samber/do 直接驱动池自身关闭，无需额外的关闭器 provider。
// InjectMessagePool registers the message channel pool provider.
// *message.Pool itself implements do.ShutdownerWithContextAndError (Shutdown
// forwards to CloseAll), so once eagerly resolved via MustInvokeMessagePool at
// startup, samber/do drives the pool's own shutdown during graceful shutdown
// without an extra closer provider.
func InjectMessagePool(i do.Injector) {
	do.Provide(i, newMessagePoolProvider())
}

// InvokeMessagePool 取得消息渠道连接池（供通知投递 / 邮件直发 / 测试发送使用）。
// InvokeMessagePool resolves the message channel pool (used by notification
// delivery, system email direct-send and test-send).
func InvokeMessagePool(i do.Injector) (*message.Pool, error) {
	return do.Invoke[*message.Pool](i)
}

// MustInvokeMessagePool panic 版本取得消息渠道连接池
// （用于启动期预解析，确保池被实例化后其 Shutdown 在优雅关闭阶段被 samber/do 驱动）。
// MustInvokeMessagePool is the panicking variant to resolve the message channel
// pool (eagerly resolved at startup so its Shutdown is driven during samber/do
// graceful shutdown).
func MustInvokeMessagePool(i do.Injector) *message.Pool {
	return do.MustInvoke[*message.Pool](i)
}
