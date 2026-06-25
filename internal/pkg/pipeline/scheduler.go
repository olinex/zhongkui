// Package pipeline 提供基于 asynq 的任务队列和分布式调度器功能。
// 该包实现了基于 etcd 分布式选举的定时任务调度器，支持多实例部署下的高可用性。
// 调度器通过 etcd 存储定时任务配置，确保配置的一致性和持久化。
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hibiken/asynq"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/clientv3util"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// schedulerLogger 是 asynq.Logger 接口的适配器。
// PeriodicTaskManager 与 Server 共享同一个 Redis 连接，
// 其内部 Scheduler.Shutdown 调用 client.Close() 时会因共享连接返回错误，
// 这是预期行为，不需要作为 error 级别日志输出。
type schedulerLogger struct {
	logger *logging.Logger
}

var _ asynq.Logger = (*schedulerLogger)(nil)

func (l *schedulerLogger) Debug(args ...any) { l.logger.Debug(args...) }
func (l *schedulerLogger) Info(args ...any)  { l.logger.Info(args...) }
func (l *schedulerLogger) Warn(args ...any)  { l.logger.Warn(args...) }
func (l *schedulerLogger) Fatal(args ...any) { l.logger.Fatal(args...) }
func (l *schedulerLogger) Error(args ...any) {
	if len(args) > 0 {
		if s, ok := args[0].(string); ok && strings.Contains(s, "Failed to close redis client connection") {
			l.logger.Debug(args...)
			return
		}
	}
	l.logger.Error(args...)
}

var (
	// schedulerName 调度器名称常量，用于标识当前服务组件。
	// 此名称会出现在日志和监控指标中，帮助识别调度器实例。
	schedulerName = "pipeline-scheduler"

	// cfgPrefix etcd 键前缀常量，用于存储所有定时任务的配置。
	// 所有定时任务配置都以此前缀存储在 etcd 中，确保配置的组织和隔离。
	cfgPrefix = etcd.WithPrefix("scheduler/cfg/")
)

// etcdBaseConfigProvider 实现了 asynq.PeriodicTaskConfigProvider 接口，
// 为定时任务管理器提供从 etcd 读取配置的能力。
// 该结构体负责将定时任务配置从 etcd 中读取并转换为 asynq 可识别的格式。
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
		// 构造 etcd 键，格式为 "scheduler/cfg/<taskType>"
		key := fmt.Sprintf(`%s%s`, cfgPrefix, taskType)

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
			e.logger.Infof(`Key %s was force flush into etcd`, key)
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
				e.logger.Infof(`Key %s was written into etcd`, key)
			} else {
				e.logger.Infof(`Key %s already exists`, key)
			}
		}
	}
	return nil
}

