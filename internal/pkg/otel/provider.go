package otel

import (
	"go.opentelemetry.io/otel/sdk/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewTraceProvider(exp sdktrace.SpanExporter, res *sdkresource.Resource) *sdktrace.TracerProvider {
	sampler := NewSampler()
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	if Env.SamplerType == SAMPLER_TAIL {
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

func NewMeterProvider(exp sdkmetric.Exporter, res *sdkresource.Resource) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exp)), metric.WithResource(res))
}

func NewLogProvider(exp sdklog.Exporter, res *sdkresource.Resource) *sdklog.LoggerProvider {
	return sdklog.NewLoggerProvider(log.WithProcessor(log.NewBatchProcessor(exp)), sdklog.WithResource(res))
}
