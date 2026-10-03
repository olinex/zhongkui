// Package otel（本文件）负责按配置创建 Trace / Metric / Log 的 OTLP 或
// 标准输出导出器。
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
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

var (
	errInvalidProvider     = errors.New("invalid provider")
	errInvalidOTLPProtocol = errors.New("invalid OTLP protocol")
)

// NewTraceExporter 按配置创建链路导出器，支持标准输出与 OTLP（http/grpc）两种方式。
func NewTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	switch Env.Provider {
	case ProviderStdout:
		return stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	case ProviderOtlp:
		switch Env.OtlpProtocol {
		case Http:
			return otlptracehttp.New(ctx)
		case Grpc:
			return otlptracegrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}

// NewMeterExporter 按配置创建指标导出器，支持标准输出与 OTLP（http/grpc）两种方式。
func NewMeterExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	switch Env.Provider {
	case ProviderStdout:
		return stdoutmetric.New(
			stdoutmetric.WithPrettyPrint(),
		)
	case ProviderOtlp:
		switch Env.OtlpProtocol {
		case Http:
			return otlpmetrichttp.New(ctx)
		case Grpc:
			return otlpmetricgrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}

// NewLogExporter 按配置创建日志导出器，支持标准输出与 OTLP（http/grpc）两种方式。
func NewLogExporter(ctx context.Context) (sdklog.Exporter, error) {
	switch Env.Provider {
	case ProviderStdout:
		return stdoutlog.New(
			stdoutlog.WithPrettyPrint(),
		)
	case ProviderOtlp:
		switch Env.OtlpProtocol {
		case Http:
			return otlploghttp.New(ctx)
		case Grpc:
			return otlploggrpc.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	default:
		return nil, errInvalidProvider
	}
}
