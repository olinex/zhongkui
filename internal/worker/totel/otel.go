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
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	ScopeName            = base.GlobalPrefix + "-tasks"
	SpanTaskDo           = "tasks.batch.do"
	SpanTaskExtract      = "tasks.batch.extract"
	SpanTaskTransform    = "tasks.batch.transform"
	SpanTaskLoad         = "tasks.batch.load"
	SpanTaskVersionDo    = "tasks.version.do"
	SpanTaskVersionLoad  = "tasks.version.load"
	SpanTaskVersionStore = "tasks.version.store"
	SpanTaskVersionNext  = "tasks.version.next"
	SpanTaskVersionClear = "tasks.version.clear"
)

var (
	once sync.Once
	// ---------- Metrics ----------
	// taskCounter 统计处理的任务总数，按类型标记
	taskCounter metric.Int64Counter
	// taskErrorCounter 统计任务处理失败次数，按类型标记
	taskErrorCounter metric.Int64Counter
	// itemsExtractedCounter 统计从数据源提取的数据项总数
	itemsExtractedCounter metric.Int64Counter
	// itemsTransformedCounter 统计转换后的数据项总数
	itemsTransformedCounter metric.Int64Counter
	// itemsLoadedCounter 统计加载到目标的数据项总数
	itemsLoadedCounter metric.Int64Counter
	// packetsExtractedCounter 统计从 PCAP 提取的原始数据包总数
	packetsExtractedCounter metric.Int64Counter
)

func InitMetrics() {
	once.Do(initMetrics)
}

func initMetrics() {
	var err, e error
	meter := otel.Meter(ScopeName)

	taskCounter, e = meter.Int64Counter(
		"tasks.processed",
		metric.WithDescription("Total number of tasks processed"),
		metric.WithUnit("{task}"),
	)
	err = errors.Join(err, e)

	taskErrorCounter, e = meter.Int64Counter(
		"tasks.errors",
		metric.WithDescription("Total number of task processing errors"),
		metric.WithUnit("{error}"),
	)
	err = errors.Join(err, e)

	itemsExtractedCounter, e = meter.Int64Counter(
		"tasks.items.extracted",
		metric.WithDescription("Total number of items extracted from data sources"),
		metric.WithUnit("{item}"),
	)
	err = errors.Join(err, e)

	itemsTransformedCounter, e = meter.Int64Counter(
		"tasks.items.transformed",
		metric.WithDescription("Total number of items transformed"),
		metric.WithUnit("{item}"),
	)
	err = errors.Join(err, e)

	itemsLoadedCounter, e = meter.Int64Counter(
		"tasks.items.loaded",
		metric.WithDescription("Total number of items loaded to targets"),
		metric.WithUnit("{item}"),
	)
	err = errors.Join(err, e)

	packetsExtractedCounter, e = meter.Int64Counter(
		"tasks.packets.extracted",
		metric.WithDescription("Total number of raw packets extracted from PCAP files"),
		metric.WithUnit("{packet}"),
	)
	err = errors.Join(err, e)

	if err != nil {
		panic(fmt.Errorf("tasks otel metrics init failed: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Span 辅助函数
// ---------------------------------------------------------------------------

// StartBatchTaskSpan 在批量任务执行时创建 span。
func StartBatchTaskSpan(ctx context.Context, taskType string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskDo,
		trace.WithAttributes(
			attribute.String("task.type", taskType),
		),
	)
	return ctx, span
}

// StartExtractSpan 在数据提取阶段创建 span。
func StartExtractSpan(ctx context.Context, taskType string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskExtract,
		trace.WithAttributes(
			attribute.String("task.type", taskType),
		),
	)
	return ctx, span
}

// StartTransformSpan 在数据转换阶段创建 span。
func StartTransformSpan(ctx context.Context, taskType string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskTransform,
		trace.WithAttributes(
			attribute.String("task.type", taskType),
		),
	)
	return ctx, span
}

// StartLoadSpan 在数据加载阶段创建 span。
func StartLoadSpan(ctx context.Context, taskType string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskLoad,
		trace.WithAttributes(
			attribute.String("task.type", taskType),
		),
	)
	return ctx, span
}

// StartVersionTaskSpan 在版本任务执行时创建 span。
func StartVersionTaskSpan(ctx context.Context, name, lastVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskVersionDo,
		trace.WithAttributes(
			attribute.String("task.name", name),
			attribute.String("task.last_version", lastVersion),
		),
	)
	return ctx, span
}

// StartVersionLoadSpan 在版本任务加载阶段创建 span。
func StartVersionLoadSpan(ctx context.Context, name, lastVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskVersionLoad,
		trace.WithAttributes(
			attribute.String("task.name", name),
			attribute.String("task.last_version", lastVersion),
		),
	)
	return ctx, span
}

// StartVersionStoreSpan 在版本任务存储阶段创建 span。
func StartVersionStoreSpan(ctx context.Context, name, nextVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskVersionStore,
		trace.WithAttributes(
			attribute.String("task.name", name),
			attribute.String("task.next_version", nextVersion),
		),
	)
	return ctx, span
}

// StartVersionNextSpan 在版本任务计算下一版本时创建 span。
func StartVersionNextSpan(ctx context.Context, name, lastVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskVersionNext,
		trace.WithAttributes(
			attribute.String("task.name", name),
			attribute.String("task.last_version", lastVersion),
		),
	)
	return ctx, span
}

// StartVersionClearSpan 在版本任务清理阶段创建 span。
func StartVersionClearSpan(ctx context.Context, name, lastVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(ScopeName)
	ctx, span := tracer.Start(ctx, SpanTaskVersionClear,
		trace.WithAttributes(
			attribute.String("task.name", name),
			attribute.String("task.last_version", lastVersion),
		),
	)
	return ctx, span
}

// ---------------------------------------------------------------------------
// Metrics 记录辅助函数
// ---------------------------------------------------------------------------

// RecordTask 记录任务处理成功。
func RecordTask(ctx context.Context, taskType string) {
	taskCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
}

// RecordTaskError 记录任务处理失败。
func RecordTaskError(ctx context.Context, taskType string) {
	taskErrorCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
}

// RecordItemsExtracted 记录提取到的数据项数量。
func RecordItemsExtracted(ctx context.Context, taskType string, count int) {
	itemsExtractedCounter.Add(ctx, int64(count), metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
}

// RecordItemsTransformed 记录转换后的数据项数量。
func RecordItemsTransformed(ctx context.Context, taskType string, count int) {
	itemsTransformedCounter.Add(ctx, int64(count), metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
}

// RecordItemsLoaded 记录加载的数据项数量。
func RecordItemsLoaded(ctx context.Context, taskType string, count int) {
	itemsLoadedCounter.Add(ctx, int64(count), metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
}

// RecordPacketsExtracted 记录提取的原始数据包数量。
func RecordPacketsExtracted(ctx context.Context, taskType string, count int) {
	packetsExtractedCounter.Add(ctx, int64(count), metric.WithAttributes(
		attribute.String("task.type", taskType),
	))
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
