// Package pipeline 提供基于 asynq 的任务队列和调度器配置管理功能。
// 该包定义了任务队列的配置结构，包括并发控制、队列优先级、调度器设置等，
// 并提供了配置提取、描述和转换为 asynq 原生配置的方法。
package pipeline

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// PREFIX 配置前缀，用于在配置文件中标识 pipeline 相关配置。
	// 例如，完整的配置路径为 "pipeline.<pipeline_name>.<config_field>"。
	PREFIX = "pipeline"
)

// Task 任务类型别名，表示 asynq 任务。
// 这是 github.com/hibiken/asynq.Task 的类型别名，用于统一任务类型。
type Task = asynq.Task

// ContextFunc 上下文函数类型，用于获取任务执行的上下文。
// 该函数在创建 asynq 配置时提供基础上下文，用于任务执行时的上下文传递。
type ContextFunc = func() context.Context

// SchedulerTaskConfig 定时任务配置结构，定义了单个定时任务的详细参数。
// 该结构用于配置通过 cron 表达式调度的周期性任务，包括执行时间、队列、重试策略等。
type SchedulerTaskConfig struct {
	// CronSpec cron 表达式，定义任务的调度时间规则。
	// 参考: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler
	CronSpec string `mapstructure:"cronSpec" json:"cron_spec" comment:"cron expression for scheduled task (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // cron 表达式

	// Queue 队列名称，指定任务被发送到的队列。
	// 默认值: "default"
	Queue string `mapstructure:"queue" json:"queue" default:"default" comment:"queue name for the scheduled task (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 队列名称

	// Timeout 任务执行超时时间，指定任务执行的最大时间限制。
	// 默认值: 24h (24小时)
	Timeout time.Duration `mapstructure:"timeout" json:"timeout" default:"24h" comment:"timeout for task execution (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-ShutdownTimeout)"` // 任务执行超时时间

	// MaxRetry 最大重试次数，指定任务失败后的重试次数。
	// 默认值: 3
	MaxRetry int `mapstructure:"maxRetry" json:"max_retry" default:"3" comment:"maximum number of retries on failure (ref: https://pkg.go.dev/github.com/hibiken/asynq#Task-Retry)"` // 最大重试次数

	// Enabled 是否启用此定时任务。
	// 默认值: false
	Enabled bool `mapstructure:"enabled" json:"enabled" default:"false" comment:"whether the scheduled task is enabled (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 是否启用

	// Payload 任务载荷数据，包含任务执行时需要的参数。
	// 默认值: 空映射 {}
	Payload map[string]any `mapstructure:"payload" json:"payload" default:"{}" comment:"task payload data (ref: https://pkg.go.dev/github.com/hibiken/asynq#Task-Payload)"` // 任务载荷数据
}

// SchedulerConfig 调度器配置结构，定义了定时任务调度器的全局设置。
// 该结构包含多个定时任务的配置映射以及调度器的运行参数。
type SchedulerConfig struct {
	// Tasks 定时任务配置映射，键为任务名称，值为对应的任务配置。
	// 默认值: 空映射 {}
	Tasks map[string]SchedulerTaskConfig `mapstructure:"tasks" default:"{}" comment:"map of scheduled task configurations (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 定时任务配置映射

	// Location 时区位置，用于解析 cron 表达式时确定时间参考。
	// 默认值: "UTC"
	Location serializer.Location `mapstructure:"location" default:"UTC" comment:"timezone location for cron expressions (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 时区位置

	// SyncInterval 同步间隔，调度器从数据库同步数据的频率。
	// 默认值: 1m (1分钟)
	SyncInterval time.Duration `mapstructure:"syncInterval" default:"1m" comment:"interval to sync scheduler data from database (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 同步间隔

	// HeartbeatInterval 心跳间隔，调度器发送心跳信号的频率。
	// 默认值: 10s (10秒)
	HeartbeatInterval time.Duration `mapstructure:"heartbeatInterval" default:"10s" comment:"interval for scheduler heartbeat (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 心跳间隔

	// ForceFlush 强制刷新，在同步时是否强制刷新调度器队列。
	// 默认值: false
	ForceFlush bool `mapstructure:"forceFlush" default:"false" comment:"force flush scheduler queue on sync (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 强制刷新
}

