package tcp

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
	SCOPE_NAME     = base.GLOBAL_PREFIX + "-tcp"
	SPAN_ROUTER    = "tcp.router"
	SPAN_RECTIFIER = "tcp.rectifier"
	SPAN_SIDE      = "tcp.side"
	SPAN_DETECT    = "tcp.detector"
	SPAN_EXTRACT   = "tcp.extractor"
	SPAN_BUILD     = "tcp.builder"
	SPAN_PARSE     = "tcp.parse"
	SPAN_CLEANUP   = "tcp.cleanup"
)

var (
	once sync.Once
	// ---------- Metrics ----------
	// packetCounter 统计处理的数据包总数，按类型标记（syn/syn_ack/ack/fin/rst/data）
	packetCounter metric.Int64Counter
	// packetOutOfOrderCounter 统计乱序数据包数量
	packetOutOfOrderCounter metric.Int64Counter
	// packetRetransmitCounter 统计重传数据包数量
	packetRetransmitCounter metric.Int64Counter
	// reassemblerCreatedCounter 统计创建的重组器数量
	reassemblerCreatedCounter metric.Int64Counter
	// reassemblerCompletedCounter 统计完成的重组器数量
	reassemblerCompletedCounter metric.Int64Counter
	// appExtractedCounter 统计提取的应用层消息数量
	appExtractedCounter metric.Int64Counter
	// appExtractionErrorCounter 统计应用层提取失败次数
	appExtractionErrorCounter metric.Int64Counter
	// bytesWrittenCounter 统计写入缓冲区的字节数
	bytesWrittenCounter metric.Int64Counter
	// cleanupFlowsRemovedCounter 统计清理时移除的流数量（已完成/超时）
	cleanupFlowsRemovedCounter metric.Int64Counter
	// cleanupFlowsEvictedCounter 统计清理时因超出容量被驱逐的流数量
	cleanupFlowsEvictedCounter metric.Int64Counter
	// processingDuration 统计各阶段处理耗时分布
	processingDuration metric.Int64Histogram
)

func InitMetrics() {
	once.Do(initMetrics)
}

func initMetrics() {
	var err, e error
	meter := otel.Meter(SCOPE_NAME)
	packetCounter, e = meter.Int64Counter(
		"tcp.packets.processed",
		metric.WithDescription("Total number of TCP packets processed"),
		metric.WithUnit("{packet}"),
	)
	err = errors.Join(err, e)

	packetOutOfOrderCounter, e = meter.Int64Counter(
		"tcp.packets.out_of_order",
		metric.WithDescription("Number of out-of-order TCP packets"),
		metric.WithUnit("{packet}"),
	)
	err = errors.Join(err, e)

	packetRetransmitCounter, e = meter.Int64Counter(
		"tcp.packets.retransmitted",
		metric.WithDescription("Number of retransmitted TCP packets"),
		metric.WithUnit("{packet}"),
	)
	err = errors.Join(err, e)

	reassemblerCreatedCounter, e = meter.Int64Counter(
		"tcp.reassemblers.created",
		metric.WithDescription("Total number of TCP reassemblers created"),
		metric.WithUnit("{reassembler}"),
	)
	err = errors.Join(err, e)

	reassemblerCompletedCounter, e = meter.Int64Counter(
		"tcp.reassemblers.completed",
		metric.WithDescription("Total number of TCP reassemblers completed"),
		metric.WithUnit("{reassembler}"),
	)
	err = errors.Join(err, e)

	appExtractedCounter, e = meter.Int64Counter(
		"tcp.app.extracted",
		metric.WithDescription("Total number of application layer messages extracted"),
		metric.WithUnit("{message}"),
	)
	err = errors.Join(err, e)

	appExtractionErrorCounter, e = meter.Int64Counter(
		"tcp.app.extraction_errors",
		metric.WithDescription("Total number of application layer extraction errors"),
		metric.WithUnit("{error}"),
	)
	err = errors.Join(err, e)

	bytesWrittenCounter, e = meter.Int64Counter(
		"tcp.bytes.written",
		metric.WithDescription("Total number of bytes written to reassembly buffers"),
		metric.WithUnit("By"),
	)
	err = errors.Join(err, e)

	cleanupFlowsRemovedCounter, e = meter.Int64Counter(
		"tcp.cleanup.flows_removed",
		metric.WithDescription("Total number of TCP flows removed during cleanup (completed/expired)"),
		metric.WithUnit("{flow}"),
	)
	err = errors.Join(err, e)

	cleanupFlowsEvictedCounter, e = meter.Int64Counter(
		"tcp.cleanup.flows_evicted",
		metric.WithDescription("Total number of TCP flows evicted due to capacity overflow"),
		metric.WithUnit("{flow}"),
	)
	err = errors.Join(err, e)

	processingDuration, e = meter.Int64Histogram(
		"tcp.processing.duration",
		metric.WithDescription("Duration of TCP packet processing stages in milliseconds"),
		metric.WithUnit("ms"),
	)
	err = errors.Join(err, e)

	if err != nil {
		panic(fmt.Errorf("tcp otel metrics init failed: %w", err))
	}
}

