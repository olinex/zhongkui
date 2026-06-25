// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
package pipeline

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// tracedClient 是 asynq.Client 的追踪包装器，为 Enqueue 和 EnqueueContext 添加 OpenTelemetry span。
type tracedClient struct {
	*asynq.Client
}

// newTracedClient 创建一个新的追踪客户端包装器。
func newTracedClient(cli *asynq.Client) *tracedClient {
	return &tracedClient{Client: cli}
}

// EnqueueContext 包装 asynq.Client.EnqueueContext，为带上下文的任务入队创建追踪 span。
func (c *tracedClient) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	spanName := fmt.Sprintf(`%s.enqueue.%s`, PREFIX, task.Type())
	_, span := otel.Tracer(fmt.Sprintf(`%s.%s`, base.GLOBAL_PREFIX, PREFIX)).Start(ctx, spanName)
	defer span.End()

	span.SetAttributes(
		attribute.String(fmt.Sprintf(`%s.task_type`, PREFIX), task.Type()),
		attribute.Int(fmt.Sprintf(`%s.task_payload_size`, PREFIX), len(task.Payload())),
	)

	info, err := c.Client.EnqueueContext(ctx, task, opts...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return info, err
}
