// Package routes 提供 ETL 管道监控和管理相关的 API 路由处理函数
// 这些函数用于通过 HTTP API 监控和管理异步任务队列（基于 asynq 库）
package routes

import (
	"fmt"
	"sync"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// PipelineProcessingTaskInfo 表示正在处理中的任务信息
// 用于 API 响应中展示当前正在执行的任务详情
type PipelineProcessingTaskInfo struct {
	ID       serializer.HexUUID `json:"id"`       // 任务唯一标识符（UUID格式）
	Queue    string             `json:"queue"`    // 任务所属队列名称
	Type     string             `json:"type"`     // 任务类型标识符
	Payload  string             `json:"payload"`  // 任务负载数据（JSON字符串）
	Started  time.Time          `json:"started"`  // 任务开始处理时间
	Deadline time.Time          `json:"deadline"` // 任务处理截止时间
}

// PipelineServerInfo 表示异步任务处理服务器信息
// 用于展示当前运行的任务处理服务器的状态和配置
type PipelineServerInfo struct {
	ID               serializer.HexUUID            `json:"id"`               // 服务器唯一标识符
	Host             string                        `json:"host"`             // 服务器主机名
	PID              int                           `json:"pid"`              // 服务器进程ID
	TotalConcurrency int                           `json:"totalConcurrency"` // 总并发处理能力
	QueueWeights     map[string]int                `json:"queueWeights"`     // 各队列权重配置
	Started          time.Time                     `json:"started"`          // 服务器启动时间
	Status           string                        `json:"status"`           // 服务器状态（active, stopped等）
	ProcessingTasks  []*PipelineProcessingTaskInfo `json:"processingTasks"`  // 当前正在处理的任务列表
}

// PipelineSchedulerInfo 表示任务调度器信息
// 用于展示周期性任务的调度配置和执行历史
type PipelineSchedulerInfo struct {
	ID       serializer.HexUUID `json:"id"`       // 调度器唯一标识符
	Spec     string             `json:"spec"`     // Cron表达式，定义调度规则
	Next     time.Time          `json:"next"`     // 下一次执行时间
	Prev     *time.Time         `json:"prev"`     // 上一次执行时间（可能为nil）
	TypeName string             `json:"typeName"` // 任务类型名称
	Payload  string             `json:"payload"`  // 任务负载数据（JSON字符串）
}

// PipelineGroupInfo 表示任务分组信息
// 用于展示聚合任务的组统计信息
type PipelineGroupInfo struct {
	Group string `json:"group"` // 分组标识符
	Size  int    `json:"size"`  // 组内任务数量
}

// PipelineQueueInfo 表示异步任务队列的完整状态信息
// 包含队列的统计数据和当前状态，用于监控和诊断队列健康状况
type PipelineQueueInfo struct {
	MemoryUsage    int64                `json:"memoryUsage"`    // 队列内存使用量（字节）
	Latency        time.Duration        `json:"latency"`        // 队列处理延迟
	Size           int                  `json:"size"`           // 队列中任务总数
	Groups         []*PipelineGroupInfo `json:"groups"`         // 队列中的任务分组列表
	Pending        int                  `json:"pending"`        // 等待处理的任务数量
	Active         int                  `json:"active"`         // 正在处理的任务数量
	Scheduled      int                  `json:"scheduled"`      // 已调度的任务数量
	Retry          int                  `json:"retry"`          // 重试中的任务数量
	Archived       int                  `json:"archived"`       // 已归档的任务数量
	Completed      int                  `json:"completed"`      // 已完成的任务数量
	Aggregating    int                  `json:"aggregating"`    // 聚合中的任务数量
	Processed      int                  `json:"processed"`      // 已处理的任务数量（当前统计周期）
	Failed         int                  `json:"failed"`         // 失败的任务数量（当前统计周期）
	ProcessedTotal int                  `json:"processedTotal"` // 总共处理的任务数量
	FailedTotal    int                  `json:"failedTotal"`    // 总共失败的任务数量
	Paused         bool                 `json:"paused"`         // 队列是否被暂停
	RetrievalTime  time.Time            `json:"retrievalTime"`  // 信息检索时间戳
}

// PipelineTaskInfo 表示单个异步任务的详细信息
// 包含任务的完整生命周期信息，用于任务监控和管理
type PipelineTaskInfo struct {
	ID            serializer.HexUUID `json:"id"`            // 任务唯一标识符
	Queue         string             `json:"queue"`         // 任务所属队列名称
	Type          string             `json:"type"`          // 任务类型标识符
	Payload       string             `json:"payload"`       // 任务负载数据（JSON字符串）
	Result        string             `json:"result"`        // 任务处理结果（JSON字符串）
	State         string             `json:"state"`         // 任务当前状态（pending, active, completed等）
	MaxRetry      int                `json:"maxRetry"`      // 最大重试次数
	Retried       int                `json:"retried"`       // 已重试次数
	LastErr       string             `json:"lastErr"`       // 最后一次错误信息
	LastFailedAt  *time.Time         `json:"lastFailedAt"`  // 最后一次失败时间
	NextProcessAt *time.Time         `json:"nextProcessAt"` // 下一次处理时间
	CompletedAt   *time.Time         `json:"completedAt"`   // 完成时间
	Deadline      *time.Time         `json:"deadline"`      // 任务截止时间
	Timeout       time.Duration      `json:"timeout"`       // 任务超时时间
	Retention     time.Duration      `json:"retention"`     // 任务保留时间
	Group         string             `json:"group"`         // 任务分组标识符
	IsOrphaned    bool               `json:"isOrphaned"`    // 是否为孤儿任务（无关联的工作进程）
}

// NewPipelineTaskInfo 将 asynq 库的原始任务信息转换为 API 友好的结构体
// 主要用于格式化时间字段和处理空值，使 API 响应更易读
//
// 参数:
//   - raw: asynq 库的原始任务信息
//
// 返回值:
//   - *PipelineTaskInfo: 转换后的任务信息结构体
func NewPipelineTaskInfo(raw *asynq.TaskInfo) *PipelineTaskInfo {
	var lastFailedAt, nextProcessAt, completedAt, deadline *time.Time
	id := serializer.MustHexUUIDv4FromString(raw.ID)
	if !raw.LastFailedAt.IsZero() {
		lastFailedAt = &raw.LastFailedAt
	}
	if !raw.NextProcessAt.IsZero() {
		nextProcessAt = &raw.NextProcessAt
	}
	if !raw.CompletedAt.IsZero() {
		completedAt = &raw.CompletedAt
	}
	if !raw.Deadline.IsZero() {
		deadline = &raw.Deadline
	}
	return &PipelineTaskInfo{
		ID:            id,
		Queue:         raw.Queue,
		Type:          raw.Type,
		Payload:       string(raw.Payload),
		Result:        string(raw.Result),
		State:         raw.State.String(),
		MaxRetry:      raw.MaxRetry,
		Retried:       raw.Retried,
		LastErr:       raw.LastErr,
		LastFailedAt:  lastFailedAt,
		NextProcessAt: nextProcessAt,
		CompletedAt:   completedAt,
		Deadline:      deadline,
		Timeout:       raw.Timeout,
		Retention:     raw.Retention,
		Group:         raw.Group,
		IsOrphaned:    raw.IsOrphaned,
	}
}

// PipelineQueueDailyStat 表示队列的每日统计信息
// 用于展示队列的历史性能指标，支持趋势分析和监控
type PipelineQueueDailyStat struct {
	Date      time.Time `json:"date"`      // 统计日期
	Processed int       `json:"processed"` // 当日处理的任务数量
	Failed    int       `json:"failed"`    // 当日失败的任务数量
}

// ETLPipelineInspectorBaseTaskIn 是任务查询的基础输入参数结构体
// 包含队列、状态和分组信息，用于筛选特定状态的任务
type ETLPipelineInspectorBaseTaskIn struct {
	Queue string `json:"queue" binding:"required"`                                                                     // 队列名称（必须）
	State string `json:"state" binding:"required,oneof=pending active aggregating scheduled retry archived completed"` // 任务状态（必须，只能是预定义值）
	Group string `json:"group"`                                                                                        // 任务分组标识符（可选）
}

// ETLPipelineInspectorDailyStatIn 是查询每日统计信息的输入参数结构体
// 指定要获取的历史天数，支持预定义的时间范围
type ETLPipelineInspectorDailyStatIn struct {
	Days uint `json:"days" binding:"required,oneof=7 15 30 90 180"` // 历史天数（必须，只能是预定义值）
}

// ETLPipelineInspectorSingleTaskIn 是单任务操作的输入参数结构体
// 用于指定要操作的单个任务所属的队列
type ETLPipelineInspectorSingleTaskIn struct {
	Queue string `json:"queue" binding:"required"` // 队列名称（必须）
}

// ETLPipelineInspectorTaskListIn 是任务列表查询的输入参数结构体
// 包含分页信息和任务筛选条件，用于获取特定状态的任务列表
type ETLPipelineInspectorTaskListIn struct {
	Pagination        // 嵌入分页参数
	Queue      string `json:"queue" binding:"required"`                                                                     // 队列名称（必须）
	State      string `json:"state" binding:"required,oneof=pending active aggregating scheduled retry archived completed"` // 任务状态（必须，只能是预定义值）
	Group      string `json:"group"`                                                                                        // 任务分组标识符（可选）
}

// ETLPipelineInspectorStateTaskDeleteIn 是删除特定状态任务的输入参数结构体
// 用于批量删除指定状态的任务
type ETLPipelineInspectorStateTaskDeleteIn struct {
	Queue string `json:"queue" binding:"required"`                                                              // 队列名称（必须）
	State string `json:"state" binding:"required,oneof=pending aggregating scheduled retry archived completed"` // 任务状态（必须，只能是预定义值）
	Group string `json:"group"`                                                                                 // 任务分组标识符（可选）
}

// ETLPipelineInspectorStateTaskRunIn 是运行特定状态任务的输入参数结构体
// 用于批量重新运行指定状态的任务
type ETLPipelineInspectorStateTaskRunIn struct {
	Queue string `json:"queue" binding:"required"`                                            // 队列名称（必须）
	State string `json:"state" binding:"required,oneof=aggregating scheduled retry archived"` // 任务状态（必须，只能是预定义值）
	Group string `json:"group"`                                                               // 任务分组标识符（可选）
}

// ETLPipelineInspectorServerListOut 是服务器列表查询的输出类型别名
// 返回所有任务处理服务器的信息列表
type ETLPipelineInspectorServerListOut = []*PipelineServerInfo

// ETLPipelineInspectorSchedulerListOut 是调度器列表查询的输出类型别名
// 返回所有任务调度器的信息列表
type ETLPipelineInspectorSchedulerListOut = []*PipelineSchedulerInfo

// ETLPipelineInspectorQueueListOut 是队列列表查询的输出类型别名
// 返回所有队列的完整状态信息，键为队列名称
type ETLPipelineInspectorQueueListOut = map[string]*PipelineQueueInfo

// ETLPipelineInspectorQueueStatOut 是队列统计查询的输出类型别名
// 返回所有队列的每日统计信息，键为队列名称
type ETLPipelineInspectorQueueStatOut = map[string][]*PipelineQueueDailyStat

// ETLPipelineInspectorTaskListOut 是任务列表查询的输出类型别名
// 返回指定状态的任务信息列表
type ETLPipelineInspectorTaskListOut = []*PipelineTaskInfo

// etlPipelineInspectorServerListGet 获取所有异步任务处理服务器的信息
// 该函数通过管道检查器获取所有服务器的状态、配置和当前处理的任务列表
//
// 路由: GET /api/v1/etl/pipeline/inspector/servers
//
// 响应: ETLPipelineInspectorServerListOut
func etlPipelineInspectorServerListGet(c *gin.Context) {
	inspector := manager.GetPipelineInspector(c)
	servers, err := inspector.Servers()
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(ETLPipelineInspectorServerListOut, 0, len(servers))
	for _, server := range servers {
		processingTasks := make([]*PipelineProcessingTaskInfo, 0, len(server.ActiveWorkers))
		for _, task := range server.ActiveWorkers {
			processingTasks = append(processingTasks, &PipelineProcessingTaskInfo{
				Queue:    task.Queue,
				ID:       serializer.MustHexUUIDv4FromString(task.TaskID),
				Type:     task.TaskType,
				Payload:  string(task.TaskPayload),
				Started:  task.Started,
				Deadline: task.Deadline,
			})
		}
		out = append(out, &PipelineServerInfo{
			ID:               serializer.MustHexUUIDv4FromString(server.ID),
			Host:             server.Host,
			PID:              server.PID,
			TotalConcurrency: server.Concurrency,
			QueueWeights:     server.Queues,
			Started:          server.Started,
			Status:           server.Status,
			ProcessingTasks:  processingTasks,
		})
	}
	manager.SimpleOK.AbortWithGinW(c, out)
}

// etlPipelineInspectorSchedulerListGet 获取所有任务调度器的信息
// 该函数返回所有周期性任务的调度配置，包括Cron表达式、下一次执行时间等
//
// 路由: GET /api/v1/etl/pipeline/inspector/schedulers
//
// 响应: ETLPipelineInspectorSchedulerListOut
func etlPipelineInspectorSchedulerListGet(c *gin.Context) {
	inspector := manager.GetPipelineInspector(c)
	schedulers, err := inspector.SchedulerEntries()
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(ETLPipelineInspectorSchedulerListOut, 0, len(schedulers))
	for _, scheduler := range schedulers {
		var prev *time.Time = nil
		if !scheduler.Prev.IsZero() {
			prev = &scheduler.Prev
		}
		info := &PipelineSchedulerInfo{
			ID:       serializer.MustHexUUIDv4FromString(scheduler.ID),
			Spec:     scheduler.Spec,
			Next:     scheduler.Next,
			Prev:     prev,
			TypeName: scheduler.Task.Type(),
			Payload:  string(scheduler.Task.Payload()),
		}
		out = append(out, info)
	}
	manager.SimpleOK.AbortWithGinW(c, out)
}

// etlPipelineInspectorQueueListGet 获取所有任务队列的完整状态信息
// 该函数并发获取每个队列的统计数据和分组信息，返回完整的队列状态
//
// 路由: GET /api/v1/etl/pipeline/inspector/queues
//
// 响应: ETLPipelineInspectorQueueListOut
func etlPipelineInspectorQueueListGet(c *gin.Context) {
	inspector := manager.GetPipelineInspector(c)
	queues, err := inspector.Queues()
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(ETLPipelineInspectorQueueListOut, len(queues))
	var mu sync.Mutex
	wg, _ := errgroupx.WithContext(c)
	for _, queue := range queues {
		func(name string) {
			wg.Go(func() error {
				groups, err := inspector.Groups(name)
				if err != nil {
					return errors.WithStack(err)
				}
				groupInfos := make([]*PipelineGroupInfo, 0, len(groups))
				for _, group := range groups {
					groupInfos = append(groupInfos, &PipelineGroupInfo{
						Group: group.Group,
						Size:  group.Size,
					})
				}
				queueInfo, err := inspector.GetQueueInfo(name)
				if err != nil {
					return errors.WithStack(err)
				}
				mu.Lock()
				out[name] = &PipelineQueueInfo{
					MemoryUsage:    queueInfo.MemoryUsage,
					Latency:        queueInfo.Latency,
					Size:           queueInfo.Size,
					Groups:         groupInfos,
					Pending:        queueInfo.Pending,
					Active:         queueInfo.Active,
					Scheduled:      queueInfo.Scheduled,
					Retry:          queueInfo.Retry,
					Archived:       queueInfo.Archived,
					Completed:      queueInfo.Completed,
					Aggregating:    queueInfo.Aggregating,
					Processed:      queueInfo.Processed,
					Failed:         queueInfo.Failed,
					ProcessedTotal: queueInfo.ProcessedTotal,
					FailedTotal:    queueInfo.FailedTotal,
					Paused:         queueInfo.Paused,
					RetrievalTime:  queueInfo.Timestamp,
				}
				mu.Unlock()
				return nil
			})
		}(queue)
	}
	if err := wg.Wait(); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return

	}
	manager.SimpleOK.AbortWithGinW(c, out)
}

