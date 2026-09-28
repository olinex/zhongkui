// Package otel（本文件）负责构建 OpenTelemetry 的 Tracer / Meter / Logger Provider，
// 并依据采样器类型组装相应的 Span 处理器。
package otel

import (
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewTraceProvider 创建链路追踪 Provider，按采样器类型装配批处理或尾部过滤处理器。
func NewTraceProvider(exp sdktrace.SpanExporter, res *sdkresource.Resource) *sdktrace.TracerProvider {
	sampler := NewSampler()
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	if Env.SamplerType == SamplerTail {
		// 尾部过滤模式：创建 filter processor 包裹 batch processor，
		// 只有通过过滤条件的 Span 才会进入批处理并导出
		batchProcessor := sdktrace.NewBatchSpanProcessor(exp)
		filterProcessor := NewFilterSpanProcessor(batchProcessor)
		opts = append(opts, sdktrace.WithSpanProcessor(filterProcessor))
	} else {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	return sdktrace.NewTracerProvider(opts...)
}

// NewMeterProvider 创建指标 Provider，使用周期性导出器上报指标。
func NewMeterProvider(exp sdkmetric.Exporter, res *sdkresource.Resource) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)), sdkmetric.WithResource(res))
}

// NewLogProvider 创建日志 Provider，使用批量处理器导出日志。
func NewLogProvider(exp sdklog.Exporter, res *sdkresource.Resource) *sdklog.LoggerProvider {
	return sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)), sdklog.WithResource(res))
}
