// Package queue 提供基于 asynq 的任务队列和调度器配置管理功能。
// 该包定义了任务队列的配置结构，包括并发控制、队列优先级、调度器设置等，
// 并提供了配置提取、描述和转换为 asynq 原生配置的方法。

package pipeline

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/hibiken/asynq"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// Prefix 配置前缀，用于在配置文件中标识任务队列相关配置。
	// 例如，完整的配置路径为 "pipeline.<queue_name>.<config_field>"。
	// 保留 "pipeline" 原值以兼容既有配置文件与运行环境。
	Prefix = "pipeline"
)

// ContextFunc 上下文函数类型，用于获取任务执行的上下文。
// 该函数在创建 asynq 配置时提供基础上下文，用于任务执行时的上下文传递。
type ContextFunc = func() context.Context

// SchedulerTaskConfig 定时任务配置结构，定义了单个定时任务的详细参数。
// 该结构用于配置通过 cron 表达式调度的周期性任务，包括执行时间、队列、重试策略等。
type SchedulerTaskConfig struct {
	// CronSpec cron 表达式，定义任务的调度时间规则。
	// 参考: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler
	CronSpec string `mapstructure:"cronSpec" json:"cron_spec" comment:"cron 表达式，定义定时任务的调度规则 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // cron 表达式

	// Interval 触发间隔，定义游标定时任务的触发周期。
	// 当大于 0 时，该任务采用游标调度模式：调度器每次运行从 etcd 读取上一次游标，
	// 依据当前时间计算出自游标以来所有需要触发的网格时间点，为每个时间点投递一个携带
	// 该时间点的独立任务（载荷见 CursorTaskPayload），全部投递成功后再将最新时间点作为
	// 新游标写回 etcd。该模式能保证积压期间错过的触发点被逐一补投、且不会重复触发同一
	// 时间点。此时 CronSpec 字段将被忽略。
	// 默认值: 0（0 表示未启用游标调度，任务仍按 CronSpec 调度）
	Interval serializer.DurationNS `mapstructure:"interval" json:"interval" default:"0" comment:"游标定时任务的触发间隔，大于 0 时启用游标调度模式并忽略 cronSpec"` // 触发间隔

	// Queue 队列名称，指定任务被发送到的队列。
	// 默认值: "default"
	Queue string `mapstructure:"queue" json:"queue" default:"default" comment:"定时任务发送到的队列名称 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 队列名称

	// Timeout 任务执行超时时间，指定任务执行的最大时间限制。
	// 默认值: 24h (24小时)
	Timeout serializer.DurationNS `mapstructure:"timeout" json:"timeout" default:"24h" comment:"任务执行超时时间 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-ShutdownTimeout)"` // 任务执行超时时间

	// MaxRetry 最大重试次数，指定任务失败后的重试次数。
	// 默认值: 3
	MaxRetry int `mapstructure:"maxRetry" json:"max_retry" default:"3" comment:"任务失败后的最大重试次数 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Task-Retry)"` // 最大重试次数

	// Enabled 是否启用此定时任务。
	// 默认值: false
	Enabled bool `mapstructure:"enabled" json:"enabled" default:"false" comment:"是否启用该定时任务 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 是否启用

	// BacklogEnabled 是否补投积压期间错过的历史触发点。
	// 长时间停机或游标异常会形成积压 (last, now]；为 false 时跳过全部积压，
	// 不生成任何历史任务，并直接推进游标到下一次未来触发点（cron）或当前网格点（interval）。
	// 默认值: true（补投积压，保持与既有语义一致）
	BacklogEnabled bool `mapstructure:"backlogEnabled" json:"backlog_enabled" default:"true" comment:"是否补投积压期间错过的历史触发点，false 时跳过全部积压"` // 是否补投积压

	// BacklogWindow 历史触发点的最大回溯窗口期。
	// 大于 0 时仅补投 [now-BacklogWindow, now] 内的触发点，更早的积压直接丢弃并推进游标；
	// 0 表示不限窗口（按 last 完整回溯）。
	// 默认值: 0（不限）
	BacklogWindow serializer.DurationNS `mapstructure:"backlogWindow" json:"backlog_window" default:"0" comment:"历史触发点最大回溯窗口期，超过该窗口的积压将丢弃，0 表示不限"` // 最大回溯窗口期

	// BacklogLimit 单次最多补投的历史触发点个数。
	// 默认值: 1000
	BacklogLimit uint `mapstructure:"backlogLimit" json:"backlog_limit" default:"1000" comment:"单次最多补投的历史触发点个数"` // 单次最多补投个数

	// Retention 任务完成/失败后的保留时长。
	// 大于 0 时，任务成功（completed）或失败（archived）后会在 asynq 队列中保留该时长，
	// 便于在"数据处理-任务列表"查看执行记录；0 表示完成任务成功即被清理（不进 completed）。
	// 默认值: 1h
	Retention serializer.DurationNS `mapstructure:"retention" json:"retention" default:"1h" comment:"任务完成/失败后的保留时长，大于 0 时成功完成后保留以便查看执行记录"` // 任务保留时长

	// Lookback 游标触发基准时刻的回看偏移。
	// interval 任务的目标分钟取 now - Lookback，确保读取的是已结束且聚合完整的分钟，
	// 而非仍在写入中的当前分钟，避免检测事件遗漏；0 表示不偏移（保持原行为）。
	// 默认值: 5m
	Lookback serializer.DurationNS `mapstructure:"lookback" json:"lookback" default:"5m" comment:"interval 任务触发基准时刻前移的回看偏移，确保读取已聚合完整的分钟，0 表示不偏移"` // 回看偏移时长

	// Payload 任务载荷数据，包含任务执行时需要的参数。
	// 默认值: 空映射 {}
	Payload map[string]any `mapstructure:"payload" json:"payload" default:"{}" comment:"任务载荷数据 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Task-Payload)"` // 任务载荷数据
}

// SchedulerConfig 调度器配置结构，定义了定时任务调度器的全局设置。
// 该结构包含多个定时任务的配置映射以及调度器的运行参数。
type SchedulerConfig struct {
	// Tasks 定时任务配置映射，键为任务名称，值为对应的任务配置。
	// 默认值: 空映射 {}
	Tasks map[string]SchedulerTaskConfig `mapstructure:"tasks" default:"{}" comment:"定时任务配置映射，键为任务名称 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 定时任务配置映射

	// Location 时区位置，用于解析 cron 表达式时确定时间参考。
	// 默认值: "UTC"
	Location serializer.Location `mapstructure:"location" default:"UTC" comment:"cron 表达式解析时区 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 时区位置

	// SyncInterval 同步间隔，调度器从数据库同步数据的频率。
	// 默认值: 1m (1分钟)
	SyncInterval serializer.DurationNS `mapstructure:"syncInterval" default:"1m" comment:"从数据库同步调度器数据的间隔 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 同步间隔

	// CursorCheckInterval 游标检查间隔，游标调度器执行一轮游标推进计算的循环间隔。
	// 每次运行从 etcd 读取各游标任务的上一次游标，依据当前时间计算并补投错过的触发点，
	// 再写回新游标。该间隔越小，触发延迟越低，但 etcd 读取频率也越高。
	// 默认值: 1m (1分钟)
	CursorCheckInterval serializer.DurationNS `mapstructure:"cursorCheckInterval" default:"1m" comment:"游标调度器检查触发点、执行游标推进计算的间隔"` // 游标检查间隔

	// HeartbeatInterval 心跳间隔，调度器发送心跳信号的频率。
	// 默认值: 10s (10秒)
	HeartbeatInterval serializer.DurationNS `mapstructure:"heartbeatInterval" default:"10s" comment:"调度器心跳发送间隔 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 心跳间隔

	// ForceFlush 强制刷新，在同步时是否强制刷新调度器队列。
	// 默认值: false
	ForceFlush bool `mapstructure:"forceFlush" default:"false" comment:"同步时是否强制刷新调度器队列 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 强制刷新
}