// etlPipelineInspectorQueuePauseToggle 切换队列的暂停/恢复状态
// 该函数根据 running 参数决定是暂停队列还是恢复队列
//
// 参数:
//   - c: Gin 上下文对象，包含 HTTP 请求信息
//   - running: 布尔值，true 表示恢复队列，false 表示暂停队列
func etlPipelineInspectorQueuePauseToggle(c *gin.Context, running bool) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	inspector := manager.GetPipelineInspector(c)
	var todo func(queue string) error
	if running {
		todo = inspector.UnpauseQueue
	} else {
		todo = inspector.PauseQueue
	}
	if err := todo(name); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// etlPipelineInspectorQueueUnpause 恢复指定队列的处理
// 该函数将暂停的队列重新激活，允许继续处理任务
//
// 路由: POST /api/v1/etl/pipeline/inspector/queue/:name/unpause
// 参数:
//   - :name - 队列名称（URL路径参数）
func etlPipelineInspectorQueueUnpause(c *gin.Context) {
	etlPipelineInspectorQueuePauseToggle(c, true)
}

// etlPipelineInspectorQueuePause 暂停指定队列的处理
// 该函数将队列暂时停止，不再处理新任务
//
// 路由: POST /api/v1/etl/pipeline/inspector/queue/:name/pause
// 参数:
//   - :name - 队列名称（URL路径参数）
func etlPipelineInspectorQueuePause(c *gin.Context) {
	etlPipelineInspectorQueuePauseToggle(c, false)
}

