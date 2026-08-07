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
	ScopeName       = base.GlobalPrefix + "-tasks"
	SpanTaskDo      = "tasks.batch.do"
	SpanTaskExtract = "tasks.batch.extract"
)

var (
	once sync.Once
	// ---------- Metrics ----------
	// taskCounter 统计处理的任务总数，按类型标记
	taskCounter metric.Int64Counter
	// taskErrorCounter 统计任务处理失败次数，按类型标记
	taskErrorCounter metric.Int64Counter
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
