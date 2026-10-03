// Package otel（本文件）提供 OpenTelemetry 的初始化入口：装配上下文传播器、
// 资源及各 Provider，并返回统一的关闭函数。
package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// Setup 初始化 OpenTelemetry，注册文本映射传播器并设置各类 Provider，
// 返回用于释放资源的关闭函数。未启用时仅返回空关闭函数。
func Setup(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown 会调用所有注册的清理函数。
	// 所有返回的错误都会被合并到一起。
	// 每个注册的清理函数仅会被调用一次。
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	if !Env.Enabled {
		return shutdown, nil
	}

	// handleErr 用户调用 shutdown 并合并返回的错误。
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// 设置上下文传播器（用于跨服务传递追踪信息）。
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// 初始化资源
	res, err := NewResource(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}

	// 初始化 tracer provider
	tracerExporter, err := NewTraceExporter(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	tracerProvider := NewTraceProvider(tracerExporter, res)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	shutdownFuncs = append(shutdownFuncs, tracerExporter.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// 初始化 meter provider
	meterExporter, err := NewMeterExporter(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	meterProvider := NewMeterProvider(meterExporter, res)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	shutdownFuncs = append(shutdownFuncs, meterExporter.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// 初始化 log provider
	logExporter, err := NewLogExporter(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	logProvider := NewLogProvider(logExporter, res)
	if err != nil {
		handleErr(err)
		return shutdown, errors.WithStack(err)
	}
	shutdownFuncs = append(shutdownFuncs, logExporter.Shutdown)
	global.SetLoggerProvider(logProvider)

	return shutdown, nil
}
