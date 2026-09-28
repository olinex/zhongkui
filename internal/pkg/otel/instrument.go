package otel

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	g "google.golang.org/grpc"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/redis"
	"zhongkui.com/zhongkui/internal/pkg/s3"
)

// AsynqServerMiddleware 返回 Asynq 任务处理中间件，为每个任务执行创建 trace span。
//
// span name 格式为 "asynq.task/{taskType}"，包含 taskType，taskID 等 attribute。
// 当 Runtime 为 nil 时返回 nil。
func AsynqServerMiddleware() asynq.MiddlewareFunc {
	if !Env.Enabled {
		return func(next asynq.Handler) asynq.Handler { return next }
	}
	tracer := otel.Tracer(fmt.Sprintf(`%s-asynq-server`, base.GlobalPrefix))
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			spanName := fmt.Sprintf(`task.process.%s`, task.Type())
			ctx, span := tracer.Start(ctx, spanName)
			defer span.End()

			span.SetAttributes(
				attribute.String("task.id", task.ResultWriter().TaskID()),
				attribute.Int("task.payload_size", len(task.Payload())),
			)

			err := next.ProcessTask(ctx, task)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return err
		})
	}
}

// TracedAsynqClient 包装 asynq.Client，在任务入队时自动创建追踪 span。
type TracedAsynqClient struct {
	*asynq.Client
	tracer trace.Tracer
}

// WrapAsynqClient 把 asynq.Client 包装为带追踪能力的 TracedAsynqClient；OTEL 未启用时返回 nil。
func WrapAsynqClient(cli *asynq.Client) *TracedAsynqClient {
	if !Env.Enabled {
		return nil
	}
	return &TracedAsynqClient{
		Client: cli,
		tracer: otel.Tracer(fmt.Sprintf(`%s-asynq-client`, base.GlobalPrefix)),
	}
}

// EnqueueContext 包装 asynq.Client.EnqueueContext，为带上下文的任务入队创建追踪 span。
func (c *TracedAsynqClient) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	spanName := fmt.Sprintf(`task.enqueue.%s`, task.Type())
	_, span := c.tracer.Start(ctx, spanName)
	defer span.End()

	info, err := c.Client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return info, err
	}
	span.SetAttributes(
		attribute.String("task.id", info.ID),
		attribute.String("task.group", info.Group),
		attribute.String("task.queue", info.Queue),
		attribute.String("task.state", info.State.String()),
		attribute.String("task.timeout", info.Timeout.String()),
		attribute.String("task.deadline", info.Deadline.Format(time.RFC3339)),
		attribute.String("task.completed_at", info.CompletedAt.Format(time.RFC3339)),
		attribute.Bool("task.is_orphaned", info.IsOrphaned),
		attribute.Int("task.max_retry", info.MaxRetry),
		attribute.Int("task.retried", info.Retried),
		attribute.Int("task.payload_size", len(info.Payload)),
	)
	return info, err
}

