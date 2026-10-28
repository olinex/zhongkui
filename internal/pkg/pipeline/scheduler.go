// Package pipeline 提供基于 asynq 的任务队列与分布式调度：投递（Client）、消费（Server）、
// 定时触发（UnifiedScheduler）与只读监控（Inspector）。
//
// ## 职责边界
//
// 本包只负责任务的「投递、消费、调度」，不承载任何数据处理逻辑：数据流水线的 extract /
// transform / load 语义属于 internal/pkg/etl，业务处理属于 internal/worker。两者以 etl.Task
// 契约衔接——asynq.Task 满足该契约，故队列任务可直接投给 Handler，本包不依赖具体业务。
//
// ## 投递与消费
//
// Client 封装 asynq 客户端，经 EnqueueContext 投递任务（并提供 NewInspector 供监控）；Server
// 封装 asynq 服务器，以 ServeMux 按任务类型路由到 etl.HandlerFuncs，并在最外层挂
// RecoverWithStack 中间件兜底 panic、把带堆栈的错误回写 asynq 供任务详情查看。是否重试由投递
// 侧的 asynq 选项（如 MaxRetry）决定，消费侧不改变该语义。DedupErrorMsg 对驱动重复拼接的错误
// 文案做去重聚合，降低日志噪声。
//
// ## 统一定时调度器
//
// UnifiedScheduler 以单一调度循环统一承担两种触发模式，二者共享同一 etcd「触发点游标」，
// 由 etcd 选举（ElectionService）保证多实例下仅领导者运行，选举胜出即启动、失败即停止：
//
//   - 游标模式（interval > 0）：把时间对齐到以 interval 为步长、以 Unix 纪元为基准的全局网格，
//     取严格晚于上次游标且不晚于当前网格点的所有网格点作为触发点；
//   - cron 模式（cronSpec != ""）：以 robfig/cron 计算自上次触发点以来的 cron 触发点。
//
// 每个触发点被投递为一条独立任务，载荷统一为 CursorTaskPayload{TriggerAt, Payload}：TriggerAt
// 承载该触发时间点，Payload 透传任务配置中的固定参数。处理器始终以载荷中的 TriggerAt 确定数据
// 范围，因此任务即使因积压或重试而延迟执行，其语义也不漂移；重复投递的载荷完全一致，处理器只要
// 幂等即可保证结果一致。全部触发点入队成功后才经 CAS 事务把游标推进到最新触发点——若已被其他
// 实例推进则放弃写入，避免游标倒退。积压行为由 BacklogPolicy 控制：关闭时不补投历史触发点、直接
// 对齐到当前网格点；窗口（Window）只补投窗口内的触发点；Limit 限制单次补投数量。游标模式另以
// Lookback 把触发基准时刻前移，确保统计的是「已结束的完整周期」而非仍在写入的当前周期。
//
// ## 配置与唯一事实来源
//
// SchedulerTaskConfig 描述单个任务（cronSpec / interval / queue / timeout / maxRetry / enabled /
// retention / lookback / payload 等，字段默认值由 default 标签声明）。配置的唯一事实来源是
// etcd：config.toml 中的默认任务在启动时经 importTaskConfigs 写入（存在即不覆盖），调度循环与
// 前端管理接口都读写同一份 etcd 配置，不存在「文件与运行时」两份状态。ToAsynqConfig 把队列
// 级配置转换为 asynq 原生配置，DescribeConfig / ExtractConfig 供配置描述与加载复用。
// `sched_manager.go` 提供任务配置的 etcd 增删改查与「立即执行」原语，供前端定时任务管理接口与
// 调度循环共用。
//
// ## 设计取舍
//
// 以单一调度循环统一 cron 与 interval 两种模式，而非为各任务各起一个调度器，目的是让「游标读写、
// 选举、补投策略、投递入口」只有一份实现：两种模式的差异被压缩为「如何计算触发点」这一步，其余
// 语义（去重、补投、CAS、幂等）完全共享，减少行为分叉。以 etcd 而非本地文件作为配置的唯一事实
// 来源，使多实例、跨重启的配置天然一致，前端改动立即被调度循环感知，不存在「文件与运行时」两份
// 状态。载荷携带各自的 TriggerAt 且重复投递完全一致，把「任务延迟执行」与「任务语义漂移」彻底
// 解耦：处理器只需按 TriggerAt 划定数据范围并保持幂等，积压与重试便不会改变业务含义。游标经 CAS
// 推进而非无条件写回，避免多实例竞争下把游标推回更早的时间点。多实例安全由 etcd 选举保证——仅
// 领导者运行调度循环，选举胜出即启动、失败即停止，故无需额外的分布式锁。
//
// ## 可观测与运维
//
// 投递与消费的分离让监控成为旁路：NewInspector 以只读方式读取 asynq 运行态，供「数据处理-任务
// 列表」展示队列与执行记录，不影响任务流转；SchedulerTaskConfig 的 retention 控制任务完成/失败
// 后的保留时长，便于事后回看。DedupErrorMsg 对驱动重复拼接的错误文案做聚合，避免同一故障刷屏。
//
// 本文件（scheduler.go）承载调度循环、游标推进与 etcd 配置读写；投递与消费见 client.go / server.go。
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/creasty/defaults"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/clientv3util"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
)

