package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

type OtelShutdowner struct {
	shutdown func(context.Context) error
}

func (s *OtelShutdowner) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
}

func otelShutdownerProvider(ctx context.Context) do.Provider[*OtelShutdowner] {
	return func(i do.Injector) (*OtelShutdowner, error) {
		shutdown, err := otel.Setup(ctx)
		if err != nil {
			return nil, err
		}
		return &OtelShutdowner{shutdown: shutdown}, nil
	}
}

func NewOtelShutdowner(ctx context.Context) do.Provider[*OtelShutdowner] {
	return otelShutdownerProvider(ctx)
}

func InjectOtelShutdowner(ctx context.Context, i do.Injector) {
	do.Provide(i, NewOtelShutdowner(ctx))
}

func InvokeOtelShutdowner(ctx context.Context, i do.Injector) (*OtelShutdowner, error) {
	return do.Invoke[*OtelShutdowner](i)
}

func MustInvokeOtelShutdowner(ctx context.Context, i do.Injector) *OtelShutdowner {
	return do.MustInvoke[*OtelShutdowner](i)
}
