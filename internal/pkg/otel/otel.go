// Package otel 提供 OpenTelemetry 可观测性基础设施，包括链路追踪 (traces)、
// 指标收集 (metrics) 和日志桥接 (logs)。
//
// 核心功能：
//   - 通过 Config 结构体支持配置文件和环境变量双重配置
//   - 通过 base.OTEL_ENV_VAR 控制整体开关，关闭时使用 noop 实现
//   - 支持 OTLP gRPC 协议将数据导出到 otel-collector 或 otel-front 等后端
//   - 与项目现有的 zap 日志系统通过 otelzap bridge 集成
//
// 典型用法：
//
//	cfg, _ := otel.ExtractConfig(viper)
//	rt, _ := otel.NewRuntime(context.Background(), cfg)
//	defer rt.Shutdown(context.Background())
//	tracer := otel.Tracer("my-component")
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/version"
)

const (
	OTLP_EXPORTER = "otlp"
)

// Runtime 封装了 OpenTelemetry 运行时组件
type Runtime struct {
	cfg            *Config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	logProvider    *sdklog.LoggerProvider
}

// NewRuntime 根据配置创建并初始化 OpenTelemetry 运行时
//
// 当 base.OTEL_ENV_VAR 为 false 时，返回 nil 并使用全局默认的 noop 实现。
// 调用方应在程序退出前调用 Runtime.Shutdown 确保数据完整导出。
func NewRuntime(ctx context.Context, name, namespace, instanceID string, cfg *Config) (*Runtime, error) {
	if !base.OTEL_ENV_VAR {
		return nil, nil
	}

	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.DeploymentEnvironment(base.ENVIRONMENT_VAR),
			semconv.ServiceName(fmt.Sprintf(`%s.%s`, base.GLOBAL_PREFIX, name)),
			semconv.ServiceVersion(version.Short()),
			semconv.ServiceNamespace(namespace),
			semconv.ServiceInstanceID(instanceID),
		),
		sdkresource.WithOSType(),
		sdkresource.WithProcessPID(),
		sdkresource.WithContainerID(),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 初始化 Traces
	tp, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if tp != nil {
		otel.SetTracerProvider(tp)
	}

	// 初始化 Metrics
	mp, err := initMeterProvider(ctx, cfg, res)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
	}

	// 初始化 Logs（通过 OTLP 导出日志）
	lp, err := initLogProvider(ctx, cfg, res)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	rt := &Runtime{cfg: cfg, tracerProvider: tp, meterProvider: mp, logProvider: lp}

	// 设置全局传播器，支持 W3C TraceContext 和 Baggage
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return rt, nil
}

// Shutdown 优雅关闭所有 OpenTelemetry 导出器，刷新待发送的数据
func (rt *Runtime) Shutdown(ctx context.Context) error {
	if rt == nil {
		return nil
	}
	var errs []error
	if rt.tracerProvider != nil {
		if err := rt.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
	}
	if rt.meterProvider != nil {
		if err := rt.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
	}
	if rt.logProvider != nil {
		if err := rt.logProvider.Shutdown(ctx); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func initTracerProvider(ctx context.Context, cfg *Config, res *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	switch cfg.TracesExporter {
	case OTLP_EXPORTER:
		var exp *otlptrace.Exporter
		if cfg.isHTTP() {
			opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlptracehttp.WithInsecure())
			}
			tmp, err := otlptracehttp.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			exp = tmp
		} else {
			opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlptracegrpc.WithInsecure())
			}
			tmp, err := otlptracegrpc.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			exp = tmp
		}
		return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res)), nil
	default:
		return nil, nil
	}
}

func initMeterProvider(ctx context.Context, cfg *Config, res *sdkresource.Resource) (*sdkmetric.MeterProvider, error) {
	switch cfg.MetricsExporter {
	case OTLP_EXPORTER:
		var provider *sdkmetric.MeterProvider
		if cfg.isHTTP() {
			opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlpmetrichttp.WithInsecure())
			}
			exp, err := otlpmetrichttp.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)), sdkmetric.WithResource(res))
		} else {
			opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlpmetricgrpc.WithInsecure())
			}
			exp, err := otlpmetricgrpc.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			provider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)), sdkmetric.WithResource(res))
		}
		if err := runtime.Start(runtime.WithMeterProvider(provider)); err != nil {
			return nil, errors.WithStack(err)
		}
		return provider, nil
	default:
		return nil, nil
	}
}

func initLogProvider(ctx context.Context, cfg *Config, res *sdkresource.Resource) (*sdklog.LoggerProvider, error) {
	switch cfg.LogsExporter {
	case OTLP_EXPORTER:
		if cfg.isHTTP() {
			opts := []otlploghttp.Option{otlploghttp.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlploghttp.WithInsecure())
			}
			exp, err := otlploghttp.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exp))), nil
		} else {
			opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(cfg.OtlpEndpoint)}
			if cfg.OtlpInsecure {
				opts = append(opts, otlploggrpc.WithInsecure())
			}
			exp, err := otlploggrpc.New(ctx, opts...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exp))), nil
		}
	default:
		return nil, nil
	}
}

// WrapLogger 将已有的 *zap.Logger 包装，使其通过 OpenTelemetry 导出日志。
//
// 当 OTEL 未启用或日志导出被禁用时，返回原始 logger 不做任何修改。
// name 参数对应 otelzap 的 logger name，用于在 log.LoggerProvider 中区分不同 logger。
func (rt *Runtime) WrapLogger(logger *zap.Logger, name string) *zap.Logger {
	if rt == nil || rt.logProvider == nil {
		return logger
	}
	core := otelzap.NewCore(name, otelzap.WithLoggerProvider(rt.logProvider))
	return logger.WithOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return zapcore.NewTee(c, core)
	}))
}

// WrapSugaredLogger 将已有的 *logging.Logger (zap.SugaredLogger) 包装，使其通过 OTEL 导出日志。
func (rt *Runtime) WrapSugaredLogger(logger *logging.Logger, name string) *logging.Logger {
	if rt == nil || rt.logProvider == nil {
		return logger
	}
	return rt.WrapLogger(logger.Desugar(), name).Sugar()
}
