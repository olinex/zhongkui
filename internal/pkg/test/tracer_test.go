//go:build profile
// +build profile

package test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStageTracerBasic(t *testing.T) {
	Convey("Test StageTracer basic Begin/End functionality", t, func() {
		tracer := NewStageTracer()

		Convey("Should record a single stage correctly", func() {
			tracer.Begin("test.stage")
			tracer.End()

			tracer.mu.Lock()
			rec, ok := tracer.records["test.stage"]
			tracer.mu.Unlock()

			So(ok, ShouldBeTrue)
			So(rec.callCount, ShouldEqual, 1)
			So(rec.totalNs, ShouldBeGreaterThan, 0)
			So(rec.minNs, ShouldEqual, rec.totalNs)
			So(rec.maxNs, ShouldEqual, rec.totalNs)
		})

		Convey("Should record multiple stages separately", func() {
			tracer.Begin("stage.a")
			tracer.End()
			tracer.Begin("stage.b")
			tracer.End()

			tracer.mu.Lock()
			So(len(tracer.records), ShouldEqual, 2)
			ra := tracer.records["stage.a"]
			rb := tracer.records["stage.b"]
			tracer.mu.Unlock()

			So(ra, ShouldNotBeNil)
			So(rb, ShouldNotBeNil)
			So(ra.callCount, ShouldEqual, 1)
			So(rb.callCount, ShouldEqual, 1)
		})

		Convey("Should accumulate multiple calls to same stage", func() {
			tracer.Begin("same.stage")
			tracer.End()
			tracer.Begin("same.stage")
			tracer.End()

			tracer.mu.Lock()
			rec := tracer.records["same.stage"]
			tracer.mu.Unlock()

			So(rec.callCount, ShouldEqual, 2)
			So(rec.totalNs, ShouldBeGreaterThanOrEqualTo, rec.minNs*2)
		})

		Convey("Should handle nested stages", func() {
			tracer.Begin("outer")
			tracer.Begin("inner")
			tracer.End()
			tracer.End()

			tracer.mu.Lock()
			So(len(tracer.records), ShouldEqual, 2)
			outer := tracer.records["outer"]
			inner := tracer.records["inner"]
			tracer.mu.Unlock()

			So(outer, ShouldNotBeNil)
			So(inner, ShouldNotBeNil)
			So(outer.callCount, ShouldEqual, 1)
			So(inner.callCount, ShouldEqual, 1)
		})

		Convey("Should handle End without Begin gracefully", func() {
			tracer.End()
			tracer.End()
			tracer.End()
		})
	})
}

func TestStageTracerMinMax(t *testing.T) {
	Convey("Test StageTracer min/max tracking", t, func() {
		Convey("Should track min and max correctly with varying durations", func() {
			tracer := NewStageTracer()

			tracer.Begin("var.stage")
			tracer.End()

			tracer.Begin("var.stage")
			tracer.End()

			tracer.mu.Lock()
			rec := tracer.records["var.stage"]
			tracer.mu.Unlock()

			So(rec.callCount, ShouldEqual, 2)
			So(rec.minNs, ShouldBeLessThanOrEqualTo, rec.maxNs)
			So(rec.totalNs, ShouldEqual, rec.minNs+rec.maxNs)
		})
	})
}

func TestStageTracerConcurrency(t *testing.T) {
	Convey("Test StageTracer concurrent safety", t, func() {
		tracer := NewStageTracer()
		var wg sync.WaitGroup
		goroutines := 10
		callsPerGoroutine := 100

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < callsPerGoroutine; j++ {
					tracer.Begin("concurrent.stage")
					tracer.End()
				}
			}(i)
		}
		wg.Wait()

		tracer.mu.Lock()
		rec := tracer.records["concurrent.stage"]
		tracer.mu.Unlock()

		So(rec, ShouldNotBeNil)
		So(rec.callCount, ShouldEqual, goroutines*callsPerGoroutine)
	})

	Convey("Test multiple concurrent stages", t, func() {
		tracer := NewStageTracer()
		var wg sync.WaitGroup
		stageNames := []string{"alpha", "beta", "gamma", "delta"}

		for _, name := range stageNames {
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					tracer.Begin(s)
					tracer.End()
				}
			}(name)
		}
		wg.Wait()

		tracer.mu.Lock()
		So(len(tracer.records), ShouldEqual, len(stageNames))
		for _, name := range stageNames {
			rec := tracer.records[name]
			So(rec, ShouldNotBeNil)
			So(rec.callCount, ShouldEqual, 50)
		}
		tracer.mu.Unlock()
	})
}

func TestStageTracerReport(t *testing.T) {
	Convey("Test StageTracer Report output", t, func() {
		tracer := NewStageTracer()

		Convey("Should output report with data", func() {
			tracer.Begin("test.stage")
			tracer.End()

			var buf bytes.Buffer
			tracer.mu.Lock()
			records := make([]*stageRecord, 0, len(tracer.records))
			for _, rec := range tracer.records {
				records = append(records, rec)
			}
			totalNs := int64(0)
			for _, rec := range records {
				totalNs += rec.totalNs
			}
			tracer.mu.Unlock()

			printReport(&buf, records, totalNs)
			output := buf.String()

			So(output, ShouldNotBeEmpty)
			So(strings.Contains(output, "=== 阶段耗时统计报告 ==="), ShouldBeTrue)
			So(strings.Contains(output, "test.stage"), ShouldBeTrue)
			So(strings.Contains(output, "100.0%"), ShouldBeTrue)
		})

		Convey("Should output empty report when no data", func() {
			var buf bytes.Buffer
			printReport(&buf, nil, 0)
			output := buf.String()
			So(output, ShouldNotBeEmpty)
			So(strings.Contains(output, "=== 阶段耗时统计报告 ==="), ShouldBeTrue)
		})
	})
}
