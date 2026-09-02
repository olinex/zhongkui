// Package pipeline 提供基于 asynq 的任务队列与分布式调度功能。
// 此文件保留游标调度模型的数据结构与纯函数：任务载荷、触发任务构造、触发点计算等；
// 调度循环与游标读写原语已合并进 UnifiedScheduler（见 scheduler.go）。
package pipeline

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// cursorLayout 游标在 etcd 中的序列化格式，使用 RFC3339Nano 保留纳秒精度。
const cursorLayout = time.RFC3339Nano

// CursorTaskPayload 游标定时任务载荷结构。
// 调度器为每个触发时间点投递一个独立任务，载荷中携带该触发时间点（游标）与任务配置
// 中的固定参数。即使任务因积压或重试而延迟执行，其载荷中的游标也不会漂移，
// 处理器始终基于载荷中的触发时间点确定数据范围，保证结果一致且不遗漏。
type CursorTaskPayload struct {
	// TriggerAt 本次触发的游标时间点，处理器应基于该时间点确定本次任务的数据范围。
	TriggerAt time.Time `json:"triggerAt"`

	// Payload 任务配置中携带的固定参数，透传给处理器。
	Payload map[string]any `json:"payload"`
}

// ParseCursorTaskPayload 解析游标任务载荷，返回触发时间点与固定参数。
func ParseCursorTaskPayload(raw []byte) (triggerAt time.Time, payload map[string]any, err error) {
	var tmp CursorTaskPayload
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return triggerAt, payload, fmt.Errorf("failed to parse cursor task payload: %w", err)
	}
	return tmp.TriggerAt, tmp.Payload, nil
}

// newTriggerTask 构造一条携带触发点时间戳的 asynq 定时任务。
// 固定参数经 params 透传并序列化进载荷，保证 "payload" 始终为对象而非 null；
// 触发时间点（游标/ cron 发生时刻）经 CursorTaskPayload.TriggerAt 承载。
func newTriggerTask(taskType string, trigger time.Time, params map[string]any, opts []asynq.Option) (*asynq.Task, error) {
	payload, err := json.Marshal(CursorTaskPayload{TriggerAt: trigger, Payload: params})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(taskType, payload, opts...), nil
}

// taskOptions 构建 asynq 任务选项（队列/超时/最大重试/保留时长）。
// Retention 大于 0 时附加 asynq.Retention，使任务成功（completed）/失败（archived）后
// 在队列中保留 cfg.Retention 时长，从而在任务列表中可查看到执行记录。
func taskOptions(cfg SchedulerTaskConfig) []asynq.Option {
	opts := []asynq.Option{
		asynq.Queue(cfg.Queue),
		asynq.Timeout(cfg.Timeout.Std()),
		asynq.MaxRetry(cfg.MaxRetry),
	}
	if cfg.Retention > 0 {
		opts = append(opts, asynq.Retention(cfg.Retention.Std()))
	}
	return opts
}

// payloadParams 返回任务配置中的固定参数，保证为可序列化的非 null 对象。
func payloadParams(cfg SchedulerTaskConfig) map[string]any {
	if cfg.Payload == nil {
		return map[string]any{}
	}
	return cfg.Payload
}

// formatCursor 格式化游标时间用于日志输出，零值输出 "<none>"。
func formatCursor(t time.Time) string {
	if t.IsZero() {
		return "<none>"
	}
	return t.Format(cursorLayout)
}

// ComputeCursorTriggers 根据上一次游标与当前时间计算需要触发的游标时间点列表。
//
// 触发时间点对齐到以 interval 为步长、以 Unix 纪元为基准的全局时间网格，
// 取严格晚于上一次游标、且不大于当前时间网格点的所有网格点；
// 首次运行（last 为零值）时视为上一次游标为当前网格点的前一网格点，
// 因此会立即触发当前网格点一次，之后按网格持续推进。
//
// 例如 interval 为 1m、last 为 09:00、now 为 09:05:45 时，
// 触发点依次为 09:01、09:02、09:03、09:04、09:05。
//
// 积压行为由 BacklogPolicy 控制：
//   - Enabled=false 时跳过全部积压，不生成任何历史触发点，游标直接对齐到当前网格点；
//   - Window 大于 0 时仅补投 [now-Window, now] 内的触发点，更早的积压直接丢弃；
//   - Limit 限制单次最多生成的触发点个数，防止长间隔导致的积压一次性实例化海量时间戳。
//
// 返回 (triggers, nextCursor)：
//   - triggers: 升序排列的触发时间点，不含上一次游标本身
//   - nextCursor: 应写回 etcd 的新游标，即最后一个触发时间点；无触发时为上一次游标
func ComputeCursorTriggers(last, now time.Time, interval time.Duration, p BacklogPolicy) ([]time.Time, time.Time) {
	if interval <= 0 {
		return nil, last
	}
	step := int64(interval)
	// floorGrid 将时间向下取整到 interval 网格点（基于 Unix 纳秒时间戳）
	floorGrid := func(t time.Time) time.Time {
		return time.Unix(0, t.UnixNano()/step*step).In(t.Location())
	}
	gridNow := floorGrid(now)
	// 禁用积压补投：游标对齐到当前网格点，不生成任何历史触发点
	if !p.Enabled {
		return nil, gridNow
	}
	var prev time.Time
	if last.IsZero() {
		prev = gridNow.Add(-interval)
	} else {
		prev = floorGrid(last)
	}
	// 窗口裁剪：超窗积压直接丢弃，仅从窗口起点网格开始回溯
	if p.Window > 0 {
		windowStart := floorGrid(now.Add(-p.Window))
		if prev.Before(windowStart) {
			prev = windowStart
		}
	}
	var triggers []time.Time
	for t := prev.Add(interval); !t.After(gridNow); t = t.Add(interval) {
		// Limit 为 0 表示不设上限
		if p.Limit > 0 && uint(len(triggers)) >= p.Limit {
			break
		}
		triggers = append(triggers, t)
	}
	if len(triggers) == 0 {
		return nil, last
	}
	return triggers, triggers[len(triggers)-1]
}
