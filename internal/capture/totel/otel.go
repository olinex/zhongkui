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
	SCOPE_NAME              = base.GLOBAL_PREFIX + "-capture"
	SPAN_RUN                = "capture.run"
	SPAN_WATCH              = "capture.watch"
	SPAN_FEEDBACK           = "capture.feedback"
	SPAN_UPLOAD             = "capture.upload"
	SPAN_PACKET_HANDLER     = "capture.packet_handler"
	SPAN_CAPTURE            = "capture.capture"
	SPAN_DUMP               = "capture.dump"
	SPAN_DUMP_ONE           = "capture.dump_one"
	SPAN_UPLOAD_FILE        = "capture.upload_file"
	SPAN_UPLOAD_ONE         = "capture.upload_one"
)

var (
	once sync.Once
	// ---------- Metrics ----------
	// segmentsDumpedCounter 统计 dump 出的数据段文件数量
	segmentsDumpedCounter metric.Int64Counter
	// segmentsUploadedCounter 统计成功上传到 S3 的数据段数量
	segmentsUploadedCounter metric.Int64Counter
	// packetsCapturedCounter 统计捕获的原始数据包数量
	packetsCapturedCounter metric.Int64Counter
	// bytesCapturedCounter 统计捕获的数据字节数
	bytesCapturedCounter metric.Int64Counter
	// dumpErrorCounter 统计 dump 阶段错误次数
	dumpErrorCounter metric.Int64Counter
	// uploadErrorCounter 统计 upload 阶段错误次数
	uploadErrorCounter metric.Int64Counter
	// processingDuration 统计各阶段处理耗时分布
	processingDuration metric.Int64Histogram
)

func InitMetrics() {
	once.Do(initMetrics)
}

func initMetrics() {
	var err, e error
	meter := otel.Meter(SCOPE_NAME)

	segmentsDumpedCounter, e = meter.Int64Counter(
		"capture.segments.dumped",
		metric.WithDescription("Total number of capture file segments dumped"),
		metric.WithUnit("{segment}"),
	)
	err = errors.Join(err, e)

	segmentsUploadedCounter, e = meter.Int64Counter(
		"capture.segments.uploaded",
		metric.WithDescription("Total number of capture file segments uploaded to S3"),
		metric.WithUnit("{segment}"),
	)
	err = errors.Join(err, e)

	packetsCapturedCounter, e = meter.Int64Counter(
		"capture.packets.captured",
		metric.WithDescription("Total number of raw packets captured"),
		metric.WithUnit("{packet}"),
	)
	err = errors.Join(err, e)

	bytesCapturedCounter, e = meter.Int64Counter(
		"capture.bytes.captured",
		metric.WithDescription("Total number of bytes captured"),
		metric.WithUnit("By"),
	)
	err = errors.Join(err, e)

	dumpErrorCounter, e = meter.Int64Counter(
		"capture.dump.errors",
		metric.WithDescription("Total number of dump phase errors"),
		metric.WithUnit("{error}"),
	)
	err = errors.Join(err, e)

	uploadErrorCounter, e = meter.Int64Counter(
		"capture.upload.errors",
		metric.WithDescription("Total number of upload phase errors"),
		metric.WithUnit("{error}"),
	)
	err = errors.Join(err, e)

	processingDuration, e = meter.Int64Histogram(
		"capture.processing.duration",
		metric.WithDescription("Duration of capture processing stages in milliseconds"),
		metric.WithUnit("ms"),
	)
	err = errors.Join(err, e)

	if err != nil {
		panic(fmt.Errorf("capture otel metrics init failed: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Span 辅助函数
// ---------------------------------------------------------------------------

// StartRunSpan 在 CaptureClient.Run 中创建 span。
func StartRunSpan(ctx context.Context, resource string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_RUN,
		trace.WithAttributes(
			attribute.String("capture.resource", resource),
		),
	)
	return ctx, span
}

// StartWatchSpan 在 watch 监控循环中创建 span。
func StartWatchSpan(ctx context.Context, resource string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_WATCH,
		trace.WithAttributes(
			attribute.String("capture.resource", resource),
		),
	)
	return ctx, span
}

// StartFeedbackSpan 在 feedback 流中创建 span。
func StartFeedbackSpan(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_FEEDBACK,
		trace.WithAttributes(
			attribute.String("capture.session_id", sessionID),
		),
	)
	return ctx, span
}

// StartUploadSpan 在 upload 生命周期管理中创建 span。
func StartUploadSpan(ctx context.Context, resource, sessionID string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_UPLOAD,
		trace.WithAttributes(
			attribute.String("capture.resource", resource),
			attribute.String("capture.session_id", sessionID),
		),
	)
	return ctx, span
}

// StartPacketHandlerSpan 在 uploadPacketHandler 中创建 span。
func StartPacketHandlerSpan(ctx context.Context, device string, configVersion int64) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_PACKET_HANDLER,
		trace.WithAttributes(
			attribute.String("capture.device", device),
			attribute.Int64("capture.config_version", configVersion),
		),
	)
	return ctx, span
}

// StartCaptureSpan 在 capturer.capture 中创建 span。
func StartCaptureSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	return tracer.Start(ctx, SPAN_CAPTURE)
}

// StartDumpSpan 在 dump 循环中创建 span。
func StartDumpSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	return tracer.Start(ctx, SPAN_DUMP)
}

// StartDumpOneSpan 在 dumpOne 中创建 span。
func StartDumpOneSpan(ctx context.Context, filePath string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_DUMP_ONE,
		trace.WithAttributes(
			attribute.String("capture.file_path", filePath),
		),
	)
	return ctx, span
}

// StartUploadFileSpan 在 upload 循环中创建 span。
func StartUploadFileSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	return tracer.Start(ctx, SPAN_UPLOAD_FILE)
}

// StartUploadOneSpan 在 uploadOne 中创建 span。
func StartUploadOneSpan(ctx context.Context, objectKey string) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_UPLOAD_ONE,
		trace.WithAttributes(
			attribute.String("capture.object_key", objectKey),
		),
	)
	return ctx, span
}

// ---------------------------------------------------------------------------
// Metrics 记录辅助函数
// ---------------------------------------------------------------------------

// RecordSegmentDumped 记录一次 dump 数据段完成。
func RecordSegmentDumped(ctx context.Context, packetCount int, byteCount int) {
	segmentsDumpedCounter.Add(ctx, 1)
	packetsCapturedCounter.Add(ctx, int64(packetCount))
	bytesCapturedCounter.Add(ctx, int64(byteCount))
}

// RecordSegmentUploaded 记录一次 S3 上传完成。
func RecordSegmentUploaded(ctx context.Context, size int64) {
	segmentsUploadedCounter.Add(ctx, 1)
}

// RecordDumpError 记录一次 dump 错误。
func RecordDumpError(ctx context.Context) {
	dumpErrorCounter.Add(ctx, 1)
}

// RecordUploadError 记录一次 upload 错误。
func RecordUploadError(ctx context.Context) {
	uploadErrorCounter.Add(ctx, 1)
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