// etlPipelineInspectorDailyStatListGet 获取所有队列的每日统计信息
// 该函数并发查询每个队列在指定天数内的历史统计数据，包括每日处理成功和失败的任务数量
//
// 路由: GET /api/v1/etl/pipeline/inspector/queues/daily-stats
// 请求体: ETLPipelineInspectorDailyStatIn
// 响应: ETLPipelineInspectorQueueStatOut
func etlPipelineInspectorDailyStatListGet(c *gin.Context) {
	inspector := manager.GetPipelineInspector(c)
	queues, err := inspector.Queues()
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var in ETLPipelineInspectorDailyStatIn
	err = c.ShouldBindBodyWithJSON(&in)
	if err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	collector := sync.Map{}
	wg, _ := errgroupx.WithContext(c)
	f := func(name string) func() error {
		return func() error {
			history, err := inspector.History(name, int(in.Days))
			if err != nil {
				return errors.WithStack(err)
			}
			stats := make([]*PipelineQueueDailyStat, 0, in.Days)
			for i := len(history) - 1; i >= 0; i-- {
				info := history[i]
				stats = append(stats, &PipelineQueueDailyStat{
					Date:      info.Date,
					Processed: info.Processed,
					Failed:    info.Failed,
				})
			}
			collector.Store(name, stats)
			return nil
		}
	}
	for _, queue := range queues {
		wg.Go(f(queue))
	}

	if err := wg.Wait(); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	out := make(ETLPipelineInspectorQueueStatOut, len(queues))
	collector.Range(func(key, value any) bool {
		if name, ok := key.(string); ok {
			if stats, ok := value.([]*PipelineQueueDailyStat); ok {
				out[name] = stats
			}
		}
		return true
	})
	manager.SimpleOK.AbortWithGinW(c, out)
}