// recordPacketType 根据数据包标志位返回类型标签。
func recordPacketType(packet TCPPacket) attribute.KeyValue {
	key := "tcp.packet.type"
	switch {
	case packet.IsRst():
		return attribute.String(key, "rst")
	case packet.IsSynAck():
		return attribute.String(key, "syn_ack")
	case packet.IsSyn():
		return attribute.String(key, "syn")
	case packet.IsFin():
		return attribute.String(key, "fin")
	case packet.IsAck():
		return attribute.String(key, "ack")
	default:
		return attribute.String(key, "data")
	}
}

// ---------------------------------------------------------------------------
// Span 辅助函数
// ---------------------------------------------------------------------------

// startRouterSpan 在路由器 addPacket 中创建 span。
func startRouterSpan(ctx context.Context, packet TCPPacket) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_ROUTER,
		trace.WithAttributes(
			attribute.String("tcp.stream_id", packet.StreamID()),
			attribute.String("tcp.src_addr", packet.SrcAddr().String()),
			attribute.Int("tcp.src_port", int(packet.SrcPort())),
			attribute.String("tcp.dst_addr", packet.DstAddr().String()),
			attribute.Int("tcp.dst_port", int(packet.DstPort())),
			recordPacketType(packet),
		),
	)
	return ctx, span
}

// startRectifierSpan 在整流器 addPacket 中创建 span。
func startRectifierSpan(ctx context.Context, packet TCPPacket, c2s bool) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_RECTIFIER,
		trace.WithAttributes(
			attribute.String("tcp.stream_id", packet.StreamID()),
			recordPacketType(packet),
			attribute.Int("tcp.payload_size", len(packet.GetPayload())),
			attribute.Bool("tcp.direction.c2s", c2s),
		),
	)
	return ctx, span
}

// startSideSpan 在单端 addPacket 中创建 span。
func startSideSpan(ctx context.Context, packet TCPPacket, c2s bool) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_SIDE,
		trace.WithAttributes(
			recordPacketType(packet),
			attribute.Int64("tcp.seq", int64(packet.SeqNum())),
			attribute.Int64("tcp.ack", int64(packet.AckNum())),
			attribute.Int("tcp.payload_size", len(packet.GetPayload())),
			attribute.Bool("tcp.direction.c2s", c2s),
		),
	)
	return ctx, span
}

// startDetectSpan 在协议检测阶段创建 span。
func startDetectSpan(ctx context.Context, protocol AppProtocol) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_DETECT,
		trace.WithAttributes(
			attribute.String("tcp.app.protocol", string(protocol)),
		),
	)
	return ctx, span
}

// startExtractSpan 在协议提取阶段创建 span。
func startExtractSpan(ctx context.Context, protocol AppProtocol) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_EXTRACT,
		trace.WithAttributes(
			attribute.String("tcp.app.protocol", string(protocol)),
		),
	)
	return ctx, span
}

// startBuildSpan 在应用层构建阶段创建 span。
func startBuildSpan(ctx context.Context, protocol AppProtocol) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_BUILD,
		trace.WithAttributes(
			attribute.String("tcp.app.protocol", string(protocol)),
		),
	)
	return ctx, span
}

// startParseSpan 在解析循环中创建 span。
func startParseSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	return tracer.Start(ctx, SPAN_PARSE)
}

// startCleanupSpan 在清理阶段创建 span。
func startCleanupSpan(ctx context.Context, force bool) (context.Context, trace.Span) {
	tracer := otel.Tracer(SCOPE_NAME)
	ctx, span := tracer.Start(ctx, SPAN_CLEANUP,
		trace.WithAttributes(
			attribute.Bool("tcp.cleanup.force", force),
		),
	)
	return ctx, span
}

// ---------------------------------------------------------------------------
// Metrics 记录辅助函数
// ---------------------------------------------------------------------------

func recordPacket(ctx context.Context, packet TCPPacket) {
	packetCounter.Add(ctx, 1, metric.WithAttributes(recordPacketType(packet)))
}

func recordOutOfOrder(ctx context.Context) {
	packetOutOfOrderCounter.Add(ctx, 1)
}

func recordRetransmit(ctx context.Context) {
	if packetRetransmitCounter == nil {
		return
	}
	packetRetransmitCounter.Add(ctx, 1)
}

func recordReassemblerCreated(ctx context.Context) {
	if reassemblerCreatedCounter == nil {
		return
	}
	reassemblerCreatedCounter.Add(ctx, 1)
}

func recordReassemblerCompleted(ctx context.Context) {
	reassemblerCompletedCounter.Add(ctx, 1)
}

func recordAppExtracted(ctx context.Context, protocol string) {
	appExtractedCounter.Add(ctx, 1,
		metric.WithAttributes(attribute.String("tcp.app.protocol", protocol)),
	)
}

func recordAppExtractionError(ctx context.Context) {
	appExtractionErrorCounter.Add(ctx, 1)
}

func recordBytesWritten(ctx context.Context, n int) {
	bytesWrittenCounter.Add(ctx, int64(n))
}

func recordCleanupFlowsRemoved(ctx context.Context, count int) {
	cleanupFlowsRemovedCounter.Add(ctx, int64(count))
}

func recordCleanupFlowsEvicted(ctx context.Context, count int) {
	cleanupFlowsEvictedCounter.Add(ctx, int64(count))
}

// ---------------------------------------------------------------------------
// 通用 span 工具
// ---------------------------------------------------------------------------

// endSpan 安全结束 span，并在发生错误时记录错误状态。
func endSpan(span trace.Span, errPtr *error) {
	if span == nil {
		return
	}
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
	}
	span.End()
}