var (
	// 编译期断言：确保 UnifiedScheduler 实现了 etcd.ElectionHandler 接口
	_ etcd.ElectionHandler = (*UnifiedScheduler)(nil)
)

var (
	// schedulerName 调度器名称常量，用于标识当前服务组件。
	// 此名称会出现在日志和监控指标中，帮助识别调度器实例。
	schedulerName = "pipeline-scheduler"

	// SchedulerCfgEtcdPrefix etcd 键前缀常量，用于存储所有定时任务的配置。
	// 所有定时任务配置都以此前缀存储在 etcd 中，确保配置的组织和隔离。
	SchedulerCfgEtcdPrefix = etcd.WithPrefix("scheduler/cfg/")

	// SchedulerCursorEtcdPrefix etcd 键前缀常量，用于存储定时任务的上一次已投递触发点。
	// 每个任务的触发点以该前缀 + 任务类型为键存储，值为最新触发时间点（RFC3339Nano）。
	// 游标（interval）与 cron 任务共用该 stamp 键，README: 见 cursor.go。
	SchedulerCursorEtcdPrefix = etcd.WithPrefix("scheduler/cursor/")
)

// etcdBaseConfigProvider 负责将定时任务配置导入 etcd，并从 etcd 读取全部定时任务配置。
// 它是调度器配置的唯一事实来源：config.toml 中的默认任务在启动时经 importTaskConfigs
// 写入 etcd，调度循环经 loadSchedulerTaskConfigs 读取，前端管理接口同样以 etcd 为读写目标。
type etcdBaseConfigProvider struct {
	// ctx 上下文对象，用于控制 etcd 操作的超时和取消。
	ctx context.Context

	// client etcd 客户端实例，用于与 etcd 集群通信。
	client *etcd.EtcdClient

	// logger 日志记录器，用于记录配置操作的相关信息。
	logger *logging.Logger

	// forceFlush 强制刷新标志，控制是否覆盖已存在的 etcd 配置键。
	// 当设置为 true 时，会强制覆盖已有配置；为 false 时，只有当键不存在时才写入。
	forceFlush bool
}

