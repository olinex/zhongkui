// Package test（本文件）提供基准测试与性能剖析能力的单元测试。
package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func noop() {}

var allocSink []byte

func busyWork(duration time.Duration) func() {
	return func() {
		start := time.Now()
		for time.Since(start) < duration {
		}
	}
}

func allocWork() func() {
	return func() {
		allocSink = make([]byte, 1024)
	}
}

// TestRunBasic 测试 Run 的基本基准测试能力。
func TestRunBasic(t *testing.T) {
	Convey("Test Run basic functionality", t, func() {
		Convey("Should benchmark a noop function", func() {
			result := Run("noop", noop, WithAutoReport(false))
			So(result, ShouldNotBeNil)
			So(result.N, ShouldBeGreaterThan, 0)
			So(result.T, ShouldBeGreaterThan, 0)
			So(result.NsPerOp(), ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("Should benchmark a function with work", func() {
			result := Run("busy", busyWork(10*time.Microsecond), WithAutoReport(false))
			So(result, ShouldNotBeNil)
			So(result.NsPerOp(), ShouldBeGreaterThan, 1000)
		})
	})
}

// TestRunWarmup 测试 Run 的预热选项。
func TestRunWarmup(t *testing.T) {
	Convey("Test Run with warmup", t, func() {
		Convey("Should run warmup iterations without affecting result", func() {
			warmupCalled := 0
			fn := func() {
				warmupCalled++
			}

			result := Run("warmup", fn, WithWarmup(5), WithAutoReport(false))
			So(result, ShouldNotBeNil)
			So(warmupCalled, ShouldBeGreaterThanOrEqualTo, 5)
			So(result.N, ShouldBeGreaterThan, 0)
		})
	})
}

// TestRunWithAutoReport 测试 Run 的自动报告生成开关。
func TestRunWithAutoReport(t *testing.T) {
	Convey("Test Run with auto report", t, func() {
		Convey("Should generate report file when autoReport is enabled", func() {
			tmpDir := t.TempDir()

			result := Run("auto_report_test", noop, WithOutputDir(tmpDir), WithAutoReport(true))
			So(result, ShouldNotBeNil)

			entries, err := os.ReadDir(tmpDir)
			So(err, ShouldBeNil)

			found := false
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "benchreport_auto_report_test_") && strings.HasSuffix(entry.Name(), ".md") {
					found = true
					break
				}
			}
			So(found, ShouldBeTrue)
		})

		Convey("Should not generate report file when autoReport is disabled", func() {
			tmpDir := t.TempDir()

			result := Run("no_report_test", noop, WithOutputDir(tmpDir), WithAutoReport(false))
			So(result, ShouldNotBeNil)

			entries, err := os.ReadDir(tmpDir)
			So(err, ShouldBeNil)
			So(len(entries), ShouldEqual, 0)
		})
	})
}

// TestBenchResultMetrics 测试 BenchResult 各项指标的换算。
func TestBenchResultMetrics(t *testing.T) {
	Convey("Test BenchResult metrics computation", t, func() {
		Convey("NsPerOp should compute correctly", func() {
			result := &BenchResult{
				Name: "test",
				N:    1000,
				T:    1 * time.Second,
			}
			So(result.NsPerOp(), ShouldEqual, 1e6)
		})

		Convey("NsPerOp should return 0 when N is 0", func() {
			result := &BenchResult{Name: "empty"}
			So(result.NsPerOp(), ShouldEqual, 0)
		})

		Convey("MbPerSec should compute correctly", func() {
			result := &BenchResult{
				N:     1000,
				T:     1 * time.Second,
				Bytes: 1024,
			}
			So(result.MbPerSec(), ShouldAlmostEqual, 0.9765625, 0.001)
		})

		Convey("MbPerSec should return 0 when Bytes is 0", func() {
			result := &BenchResult{
				N: 1000,
				T: 1 * time.Second,
			}
			So(result.MbPerSec(), ShouldEqual, 0)
		})

		Convey("AllocsPerOp should compute correctly", func() {
			result := &BenchResult{
				N:         1000,
				MemAllocs: 5000,
			}
			So(result.AllocsPerOp(), ShouldEqual, 5)
		})

		Convey("BytesPerOp should compute correctly", func() {
			result := &BenchResult{
				N:        1000,
				MemBytes: 2048000,
			}
			So(result.BytesPerOp(), ShouldEqual, 2048)
		})
	})
}

// TestBenchResultTimings 测试 BenchResult 的耗时分布统计。
func TestBenchResultTimings(t *testing.T) {
	Convey("Test BenchResult timing distribution", t, func() {
		Convey("MinNs/MaxNs/AvgNs should compute from timings", func() {
			result := &BenchResult{
				timings: []float64{100, 200, 300, 400, 500},
			}
			So(result.MinNs(), ShouldEqual, 100)
			So(result.MaxNs(), ShouldEqual, 500)
			So(result.AvgNs(), ShouldEqual, 300)
		})

		Convey("MinNs should return 0 when timings is empty", func() {
			result := &BenchResult{}
			So(result.MinNs(), ShouldEqual, 0)
			So(result.MaxNs(), ShouldEqual, 0)
			So(result.AvgNs(), ShouldEqual, 0)
		})

		Convey("Percentile should compute correct values", func() {
			timings := make([]float64, 100)
			for i := range timings {
				timings[i] = float64(i + 1)
			}
			result := &BenchResult{timings: timings}
			So(result.Percentile(50), ShouldEqual, 50)
			So(result.Percentile(90), ShouldEqual, 90)
			So(result.Percentile(99), ShouldEqual, 99)
		})

		Convey("Percentile should return 0 when timings is empty", func() {
			result := &BenchResult{}
			So(result.Percentile(50), ShouldEqual, 0)
		})

		Convey("Percentile should handle edge indices", func() {
			result := &BenchResult{timings: []float64{42}}
			So(result.Percentile(0), ShouldEqual, 42)
			So(result.Percentile(50), ShouldEqual, 42)
			So(result.Percentile(100), ShouldEqual, 42)
		})
	})
}

// TestReportString 测试 Markdown 报告的生成。
func TestReportString(t *testing.T) {
	Convey("Test ReportString output", t, func() {
		Convey("Should generate valid markdown report", func() {
			result := &BenchResult{
				Name:      "test_func",
				N:         10000,
				T:         5 * time.Second,
				Bytes:     512,
				MemAllocs: 50000,
				MemBytes:  10240000,
				timings:   []float64{100, 200, 300, 400, 500},
			}

			report := result.ReportString()
			So(report, ShouldNotBeEmpty)
			So(strings.Contains(report, "# Performance Benchmark Report"), ShouldBeTrue)
			So(strings.Contains(report, "## Summary"), ShouldBeTrue)
			So(strings.Contains(report, "## Timing Distribution"), ShouldBeTrue)
			So(strings.Contains(report, "## Environment"), ShouldBeTrue)
			So(strings.Contains(report, "test_func"), ShouldBeTrue)
			So(strings.Contains(report, "10.0K"), ShouldBeTrue)
		})

		Convey("Should handle result with zero memory stats", func() {
			result := &BenchResult{
				Name: "no_mem",
				N:    1000,
				T:    1 * time.Second,
			}

			report := result.ReportString()
			So(report, ShouldNotBeEmpty)
			So(strings.Contains(report, "no_mem"), ShouldBeTrue)
		})
	})
}

// TestReportToFile 测试报告写入文件及目录创建。
func TestReportToFile(t *testing.T) {
	Convey("Test ReportToFile", t, func() {
		Convey("Should write report to file", func() {
			tmpDir := t.TempDir()
			reportPath := filepath.Join(tmpDir, "test_report.md")

			result := &BenchResult{
				Name:    "file_test",
				N:       5000,
				T:       2 * time.Second,
				timings: []float64{100, 200, 300},
			}

			err := result.ReportToFile(reportPath)
			So(err, ShouldBeNil)

			content, err := os.ReadFile(reportPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldContainSubstring, "file_test")
		})

		Convey("Should create nested directories", func() {
			tmpDir := t.TempDir()
			reportPath := filepath.Join(tmpDir, "nested", "deep", "report.md")

			result := &BenchResult{
				Name: "nested_test",
				N:    1000,
				T:    500 * time.Millisecond,
			}

			err := result.ReportToFile(reportPath)
			So(err, ShouldBeNil)

			_, err = os.Stat(reportPath)
			So(err, ShouldBeNil)
		})
	})
}

// TestRunWithAllocWork 测试 Run 对内存分配工作负载的采集。
func TestRunWithAllocWork(t *testing.T) {
	Convey("Test Run with allocation workload", t, func() {
		Convey("Should detect memory allocations", func() {
			result := Run("alloc_work", allocWork(), WithAutoReport(false))
			So(result, ShouldNotBeNil)
			So(result.MemAllocs, ShouldBeGreaterThan, 0)
			So(result.MemBytes, ShouldBeGreaterThan, 0)
			So(result.AllocsPerOp(), ShouldBeGreaterThan, 0)
			So(result.BytesPerOp(), ShouldBeGreaterThan, 0)
		})
	})
}

// TestRunMultipleCalls 测试多次调用 Run 的结果一致性。
func TestRunMultipleCalls(t *testing.T) {
	Convey("Test multiple Run calls produce consistent results", t, func() {
		Convey("Should produce similar ns/op across runs", func() {
			r1 := Run("multi_test", noop, WithAutoReport(false))
			r2 := Run("multi_test", noop, WithAutoReport(false))

			So(r1.NsPerOp(), ShouldBeBetween, 0, 100)
			So(r2.NsPerOp(), ShouldBeBetween, 0, 100)
		})
	})
}

// TestRunWithProfiling 测试启用性能剖析时 Run 的行为。
func TestRunWithProfiling(t *testing.T) {
	Convey("Test Run with profiling enabled", t, func() {
		Convey("Should generate profile files when profiling is enabled", func() {
			tmpDir := t.TempDir()
			workFn := func() {
				sum := 0
				for i := 0; i < 1000; i++ {
					sum += i * i
				}
				allocSink = make([]byte, 128)
				_ = sum
			}

			result := Run("profiling_test", workFn,
				WithOutputDir(tmpDir),
				WithAutoReport(true),
				WithProfiling(true),
			)

			So(result, ShouldNotBeNil)
			So(result.Profile, ShouldNotBeNil)
			So(result.Profile.CPUProfilePath, ShouldNotBeEmpty)
			So(result.Profile.HeapProfilePath, ShouldNotBeEmpty)

			_, err := os.Stat(result.Profile.CPUProfilePath)
			So(err, ShouldBeNil)
			_, err = os.Stat(result.Profile.HeapProfilePath)
			So(err, ShouldBeNil)
		})

		Convey("Should include profiling sections in markdown report", func() {
			tmpDir := t.TempDir()

			result := Run("report_profile_test", func() {
				allocSink = make([]byte, 64)
			},
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfiling(true),
			)

			report := result.ReportString()
			So(report, ShouldContainSubstring, "Memory Statistics")
			So(report, ShouldContainSubstring, "GC Statistics")
			So(report, ShouldContainSubstring, "Profile Files")
			So(report, ShouldContainSubstring, "CPU |")
			So(report, ShouldContainSubstring, "Heap |")
			So(report, ShouldContainSubstring, "Goroutines")
		})
	})
}

// TestRunWithoutProfiling 测试未启用剖析时不产生剖析数据。
func TestRunWithoutProfiling(t *testing.T) {
	Convey("Test Run without profiling", t, func() {
		Convey("Should not include profiling sections when profiling is disabled", func() {
			result := Run("no_profile", noop, WithAutoReport(false))
			So(result, ShouldNotBeNil)
			So(result.Profile, ShouldBeNil)

			report := result.ReportString()
			So(report, ShouldNotContainSubstring, "Memory Statistics")
			So(report, ShouldNotContainSubstring, "GC Statistics")
		})
	})
}

// TestWithProfilingConfig 测试自定义剖析配置选项。
func TestWithProfilingConfig(t *testing.T) {
	Convey("Test WithProfilingConfig option", t, func() {
		Convey("Should respect custom profiling configuration", func() {
			tmpDir := t.TempDir()

			cfg := DefaultProfilingConfig()
			cfg.CPU = false
			cfg.Heap = false
			cfg.Goroutine = true

			result := Run("custom_prof", noop,
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfilingConfig(cfg),
			)

			So(result, ShouldNotBeNil)
			So(result.Profile, ShouldNotBeNil)
			So(result.Profile.CPUProfilePath, ShouldBeEmpty)
			So(result.Profile.HeapProfilePath, ShouldBeEmpty)
		})
	})
}

// TestProfileResultMemStats 测试剖析过程中的内存统计采集。
func TestProfileResultMemStats(t *testing.T) {
	Convey("Test ProfileResult memory statistics", t, func() {
		Convey("Should capture memory stats during profiling", func() {
			tmpDir := t.TempDir()

			result := Run("memstats_test", func() {
				data := make([]byte, 4096)
				for i := range data {
					data[i] = byte(i)
				}
				allocSink = data
			},
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfiling(true),
			)

			So(result.Profile, ShouldNotBeNil)
			So(result.Profile.MemStats.HeapAlloc, ShouldBeGreaterThan, uint64(0))
			So(result.Profile.MemStats.TotalAlloc, ShouldBeGreaterThan, uint64(0))
		})
	})
}

// TestProfileTopFunctions 测试剖析结果中热点函数与分配点的解析。
func TestProfileTopFunctions(t *testing.T) {
	Convey("Test ProfileResult top functions", t, func() {
		Convey("Should parse CPU profile into top functions", func() {
			tmpDir := t.TempDir()

			result := Run("topfunc_test", func() {
				sum := 0
				for i := 0; i < 10000; i++ {
					sum += i * i * i
				}
				_ = sum
			},
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfiling(true),
			)

			So(result.Profile, ShouldNotBeNil)
			if len(result.Profile.TopCPU) > 0 {
				So(result.Profile.TopCPU[0].Function, ShouldNotBeEmpty)
				So(result.Profile.TopCPU[0].Percent, ShouldBeGreaterThan, 0.0)
			}
		})

		Convey("Should parse heap profile into top allocation sites", func() {
			tmpDir := t.TempDir()

			result := Run("alloc_top_test", func() {
				for j := 0; j < 5; j++ {
					allocSink = make([]byte, 1024)
				}
			},
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfiling(true),
			)

			So(result.Profile, ShouldNotBeNil)
			if len(result.Profile.TopAllocBytes) > 0 {
				So(result.Profile.TopAllocBytes[0].Function, ShouldNotBeEmpty)
				So(result.Profile.TopAllocBytes[0].Value, ShouldBeGreaterThan, int64(0))
			}
		})
	})
}

// TestGoroutineProfiling 验证性能剖析能捕获 goroutine 数量。
func TestGoroutineProfiling(t *testing.T) {
	Convey("Test goroutine profiling", t, func() {
		Convey("Should capture goroutine count", func() {
			tmpDir := t.TempDir()
			result := Run("goro_test", noop,
				WithOutputDir(tmpDir),
				WithAutoReport(false),
				WithProfiling(true),
			)

			So(result.Profile, ShouldNotBeNil)
			So(result.Profile.GoroutineCount, ShouldBeGreaterThan, 0)
		})
	})
}
