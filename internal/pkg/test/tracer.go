//go:build profile
// +build profile

package test

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/pterm/pterm"
)

var Tracer = NewStageTracer()

type stageRecord struct {
	name      string
	callCount int64
	totalNs   int64
	minNs     int64
	maxNs     int64
}

type stackEntry struct {
	name  string
	start time.Time
}

type StageTracer struct {
	mu      sync.Mutex
	records map[string]*stageRecord
	stack   []stackEntry
	written bool
}

func NewStageTracer() *StageTracer {
	return &StageTracer{
		records: make(map[string]*stageRecord, 64),
		stack:   make([]stackEntry, 0, 64),
	}
}

func (t *StageTracer) Begin(name string) {
	t.mu.Lock()
	t.stack = append(t.stack, stackEntry{name: name, start: time.Now()})
	t.mu.Unlock()
}

func (t *StageTracer) End() {
	t.mu.Lock()
	if len(t.stack) == 0 {
		t.mu.Unlock()
		return
	}
	entry := t.stack[len(t.stack)-1]
	t.stack = t.stack[:len(t.stack)-1]
	elapsed := time.Since(entry.start)
	ns := elapsed.Nanoseconds()

	rec, ok := t.records[entry.name]
	if !ok {
		rec = &stageRecord{name: entry.name, minNs: ns, maxNs: ns}
		t.records[entry.name] = rec
	}
	rec.callCount++
	rec.totalNs += ns
	if ns < rec.minNs {
		rec.minNs = ns
	}
	if ns > rec.maxNs {
		rec.maxNs = ns
	}
	t.mu.Unlock()
}

func (t *StageTracer) Report() {
	t.mu.Lock()
	records := make([]*stageRecord, 0, len(t.records))
	for _, rec := range t.records {
		records = append(records, rec)
	}
	totalNs := int64(0)
	for _, rec := range records {
		totalNs += rec.totalNs
	}
	t.mu.Unlock()

	sort.Slice(records, func(i, j int) bool {
		return records[i].totalNs > records[j].totalNs
	})

	printReport(records, totalNs)
}

func printReport(records []*stageRecord, totalNs int64) {
	pterm.DefaultHeader.WithFullWidth().Println("阶段耗时统计报告")

	if len(records) == 0 {
		pterm.Warning.Println("无数据")
		return
	}

	data := pterm.TableData{
		{"阶段名称", "调用次数", "总耗时", "平均耗时", "最小耗时", "最大耗时", "占比"},
	}

	for _, rec := range records {
		avg := rec.totalNs / rec.callCount
		ratio := 0.0
		if totalNs > 0 {
			ratio = float64(rec.totalNs) / float64(totalNs) * 100
		}

		data = append(data, []string{
			rec.name,
			fmt.Sprintf("%d", rec.callCount),
			formatNS(rec.totalNs),
			formatNS(avg),
			formatNS(rec.minNs),
			formatNS(rec.maxNs),
			fmt.Sprintf("%.1f%%", ratio),
		})
	}

	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		pterm.Error.Printf("error rendering report table: %v\n", err)
	}
	pterm.Info.Printf("总记录数: %d\n", len(records))
}

func formatNS(ns int64) string {
	return (time.Duration(ns) * time.Nanosecond).String()
}
