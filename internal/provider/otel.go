package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/otel"
)

// OtelShutdowner 封装 OpenTelemetry 的关闭函数，供依赖注入器统一管理其生命周期。
type OtelShutdowner struct {
	// shutdown OpenTelemetry 关闭函数，由 Shutdown 调用以释放资源。
	shutdown func(context.Context) error
}

// Shutdown 调用底层关闭函数，释放 OpenTelemetry 相关资源。
func (s *OtelShutdowner) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
}

func otelShutdownerProvider(ctx context.Context) do.Provider[*OtelShutdowner] {
	return func(i do.Injector) (*OtelShutdowner, error) {
		shutdown, err := otel.Setup(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &OtelShutdowner{shutdown: shutdown}, nil
	}
}

// NewOtelShutdowner 返回 OpenTelemetry 关闭器的依赖注入 provider 工厂。
func NewOtelShutdowner(ctx context.Context) do.Provider[*OtelShutdowner] {
	return otelShutdownerProvider(ctx)
}

// InjectOtelShutdowner 向依赖注入器注册 OpenTelemetry 关闭器 provider。
func InjectOtelShutdowner(ctx context.Context, i do.Injector) {
	do.Provide(i, NewOtelShutdowner(ctx))
}

// InvokeOtelShutdowner 从依赖注入器解析 OpenTelemetry 关闭器，错误由调用方处理。
func InvokeOtelShutdowner(ctx context.Context, i do.Injector) (*OtelShutdowner, error) {
	return do.Invoke[*OtelShutdowner](i)
}

// MustInvokeOtelShutdowner 从依赖注入器解析 OpenTelemetry 关闭器，失败时 panic（仅用于启动关键依赖）。
func MustInvokeOtelShutdowner(ctx context.Context, i do.Injector) *OtelShutdowner {
	return do.MustInvoke[*OtelShutdowner](i)
}
