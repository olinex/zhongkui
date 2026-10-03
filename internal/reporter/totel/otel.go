// Package totel 提供远程 Reporter 服务的 OpenTelemetry 埋点：scope/span 名称常量、span 辅助函数与指标记录函数。
package totel

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Reporter 服务的 OpenTelemetry scope 与 span 名称常量。
const (
	// ScopeName Reporter 服务的 tracer/meter scope 名称
	ScopeName = base.GlobalPrefix + "-reporter"
	// SpanRun Run 生命周期的 span 名称
	SpanRun = "reporter.run"
	// SpanWatch Watch 流式接口的 span 名称
	SpanWatch = "reporter.watch"
	// SpanFeedback Feedback 流式接口的 span 名称
	SpanFeedback = "reporter.feedback"
)

var (
	once sync.Once
	// ---------- 指标 ----------
	// watchConnectionsCounter 统计 Watch 连接数
	watchConnectionsCounter metric.Int64Counter
	// watchEventsCounter 统计 Watch 推送的事件数，按事件类型标记
	watchEventsCounter metric.Int64Counter
	// feedbackRequestsCounter 统计接收到的 Feedback 请求数
	feedbackRequestsCounter metric.Int64Counter
	// feedbackTasksEnqueuedCounter 统计 Feedback 触发的异步任务数
	feedbackTasksEnqueuedCounter metric.Int64Counter
)

// InitMetrics 初始化 Reporter 服务的 OpenTelemetry 指标，仅执行一次。
func InitMetrics() {
	once.Do(initMetrics)
}

func initMetrics() {
	var err, e error
	meter := otel.Meter(ScopeName)

	watchConnectionsCounter, e = meter.Int64Counter(
		"reporter.watch.connections",
		metric.WithDescription("Total number of Watch stream connections"),
		metric.WithUnit("{connection}"),
	)
	err = errors.Join(err, e)

	watchEventsCounter, e = meter.Int64Counter(
		"reporter.watch.events",
		metric.WithDescription("Total number of Watch config change events pushed"),
		metric.WithUnit("{event}"),
	)
	err = errors.Join(err, e)

	feedbackRequestsCounter, e = meter.Int64Counter(
		"reporter.feedback.requests",
		metric.WithDescription("Total number of Feedback requests received"),
		metric.WithUnit("{request}"),
	)
	err = errors.Join(err, e)

	feedbackTasksEnqueuedCounter, e = meter.Int64Counter(
		"reporter.feedback.tasks_enqueued",
		metric.WithDescription("Total number of async tasks enqueued via Feedback"),
		metric.WithUnit("{task}"),
	)
	err = errors.Join(err, e)

	if err != nil {
		panic(fmt.Errorf("reporter otel metrics init failed: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Span 辅助函数
// ---------------------------------------------------------------------------

// StartRunSpan 在 ReporterService.Run 中创建 span。
func StartRunSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	return tracer.Start(ctx, SpanRun)
}

// StartWatchSpan 在 Watch 流式接口中创建 span。
func StartWatchSpan(ctx context.Context, resourceKey string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanWatch,
		trace.WithAttributes(
			attribute.String("reporter.watch.key", resourceKey),
		),
	)
	return ctx, span
}

// StartFeedbackSpan 在 Feedback 流式接口中创建 span。
func StartFeedbackSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	return tracer.Start(ctx, SpanFeedback)
}

// ---------------------------------------------------------------------------
// Metrics 记录辅助函数
// ---------------------------------------------------------------------------

// RecordWatchConnection 记录一次 Watch 连接建立。
func RecordWatchConnection(ctx context.Context) {
	watchConnectionsCounter.Add(ctx, 1)
}

// RecordWatchEvent 记录一次 Watch 配置变更事件。
func RecordWatchEvent(ctx context.Context, eventType string) {
	watchEventsCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reporter.watch.event_type", eventType),
	))
}

// RecordFeedbackRequest 记录一次 Feedback 请求。
func RecordFeedbackRequest(ctx context.Context) {
	feedbackRequestsCounter.Add(ctx, 1)
}

// RecordFeedbackTaskEnqueued 记录一次 Feedback 触发的异步任务入队。
func RecordFeedbackTaskEnqueued(ctx context.Context) {
	feedbackTasksEnqueuedCounter.Add(ctx, 1)
}

// ---------------------------------------------------------------------------
// 通用 span 工具
// ---------------------------------------------------------------------------

// EndSpan 安全结束 span，并在发生错误时记录错误状态。
func EndSpan(span trace.Span, errPtr *error) {
	if span == nil {
		return
	}
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
	}
	span.End()
}
