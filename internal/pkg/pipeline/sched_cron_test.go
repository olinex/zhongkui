// Package pipeline 提供基于 asynq 的任务队列与分布式调度功能测试。
// 此文件包含 cron 表达式触发时间点计算逻辑的单元测试。

package pipeline

import (
	"testing"
	"time"

	"github.com/creasty/defaults"
	"github.com/robfig/cron/v3"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// mustCron 解析标准 cron 表达式；失败时 panic。仅用于测试。
func mustCron(t *testing.T, spec string, loc *time.Location) cron.Schedule {
	t.Helper()
	s, err := parseCronSchedule(spec, loc)
	if err != nil {
		t.Fatalf("parse cron %q failed: %v", spec, err)
	}
	return s
}

// TestComputeCronTriggers 测试 ComputeCronTriggers 触发时间点计算逻辑。
func TestComputeCronTriggers(t *testing.T) {
	Convey("测试 ComputeCronTriggers 触发时间点计算", t, func() {
		loc := time.UTC
		at := func(h, m int) time.Time {
			return time.Date(2026, 8, 11, h, m, 0, 0, loc)
		}
		hourly := mustCron(t, "0 * * * *", loc) // 每个整点
		equalTriggers := func(got []time.Time, want ...time.Time) {
			So(len(got), ShouldEqual, len(want))
			for i := range want {
				So(got[i].Equal(want[i]), ShouldBeTrue)
			}
		}

		Convey("首次运行：无 stamp 时立即投递下一次触发点并以之为新 stamp", func() {
			triggers, next := ComputeCronTriggers(time.Time{}, at(9, 5), hourly, BacklogPolicy{Enabled: true})
			So(len(triggers), ShouldEqual, 1)
			So(triggers[0].Equal(at(10, 0)), ShouldBeTrue)
			So(next.Equal(at(10, 0)), ShouldBeTrue)
		})

		Convey("积压场景：stamp 08:00、当前 10:30，应补投 09:00 与 10:00 两个触发点", func() {
			triggers, next := ComputeCronTriggers(at(8, 0), at(10, 30), hourly, BacklogPolicy{Enabled: true})
			equalTriggers(triggers, at(9, 0), at(10, 0))
			So(next.Equal(at(10, 0)), ShouldBeTrue)
		})

		Convey("same 触发点：stamp 10:00、当前 10:30（未跨越新整点），无需补投且 stamp 不变", func() {
			triggers, next := ComputeCronTriggers(at(10, 0), at(10, 30), hourly, BacklogPolicy{Enabled: true})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(10, 0)), ShouldBeTrue)
		})

		Convey("stamp 已指向未来触发点，尚未到点：不触发", func() {
			first, _ := ComputeCronTriggers(time.Time{}, at(9, 5), hourly, BacklogPolicy{Enabled: true}) // first = 10:00
			triggers, next := ComputeCronTriggers(first[0], at(9, 30), hourly, BacklogPolicy{Enabled: true})
			So(triggers, ShouldBeNil)
			So(next.Equal(first[0]), ShouldBeTrue)
		})

		Convey("schedule 为空：返回空触发并保持 stamp 不变", func() {
			triggers, next := ComputeCronTriggers(at(9, 0), at(10, 30), nil, BacklogPolicy{Enabled: true})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(9, 0)), ShouldBeTrue)
		})

		Convey("禁用积压补投：跳过历史触发点，仅投递不晚于当前时刻的最近一次触发点", func() {
			triggers, next := ComputeCronTriggers(at(8, 0), at(10, 30), hourly, BacklogPolicy{Enabled: false})
			equalTriggers(triggers, at(10, 0))
			So(next.Equal(at(10, 0)), ShouldBeTrue)
		})

		Convey("禁用积压补投且尚未到点：不触发且 stamp 保持不变", func() {
			triggers, next := ComputeCronTriggers(at(11, 0), at(10, 30), hourly, BacklogPolicy{Enabled: false})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(11, 0)), ShouldBeTrue)
		})

		Convey("窗口裁剪：仅补投窗口期内的触发点，超窗积压直接丢弃", func() {
			// stamp 08:00、当前 10:30、窗口 90m：仅补投 09:00 与 10:00，丢弃更早积压
			triggers, next := ComputeCronTriggers(at(8, 0), at(10, 30), hourly, BacklogPolicy{Enabled: true, Window: 90 * time.Minute})
			equalTriggers(triggers, at(9, 0), at(10, 0))
			So(next.Equal(at(10, 0)), ShouldBeTrue)
		})

		Convey("窗口裁剪后无触发点：保持 stamp 不变", func() {
			// stamp 08:00、当前 10:30、窗口 10m：窗口起点 10:20 之后的下一整点为 11:00，超过 now
			triggers, next := ComputeCronTriggers(at(8, 0), at(10, 30), hourly, BacklogPolicy{Enabled: true, Window: 10 * time.Minute})
			So(triggers, ShouldBeNil)
			So(next.Equal(at(8, 0)), ShouldBeTrue)
		})

		Convey("生成上限：积压过大时截断本轮，stamp 推进到最后一个生成触发点", func() {
			triggers, next := ComputeCronTriggers(at(8, 0), at(13, 0), hourly, BacklogPolicy{Enabled: true, Limit: 3})
			equalTriggers(triggers, at(9, 0), at(10, 0), at(11, 0))
			So(next.Equal(at(11, 0)), ShouldBeTrue)
		})
	})
}

