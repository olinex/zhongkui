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
	return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
}

func NewMeterProvider(exp sdkmetric.Exporter, res *sdkresource.Resource) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exp)), metric.WithResource(res))
}

func NewLogProvider(exp sdklog.Exporter, res *sdkresource.Resource) *sdklog.LoggerProvider {
	return sdklog.NewLoggerProvider(log.WithProcessor(log.NewBatchProcessor(exp)), sdklog.WithResource(res))
}
