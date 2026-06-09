// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。
package pipeline

import (
	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// Inspector 任务队列检查器类型别名。
// 这是 github.com/hibiken/asynq.Inspector 的类型别名，用于统一检查器类型。
// Inspector 提供了监控和管理 asynq 任务队列状态的功能，包括：
//   - 查看队列统计信息（队列大小、延迟、内存使用等）
//   - 列出不同状态的任务（待处理、活动、重试、已归档、已完成等）
//   - 管理任务（删除、归档、取消、重新排队等）
//   - 查看服务器信息（工作节点状态、并发度、处理中的任务等）
//   - 查看调度器信息（定时任务配置和执行计划）
//   - 暂停/恢复队列处理
//   - 查看每日统计信息
//
// Inspector 是只读的监控接口，不提供任务提交或处理功能。
// 对于任务提交，请使用 Client；对于任务处理，请使用 Server。
//
// 主要方法包括:
//   - ListQueues: 列出所有队列及其统计信息
//   - ListPendingTasks: 列出队列中待处理的任务
//   - ListActiveTasks: 列出当前正在处理的任务
//   - ListScheduledTasks: 列出计划执行的任务
//   - ListRetryTasks: 列出重试队列中的任务
//   - ListArchivedTasks: 列出已归档的任务
//   - ListCompletedTasks: 列出已完成的任务
//   - ListAggregatingTasks: 列出聚合中的任务
//   - ListServers: 列出所有工作服务器
//   - ListSchedulerEntries: 列出所有调度器条目
//   - GetQueueInfo: 获取队列详细信息
//   - DeleteTask: 删除指定任务
//   - ArchiveTask: 归档指定任务
//   - RunTask: 立即运行指定任务
//   - CancelTask: 取消指定任务
//   - PauseQueue: 暂停队列处理
//   - UnpauseQueue: 恢复队列处理
//   - DeleteAllPendingTasks: 删除所有待处理任务
//   - DeleteAllScheduledTasks: 删除所有计划任务
//   - DeleteAllRetryTasks: 删除所有重试任务
//   - DeleteAllArchivedTasks: 删除所有已归档任务
//   - DeleteAllCompletedTasks: 删除所有已完成任务
//   - ArchiveAllPendingTasks: 归档所有待处理任务
//   - ArchiveAllScheduledTasks: 归档所有计划任务
//   - ArchiveAllRetryTasks: 归档所有重试任务
//   - ArchiveAllCompletedTasks: 归档所有已完成任务
//   - RunAllScheduledTasks: 运行所有计划任务
//   - RunAllRetryTasks: 运行所有重试任务
//   - RunAllArchivedTasks: 运行所有已归档任务
//
// 示例用法:
//
//	// 创建 Redis 客户端
//	redisCfg := &redis.Config{Type: "normal", Addr: "localhost:6379"}
//	redisClient := redis.NewClient(redisCfg)
//	defer redisClient.Close()
//
//	// 创建检查器
//	inspector := NewInspector(redisClient)
//
//	// 列出所有队列
//	queues, err := inspector.ListQueues()
//	if err != nil {
//	    // 处理错误
//	}
//
//	// 列出待处理任务
//	tasks, err := inspector.ListPendingTasks("default", asynq.Page(1), asynq.PageSize(10))
//	if err != nil {
//	    // 处理错误
//	}
//
//	// 获取队列信息
//	info, err := inspector.GetQueueInfo("default")
//	if err != nil {
//	    // 处理错误
//	}
type Inspector = asynq.Inspector

// NewInspector 创建一个新的 Pipeline 检查器。
// 该函数基于传入的 Redis 客户端创建一个 asynq.Inspector 实例。
// asynq 检查器用于监控和管理 Redis 服务器中的异步任务队列状态。
//
// 参数:
//
//	cli: *redis.Client - Redis 客户端实例，用于连接 Redis 服务器。
//	  该客户端必须已经正确配置并可以连接到 Redis 服务器。
//	  检查器通过此客户端与 Redis 通信，读取任务队列的状态信息。
//
// 返回:
//
//	*Inspector - 新创建的 asynq 检查器实例，可以用于监控和管理任务队列。
//
// 注意事项:
//  1. 传入的 Redis 客户端必须已经建立有效的连接
//  2. 检查器是线程安全的，可以在多个 goroutine 中并发使用
//  3. 检查器主要用于监控和管理，不执行任务处理
//  4. 检查器操作可能会对 Redis 服务器产生读取负载，请合理控制调用频率
//  5. 对于大规模队列，某些操作（如列出所有任务）可能会消耗较多内存和时间
//
// 使用场景:
//  1. 监控任务队列的健康状况和性能指标
//  2. 排查任务处理问题（失败任务、超时任务等）
//  3. 管理任务队列（清理、归档、重新排队等）
//  4. 查看系统资源使用情况（内存、并发度等）
//  5. 生成任务处理报告和统计信息
//
// 示例:
//
//	// 创建 Redis 客户端
//	redisCfg := &redis.Config{
//	    Type: "normal",
//	    Addr: "localhost:6379",
//	    DB:   0,
//	}
//	redisClient := redis.NewClient(redisCfg)
//	defer redisClient.Close()
//
//	// 创建检查器
//	inspector := NewInspector(redisClient)
//
//	// 使用检查器监控队列
//	queues, err := inspector.ListQueues()
//	if err != nil {
//	    log.Printf("获取队列列表失败: %v", err)
//	    return
//	}
//
//	for name, info := range queues {
//	    log.Printf("队列 %s: 大小=%d, 延迟=%v, 内存使用=%d",
//	        name, info.Size, info.Latency, info.MemoryUsage)
//	}
func NewInspector(cli *redis.Client) *Inspector {
	return asynq.NewInspectorFromRedisClient(cli)
}