// importTaskConfigs 将定时任务配置导入到 etcd 中。
// 该方法遍历提供的配置映射，将每个任务的配置序列化后存储到 etcd 中。
// 根据 forceFlush 标志决定是否覆盖已存在的配置键。
//
// 参数:
//   - cfgs: 任务配置映射，键为任务类型名称，值为对应的任务配置
//
// 返回值:
//   - error: 如果导入过程中出现错误则返回错误，否则返回 nil
func (e *etcdBaseConfigProvider) importTaskConfigs(cfgs map[string]SchedulerTaskConfig) error {
	for taskType, cfg := range cfgs {
		// 嵌套 map value 不会经过 defaults.Set，需要手动补齐默认值
		if err := defaults.Set(&cfg); err != nil {
			return errors.WithStack(err)
		}
		// 构造 etcd 键，格式为 "scheduler/cfg/<taskType>"
		key := fmt.Sprintf(`%s%s`, SchedulerCfgEtcdPrefix, taskType)

		// 序列化任务配置为 JSON
		value, err := json.Marshal(cfg)
		if err != nil {
			return errors.WithStack(err)
		}

		if e.forceFlush {
			// 强制刷新模式：直接覆盖现有配置
			if _, err := e.client.Put(e.ctx, key, string(value)); err != nil {
				return errors.WithStack(err)
			}
			e.logger.Ctx(e.ctx).Infof(`Key %s was force flush into etcd`, key)
		} else {
			// 安全模式：仅当键不存在时才写入配置（事务操作）
			rsp, err := e.client.Txn(e.ctx).
				If(clientv3util.KeyMissing(key)).
				Then(clientv3.OpPut(key, string(value))).
				Commit()
			if err != nil {
				return errors.WithStack(err)
			}
			if rsp.Succeeded {
				e.logger.Ctx(e.ctx).Infof(`Key %s was written into etcd`, key)
			} else {
				e.logger.Ctx(e.ctx).Infof(`Key %s already exists`, key)
			}
		}
	}
	return nil
}

// loadSchedulerTaskConfigs 从 etcd 读取全部定时任务配置并应用默认值。
// 返回以任务类型为键的配置映射，包含未启用的任务，由调用方按需过滤。
// 调度循环与前端管理接口共用，保证统一调度器各消费方看到的配置一致。
func loadSchedulerTaskConfigs(ctx context.Context, client *etcd.EtcdClient) (map[string]SchedulerTaskConfig, error) {
	// 从 etcd 中获取所有以 cfgPrefix 为前缀的键值对
	rsp, err := client.Get(ctx, SchedulerCfgEtcdPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	configs := make(map[string]SchedulerTaskConfig, len(rsp.Kvs))
	for _, kv := range rsp.Kvs {
		var sCfg SchedulerTaskConfig
		// 填充默认值，确保未设置或为零值的字段有合理的默认值
		// 例如 Timeout 默认 24h，避免 0 值导致任务立即超时
		if err := defaults.Set(&sCfg); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := json.Unmarshal(kv.Value, &sCfg); err != nil {
			return nil, errors.WithStack(err)
		}
		// 从 etcd 键中提取任务类型（去除前缀）
		taskType := strings.TrimPrefix(string(kv.Key), SchedulerCfgEtcdPrefix)
		configs[taskType] = sCfg
	}
	return configs, nil
}

// UnifiedScheduler 实现了 etcd.ElectionService 接口的统一定时任务调度器。
// 该结构体以单一调度循环统一承担 cron 表达式（cronSpec）与游标间隔（interval）
// 两种模式的触发点投递：每次运行读取 etcd 全部任务配置，
//   - interval > 0：沿用游标模型，依上次触发点定步长补投；
//   - cronSpec != ""：以 robfig/cron 计算自上次触发点以来的 cron 触发点并补投。
//
// 两种模式共享同一 etcd stamp（触发点游标）与 CAS 推进，均以独立 asynq 任务投递，
// 载荷统一为 CursorTaskPayload{TriggerAt, Payload}。当 etcd 选举胜出后启动，失败则停止。
type UnifiedScheduler struct {
	// client etcd 客户端实例，用于读取任务配置与读写游标。
	client *etcd.EtcdClient

	// enq 任务投递客户端，统一用于向 asynq 队列写入触发任务。
	enq enqueuer

	// loc 时区位置，用于解析 cron 表达式与计算触发点。
	loc *time.Location

	// checkInterval 调度循环检查间隔，每轮执行一次触发点计算与投递。
	checkInterval time.Duration

	// logger 日志记录器。
	logger *logging.Logger

	// stop 停止信号通道，Shutdown 关闭后调度循环退出。
	stop chan struct{}

	// stopOnce 保证 stop 通道仅被关闭一次。
	stopOnce sync.Once
}

// Name 返回调度器的名称。
// 该方法实现了 etcd.ElectionService 接口，用于标识当前服务组件。
//
// 返回值:
//   - string: 调度器名称常量 "pipeline-scheduler"
func (s *UnifiedScheduler) Name() string {
	return schedulerName
}

// CandidateID 返回当前调度器实例的唯一候选者标识。
// 该方法实现了 etcd.ElectionService 接口，用于在 etcd 选举中唯一标识当前实例。
// 标识格式为 "<hostname>-<gid>-<pid>"，确保在多实例环境中每个实例都有唯一标识。
//
// 返回值:
//   - string: 唯一的候选者标识符
func (s *UnifiedScheduler) CandidateID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	// 结合主机名、进程组ID和进程ID生成唯一标识
	return fmt.Sprintf(`%s-%d-%d`, hostname, os.Getgid(), os.Getpid())
}

