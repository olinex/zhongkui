//go:build profile
// +build profile

// Package test 提供测试与基准测试的通用辅助设施：阶段耗时追踪（StageTracer）
// 与基准运行、性能剖析工具。
package test

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// Tracer 全局阶段耗时追踪器实例，供各包在热点路径埋点。
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

// StageTracer 阶段耗时追踪器：以 Begin/End 配对记录各阶段的调用次数与耗时分布。
type StageTracer struct {
	// mu 保护 records 与 stack 的互斥锁。
	mu sync.Mutex
	// records 各阶段名称到耗时记录的映射。
	records map[string]*stageRecord
	// stack 未结束阶段的嵌套计时栈。
	stack []stackEntry
	// written 标记报告是否已输出。
	written bool
}

// NewStageTracer 创建一个 StageTracer 实例。
func NewStageTracer() *StageTracer {
	return &StageTracer{
		records: make(map[string]*stageRecord, 64),
		stack:   make([]stackEntry, 0, 64),
	}
}

// Begin 开始一个命名阶段的计时，支持嵌套调用。
func (t *StageTracer) Begin(name string) {
	t.mu.Lock()
	t.stack = append(t.stack, stackEntry{name: name, start: time.Now()})
	t.mu.Unlock()
}

// End 结束最近一次 Begin 开启的阶段并累计其耗时。
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

// Report 汇总各阶段的调用次数与耗时并输出统计报告。
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

	printReport(nil, records, totalNs)
}

// printReport 将阶段耗时统计报告渲染到指定的 writer（nil 表示 pterm 默认输出）
func printReport(w io.Writer, records []*stageRecord, totalNs int64) {
	pterm.DefaultHeader.WithFullWidth().WithWriter(w).Println(translate.T("cli.tracer.report-title", nil))

	if len(records) == 0 {
		pterm.Warning.WithWriter(w).Println(translate.T("cli.tracer.no-data", nil))
		return
	}

	data := pterm.TableData{
		{
			translate.T("cli.tracer.col-stage", nil),
			translate.T("cli.tracer.col-calls", nil),
			translate.T("cli.tracer.col-total", nil),
			translate.T("cli.tracer.col-avg", nil),
			translate.T("cli.tracer.col-min", nil),
			translate.T("cli.tracer.col-max", nil),
			translate.T("cli.tracer.col-ratio", nil),
		},
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

	if err := pterm.DefaultTable.WithHasHeader().WithData(data).WithWriter(w).Render(); err != nil {
		pterm.Error.WithWriter(w).Println(translate.T("cli.tracer.render-error", map[string]any{"Err": err}))
	}
	pterm.Info.WithWriter(w).Println(translate.T("cli.tracer.total-records", map[string]any{"Count": len(records)}))
}

func formatNS(ns int64) string {
	return (time.Duration(ns) * time.Nanosecond).String()
}