// etlPipelineInspectorTaskListGet 获取指定状态的任务列表
// 该函数根据队列、状态和分组条件分页查询任务列表，支持多种任务状态查询
//
// 路由: GET /api/v1/etl/pipeline/inspector/tasks
// 请求体: ETLPipelineInspectorTaskListIn
// 响应: ETLPipelineInspectorTaskListOut
func etlPipelineInspectorTaskListGet(c *gin.Context) {
	var in ETLPipelineInspectorTaskListIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	page := asynq.Page(int(in.Page))
	pageSize := asynq.PageSize(int(in.PageSize))
	inspector := manager.GetPipelineInspector(c)
	var lister func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	switch in.State {
	case asynq.TaskStateActive.String():
		lister = inspector.ListActiveTasks
	case asynq.TaskStatePending.String():
		lister = inspector.ListPendingTasks
	case asynq.TaskStateScheduled.String():
		lister = inspector.ListScheduledTasks
	case asynq.TaskStateRetry.String():
		lister = inspector.ListRetryTasks
	case asynq.TaskStateArchived.String():
		lister = inspector.ListArchivedTasks
	case asynq.TaskStateCompleted.String():
		lister = inspector.ListCompletedTasks
	case asynq.TaskStateAggregating.String():
		lister = func(s string, lo ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
			return inspector.ListAggregatingTasks(s, in.Group, lo...)
		}
	default:
		panic(fmt.Sprintf("bad state %s does not support", in.State))
	}
	if rawTasks, err := lister(in.Queue, page, pageSize); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		tasks := make(ETLPipelineInspectorTaskListOut, 0, len(rawTasks))
		for _, rawTask := range rawTasks {
			tasks = append(tasks, NewPipelineTaskInfo(rawTask))
		}
		manager.SimpleOK.AbortWithGinW(c, tasks)
		return
	}
}