// Start 启动统一调度循环，阻塞运行直到上下文取消或 Shutdown 被调用。
// 该方法实现了 etcd.ElectionService 接口，在 etcd 选举胜出后被调用。
// 启动后立即执行一轮调度，随后按检查间隔循环执行。
//
// 参数:
//   - ctx context.Context: 上下文对象，取消后调度器停止运行
//
// 返回值:
//   - error: 始终返回 nil（单轮调度失败仅记录日志，不影响循环继续）
func (s *UnifiedScheduler) Start(ctx context.Context) error {
	if err := s.runAll(ctx); err != nil {
		s.logger.Ctx(ctx).Errorf(`Scheduler first run error: %+v`, err)
	}
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stop:
			return nil
		case <-ticker.C:
			if err := s.runAll(ctx); err != nil {
				s.logger.Ctx(ctx).Errorf(`Scheduler run error: %+v`, err)
			}
		}
	}
}

// Shutdown 停止统一调度循环。
// 该方法实现了 etcd.ElectionService 接口，在 etcd 选举失败或服务停止时被调用。
// 通过 sync.Once 保证关闭动作仅执行一次。
//
// 参数:
//   - _ context.Context: 上下文对象（虽然当前未使用，但保持接口一致性）
func (s *UnifiedScheduler) Shutdown(_ context.Context) error {
	s.stopOnce.Do(func() { close(s.stop) })
	return nil
}

// runAll 读取全部任务配置，为所有启用的任务执行一轮触发点推进。
// 单个任务失败仅记录日志，不影响其余任务的推进。
func (s *UnifiedScheduler) runAll(ctx context.Context) error {
	configs, err := loadSchedulerTaskConfigs(ctx, s.client)
	if err != nil {
		return errors.WithStack(err)
	}
	for taskType, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if cfg.Interval > 0 {
			if err := s.cursorRunOnce(ctx, taskType, cfg); err != nil {
				s.logger.Ctx(ctx).Errorf(`Cursor task %s run error: %+v`, taskType, err)
			}
			continue
		}
		if cfg.CronSpec != "" {
			if err := s.cronRunOnce(ctx, taskType, cfg); err != nil {
				s.logger.Ctx(ctx).Errorf(`Cron task %s run error: %+v`, taskType, err)
			}
		}
	}
	return nil
}

