// Package test（本文件）提供基准测试运行与报告生成能力：Run 运行基准并采集耗时、
// 内存等指标，BenchResult 负责统计与 Markdown 报告输出。
package test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"text/template"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

type benchConfig struct {
	warmupCount int
	outputDir   string
	autoReport  bool
	profiling   bool
	profCfg     ProfilingConfig
}

func defaultConfig() benchConfig {
	return benchConfig{
		warmupCount: 0,
		outputDir:   ".",
		autoReport:  true,
		profCfg:     DefaultProfilingConfig(),
	}
}

// BenchOption 定义基准测试的运行选项。
type BenchOption func(*benchConfig)

// WithWarmup 设置基准运行前的预热次数。
func WithWarmup(n int) BenchOption {
	return func(c *benchConfig) {
		c.warmupCount = n
	}
}

// WithOutputDir 设置报告输出目录。
func WithOutputDir(dir string) BenchOption {
	return func(c *benchConfig) {
		c.outputDir = dir
	}
}

// WithAutoReport 设置是否自动生成报告文件。
func WithAutoReport(enabled bool) BenchOption {
	return func(c *benchConfig) {
		c.autoReport = enabled
	}
}

// WithProfiling 设置是否启用性能剖析。
func WithProfiling(enabled bool) BenchOption {
	return func(c *benchConfig) {
		c.profiling = enabled
	}
}

// WithProfilingConfig 设置性能剖析配置并启用剖析。
func WithProfilingConfig(cfg ProfilingConfig) BenchOption {
	return func(c *benchConfig) {
		c.profiling = true
		c.profCfg = cfg
	}
}

// BenchResult 保存单次基准测试的统计结果。
type BenchResult struct {
	// Name 基准测试名称。
	Name string
	// N 迭代次数。
	N int
	// T 总耗时。
	T time.Duration
	// Bytes 单次迭代处理的字节数。
	Bytes int64
	// MemAllocs 内存分配次数。
	MemAllocs int64
	// MemBytes 内存分配字节数。
	MemBytes int64

	// timings 各次采样的耗时列表（纳秒）。
	timings []float64
	// Profile 性能剖析结果，未启用剖析时为 nil。
	Profile *ProfileResult
}

// NsPerOp 返回单次操作的纳秒耗时。
func (r *BenchResult) NsPerOp() float64 {
	if r.N == 0 {
		return 0
	}
	return float64(r.T.Nanoseconds()) / float64(r.N)
}

// MbPerSec 返回吞吐量（MB/s）。
func (r *BenchResult) MbPerSec() float64 {
	if r.T.Seconds() <= 0 || r.Bytes <= 0 {
		return 0
	}
	return float64(r.Bytes) * float64(r.N) / r.T.Seconds() / 1024 / 1024
}

// AllocsPerOp 返回单次操作的内存分配次数。
func (r *BenchResult) AllocsPerOp() int64 {
	if r.N == 0 {
		return 0
	}
	return r.MemAllocs / int64(r.N)
}

// BytesPerOp 返回单次操作的内存分配字节数。
func (r *BenchResult) BytesPerOp() int64 {
	if r.N == 0 {
		return 0
	}
	return r.MemBytes / int64(r.N)
}