// etlPipelineInspectorStateTaskDelete 批量删除指定状态的所有任务
// 该函数根据队列、状态和分组条件删除所有匹配的任务，返回删除的任务数量
//
// 路由: POST /api/v1/etl/pipeline/inspector/tasks/delete
// 请求体: ETLPipelineInspectorStateTaskDeleteIn
// 响应: 删除的任务数量（整数）
func etlPipelineInspectorStateTaskDelete(c *gin.Context) {
	var in ETLPipelineInspectorStateTaskDeleteIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var deleter func(s string) (int, error)
	inspector := manager.GetPipelineInspector(c)
	switch in.State {
	case asynq.TaskStatePending.String():
		deleter = inspector.DeleteAllPendingTasks
	case asynq.TaskStateScheduled.String():
		deleter = inspector.DeleteAllScheduledTasks
	case asynq.TaskStateRetry.String():
		deleter = inspector.DeleteAllRetryTasks
	case asynq.TaskStateArchived.String():
		deleter = inspector.DeleteAllArchivedTasks
	case asynq.TaskStateCompleted.String():
		deleter = inspector.DeleteAllCompletedTasks
	case asynq.TaskStateAggregating.String():
		deleter = func(s string) (int, error) {
			return inspector.DeleteAllAggregatingTasks(s, in.Group)
		}
	default:
		panic(fmt.Sprintf("bad state %s does not support", in.State))
	}
	if count, err := deleter(in.Queue); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinW(c, count)
		return
	}
}