// InjectRedisClient 为 Redis 客户端注入 OpenTelemetry 的链路追踪与指标采集；OTEL 未启用时不作处理。
func InjectRedisClient(client *redis.Client) error {
	if !Env.Enabled {
		return nil
	}
	if err := redisotel.InstrumentTracing(client.Client); err != nil {
		return errors.WithStack(err)
	}
	if err := redisotel.InstrumentMetrics(client.Client); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// contextStrippingCore 从 zap 字段中过滤掉 context.Context 类型的值，
// 避免这些字段被序列化到 stdout/控制台输出中产生冗余信息。
// context 仅保留给 otelzap core 用于 trace 关联。
type contextStrippingCore struct {
	zapcore.Core
}

func (c *contextStrippingCore) With(fields []zapcore.Field) zapcore.Core {
	return &contextStrippingCore{c.Core.With(stripContextFields(fields))}
}

func (c *contextStrippingCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return c.Core.Check(ent, ce)
}

func (c *contextStrippingCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	return c.Core.Write(ent, stripContextFields(fields))
}

func stripContextFields(fields []zapcore.Field) []zapcore.Field {
	if len(fields) == 0 {
		return fields
	}
	filtered := make([]zapcore.Field, 0, len(fields))
	for _, f := range fields {
		if _, ok := f.Interface.(context.Context); ok {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// WrapLogger 将已有的 *zap.Logger 包装，使其通过 OpenTelemetry 导出日志。
//
// 当 OTEL 未启用或日志导出被禁用时，返回原始 logger 不做任何修改。
// name 参数对应 otelzap 的 logger name，用于在 log.LoggerProvider 中区分不同 logger。
func WrapLogger(logger *zap.Logger, name string) *zap.Logger {
	if !Env.Enabled {
		return logger
	}
	otelCore := otelzap.NewCore(name, otelzap.WithLoggerProvider(global.GetLoggerProvider()))
	// 原始 console core 用 contextStrippingCore 包裹，去掉 context.Context 字段避免 stdout 冗余
	consoleCore := &contextStrippingCore{logger.Core()}
	return logger.WithOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core {
		return zapcore.NewTee(consoleCore, otelCore)
	}))
}

// WrapSugaredLogger 将已有的 *logging.Logger 包装，使其通过 OTEL 导出日志。
// 同时启用 logging.EnableCtx()，使 logger.Ctx(ctx) 将 context 注入日志字段供 otelzap trace 关联。
func WrapSugaredLogger(logger *logging.Logger, name string) *logging.Logger {
	if !Env.Enabled {
		return logger
	}
	logging.EnableCtx()
	wrapped := WrapLogger(logger.Desugar(), name).Sugar()
	return &logging.Logger{SugaredLogger: wrapped}
}

// GormPlugin 返回 GORM 的 OTEL 插件，自动为数据库操作创建 trace span。
//
// 当 Runtime 为 nil 时返回 nil。
func GormPlugin() gorm.Plugin {
	if !Env.Enabled {
		return nil
	}
	return tracing.NewPlugin()
}

// GinMiddleware 返回基于 otelgin 的 Gin 中间件，为 HTTP 请求创建追踪 span；OTEL 未启用时返回 nil。
func GinMiddleware(name string) gin.HandlerFunc {
	if !Env.Enabled {
		return nil
	}
	return otelgin.Middleware(name)
}

// GrpcServerOptions 返回 gRPC Server 的 OTEL stats handler 选项。
//
// 当 Runtime 为 nil 时返回空切片。
func GrpcServerOptions() []g.ServerOption {
	if !Env.Enabled {
		return nil
	}
	return []g.ServerOption{
		g.StatsHandler(otelgrpc.NewServerHandler()),
	}
}

// GrpcClientOptions 返回 gRPC Client 的 OTEL stats handler 拨号选项。
//
// 当 Runtime 为 nil 时返回空切片。
func GrpcClientOptions() []g.DialOption {
	if !Env.Enabled {
		return nil
	}
	return []g.DialOption{
		g.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}

// EtcdDialOptions 返回 etcd 客户端使用的 OTEL gRPC 拨号选项。
//
// etcd client/v3 底层使用 gRPC 通信，通过注入 otelgrpc 拦截器实现链路追踪。
// 当 Runtime 为 nil 时返回空切片。
func EtcdDialOptions() []g.DialOption {
	if !Env.Enabled {
		return nil
	}
	return []g.DialOption{
		g.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}

// S3ClientOptions 返回 S3 客户端的 OTEL 追踪选项（经自定义 RoundTripper 埋点）；OTEL 未启用时返回 nil。
func S3ClientOptions() []s3.ClientOption {
	if !Env.Enabled {
		return nil
	}
	return []s3.ClientOption{
		s3.WithTransport(s3.NewOtelRoundTripper(http.DefaultTransport)),
	}
}
