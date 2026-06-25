package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

// newOtelProvider 创建 OTEL Runtime 的依赖注入提供者
//
// 当 base.OTEL_ENV_VAR 为 false 时，返回 (nil, nil)，调用方应判空处理。
// serviceName 用于覆写配置中的 ServiceName，传入空字符串则使用配置默认值。
func newOtelProvider(ctx context.Context, serviceName string) do.Provider[*otel.Runtime] {
	return func(i do.Injector) (*otel.Runtime, error) {
		v, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := otel.ExtractConfig(v)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if serviceName != "" {
			cfg.ServiceName = serviceName
		}
		rt, err := otel.NewRuntime(ctx, cfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return rt, nil
	}
}

// InjectOtel 将 OTEL Runtime 注入到 DI 容器
//
// 应在 PersistentPreRunE 中调用，确保在其他需要可观测性的组件之前完成初始化。
// ctx 用于 OTLP 导出器的 gRPC 连接建立。
// serviceName 用于设置 OTEL 服务名，便于在可观测平台中区分不同子进程。
func InjectOtel(ctx context.Context, i do.Injector, serviceName string) {
	do.Provide(i, newOtelProvider(ctx, serviceName))
}

// InvokeOtel 从 DI 容器获取 OTEL Runtime
//
// 当 OTEL 未启用时返回 (nil, nil)。
func InvokeOtel(i do.Injector) (*otel.Runtime, error) {
	return do.Invoke[*otel.Runtime](i)
}

// MustInvokeOtel 从 DI 容器获取 OTEL Runtime，失败时 panic
//
// 当 OTEL 未启用时返回 nil。
func MustInvokeOtel(i do.Injector) *otel.Runtime {
	return do.MustInvoke[*otel.Runtime](i)
}
