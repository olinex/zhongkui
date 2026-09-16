package health

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"zhongkui.com/zhongkui/internal/pkg/status"
)

// healthMetrics 持有健康探针状态指标的同步 gauge 仪器。
// 指标经全局 MeterProvider 发布，复用既有 OTLP 管道导出，不新增 exporter/reader；
// OTEL 未启用时全局 meter 为 noop，记录被丢弃，无需分支短路。
// healthMetrics holds the sync-gauge instruments publishing health-probe states.
// Metrics are emitted through the global MeterProvider and exported via the existing
// OTLP pipeline (no new exporter/reader). When OTEL is disabled the global meter is a
// no-op and records are dropped, so no branch is needed here.
type healthMetrics struct {
	live     metric.Int64Gauge
	ready    metric.Int64Gauge
	startup  metric.Int64Gauge
	depReady metric.Int64Gauge
}

// newHealthMetrics 创建健康探针状态指标仪器。
func newHealthMetrics() *healthMetrics {
	meter := otel.Meter("zhongkui.health")
	m := &healthMetrics{}
	if g, err := meter.Int64Gauge("zk.health.live"); err == nil {
		m.live = g
	}
	if g, err := meter.Int64Gauge("zk.health.ready"); err == nil {
		m.ready = g
	}
	if g, err := meter.Int64Gauge("zk.health.startup"); err == nil {
		m.startup = g
	}
	if g, err := meter.Int64Gauge("zk.health.dependency.ready"); err == nil {
		m.depReady = g
	}
	return m
}

// boolGauge 将布尔状态收敛为 0/1 指标值。
func boolGauge(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// record 发布一次健康状态快照：live/ready/started 及逐依赖就绪指标。
// deps 可空；依赖指标按 dependency.name 属性逐项发布 0/1。
func (m *healthMetrics) record(ctx context.Context, live, ready, started bool, deps []status.Status) {
	if m == nil {
		return
	}
	if m.live != nil {
		m.live.Record(ctx, boolGauge(live))
	}
	if m.ready != nil {
		m.ready.Record(ctx, boolGauge(ready))
	}
	if m.startup != nil {
		m.startup.Record(ctx, boolGauge(started))
	}
	if m.depReady != nil {
		for _, st := range deps {
			m.depReady.Record(ctx, boolGauge(st.Reachable),
				metric.WithAttributes(attribute.String("dependency.name", st.Name)))
		}
	}
}