// Package pipeline 提供基于 asynq 的任务队列与分布式调度功能测试。
// 此文件包含游标定时任务调度器的核心计算逻辑与载荷序列化单元测试。
package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// TestComputeCursorTriggers 测试 ComputeCursorTriggers 触发时间点计算逻辑。
func TestComputeCursorTriggers(t *testing.T) {
	Convey("测试 ComputeCursorTriggers 触发时间点计算", t, func() {
		loc := time.UTC
		at := func(h, m, s int) time.Time {
			return time.Date(2026, 8, 11, h, m, s, 0, loc)
		}
		equalTriggers := func(got []time.Time, want ...time.Time) {
			So(len(got), ShouldEqual, len(want))
			for i := range want {
				So(got[i].Equal(want[i]), ShouldBeTrue)
			}
		}

		Convey("积压场景：游标 09:00、当前 09:05:45、间隔 1m，应补投 09:01~09:05 共 5 个触发点", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 5, 45), time.Minute, BacklogPolicy{})
			equalTriggers(triggers, at(9, 1, 0), at(9, 2, 0), at(9, 3, 0), at(9, 4, 0), at(9, 5, 0))
			So(next.Equal(at(9, 5, 0)), ShouldBeTrue)
		})

		Convey("首次运行：无游标时立即触发当前网格点", func() {
			triggers, next := ComputeCursorTriggers(time.Time{}, at(9, 5, 45), time.Minute, BacklogPolicy{})
			equalTriggers(triggers, at(9, 5, 0))
			So(next.Equal(at(9, 5, 0)), ShouldBeTrue)
		})

		Convey("无新增触发点：游标与当前时间处于同一网格点内", func() {
			triggers, next := ComputeCursorTriggers(at(9, 5, 0), at(9, 5, 30), time.Minute, BacklogPolicy{})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(9, 5, 0)), ShouldBeTrue)
		})

		Convey("间隔为 0：返回空触发并保持游标不变", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 5, 0), 0, BacklogPolicy{})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(9, 0, 0)), ShouldBeTrue)
		})

		Convey("游标晚于当前时间（时钟回拨）：不触发且游标保持不变", func() {
			triggers, next := ComputeCursorTriggers(at(9, 6, 0), at(9, 5, 0), time.Minute, BacklogPolicy{})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(9, 6, 0)), ShouldBeTrue)
		})

		Convey("非对齐游标：游标被向下取整到网格点后计算", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 30), at(9, 2, 0), time.Minute, BacklogPolicy{})
			equalTriggers(triggers, at(9, 1, 0), at(9, 2, 0))
			So(next.Equal(at(9, 2, 0)), ShouldBeTrue)
		})

		Convey("小于分钟间隔：按 30s 网格推进", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 1, 0), 30*time.Second, BacklogPolicy{})
			equalTriggers(triggers, at(9, 0, 30), at(9, 1, 0))
			So(next.Equal(at(9, 1, 0)), ShouldBeTrue)
		})

		Convey("禁用积压补投：不生成历史触发点，游标直接对齐到当前网格点", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 5, 45), time.Minute, BacklogPolicy{Enabled: false})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(9, 5, 0)), ShouldBeTrue)
		})

		Convey("窗口裁剪：仅补投窗口期内的触发点", func() {
			// 游标 09:00、当前 09:10:45、窗口 2m：窗口起点 09:08:45 -> 网格 09:08；仅补投 09:09、09:10
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 10, 45), time.Minute, BacklogPolicy{Window: 2 * time.Minute})
			equalTriggers(triggers, at(9, 9, 0), at(9, 10, 0))
			So(next.Equal(at(9, 10, 0)), ShouldBeTrue)
		})

		Convey("生成上限：积压过大时截断本轮，游标推进到最后一个生成触发点", func() {
			triggers, next := ComputeCursorTriggers(at(9, 0, 0), at(9, 20, 0), time.Minute, BacklogPolicy{Limit: 3})
			equalTriggers(triggers, at(9, 1, 0), at(9, 2, 0), at(9, 3, 0))
			So(next.Equal(at(9, 3, 0)), ShouldBeTrue)
		})
	})
}

// TestCursorTaskPayload 测试游标任务载荷的序列化与反序列化。
func TestCursorTaskPayload(t *testing.T) {
	Convey("测试 CursorTaskPayload 序列化", t, func() {
		trigger := time.Date(2026, 8, 11, 9, 5, 0, 0, time.UTC)
		payload := CursorTaskPayload{
			TriggerAt: trigger,
			Payload:   map[string]any{"region": "cn"},
		}
		raw, err := json.Marshal(payload)
		So(err, ShouldBeNil)

		So(string(raw), ShouldContainSubstring, `"triggerAt"`)
		So(string(raw), ShouldContainSubstring, `"region":"cn"`)

		var decoded CursorTaskPayload
		So(json.Unmarshal(raw, &decoded), ShouldBeNil)
		So(decoded.TriggerAt.Equal(trigger), ShouldBeTrue)
		So(decoded.Payload["region"], ShouldEqual, "cn")
	})
}