// Config Pipeline 配置结构，定义了任务队列服务的完整配置参数。
// 该结构包含并发控制、队列管理、监控指标和调度器配置等各个方面。
type Config struct {
	// Concurrency 最大并发数，控制同时处理的任务数量。
	// -1 表示无限制，0 表示禁用并发处理。
	// 默认值: -1
	Concurrency int `mapstructure:"concurrency" default:"-1" comment:"maximum number of concurrent processing tasks, -1 means unlimited (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-Concurrency)"` // 最大并发数，-1 表示无限制

	// TaskCheckInterval 任务检查间隔，检查队列中是否有新任务的频率。
	// 默认值: 1s (1秒)
	TaskCheckInterval time.Duration `mapstructure:"taskCheckInterval" default:"1s" comment:"interval to check for new tasks in queues (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-TaskCheckInterval)"` // 任务检查间隔

	// HealthCheckInterval 健康检查间隔，检查工作节点健康状态的频率。
	// 默认值: 15s (15秒)
	HealthCheckInterval time.Duration `mapstructure:"healthCheckInterval" default:"15s" comment:"interval to check for workers health (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-HealthCheckInterval)"` // 健康检查间隔

	// DelayedTaskCheckInterval 延迟任务检查间隔，检查延迟任务的频率。
	// 默认值: 5s (5秒)
	DelayedTaskCheckInterval time.Duration `mapstructure:"delayedTaskCheckInterval" default:"5s" comment:"interval to check for delayed tasks (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-DelayedTaskCheckInterval)"` // 延迟任务检查间隔

	// ShutdownTimeout 关闭超时时间，优雅关闭的最大等待时间。
	// 默认值: 10s (10秒)
	ShutdownTimeout time.Duration `mapstructure:"shutdownTimeout" default:"10s" comment:"timeout for graceful shutdown (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-ShutdownTimeout)"` // 关闭超时时间

	// Queues 队列优先级配置，定义不同队列的优先级权重。
	// 数值越高表示优先级越高，格式为 {"队列名": 权重}。
	// 默认值: {"critical": 6, "default": 3, "low": 1}
	Queues map[string]int `mapstructure:"queues" default:"{\"critical\": 6, \"default\": 3, \"low\": 1}" comment:"queue priorities, higher number means higher priority (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-Queues)"` // 队列优先级配置

	// StrictPriority 严格优先级模式，是否严格按照队列优先级顺序处理任务。
	// 如果为 true，则高优先级队列的任务会完全处理完后才处理低优先级队列。
	// 默认值: false
	StrictPriority bool `mapstructure:"strictPriority" default:"false" comment:"enable strict priority mode, process queues in order (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-StrictPriority)"` // 严格优先级模式

	// Scheduler 调度器配置，包含定时任务的相关设置。
	Scheduler SchedulerConfig `mapstructure:"scheduler" comment:"scheduler configuration for scheduled tasks (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 调度器配置

	// Params 任务配置映射，在任务执行时会动态注入任务函数。
	// 默认值: 空映射 {}
	Params map[string]map[string]any `mapstructure:"params" default:"{}" comment:"map of task configurations"` // 任务配置映射
}

// ToAsynqConfig 将 pipeline 配置转换为 asynq 原生配置。
// 此方法将 pipeline.Config 中的设置转换为 github.com/hibiken/asynq.Config 格式，
// 以便直接用于创建 asynq 服务器。
//
// 参数说明:
//   - ctxFunc: 上下文函数，提供任务执行时的基础上下文
//   - logger: 日志记录器，用于 asynq 服务器的日志输出
//
// 返回值:
//
//	返回配置好的 asynq.Config 结构，可直接用于创建 asynq.NewServer
//
// 注意事项:
//   - DelayedTaskCheckInterval 字段未在返回的配置中设置，因为 asynq 已弃用此字段
//   - 返回的配置包含了所有必要的队列管理、并发控制和监控设置
func (c *Config) ToAsynqConfig(
	ctxFunc ContextFunc,
	logger *logging.Logger,
) asynq.Config {
	return asynq.Config{
		BaseContext:         ctxFunc,
		Concurrency:         c.Concurrency,
		TaskCheckInterval:   c.TaskCheckInterval,
		HealthCheckInterval: c.HealthCheckInterval,
		ShutdownTimeout:     c.ShutdownTimeout,
		Queues:              c.Queues,
		StrictPriority:      c.StrictPriority,
		Logger:              logger,
	}
}

func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](PREFIX)
}

func ExtractConfig(v *viper.Viper) (*Config, error) {
	return config.ExtractConfig[Config](v, PREFIX)
}
