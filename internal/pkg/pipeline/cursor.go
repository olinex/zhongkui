// Package pipeline 提供基于 asynq 的任务队列与分布式调度功能。
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/clientv3util"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
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

func ParseCursorTaskPayload(raw []byte) (triggerAt time.Time, payload map[string]any, err error) {
	var tmp CursorTaskPayload
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return triggerAt, payload, fmt.Errorf("failed to parse cursor task payload: %w", err)
	}
	return tmp.TriggerAt, tmp.Payload, nil
}

// CursorScheduler 游标定时任务调度器。
// 与基于 cron 表达式的 asynq 调度器互补：任务配置中 Interval 大于 0 的任务由本调度器
// 采用"游标推进"模型调度。每次运行从 etcd 读取上一次游标，依据当前时间计算自游标以来
// 所有需要触发的网格时间点，为每个时间点投递一个携带该时间点的独立任务，全部投递成功
// 后再将最新时间点作为新游标写回 etcd。
//
// 该模型天然规避了固定参数调度在积压场景下的缺陷：
//   - 不漏触发：积压期间错过的每个网格时间点都会在恢复后被逐一补投
//   - 不重复触发同一时间点：每个触发点对应独立任务且载荷携带自身时间点，游标推进后不再重投
//   - 游标不漂移：游标仅在全部任务投递成功后推进，任务重试不会影响游标
type CursorScheduler struct {
	// client etcd 客户端，用于读写任务游标。
	client *etcd.EtcdClient

	// enq 任务投递客户端，用于将触发任务写入 asynq 队列。
	enq enqueuer

	// interval 检查间隔，调度器每隔该间隔执行一轮游标推进计算。
	interval time.Duration

	// logger 日志记录器。
	logger *logging.Logger

	// stop 停止信号通道，Shutdown 关闭后调度循环退出。
	stop chan struct{}

	// stopOnce 保证 stop 通道仅被关闭一次。
	stopOnce sync.Once
}

// NewCursorScheduler 创建游标定时任务调度器。
// 参数说明:
//   - client: etcd 客户端，用于读写任务游标
//   - enq: 任务投递客户端（如 pipeline.Client），用于将触发任务写入队列
//   - interval: 检查间隔，调度器按此间隔执行游标推进计算；不大于 0 时回退为 1m
//   - logger: 日志记录器
func NewCursorScheduler(
	client *etcd.EtcdClient,
	enq enqueuer,
	interval time.Duration,
	logger *logging.Logger,
) *CursorScheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &CursorScheduler{
		client:   client,
		enq:      enq,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

// Start 启动游标调度循环，阻塞运行直到上下文取消或 Shutdown 被调用。
// 启动后立即执行一轮游标推进，随后按检查间隔循环执行。
//
// 参数:
//   - ctx context.Context: 上下文对象，取消后调度循环退出
//
// 返回值:
//   - error: 始终返回 nil（单轮推进失败仅记录日志，不影响循环继续）
func (c *CursorScheduler) Start(ctx context.Context) error {
	if err := c.runAll(ctx); err != nil {
		c.logger.Ctx(ctx).Errorf(`Cursor scheduler first run error: %+v`, err)
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.stop:
			return nil
		case <-ticker.C:
			if err := c.runAll(ctx); err != nil {
				c.logger.Ctx(ctx).Errorf(`Cursor scheduler run error: %+v`, err)
			}
		}
	}
}

// Shutdown 停止游标调度循环。
// 与 Election.Shutdown 配合，确保调度器退出时游标循环随之停止。
func (c *CursorScheduler) Shutdown() {
	c.stopOnce.Do(func() { close(c.stop) })
}

// runAll 读取全部任务配置，为所有启用且采用游标调度模式的任务执行一轮游标推进。
// 单个任务推进失败仅记录日志，不影响其余任务的推进。
func (c *CursorScheduler) runAll(ctx context.Context) error {
	configs, err := loadSchedulerTaskConfigs(ctx, c.client)
	if err != nil {
		return errors.WithStack(err)
	}
	for taskType, cfg := range configs {
		// 仅处理启用且配置了触发间隔的游标任务
		if !cfg.Enabled || cfg.Interval <= 0 {
			continue
		}
		if err := c.runOnce(ctx, taskType, cfg); err != nil {
			c.logger.Ctx(ctx).Errorf(`Cursor task %s run error: %+v`, taskType, err)
		}
	}
	return nil
}