// MinNs 返回采样耗时中的最小值（纳秒）。
func (r *BenchResult) MinNs() float64 {
	if len(r.timings) == 0 {
		return 0
	}
	min := r.timings[0]
	for _, v := range r.timings[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// MaxNs 返回采样耗时中的最大值（纳秒）。
func (r *BenchResult) MaxNs() float64 {
	if len(r.timings) == 0 {
		return 0
	}
	max := r.timings[0]
	for _, v := range r.timings[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// AvgNs 返回采样耗时的平均值（纳秒）。
func (r *BenchResult) AvgNs() float64 {
	if len(r.timings) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range r.timings {
		sum += v
	}
	return sum / float64(len(r.timings))
}

// Percentile 返回给定百分位（0-100）的采样耗时（纳秒）。
func (r *BenchResult) Percentile(p float64) float64 {
	if len(r.timings) == 0 {
		return 0
	}
	sorted := make([]float64, len(r.timings))
	copy(sorted, r.timings)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p/100.0*float64(len(sorted))) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

type reportData struct {
	Name        string
	N           string
	NsPerOp     string
	MbPerSec    string
	BytesPerOp  string
	AllocsPerOp string
	MinNs       string
	MaxNs       string
	AvgNs       string
	P50Ns       string
	P90Ns       string
	P99Ns       string
	GoVersion   string
	GOOS        string
	GOARCH      string
	NumCPU      int
	Timestamp   string
	Duration    string

	HasProfiling        bool
	HasCPUProfile       bool
	HasHeapProfile      bool
	HasTraceProfile     bool
	HasGoroutineProfile bool
	HasMutexProfile     bool
	HasBlockProfile     bool
	CPUProfilePath      string
	HeapProfilePath     string
	GoroutinePath       string
	MutexPath           string
	BlockPath           string

	MemHeapAlloc     string
	MemHeapInuse     string
	MemStackInuse    string
	MemTotalAlloc    string
	MemAllocObjects  string
	MemGCSys         string
	GCCount          string
	GCPauseTotal     string
	GCPauseAvg       string
	GoroutineCount   string
	MutexContentions string
	MutexDelayTotal  string

	TopCPU          []profileEntryData
	TopAllocBytes   []profileEntryData
	TopAllocObjects []profileEntryData
}

type profileEntryData struct {
	Function string
	Value    string
	Percent  string
}

func buildProfileEntries(entries []ProfileEntry) []profileEntryData {
	if len(entries) == 0 {
		return nil
	}
	result := make([]profileEntryData, len(entries))
	for i, e := range entries {
		result[i] = profileEntryData{
			Function: e.Function,
			Value:    formatProfileValue(e.Value),
			Percent:  fmt.Sprintf("%.1f%%", e.Percent),
		}
	}
	return result
}

func formatProfileValue(v int64) string {
	if v > 1e9 {
		return fmt.Sprintf("%.2fs", float64(v)/1e9)
	}
	if v > 1e6 {
		return fmt.Sprintf("%.2fms", float64(v)/1e6)
	}
	if v > 1e3 {
		return fmt.Sprintf("%.2fµs", float64(v)/1e3)
	}
	return fmt.Sprintf("%dns", v)
}

const reportTemplate = `# Performance Benchmark Report

## Summary

| Metric | Value |
|--------|-------|
| **Name** | {{.Name}} |
| **Iterations** | {{.N}} |
| **Duration** | {{.Duration}} |
| **ns/op** | {{.NsPerOp}} |
| **B/op** | {{.BytesPerOp}} |
| **allocs/op** | {{.AllocsPerOp}} |
{{if .MbPerSec}}| **MB/s** | {{.MbPerSec}} |{{end}}

## Timing Distribution

| Metric | Value |
|--------|-------|
| **Min** | {{.MinNs}} |
| **Max** | {{.MaxNs}} |
| **Avg** | {{.AvgNs}} |
| **P50 (Median)** | {{.P50Ns}} |
| **P90** | {{.P90Ns}} |
| **P99** | {{.P99Ns}} |

{{if .HasProfiling}}
## Memory Statistics

| Metric | Value |
|--------|-------|
| **HeapAlloc** | {{.MemHeapAlloc}} |
| **HeapInuse** | {{.MemHeapInuse}} |
| **StackInuse** | {{.MemStackInuse}} |
| **TotalAlloc** | {{.MemTotalAlloc}} |
| **AllocObjects** | {{.MemAllocObjects}} |
| **GCSys** | {{.MemGCSys}} |

## GC Statistics

| Metric | Value |
|--------|-------|
| **GCCount** | {{.GCCount}} |
| **GCPauseTotal** | {{.GCPauseTotal}} |
| **GCPauseAvg** | {{.GCPauseAvg}} |
| **Goroutines** | {{.GoroutineCount}} |
{{if .MutexContentions}}| **MutexContentions** | {{.MutexContentions}} |
| **MutexDelayTotal** | {{.MutexDelayTotal}} |{{end}}

{{if .TopCPU}}
## Top CPU Consumers

| Function | Self | Percent |
|----------|------|---------|
{{range .TopCPU}}| ` + "`{{.Function}}`" + ` | {{.Value}} | {{.Percent}} |
{{end}}{{end}}
{{if .TopAllocBytes}}
## Top Allocation Sites (by bytes)

| Function | Allocated | Percent |
|----------|-----------|---------|
{{range .TopAllocBytes}}| ` + "`{{.Function}}`" + ` | {{.Value}} | {{.Percent}} |
{{end}}{{end}}
{{if .TopAllocObjects}}
## Top Allocation Sites (by objects)

| Function | Objects | Percent |
|----------|---------|---------|
{{range .TopAllocObjects}}| ` + "`{{.Function}}`" + ` | {{.Value}} | {{.Percent}} |
{{end}}{{end}}

## Profile Files

| Profile | Path |
|---------|------|
{{if .HasCPUProfile}}| CPU | {{.CPUProfilePath}} |
{{end}}{{if .HasHeapProfile}}| Heap | {{.HeapProfilePath}} |
{{end}}{{if .HasGoroutineProfile}}| Goroutine | {{.GoroutinePath}} |
{{end}}{{if .HasMutexProfile}}| Mutex | {{.MutexPath}} |
{{end}}{{if .HasBlockProfile}}| Block | {{.BlockPath}} |
{{end}}
{{end}}
## Environment

| Metric | Value |
|--------|-------|
| **Go Version** | {{.GoVersion}} |
| **OS** | {{.GOOS}} |
| **Arch** | {{.GOARCH}} |
| **CPUs** | {{.NumCPU}} |
| **Timestamp** | {{.Timestamp}} |
`

// ReportString 生成 Markdown 格式的基准测试报告。
func (r *BenchResult) ReportString() string {
	data := r.buildReportData()
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"profileValue": func(v int64) string {
			return formatProfileValue(v)
		},
	}).Parse(reportTemplate))
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("error generating report: %v", err)
	}
	return buf.String()
}

