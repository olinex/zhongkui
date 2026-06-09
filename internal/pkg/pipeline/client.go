// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。
package pipeline

import (
	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// Client 任务队列客户端类型别名。
// 这是 github.com/hibiken/asynq.Client 的类型别名，用于统一客户端类型。
// Client 提供了向任务队列提交任务、关闭连接等核心功能。
// 主要方法包括:
//   - Enqueue: 将任务添加到队列中
//   - EnqueueContext: 在指定上下文中将任务添加到队列
//   - Close: 关闭客户端连接
//   - SetDefaultOptions: 设置任务的默认选项
//
// 示例用法:
//
//	client := NewClient(redisClient)
//	task := asynq.NewTask("email:send", []byte("payload"))
//	info, err := client.Enqueue(task)
//	if err != nil {
//	    // 处理错误
//	}
type Client = asynq.Client

// NewClient 创建一个新的 Pipeline 客户端。
// 该函数基于传入的 Redis 客户端创建一个 asynq.Client 实例。
// asynq 客户端用于向 Redis 服务器提交异步任务。
//
// 参数:
//
//	cli: *redis.Client - Redis 客户端实例，用于连接 Redis 服务器。
//	  该客户端必须已经正确配置并可以连接到 Redis 服务器。
//
// 返回:
//
//	*Client - 新创建的 asynq 客户端实例，可以用于提交任务到队列。
//
// 注意事项:
//  1. 传入的 Redis 客户端必须已经建立有效的连接
//  2. 创建的客户端在使用完毕后应该调用 Close() 方法关闭连接
//  3. 客户端是线程安全的，可以在多个 goroutine 中并发使用
//
// 示例:
//
//	// 创建 Redis 客户端
//	redisClient := redis.NewClient(&redis.Options{
//	    Addr: "localhost:6379",
//	})
//
//	// 创建 Pipeline 客户端
//	pipelineClient := NewClient(redisClient)
//
//	// 使用客户端提交任务
//	task := asynq.NewTask("process:data", []byte(`{"id": 123}`))
//	info, err := pipelineClient.Enqueue(task)
//	if err != nil {
//	    // 处理错误
//	}
//
//	// 使用完毕后关闭客户端
//	defer pipelineClient.Close()
func NewClient(cli *redis.Client) *Client {
	// 使用 asynq 库提供的 NewClientFromRedisClient 函数创建客户端
	// 该函数会将 Redis 客户端包装为 asynq 客户端，复用相同的连接
	return asynq.NewClientFromRedisClient(cli)
}
