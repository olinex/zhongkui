package bench

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// CaptureMetrics 单次抓包基准测试的性能指标集合。
type CaptureMetrics struct {
	// Name 基准用例名称。
	Name string
	// SentCount 发送的数据包数量。
	SentCount int
	// CapturedCount 实际捕获到的数据包数量。
	CapturedCount int
	// TotalBytes 捕获到的总字节数。
	TotalBytes int64
	// Duration 本次抓包持续时间。
	Duration time.Duration
	// PPS 每秒包数（packets per second）。
	PPS float64
	// BPS 每秒字节数（bytes per second）。
	BPS float64
	// LostCount 丢失的数据包数量。
	LostCount int
	// LossRate 丢包率（百分比）。
	LossRate float64
}

var (
	resultsMu sync.Mutex
	results   []*CaptureMetrics
)

// Compute 根据持续时间与计数计算 PPS、BPS、丢包数与丢包率。
func (m *CaptureMetrics) Compute() {
	if m.Duration > 0 {
		m.PPS = float64(m.CapturedCount) / m.Duration.Seconds()
		m.BPS = float64(m.TotalBytes) / m.Duration.Seconds()
	}
	m.LostCount = m.SentCount - m.CapturedCount
	if m.SentCount > 0 {
		m.LossRate = float64(m.LostCount) / float64(m.SentCount) * 100
	}
}

// ReportToBenchmark 将指标上报给基准测试框架并追加到全局结果集。
func (m *CaptureMetrics) ReportToBenchmark(b *testing.B) {
	b.ReportMetric(m.PPS, "pps")
	b.ReportMetric(m.BPS, "bps")
	b.ReportMetric(m.LossRate, "loss_rate")

	resultsMu.Lock()
	results = append(results, m)
	resultsMu.Unlock()
}

// PrintResultsTable 以表格形式打印所有收集到的基准指标。
func PrintResultsTable() {
	resultsMu.Lock()
	defer resultsMu.Unlock()

	if len(results) == 0 {
		return
	}

	data := pterm.TableData{
		{
			translate.T("cli.bench.col-name", nil),
			translate.T("cli.bench.col-sent", nil),
			translate.T("cli.bench.col-captured", nil),
			translate.T("cli.bench.col-lost", nil),
			translate.T("cli.bench.col-loss-rate", nil),
			translate.T("cli.bench.col-pps", nil),
			translate.T("cli.bench.col-bps", nil),
			translate.T("cli.bench.col-duration", nil),
		},
	}

	for _, m := range results {
		data = append(data, []string{
			m.Name,
			strconv.FormatInt(int64(m.SentCount), 10),
			strconv.FormatInt(int64(m.CapturedCount), 10),
			strconv.FormatInt(int64(m.LostCount), 10),
			strconv.FormatFloat(m.LossRate, 'f', 2, 64) + "%",
			formatPPS(m.PPS),
			formatPPS(m.BPS),
			m.Duration.String(),
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(data).Render()
}

func formatPPS(pps float64) string {
	if pps >= 1000000 {
		return fmt.Sprintf("%.2fM", pps/1e6)
	}
	if pps >= 1000 {
		return fmt.Sprintf("%.1fK", pps/1e3)
	}
	return fmt.Sprintf("%.1f", pps)
}