// etlPipelineInspectorStateTaskArchive 批量归档指定状态的所有任务
// 该函数根据队列、状态和分组条件归档所有匹配的任务，返回归档的任务数量
// 归档操作将任务移动到归档状态而非永久删除，便于后续审计和恢复
//
// 路由: POST /api/v1/etl/pipeline/inspector/tasks/archive
// 请求体: ETLPipelineInspectorStateTaskDeleteIn
// 响应: 归档的任务数量（整数）
func etlPipelineInspectorStateTaskArchive(c *gin.Context) {
	var in ETLPipelineInspectorStateTaskDeleteIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var archiver func(s string) (int, error)
	inspector := manager.GetPipelineInspector(c)
	switch in.State {
	case asynq.TaskStatePending.String():
		archiver = inspector.ArchiveAllPendingTasks
	case asynq.TaskStateScheduled.String():
		archiver = inspector.ArchiveAllScheduledTasks
	case asynq.TaskStateRetry.String():
		archiver = inspector.ArchiveAllRetryTasks
	case asynq.TaskStateAggregating.String():
		archiver = func(s string) (int, error) {
			return inspector.ArchiveAllAggregatingTasks(s, in.Group)
		}
	default:
		panic(fmt.Sprintf("bad state %s does not support", in.State))
	}
	if count, err := archiver(in.Queue); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinW(c, count)
		return
	}
}