// GetConfigs 从 etcd 中获取所有定时任务配置。
// 该方法实现了 asynq.PeriodicTaskConfigProvider 接口，定时任务管理器会定期调用此方法
// 来获取最新的任务配置，并根据配置的变化动态调整任务调度。
//
// 返回值:
//   - []*asynq.PeriodicTaskConfig: 解析后的定时任务配置列表
//   - error: 如果获取或解析过程中出现错误则返回错误，否则返回 nil
func (e *etcdBaseConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	// 从 etcd 中获取所有以 cfgPrefix 为前缀的键值对
	rsp, err := e.client.Get(e.ctx, cfgPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var configs []*asynq.PeriodicTaskConfig
	for _, kv := range rsp.Kvs {
		// 解析 JSON 格式的任务配置
		var sCfg SchedulerTaskConfig
		if err := json.Unmarshal(kv.Value, &sCfg); err != nil {
			return nil, errors.WithStack(err)
		}

		// 跳过未启用的任务配置
		if !sCfg.Enabled {
			continue
		}

		// 构建 asynq 任务选项
		opts := []asynq.Option{
			asynq.Queue(sCfg.Queue),
			asynq.Timeout(sCfg.Timeout),
			asynq.MaxRetry(sCfg.MaxRetry),
		}

		// 从 etcd 键中提取任务类型（去除前缀）
		taskType := strings.TrimLeft(string(kv.Key), cfgPrefix)

		// 创建定时任务配置
		cfg := &asynq.PeriodicTaskConfig{
			Cronspec: sCfg.CronSpec,
			Task: asynq.NewTask(
				taskType,
				kv.Value, // 使用原始 JSON 作为任务载荷
				opts...,
			),
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

// Scheduler 实现了 etcd.ElectionService 接口的调度器结构体。
// 该结构体包装了 asynq.PeriodicTaskManager，提供了分布式选举所需的生命周期管理功能。
// 当 etcd 选举胜出后，调度器实例将启动定时任务管理器；选举失败时则停止。
type Scheduler struct {
	// manager asynq 定时任务管理器实例，负责调度和执行定时任务。
	manager *asynq.PeriodicTaskManager
}

// Name 返回调度器的名称。
// 该方法实现了 etcd.ElectionService 接口，用于标识当前服务组件。
//
// 返回值:
//   - string: 调度器名称常量 "pipeline-scheduler"
func (s *Scheduler) Name() string {
	return schedulerName
}

// CandidateID 返回当前调度器实例的唯一候选者标识。
// 该方法实现了 etcd.ElectionService 接口，用于在 etcd 选举中唯一标识当前实例。
// 标识格式为 "<hostname>-<gid>-<pid>"，确保在多实例环境中每个实例都有唯一标识。
//
// 返回值:
//   - string: 唯一的候选者标识符
func (s *Scheduler) CandidateID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	// 结合主机名、进程组ID和进程ID生成唯一标识
	return fmt.Sprintf(`%s-%d-%d`, hostname, os.Getgid(), os.Getpid())
}

// Start 启动定时任务管理器。
// 该方法实现了 etcd.ElectionService 接口，在 etcd 选举胜出后被调用。
// 启动后，调度器会开始根据 etcd 中的配置定期调度和执行定时任务。
//
// 参数:
//   - _ context.Context: 上下文对象（虽然当前未使用，但保持接口一致性）
//
// 返回值:
//   - error: 如果启动过程中出现错误则返回错误，否则返回 nil
func (s *Scheduler) Start(_ context.Context) error {
	return s.manager.Start()
}

// Shutdown 停止定时任务管理器。
// 该方法实现了 etcd.ElectionService 接口，在 etcd 选举失败或服务停止时被调用。
// 停止后，调度器将不再调度新的定时任务，但会等待正在执行的任务完成。
//
// 参数:
//   - _ context.Context: 上下文对象（虽然当前未使用，但保持接口一致性）
func (s *Scheduler) Shutdown(_ context.Context) {
	s.manager.Shutdown()
}

// NewElectionScheduler 创建并返回一个基于 etcd 选举的调度器实例。
// 该函数初始化调度器的核心组件，包括：
// 1. 配置提供器（从 etcd 读取配置）
// 2. 定时任务管理器（负责调度和执行任务）
// 3. etcd 选举实例（确保多实例环境下的高可用性）
//
// 参数:
//   - ctx context.Context: 上下文对象，用于控制调度器的生命周期
//   - client *etcd.EtcdClient: etcd 客户端实例，用于配置存储和选举
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

	// 创建 asynq 定时任务管理器
	manager, err := asynq.NewPeriodicTaskManager(
		asynq.PeriodicTaskManagerOpts{
			RedisUniversalClient:       redis.Client,
			PeriodicTaskConfigProvider: &provider,
			SyncInterval:               cfg.SyncInterval,
			SchedulerOpts: &asynq.SchedulerOpts{
				HeartbeatInterval: cfg.HeartbeatInterval,
				Logger:            &schedulerLogger{logger: logger},
				Location:          loc,
			},
		},
	)
	if err != nil {
		panic(fmt.Sprintf(`Failed to create scheduler cause: %v`, err))
	}

	// 创建调度器实例
	scheduler := Scheduler{
		manager: manager,
	}

	// 返回 etcd 选举实例，用于多实例环境下的领导者选举
	return etcd.NewElection(
		ctx,
		client,
		logger,
		&scheduler,
	)
}