// TestSchedulerTaskConfigBacklogDefault 验证积压补投开关的默认值与显式声明语义。
// 任务配置在导入 etcd 前会经 defaults.Set 补默认值：默认值必须与零值一致，
// 否则配置文件中显式声明的 false 会被当成零值覆盖回默认值（历史上因此导致
// backlog 关闭的任务在 worker 启动时仍补投大量历史任务）。
func TestSchedulerTaskConfigBacklogDefault(t *testing.T) {
	Convey("BacklogEnabled 默认关闭且显式声明不被覆盖", t, func() {
		var cfg SchedulerTaskConfig
		So(defaults.Set(&cfg), ShouldBeNil)
		So(cfg.BacklogEnabled, ShouldBeFalse)

		cfg = SchedulerTaskConfig{BacklogEnabled: false}
		So(defaults.Set(&cfg), ShouldBeNil)
		So(cfg.BacklogEnabled, ShouldBeFalse)

		cfg = SchedulerTaskConfig{BacklogEnabled: true}
		So(defaults.Set(&cfg), ShouldBeNil)
		So(cfg.BacklogEnabled, ShouldBeTrue)
	})
}

// TestValidateSchedule 测试任务调度模式互斥校验。
func TestValidateSchedule(t *testing.T) {
	Convey("测试 ValidateSchedule 互斥校验", t, func() {
		Convey("仅 cronSpec：合法", func() {
			So(SchedulerTaskConfig{CronSpec: "0 * * * *"}.ValidateSchedule(), ShouldBeNil)
		})

		Convey("仅 interval：合法", func() {
			So(SchedulerTaskConfig{Interval: serializer.DurationNS(time.Minute)}.ValidateSchedule(), ShouldBeNil)
		})

		Convey("同时设置 cronSpec 与 interval：非法", func() {
			err := SchedulerTaskConfig{CronSpec: "0 * * * *", Interval: serializer.DurationNS(time.Minute)}.ValidateSchedule()
			So(err, ShouldNotBeNil)
		})

		Convey("二者皆空：非法", func() {
			err := SchedulerTaskConfig{}.ValidateSchedule()
			So(err, ShouldNotBeNil)
		})
	})
}

// TestParseCronSchedule 测试 cron 表达式解析与应用时区。
func TestParseCronSchedule(t *testing.T) {
	Convey("测试 parseCronSchedule 时区应用", t, func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		So(err, ShouldBeNil)
		s, err := parseCronSchedule("0 9 * * *", loc)
		So(err, ShouldBeNil)

		ss, ok := s.(*cron.SpecSchedule)
		So(ok, ShouldBeTrue)
		So(ss.Location, ShouldEqual, loc)

		Convey("无效表达式返回错误", func() {
			_, err := parseCronSchedule("not-a-cron", loc)
			So(err, ShouldNotBeNil)
		})
	})
}
