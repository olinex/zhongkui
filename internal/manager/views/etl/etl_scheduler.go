package etl

import (
	"strings"

	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// EtlSchedulerTaskIn 是定时任务的完整配置输入参数结构体。
// 用于创建与更新：TypeName 唯一标识任务，CronSpec 与 Interval 互斥二选一，
// 其余字段为队列、超时、重试、启用状态与固定载荷。
type EtlSchedulerTaskIn struct {
	TypeName       string                `json:"typeName" binding:"required"` // 任务类型名称（唯一标识，必须）
	CronSpec       string                `json:"cronSpec"`                    // cron 表达式，定义调度规则
	Interval       serializer.DurationNS `json:"interval"`                    // 游标触发间隔，大于 0 时启用游标调度并忽略 cronSpec
	Queue          string                `json:"queue" binding:"required"`    // 任务所属队列名称（必须）
	Timeout        serializer.DurationNS `json:"timeout"`                     // 任务执行超时时间
	MaxRetry       int                   `json:"maxRetry"`                    // 最大重试次数
	Enabled        bool                  `json:"enabled"`                     // 是否启用该定时任务
	Payload        map[string]any        `json:"payload"`                     // 任务载荷数据（固定入参）
	BacklogEnabled bool                  `json:"backlogEnabled"`              // 是否补投积压期间错过的历史触发点
	BacklogWindow  serializer.DurationNS `json:"backlogWindow"`               // 历史触发点最大回溯窗口期，0 表示不限
	BacklogLimit   uint                  `json:"backlogLimit"`                // 单次最多补投历史触发点个数，默认 1000
	Retention      serializer.DurationNS `json:"retention"`                   // 任务完成/失败后的保留时长，大于 0 时成功完成后保留以便查看执行记录
	Lookback       serializer.DurationNS `json:"lookback"`                    // interval 任务触发基准时刻前移的回看偏移，确保读取已聚合完整的分钟，默认 5m
}

// EtlSchedulerTaskToggleIn 是定时任务启停的输入参数结构体。
// 仅需任务类型与目标启用状态。
type EtlSchedulerTaskToggleIn struct {
	TypeName string `json:"typeName" binding:"required"` // 任务类型名称（唯一标识，必须）
	Enabled  bool   `json:"enabled"`                     // 目标启用状态
}

// EtlSchedulerTaskKeyIn 是按任务类型定位单个定时任务的输入参数结构体。
// 供删除与"立即执行"接口复用。
type EtlSchedulerTaskKeyIn struct {
	TypeName string `json:"typeName" binding:"required"` // 任务类型名称（唯一标识，必须）
}

// validateSchedulerSchedule 校验定时任务调度模式互斥：cronSpec 与 interval 二选一。
// 同时设置（冲突）或同时为空（未设置）均判定非法并写入 i18n 校验错误；
// 合法时返回 true，非法时返回 false 且已向响应写入错误。
func validateSchedulerSchedule(c *gin.Context, cronSpec string, interval serializer.DurationNS) bool {
	hasCron := strings.TrimSpace(cronSpec) != ""
	hasInterval := interval > 0
	switch {
	case hasCron && hasInterval:
		response.CreateSchedulerTaskError("schedulerScheduleConflict").AbortWithGinH(c)
		return false
	case !hasCron && !hasInterval:
		response.CreateSchedulerTaskError("schedulerScheduleEmpty").AbortWithGinH(c)
		return false
	default:
		return true
	}
}

// etlSchedulerTaskInToConfig 将定时任务完整配置输入转换为调度器配置结构。
// 队列/超时/重试等缺失的默认值由 SaveSchedulerTaskConfig 内的 defaults.Set 补齐。
func etlSchedulerTaskInToConfig(in EtlSchedulerTaskIn) pipeline.SchedulerTaskConfig {
	return pipeline.SchedulerTaskConfig{
		CronSpec:       in.CronSpec,
		Interval:       in.Interval,
		Queue:          in.Queue,
		Timeout:        in.Timeout,
		MaxRetry:       in.MaxRetry,
		Enabled:        in.Enabled,
		Payload:        in.Payload,
		BacklogEnabled: in.BacklogEnabled,
		BacklogWindow:  in.BacklogWindow,
		BacklogLimit:   in.BacklogLimit,
		Retention:      in.Retention,
		Lookback:       in.Lookback,
	}
}

// EtlSchedulerTaskCreate 创建一个新的定时任务配置。
// 先校验调度模式互斥，再确保任务类型不重复，最后写入 etcd。
//
// 路由: POST /api/v1/etl/scheduler/task/create
// 请求体: EtlSchedulerTaskIn
func EtlSchedulerTaskCreate(c *gin.Context) {
	var in EtlSchedulerTaskIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if !validateSchedulerSchedule(c, in.CronSpec, in.Interval) {
		return
	}
	client := middlewares.GetEtcdClient(c)
	configs, err := pipeline.LoadSchedulerTaskConfigs(c, client)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if _, exists := configs[in.TypeName]; exists {
		response.CreateSchedulerTaskError("schedulerTaskCreateExist", in.TypeName).AbortWithGinH(c)
		return
	}
	if err := pipeline.SaveSchedulerTaskConfig(c, client, in.TypeName, etlSchedulerTaskInToConfig(in)); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// EtlSchedulerTaskUpdate 更新一个已存在的定时任务配置。
// 先校验调度模式互斥，再确保任务类型存在，最后覆盖写入 etcd。
//
// 路由: POST /api/v1/etl/scheduler/task/update
// 请求体: EtlSchedulerTaskIn
func EtlSchedulerTaskUpdate(c *gin.Context) {
	var in EtlSchedulerTaskIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if !validateSchedulerSchedule(c, in.CronSpec, in.Interval) {
		return
	}
	client := middlewares.GetEtcdClient(c)
	configs, err := pipeline.LoadSchedulerTaskConfigs(c, client)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if _, exists := configs[in.TypeName]; !exists {
		response.CreateSchedulerTaskError("schedulerTaskUpdateNotExist", in.TypeName).AbortWithGinH(c)
		return
	}
	if err := pipeline.SaveSchedulerTaskConfig(c, client, in.TypeName, etlSchedulerTaskInToConfig(in)); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// EtlSchedulerTaskToggle 切换定时任务的启用状态。
// 仅调整 Enabled 字段并回写 etcd，不改动其余配置。
//
// 路由: POST /api/v1/etl/scheduler/task/toggle
// 请求体: EtlSchedulerTaskToggleIn
func EtlSchedulerTaskToggle(c *gin.Context) {
	var in EtlSchedulerTaskToggleIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	client := middlewares.GetEtcdClient(c)
	configs, err := pipeline.LoadSchedulerTaskConfigs(c, client)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	cfg, exists := configs[in.TypeName]
	if !exists {
		response.CreateSchedulerTaskError("schedulerTaskUpdateNotExist", in.TypeName).AbortWithGinH(c)
		return
	}
	cfg.Enabled = in.Enabled
	if err := pipeline.SaveSchedulerTaskConfig(c, client, in.TypeName, cfg); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// EtlSchedulerTaskDelete 删除一个定时任务配置。
// 仅删除配置本身，不影响已在队列中排队的任务实例。
//
// 路由: POST /api/v1/etl/scheduler/task/delete
// 请求体: EtlSchedulerTaskKeyIn
func EtlSchedulerTaskDelete(c *gin.Context) {
	var in EtlSchedulerTaskKeyIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	client := middlewares.GetEtcdClient(c)
	configs, err := pipeline.LoadSchedulerTaskConfigs(c, client)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if _, exists := configs[in.TypeName]; !exists {
		response.CreateSchedulerTaskError("schedulerTaskUpdateNotExist", in.TypeName).AbortWithGinH(c)
		return
	}
	if err := pipeline.DeleteSchedulerTaskConfig(c, client, in.TypeName); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// EtlSchedulerTaskRun 立即执行一个定时任务。
// 以该任务当前配置的队列/超时/重试参数将固定载荷投递到队列，触发时间点为当前时刻。
//
// 路由: POST /api/v1/etl/scheduler/task/run
// 请求体: EtlSchedulerTaskKeyIn
func EtlSchedulerTaskRun(c *gin.Context) {
	var in EtlSchedulerTaskKeyIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	client := middlewares.GetEtcdClient(c)
	configs, err := pipeline.LoadSchedulerTaskConfigs(c, client)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	cfg, exists := configs[in.TypeName]
	if !exists {
		response.CreateSchedulerTaskError("schedulerTaskUpdateNotExist", in.TypeName).AbortWithGinH(c)
		return
	}
	enq := middlewares.GetPipelineClient(c)
	if err := pipeline.EnqueueSchedulerTaskNow(c, enq, in.TypeName, cfg); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
