package otel

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var errInvalidOTLPProtocol = errors.New("invalid OTLP protocol")

func NewTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if base.Env.Debug {
		return stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	}
	switch Env.OtlpProtocol {
	case HTTP:
		return otlptracehttp.New(ctx)
	case GRPC:
		return otlptracegrpc.New(ctx)
	default:
		return nil, errInvalidOTLPProtocol
	}
}

func NewMeterExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	if base.Env.Debug {
		return stdoutmetric.New(
			stdoutmetric.WithPrettyPrint(),
		)
	}
	switch Env.OtlpProtocol {
	case HTTP:
		return otlpmetrichttp.New(ctx)
	case GRPC:
		return otlpmetricgrpc.New(ctx)
	default:
		return nil, errInvalidOTLPProtocol
	}
}

func NewLogExporter(ctx context.Context) (sdklog.Exporter, error) {
	if base.Env.Debug {
		return stdoutlog.New(
			stdoutlog.WithPrettyPrint(),
		)
	}
	switch Env.OtlpProtocol {
	case HTTP:
		return otlploghttp.New(ctx)
	case GRPC:
		return otlploggrpc.New(ctx)
	default:
		return nil, errInvalidOTLPProtocol
	}
}