// CustomConfig 自定义配置结构，定义任务和处理器配置映射。
type CustomConfig struct {
	// Tasks 任务配置映射，在任务执行时会动态注入任务函数。
	// 默认值: 空映射 {}
	Tasks map[string]map[string]any `mapstructure:"tasks" default:"{}" comment:"任务配置映射，执行时动态注入任务函数"` // 任务配置映射
	// Processors 处理器配置映射，在任务执行时会动态注入处理器函数。
	// 默认值: 空映射 {}
	Processors map[string]map[string]any `mapstructure:"processors" default:"{}" comment:"处理器配置映射，执行时动态注入处理器函数"` // 处理器配置映射
}

// Config 配置结构，定义了任务队列服务的完整配置参数。
// 该结构包含并发控制、队列管理、监控指标和调度器配置等各个方面。
type Config struct {
	// Concurrency 最大并发数，控制同时处理的任务数量。
	// -1 表示无限制，0 表示禁用并发处理。
	// 默认值: -1
	Concurrency int `mapstructure:"concurrency" default:"-1" comment:"最大并发处理任务数，-1 表示无限制 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-Concurrency)"` // 最大并发数，-1 表示无限制

	// TaskCheckInterval 任务检查间隔，检查队列中是否有新任务的频率。
	// 默认值: 1s (1秒)
	TaskCheckInterval serializer.DurationNS `mapstructure:"taskCheckInterval" default:"1s" comment:"检查队列中新任务的间隔 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-TaskCheckInterval)"` // 任务检查间隔

	// HealthCheckInterval 健康检查间隔，检查工作节点健康状态的频率。
	// 默认值: 15s (15秒)
	HealthCheckInterval serializer.DurationNS `mapstructure:"healthCheckInterval" default:"15s" comment:"检查工作节点健康状态的间隔 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-HealthCheckInterval)"` // 健康检查间隔

	// DelayedTaskCheckInterval 延迟任务检查间隔，检查延迟任务的频率。
	// 默认值: 5s (5秒)
	DelayedTaskCheckInterval serializer.DurationNS `mapstructure:"delayedTaskCheckInterval" default:"5s" comment:"检查延迟任务的间隔 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-DelayedTaskCheckInterval)"` // 延迟任务检查间隔

	// ShutdownTimeout 关闭超时时间，优雅关闭的最大等待时间。
	// 默认值: 10s (10秒)
	ShutdownTimeout serializer.DurationNS `mapstructure:"shutdownTimeout" default:"10s" comment:"优雅关闭的最大等待时间 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-ShutdownTimeout)"` // 关闭超时时间

	// Queues 队列优先级配置，定义不同队列的优先级权重。
	// 数值越高表示优先级越高，格式为 {"队列名": 权重}。
	// 默认值: {"critical": 6, "default": 3, "low": 1}
	Queues map[string]int `mapstructure:"queues" default:"{\"critical\": 6, \"default\": 3, \"low\": 1}" comment:"队列优先级，数值越高优先级越高 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-Queues)"` // 队列优先级配置

	// StrictPriority 严格优先级模式，是否严格按照队列优先级顺序处理任务。
	// 如果为 true，则高优先级队列的任务会完全处理完后才处理低优先级队列。
	// 默认值: false
	StrictPriority bool `mapstructure:"strictPriority" default:"false" comment:"严格优先级模式，按顺序处理队列 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Config-StrictPriority)"` // 严格优先级模式

	// Scheduler 调度器配置，包含定时任务的相关设置。
	Scheduler SchedulerConfig `mapstructure:"scheduler" comment:"定时任务调度器配置 (ref: https://pkg.go.dev/github.com/hibiken/asynq#Scheduler)"` // 调度器配置

	// Custom 自定义配置，在任务执行时会动态注入任务和处理器函数。
	// 默认值: 空映射 {}
	Custom CustomConfig `mapstructure:"custom" comment:"自定义参数，执行时动态注入任务和处理器函数"` //
}

// ToAsynqConfig 将任务队列配置转换为 asynq 原生配置。
// 此方法将 Config 中的设置转换为 github.com/hibiken/asynq.Config 格式，
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
		TaskCheckInterval:   c.TaskCheckInterval.Std(),
		HealthCheckInterval: c.HealthCheckInterval.Std(),
		ShutdownTimeout:     c.ShutdownTimeout.Std(),
		Queues:              c.Queues,
		StrictPriority:      c.StrictPriority,
		Logger:              logger,
	}
}

// DescribeConfig 生成任务队列配置的描述信息
// 利用反射分析 Config 结构体的字段标签（mapstructure、default、comment），
// 生成结构化的配置描述列表，供文档或 UI 动态展示配置项。
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 Viper 配置中提取任务队列配置
// 以 Prefix（"pipeline"）为键前缀从 viper.Viper 实例中读取配置，
// 反序列化为 Config 结构体并应用 default 标签的默认值。
// 返回 (*Config, error)：成功时返回已填充的配置指针，失败时返回错误。
func ExtractConfig(viper *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, Prefix)
}