// etlPipelineInspectorStateTaskRun 批量运行指定状态的所有任务
// 该函数根据队列、状态和分组条件立即运行所有匹配的任务，返回运行的任务数量
// 运行操作将任务立即放入队列执行，跳过调度时间限制
//
// 路由: POST /api/v1/etl/pipeline/inspector/tasks/run
// 请求体: ETLPipelineInspectorStateTaskRunIn
// 响应: 运行的任务数量（整数）
func etlPipelineInspectorStateTaskRun(c *gin.Context) {
	var in ETLPipelineInspectorStateTaskRunIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var runner func(s string) (int, error)
	inspector := manager.GetPipelineInspector(c)
	switch in.State {
	case asynq.TaskStateScheduled.String():
		runner = inspector.RunAllScheduledTasks
	case asynq.TaskStateRetry.String():
		runner = inspector.RunAllRetryTasks
	case asynq.TaskStateArchived.String():
		runner = inspector.RunAllArchivedTasks
	case asynq.TaskStateAggregating.String():
		runner = func(s string) (int, error) {
			return inspector.RunAllAggregatingTasks(s, in.Group)
		}
	default:
		panic(fmt.Sprintf("bad state %s does not support", in.State))
	}
	if count, err := runner(in.Queue); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinW(c, count)
		return
	}
}

// etlPipelineInspectorTaskDelete 删除指定的单个任务
// 该函数通过任务ID和队列名称删除指定的异步任务
//
// 路由: POST /api/v1/etl/inspector/task/:id/delete
// 参数:
//   - :id - 任务ID（URL路径参数）
//   - queue - 队列名称（JSON请求体）
func etlPipelineInspectorTaskDelete(c *gin.Context) {
	id, ok := getUUIDParam(c, "id")
	if !ok {
		return
	}
	var in ETLPipelineInspectorSingleTaskIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	inspector := manager.GetPipelineInspector(c)
	if err := inspector.DeleteTask(in.Queue, id.String()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// etlPipelineInspectorTaskArchive 归档指定的单个任务
// 该函数将指定的任务移动到归档状态，而不是永久删除
//
// 路由: POST /api/v1/etl/inspector/task/:id/archive
// 参数:
//   - :id - 任务ID（URL路径参数）
//   - queue - 队列名称（JSON请求体）
func etlPipelineInspectorTaskArchive(c *gin.Context) {
	id, ok := getUUIDParam(c, "id")
	if !ok {
		return
	}
	var in ETLPipelineInspectorSingleTaskIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	inspector := manager.GetPipelineInspector(c)
	if err := inspector.ArchiveTask(in.Queue, id.String()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// etlPipelineInspectorTaskRun 立即运行指定的单个任务
// 该函数将指定的任务立即放入队列执行，跳过调度时间
//
// 路由: POST /api/v1/etl/inspector/task/:id/run
// 参数:
//   - :id - 任务ID（URL路径参数）
//   - queue - 队列名称（JSON请求体）
func etlPipelineInspectorTaskRun(c *gin.Context) {
	id, ok := getUUIDParam(c, "id")
	if !ok {
		return
	}
	var in ETLPipelineInspectorSingleTaskIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	inspector := manager.GetPipelineInspector(c)
	if err := inspector.RunTask(in.Queue, id.String()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// etlPipelineInspectorTaskCancelPorcessing 取消正在处理中的指定任务
// 该函数强制停止正在执行的任务（注意：函数名有拼写错误，应为Processing）
// 取消操作会中断当前正在执行的任务，但不会删除任务记录
//
// 路由: POST /api/v1/etl/inspector/task/:id/cancel-processing
// 参数:
//   - :id - 任务ID（URL路径参数）
//
// 响应: 操作状态（成功或失败）
func etlPipelineInspectorTaskCancelPorcessing(c *gin.Context) {
	id, ok := getUUIDParam(c, "id")
	if !ok {
		return
	}
	inspector := manager.GetPipelineInspector(c)
	if err := inspector.CancelProcessing(id.String()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}