// runOnce 为单个游标任务执行一轮游标推进。
// 流程：读取游标 → 计算触发时间点 → 逐点入队 → CAS 写回新游标。
// 只有全部触发点入队成功后才推进游标；中途失败时游标保持不变，下一轮会重新计算补投。
// 重复投递的任务载荷一致（携带相同触发时间点），处理器具备幂等性即可保证结果一致。
func (c *CursorScheduler) runOnce(ctx context.Context, taskType string, cfg SchedulerTaskConfig) error {
	last, err := c.getCursor(ctx, taskType)
	if err != nil {
		return errors.WithStack(err)
	}
	triggers, next := ComputeCursorTriggers(last, time.Now(), cfg.Interval)
	if len(triggers) == 0 {
		return nil
	}
	opts := []asynq.Option{
		asynq.Queue(cfg.Queue),
		asynq.Timeout(cfg.Timeout),
		asynq.MaxRetry(cfg.MaxRetry),
	}
	// 固定参数透传给每个触发点；保证序列化后 "payload" 始终为对象而非 null
	params := cfg.Payload
	if params == nil {
		params = map[string]any{}
	}
	for _, trigger := range triggers {
		payload, err := json.Marshal(CursorTaskPayload{TriggerAt: trigger, Payload: params})
		if err != nil {
			return errors.WithStack(err)
		}
		task := asynq.NewTask(taskType, payload, opts...)
		if _, err := c.enq.EnqueueContext(ctx, task); err != nil {
			return errors.WithStack(err)
		}
	}
	c.logger.Ctx(ctx).Infof(
		`Cursor task %s enqueued %d triggers [%s ~ %s], cursor %s -> %s`,
		taskType,
		len(triggers),
		triggers[0].Format(cursorLayout),
		triggers[len(triggers)-1].Format(cursorLayout),
		formatCursor(last),
		formatCursor(next),
	)
	return c.setCursor(ctx, taskType, last, next)
}

// formatCursor 格式化游标时间用于日志输出，零值输出 "<none>"。
func formatCursor(t time.Time) string {
	if t.IsZero() {
		return "<none>"
	}
	return t.Format(cursorLayout)
}

// cursorKey 返回指定任务的游标 etcd 键。
func (c *CursorScheduler) cursorKey(taskType string) string {
	return fmt.Sprintf(`%s%s`, SchedulerCursorEtcdPrefix, taskType)
}

// getCursor 读取任务的上一次游标，尚未存储时返回零值。
func (c *CursorScheduler) getCursor(ctx context.Context, taskType string) (time.Time, error) {
	rsp, err := c.client.Get(ctx, c.cursorKey(taskType))
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	if len(rsp.Kvs) == 0 {
		return time.Time{}, nil
	}
	t, err := time.Parse(cursorLayout, string(rsp.Kvs[0].Value))
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	return t, nil
}

// setCursor 通过 CAS 事务将游标推进到 next。
// 首写时要求游标键不存在；已存在时要求当前值仍为本次读取的 last，
// 若已被其他实例推进则放弃写入并记录警告，避免游标倒退。
func (c *CursorScheduler) setCursor(ctx context.Context, taskType string, last, next time.Time) error {
	key := c.cursorKey(taskType)
	if last.IsZero() {
		// 首次写入：仅当游标键不存在时写入
		rsp, err := c.client.Txn(ctx).
			If(clientv3util.KeyMissing(key)).
			Then(clientv3.OpPut(key, next.Format(cursorLayout))).
			Commit()
		if err != nil {
			return errors.WithStack(err)
		}
		if !rsp.Succeeded {
			c.logger.Ctx(ctx).Warnf(`Cursor %s already initialized by another instance, skip`, key)
		}
		return nil
	}
	// 游标已存在：CAS 保证仅当当前值仍为 last 时推进到 next
	rsp, err := c.client.Txn(ctx).
		If(clientv3.Compare(clientv3.Value(key), "=", last.Format(cursorLayout))).
		Then(clientv3.OpPut(key, next.Format(cursorLayout))).
		Commit()
	if err != nil {
		return errors.WithStack(err)
	}
	if !rsp.Succeeded {
		c.logger.Ctx(ctx).Warnf(`Cursor %s has been advanced by another instance, skip advance to %s`, key, next.Format(cursorLayout))
	}
	return nil
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
// 返回 (triggers, nextCursor)：
//   - triggers: 升序排列的触发时间点，不含上一次游标本身
//   - nextCursor: 应写回 etcd 的新游标，即最后一个触发时间点；无触发时为上一次游标
func ComputeCursorTriggers(last, now time.Time, interval time.Duration) ([]time.Time, time.Time) {
	if interval <= 0 {
		return nil, last
	}
	step := int64(interval)
	// floorGrid 将时间向下取整到 interval 网格点（基于 Unix 纳秒时间戳）
	floorGrid := func(t time.Time) time.Time {
		return time.Unix(0, t.UnixNano()/step*step).In(t.Location())
	}
	gridNow := floorGrid(now)
	var prev time.Time
	if last.IsZero() {
		prev = gridNow.Add(-interval)
	} else {
		prev = floorGrid(last)
	}
	var triggers []time.Time
	for t := prev.Add(interval); !t.After(gridNow); t = t.Add(interval) {
		triggers = append(triggers, t)
	}
	if len(triggers) == 0 {
		return nil, last
	}
	return triggers, triggers[len(triggers)-1]
}
