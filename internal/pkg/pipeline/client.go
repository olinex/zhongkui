// Package queue 提供基于 asynq 的事件驱动任务队列功能。
// 该包承载任务队列客户端、服务器、调度器、配置管理和中间件等模块，
// 与数据处理流水线（etl 包）解耦，仅负责任务的投递与消费。

package pipeline

import (
	"context"

	"github.com/hibiken/asynq"
	"zhongkuisec.com/zhongkui/internal/pkg/otel"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
)

// enqueuer 定义了 asynq.Client 的核心方法接口，支持单元测试和追踪包装。
type enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	Close() error
}

// Client 任务队列客户端类型。
// Client 提供了向任务队列提交任务、关闭连接等核心功能。
// 主要方法包括:
//   - Enqueue: 将任务添加到队列中
//   - EnqueueContext: 在指定上下文中将任务添加到队列
//   - Close: 关闭客户端连接
//
// 示例用法:
//
//	client := NewClient(redisClient)
//	task := asynq.NewTask("email:send", []byte("payload"))
//	info, err := client.Enqueue(task)
//	if err != nil {
//	    // 处理错误
//	}
type Client struct {
	// cli asynq 客户端接口，已按 OTEL 是否启用包装追踪能力。
	cli enqueuer
}

// NewClient 创建一个新的任务队列客户端。
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
//	// 创建任务队列客户端
//	queueClient := NewClient(redisClient)
//
//	// 使用客户端提交任务
//	task := asynq.NewTask("process:data", []byte(`{"id": 123}`))
//	info, err := queueClient.Enqueue(task)
//	if err != nil {
//	    // 处理错误
//	}
//
//	// 使用完毕后关闭客户端
//	defer queueClient.Close()
func NewClient(cli *redis.Client) *Client {
	client := asynq.NewClientFromRedisClient(cli.Client)
	if wrap := otel.WrapAsynqClient(client); wrap != nil {
		return &Client{
			cli: wrap,
		}
	} else {
		return &Client{
			cli: client,
		}
	}
}

// EnqueueContext 在指定上下文中将任务添加到队列。
func (c *Client) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return c.cli.EnqueueContext(ctx, task, opts...)
}

// Close 关闭客户端连接。
func (c *Client) Close() error {
	return c.cli.Close()
}
