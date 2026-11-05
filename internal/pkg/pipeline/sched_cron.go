package pipeline

import (
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// BacklogPolicy 定时任务积压（历史）触发点的补投控制策略。
// 用于限制长时间停机或游标异常导致的积压 (last, now] 一次性生成海量触发点。
type BacklogPolicy struct {
	// Enabled 是否补投积压触发点；false 时跳过全部积压并推进游标到未来位置。
	Enabled bool
	// Window 最大回溯窗口期；大于 0 时仅补投 [now-Window, now] 内的触发点，0 表示不限。
	Window time.Duration
	// Limit 单次最多补投个数。
	Limit uint
}

// backlogPolicy 从任务配置构建积压补投策略。
func backlogPolicy(c SchedulerTaskConfig) BacklogPolicy {
	return BacklogPolicy{
		Enabled: c.BacklogEnabled,
		Window:  c.BacklogWindow.Std(),
		Limit:   c.BacklogLimit,
	}
}

// ValidateSchedule 校验任务调度模式：cronSpec 与 interval 二选一（互斥）。
// 同时设置或同时为空均返回错误，保证每个定时任务只能采用一种触发方式。
func (c SchedulerTaskConfig) ValidateSchedule() error {
	hasCron := strings.TrimSpace(c.CronSpec) != ""
	hasInterval := c.Interval > 0
	switch {
	case hasCron && hasInterval:
		return errors.New("scheduler task config: cronSpec and interval are mutually exclusive, set only one")
	case !hasCron && !hasInterval:
		return errors.New("scheduler task config: either cronSpec or interval must be set")
	default:
		return nil
	}
}

// parseCronSchedule 解析标准 cron 表达式并应用指定时区。
// 表达式示例："0 * * * *"（每小时）、"*/5 * * * *"（每5分钟）；
// 也支持描述符（如 "@daily"）。解析后的 Schedule 应用于 loc 时区。
func parseCronSchedule(spec string, loc *time.Location) (cron.Schedule, error) {
	s, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// SpecSchedule 是常见 cron 表达式解析结果；其 Next 依据 s.Location 计算，
	// 失败时回退到入参时间自身时区。显式覆盖为配置时区，保证跨实例一致。
	if ss, ok := s.(*cron.SpecSchedule); ok && loc != nil {
		ss.Location = loc
	}
	return s, nil
}

// ComputeCronTriggers 根据上一次已投递触发点（stamp）与当前时间计算本次应投递的 cron 触发点列表。
//
// 语义与游标（interval）模式对齐：仅投递 (last, now] 内的触发点，每个触发点独立投递；
// 首次运行（last 为零值）时立即投递一个下一次触发点并以其作为 stamp，避免服务启动长时间无触发。
// 与游标模式一致：积压期间错过的每个触发点都会被逐一补投，且同一触发点不会被重复投递。
//
// 积压行为由 BacklogPolicy 控制：
//   - Enabled=false 时跳过全部积压，仅投递不晚于当前时刻的最近一次触发点（保证当次触发
//     不会被 schedule.Next 跳过），游标推进到该触发点；
//   - Window 大于 0 时仅补投 [now-Window, now] 内的触发点，更早的积压直接丢弃；
//   - Limit 限制单次最多生成的触发点个数，达到上限即截断本轮的 nextCursor，
//     交由下一轮继续追赶，避免一次性实例化海量时间戳；
//   - 循环内置"必须严格晚于当前游标"的守卫，防止 cron 库 Next 意外不前进导致的死循环。
//
// 返回 (triggers, nextCursor)：
//   - triggers: 升序排列、须投递的触发点列表；首轮非零时为 (last, now] 内的发生时刻
//   - nextCursor: 应写回 etcd 的新 stamp，即最后一个触发点；无触发时返回原 last
func ComputeCronTriggers(last, now time.Time, schedule cron.Schedule, p BacklogPolicy) ([]time.Time, time.Time) {
	if schedule == nil {
		return nil, last
	}
	if last.IsZero() {
		// 首轮：投递下一次触发点并以此为新 stamp
		first := schedule.Next(now)
		return []time.Time{first}, first
	}
	// 禁用积压补投：跳过全部历史触发点，但仍需投递「不晚于当前时刻的最近一次触发点」。
	// 不能直接返回 schedule.Next(now)：它严格指向未来，会把游标推过当前触发点，
	// 之后每轮又取更晚的触发点，导致该任务从此永远不会执行。
	if !p.Enabled {
		latest := time.Time{}
		cursor := last
		for {
			next := schedule.Next(cursor)
			if next.After(now) {
				break
			}
			// 严格递增守卫：库异常不前进时立即退出，避免死循环
			if !next.After(cursor) {
				break
			}
			latest = next
			cursor = next
		}
		if latest.IsZero() {
			return nil, last
		}
		return []time.Time{latest}, latest
	}
	// 窗口裁剪：超窗积压直接丢弃，仅保留窗口期 [now-Window, now] 内的触发点（含边界）
	var triggers []time.Time
	t := last
	for {
		next := schedule.Next(t)
		// 严格递增守卫：cron 库应保证 next 晚于 t；若不前进则立即退出，避免死循环
		if !next.After(t) {
			break
		}
		if next.After(now) {
			break
		}
		// 窗口裁剪：窗口起点之前的积压触发点直接跳过
		if p.Window > 0 && next.Before(now.Add(-p.Window)) {
			t = next
			continue
		}
		// 生成上限：积压过大时截断本轮，剩余触发点由后续轮次追赶（Limit 为 0 表示不设上限）
		if p.Limit > 0 && uint(len(triggers)) >= p.Limit {
			break
		}
		triggers = append(triggers, next)
		t = next
	}
	if len(triggers) == 0 {
		return nil, last
	}
	// t 即为最后一个已生成触发点，作为新 stamp
	return triggers, t
}