// ReportToFile 将报告写入指定文件，必要时创建父目录。
func (r *BenchResult) ReportToFile(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
	}

	report := r.ReportString()
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		return fmt.Errorf("failed to write report to %s: %w", path, err)
	}
	return nil
}

func (r *BenchResult) buildReportData() reportData {
	nsPerOp := r.NsPerOp()
	mbs := r.MbPerSec()
	mbStr := ""
	if mbs > 0 {
		mbStr = fmt.Sprintf("%.2f", mbs)
	}

	data := reportData{
		Name:        r.Name,
		N:           formatInt(int64(r.N)),
		NsPerOp:     formatNSDuration(nsPerOp),
		MbPerSec:    mbStr,
		BytesPerOp:  formatBytes(r.BytesPerOp()),
		AllocsPerOp: formatInt(r.AllocsPerOp()),
		MinNs:       formatNSDuration(r.MinNs()),
		MaxNs:       formatNSDuration(r.MaxNs()),
		AvgNs:       formatNSDuration(r.AvgNs()),
		P50Ns:       formatNSDuration(r.Percentile(50)),
		P90Ns:       formatNSDuration(r.Percentile(90)),
		P99Ns:       formatNSDuration(r.Percentile(99)),
		GoVersion:   runtime.Version(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		Timestamp:   time.Now().Format(time.RFC3339),
		Duration:    formatDuration(r.T),
	}

	if r.Profile != nil {
		p := r.Profile
		data.HasProfiling = true

		if p.CPUProfilePath != "" {
			data.HasCPUProfile = true
			data.CPUProfilePath = p.CPUProfilePath
		}
		if p.HeapProfilePath != "" {
			data.HasHeapProfile = true
			data.HeapProfilePath = p.HeapProfilePath
		}
		if p.GoroutinePath != "" {
			data.HasGoroutineProfile = true
			data.GoroutinePath = p.GoroutinePath
		}
		if p.MutexPath != "" {
			data.HasMutexProfile = true
			data.MutexPath = p.MutexPath
		}
		if p.BlockPath != "" {
			data.HasBlockProfile = true
			data.BlockPath = p.BlockPath
		}

		m := &p.MemStats
		data.MemHeapAlloc = formatBytes(int64(m.HeapAlloc))
		data.MemHeapInuse = formatBytes(int64(m.HeapInuse))
		data.MemStackInuse = formatBytes(int64(m.StackInuse))
		data.MemTotalAlloc = formatBytes(int64(m.TotalAlloc))
		data.MemAllocObjects = formatInt(int64(m.Mallocs - m.Frees))
		data.MemGCSys = formatBytes(int64(m.GCSys))
		data.GCCount = formatInt(int64(m.NumGC))

		pauseTotal := time.Duration(m.PauseTotalNs)
		data.GCPauseTotal = formatDuration(pauseTotal)
		if m.NumGC > 0 {
			data.GCPauseAvg = formatDuration(pauseTotal / time.Duration(m.NumGC))
		} else {
			data.GCPauseAvg = "0s"
		}

		data.GoroutineCount = formatInt(int64(p.GoroutineCount))
		data.MutexContentions = formatInt(p.MutexContentions)
		data.MutexDelayTotal = formatDuration(p.MutexDelayTotal)

		data.TopCPU = buildProfileEntries(p.TopCPU)
		data.TopAllocBytes = buildProfileEntries(p.TopAllocBytes)
		data.TopAllocObjects = buildProfileEntries(p.TopAllocObjects)
	}

	return data
}

func sampleTimings(fn func(), targetSamples int) []float64 {
	timings := make([]float64, 0, targetSamples)
	for i := 0; i < targetSamples; i++ {
		start := time.Now()
		fn()
		timings = append(timings, float64(time.Since(start).Nanoseconds()))
	}
	return timings
}

// Run 运行基准测试并返回统计结果，可选生成报告与采集剖析数据。
func Run(name string, fn func(), opts ...BenchOption) *BenchResult {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	for i := 0; i < cfg.warmupCount; i++ {
		fn()
	}

	var profResult *ProfileResult
	if cfg.profiling {
		profDir := newProfileOutputDir(cfg.outputDir)
		profResult = collectProfiles(name, profDir, cfg.profCfg)
	}

	stdResult := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fn()
		}
	})

	if profResult != nil {
		finalizeProfiles(profResult, cfg.profCfg)
	}

	nsPerOp := float64(stdResult.T.Nanoseconds()) / float64(stdResult.N)
	targetSamples := int(200 * float64(time.Millisecond.Nanoseconds()) / nsPerOp)
	if targetSamples < 100 {
		targetSamples = 100
	}
	if targetSamples > 10000 {
		targetSamples = 10000
	}

	timings := sampleTimings(fn, targetSamples)

	result := &BenchResult{
		Name:      name,
		N:         stdResult.N,
		T:         stdResult.T,
		Bytes:     stdResult.Bytes,
		MemAllocs: int64(stdResult.MemAllocs),
		MemBytes:  int64(stdResult.MemBytes),
		timings:   timings,
		Profile:   profResult,
	}

	if cfg.autoReport {
		timestamp := time.Now().Format("20060102_150405")
		safeName := safeFileName(name)
		if safeName == "" {
			safeName = "benchmark"
		}
		reportPath := filepath.Join(cfg.outputDir, fmt.Sprintf("benchreport_%s_%s.md", safeName, timestamp))
		if err := result.ReportToFile(reportPath); err != nil {
			fmt.Fprintln(os.Stderr, translate.T("cli.bench.report-save-failed", map[string]any{"Err": err}))
		} else {
			fmt.Fprintln(os.Stderr, translate.T("cli.bench.report-saved", map[string]any{"Path": reportPath}))
		}
	}

	summary := fmt.Sprintf("%-40s %12d runs   %12s/op", name, stdResult.N, formatNSDuration(nsPerOp))
	if int64(stdResult.MemAllocs) > 0 {
		summary += fmt.Sprintf("   %8d allocs/op   %8s/op",
			int64(stdResult.MemAllocs)/int64(stdResult.N),
			formatBytes(int64(stdResult.MemBytes)/int64(stdResult.N)))
	}
	fmt.Fprintf(os.Stderr, "%s\n", summary)

	return result
}

func formatInt(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fG", float64(n)/1000000000)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/1024/1024)
}

func formatNSDuration(ns float64) string {
	d := time.Duration(ns) * time.Nanosecond
	return formatDuration(d)
}

func formatDuration(d time.Duration) string {
	if d > time.Second {
		return d.Round(time.Microsecond * 100).String()
	}
	if d > time.Microsecond {
		return d.Round(time.Nanosecond * 100).String()
	}
	return d.String()
}