// cronRunOnce 为单个 cron 任务执行一轮触发点推进。
// 流程：解析 cron 表达式 → 读取 stamp → 计算 (last, now] 内触发点 → 逐点入队 → CAS 写回新 stamp。
// 语义与游标模式一致：积压错过的触发点逐一补投、同一触发点不重复投递（载荷携带自身触发时间点）。
func (s *UnifiedScheduler) cronRunOnce(ctx context.Context, taskType string, cfg SchedulerTaskConfig) error {
	schedule, err := parseCronSchedule(cfg.CronSpec, s.loc)
	if err != nil {
		return errors.WithStack(err)
	}
	last, err := s.getCursor(ctx, taskType)
	if err != nil {
		return errors.WithStack(err)
	}
	triggers, next := ComputeCronTriggers(last, time.Now().In(s.loc), schedule, backlogPolicy(cfg))
	if len(triggers) == 0 {
		// 无触发点：若游标已推进（如禁用积压补投时直接跳到下一次未来触发点），
		// 仍需写回，否则积压会反复被跳过而游标永不走位
		if next.Equal(last) {
			return nil
		}
		return s.setCursor(ctx, taskType, last, next)
	}
	opts := taskOptions(cfg)
	params := payloadParams(cfg)
	for _, trigger := range triggers {
		task, err := newTriggerTask(taskType, trigger, params, opts)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := s.enq.EnqueueContext(ctx, task); err != nil {
			return errors.WithStack(err)
		}
	}
	s.logger.Ctx(ctx).Infof(
		`Cron task %s enqueued %d triggers [%s ~ %s], cursor %s -> %s`,
		taskType,
		len(triggers),
		triggers[0].Format(cursorLayout),
		triggers[len(triggers)-1].Format(cursorLayout),
		formatCursor(last),
		formatCursor(next),
	)
	return s.setCursor(ctx, taskType, last, next)
}

// cursorRunOnce 为单个 interval 任务执行一轮游标推进。
// 流程：读取游标 → 计算触发时间点 → 逐点入队 → CAS 写回新游标。
// 只有全部触发点入队成功后才推进游标；中途失败时游标保持不变，下一轮会重新计算补投。
// 重复投递的任务载荷一致（携带相同触发时间点），处理器具备幂等性即可保证结果一致。
// 触发基准时刻取 now - Lookback：目标分钟为"已结束的完整分钟"，避免读取仍在写入/未聚合
// 完成的当前分钟导致事件遗漏；Lookback 由配置指定（默认 5m），按任务可调。
func (s *UnifiedScheduler) cursorRunOnce(ctx context.Context, taskType string, cfg SchedulerTaskConfig) error {
	last, err := s.getCursor(ctx, taskType)
	if err != nil {
		return errors.WithStack(err)
	}
	now := time.Now().Add(-cfg.Lookback.Std())
	triggers, next := ComputeCursorTriggers(last, now, cfg.Interval.Std(), backlogPolicy(cfg))
	if len(triggers) == 0 {
		// 无触发点：若游标已推进（如禁用积压补投时直接对齐到当前网格点），
		// 仍需写回，否则积压会反复被跳过而游标永不走位
		if next.Equal(last) {
			return nil
		}
		return s.setCursor(ctx, taskType, last, next)
	}
	opts := taskOptions(cfg)
	// 固定参数透传给每个触发点；保证序列化后 "payload" 始终为对象而非 null
	params := payloadParams(cfg)
	for _, trigger := range triggers {
		task, err := newTriggerTask(taskType, trigger, params, opts)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := s.enq.EnqueueContext(ctx, task); err != nil {
			return errors.WithStack(err)
		}
	}
	s.logger.Ctx(ctx).Infof(
		`Cursor task %s enqueued %d triggers [%s ~ %s], cursor %s -> %s`,
		taskType,
		len(triggers),
		triggers[0].Format(cursorLayout),
		triggers[len(triggers)-1].Format(cursorLayout),
		formatCursor(last),
		formatCursor(next),
	)
	return s.setCursor(ctx, taskType, last, next)
}

// cursorKey 返回指定任务的游标 etcd 键。
func (s *UnifiedScheduler) cursorKey(taskType string) string {
	return fmt.Sprintf(`%s%s`, SchedulerCursorEtcdPrefix, taskType)
}

