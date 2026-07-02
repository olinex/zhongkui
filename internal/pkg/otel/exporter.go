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
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	errInvalidProvider     = errors.New("invalid provider")
	errInvalidOTLPProtocol = errors.New("invalid OTLP protocol")
)

func NewTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	switch Env.Provider {
	case PROVIDER_STDOUT:
		return stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	case PROVIDER_OTLP:
		switch Env.OtlpProtocol {
		case HTTP:
			return otlptracehttp.New(ctx)
		case GRPC:
			return otlptracegrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}

func NewMeterExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	switch Env.Provider {
	case PROVIDER_STDOUT:
		return stdoutmetric.New(
			stdoutmetric.WithPrettyPrint(),
		)
	case PROVIDER_OTLP:
		switch Env.OtlpProtocol {
		case HTTP:
			return otlpmetrichttp.New(ctx)
		case GRPC:
			return otlpmetricgrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}

func NewLogExporter(ctx context.Context) (sdklog.Exporter, error) {
	switch Env.Provider {
	case PROVIDER_STDOUT:
		return stdoutlog.New(
			stdoutlog.WithPrettyPrint(),
		)
	case PROVIDER_OTLP:
		switch Env.OtlpProtocol {
		case HTTP:
			return otlploghttp.New(ctx)
		case GRPC:
			return otlploggrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}
