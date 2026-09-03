package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/message"
)

// MessagePoolShutdowner 在进程优雅关闭时关闭消息渠道资源池，
// 释放全部存活连接（如 SMTP 长连接），作为 DI 生命周期服务接入 do 的关闭阶段。
// MessagePoolShutdowner closes the message channel pools on graceful process
// shutdown, releasing all live connections (e.g. SMTP long connections), and is
// wired into do's shutdown phase as a DI lifecycle service.
type MessagePoolShutdowner struct{}

// Shutdown 关闭全部消息渠道资源池并销毁存活连接。
// Shutdown closes all message channel pools and destroys live connections.
func (s *MessagePoolShutdowner) Shutdown(ctx context.Context) error {
	message.ClosePools(ctx)
	return nil
}

// newMessagePoolShutdownerProvider 提供进程级单例的渠道资源池关闭器（关闭操作幂等）。
// newMessagePoolShutdownerProvider provides a process-wide singleton channel-pool
// closer (the underlying close is idempotent).
func newMessagePoolShutdownerProvider() do.Provider[*MessagePoolShutdowner] {
	return func(_ do.Injector) (*MessagePoolShutdowner, error) {
		return &MessagePoolShutdowner{}, nil
	}
}

// InjectMessagePoolShutdowner 注册渠道资源池关闭器 provider。
// InjectMessagePoolShutdowner registers the channel-pool closer provider.
func InjectMessagePoolShutdowner(i do.Injector) {
	do.Provide(i, newMessagePoolShutdownerProvider())
}

// InvokeMessagePoolShutdowner 取得渠道资源池关闭器（用于启动期解析）。
// InvokeMessagePoolShutdowner resolves the channel-pool closer (for eager resolve at startup).
func InvokeMessagePoolShutdowner(i do.Injector) (*MessagePoolShutdowner, error) {
	return do.Invoke[*MessagePoolShutdowner](i)
}

// MustInvokeMessagePoolShutdowner panic 版本取得渠道资源池关闭器。
// MustInvokeMessagePoolShutdowner is the panicking variant to resolve the closer.
func MustInvokeMessagePoolShutdowner(i do.Injector) *MessagePoolShutdowner {
	return do.MustInvoke[*MessagePoolShutdowner](i)
}