// getCursor 读取任务的上一次游标，尚未存储时返回零值。
func (s *UnifiedScheduler) getCursor(ctx context.Context, taskType string) (time.Time, error) {
	rsp, err := s.client.Get(ctx, s.cursorKey(taskType))
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
func (s *UnifiedScheduler) setCursor(ctx context.Context, taskType string, last, next time.Time) error {
	key := s.cursorKey(taskType)
	if last.IsZero() {
		// 首次写入：仅当游标键不存在时写入
		rsp, err := s.client.Txn(ctx).
			If(clientv3util.KeyMissing(key)).
			Then(clientv3.OpPut(key, next.Format(cursorLayout))).
			Commit()
		if err != nil {
			return errors.WithStack(err)
		}
		if !rsp.Succeeded {
			s.logger.Ctx(ctx).Warnf(`Cursor %s already initialized by another instance, skip`, key)
		}
		return nil
	}
	// 游标已存在：CAS 保证仅当当前值仍为 last 时推进到 next
	rsp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.Value(key), "=", last.Format(cursorLayout))).
		Then(clientv3.OpPut(key, next.Format(cursorLayout))).
		Commit()
	if err != nil {
		return errors.WithStack(err)
	}
	if !rsp.Succeeded {
		s.logger.Ctx(ctx).Warnf(`Cursor %s has been advanced by another instance, skip advance to %s`, key, next.Format(cursorLayout))
	}
	return nil
}

// NewElectionScheduler 创建并返回一个基于 etcd 选举的统一调度器实例。
// 该函数初始化调度器的核心组件，包括：
// 1. 配置提供器（配置从 config.toml 导入 etcd，从 etcd 读取）
// 2. 统一调度器（以单一循环承担 cron 与 interval 两种模式的触发点投递）
// 3. etcd 选举实例（确保多实例环境下的高可用性）
//
// 参数:
//   - ctx context.Context: 上下文对象，用于控制调度器的生命周期
//   - client *etcd.EtcdClient: etcd 客户端实例，用于配置/触发点存储和选举
//   - redis *redis.Client: Redis 客户端实例，用于任务队列存储
//   - cfg *SchedulerConfig: 调度器配置，包含任务配置和运行参数
//   - logger *logging.Logger: 日志记录器，用于记录调度器运行信息
//
// 返回值:
//   - *etcd.Election: 初始化好的 etcd 选举实例，调用者需启动此选举实例
//
// 注意: 如果初始化过程中出现致命错误（如配置导入失败、时区解析失败等），函数会 panic。
func NewElectionScheduler(
	ctx context.Context,
	client *etcd.EtcdClient,
	redis *redis.Client,
	cfg *SchedulerConfig,
	logger *logging.Logger,
) *etcd.Election {
	// 初始化配置提供器
	provider := etcdBaseConfigProvider{
		ctx:        ctx,
		client:     client,
		logger:     logger,
		forceFlush: cfg.ForceFlush,
	}

	// 将默认任务配置导入到 etcd 中
	err := provider.importTaskConfigs(cfg.Tasks)
	if err != nil {
		panic(fmt.Sprintf(`Failed to import default task config cause: %v`, err))
	}

	// 解析时区配置
	loc, err := cfg.Location.ToSTD()
	if err != nil {
		panic(fmt.Sprintf(`Failed to parse location cause: %v`, err))
	}

	// 任务投递客户端，与 PipelineServer 共享同一 Redis 连接，
	// 故 Shutdown 时仅停止调度循环，不关闭 client，避免关闭共享连接
	enq := NewClient(redis)

	// 创建统一调度器实例
	scheduler := &UnifiedScheduler{
		client:        client,
		enq:           enq,
		loc:           loc,
		checkInterval: cfg.CursorCheckInterval.Std(),
		logger:        logger,
		stop:          make(chan struct{}),
	}

	// 返回 etcd 选举实例，用于多实例环境下的领导者选举
	return etcd.NewElection(
		ctx,
		client,
		logger,
		scheduler,
	)
}
