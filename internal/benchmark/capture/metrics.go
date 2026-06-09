package bench

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pterm/pterm"
)

type CaptureMetrics struct {
	Name          string
	SentCount     int
	CapturedCount int
	TotalBytes    int64
	Duration      time.Duration
	PPS           float64
	BPS           float64
	LostCount     int
	LossRate      float64
}

var (
	resultsMu sync.Mutex
	results   []*CaptureMetrics
)

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

func (m *CaptureMetrics) ReportToBenchmark(b *testing.B) {
	b.ReportMetric(m.PPS, "pps")
	b.ReportMetric(m.BPS, "bps")
	b.ReportMetric(m.LossRate, "loss_rate")

	resultsMu.Lock()
	results = append(results, m)
	resultsMu.Unlock()
}

func PrintResultsTable() {
	resultsMu.Lock()
	defer resultsMu.Unlock()

	if len(results) == 0 {
		return
	}

	data := pterm.TableData{
		{"Benchmark", "Sent", "Captured", "Lost", "Loss Rate", "PPS", "BPS", "Duration"},
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
