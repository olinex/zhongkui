package otel

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	g "google.golang.org/grpc"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	otelgrpc "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// GrpcServerOptions 返回 gRPC Server 的 OTEL stats handler 选项。
//
// 当 Runtime 为 nil 时返回空切片。
func (rt *Runtime) GrpcServerOptions() []g.ServerOption {
	if rt == nil {
		return nil
	}
	return []g.ServerOption{
		g.StatsHandler(otelgrpc.NewServerHandler()),
	}
}

// GrpcClientOptions 返回 gRPC Client 的 OTEL stats handler 拨号选项。
//
// 当 Runtime 为 nil 时返回空切片。
func (rt *Runtime) GrpcClientOptions() []g.DialOption {
	if rt == nil {
		return nil
	}
	return []g.DialOption{
		g.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}

// GormPlugin 返回 GORM 的 OTEL 插件，自动为数据库操作创建 trace span。
//
// 当 Runtime 为 nil 时返回 nil。
func (rt *Runtime) GormPlugin() gorm.Plugin {
	if rt == nil {
		return nil
	}
	return tracing.NewPlugin()
}

// AsynqMiddleware 返回 Asynq 任务处理中间件，为每个任务执行创建 trace span。
//
// span name 格式为 "asynq.task/{taskType}"，包含 taskType，taskID 等 attribute。
// 当 Runtime 为 nil 时返回 nil。
func (rt *Runtime) AsynqMiddleware() asynq.MiddlewareFunc {
	if rt == nil {
		return nil
	}
	tracer := otel.Tracer("zhongkui-asynq")
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			spanName := fmt.Sprintf("asynq.task/%s", task.Type())
			ctx, span := tracer.Start(ctx, spanName)
			defer span.End()

			span.SetAttributes(
				attribute.String("asynq.task_type", task.Type()),
				attribute.String("asynq.task_id", task.ResultWriter().TaskID()),
				attribute.Int("asynq.task_payload_size", len(task.Payload())),
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

// EtcdDialOptions 返回 etcd 客户端使用的 OTEL gRPC 拨号选项。
//
// etcd client/v3 底层使用 gRPC 通信，通过注入 otelgrpc 拦截器实现链路追踪。
// 当 Runtime 为 nil 时返回空切片。
func (rt *Runtime) EtcdDialOptions() []g.DialOption {
	if rt == nil {
		return nil
	}
	return []g.DialOption{
		g.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
}
