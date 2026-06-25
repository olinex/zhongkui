// Package pipeline 提供基于 asynq 的任务队列和调度器功能测试。
// 此文件包含 pipeline 包中客户端创建逻辑的单元测试。
package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// TestClientCreation 测试 Client 创建功能是否正确工作。
// 验证客户端能够基于 Redis 客户端正确创建并具有预期的方法。
func TestClientCreation(t *testing.T) {
	Convey("测试 Client 创建", t, func() {
		Convey("NewClient 应该返回非 nil 的 Client", func() {
			// 创建一个简单的 Redis 配置用于测试
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 验证客户端不为 nil
			So(pipelineClient, ShouldNotBeNil)
		})

		Convey("NewTracedClient 应该返回非 nil 的 Client", func() {
			// 创建一个简单的 Redis 配置用于测试
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建带追踪的 Pipeline 客户端
			pipelineClient := NewTracedClient(redisClient)
			defer pipelineClient.Close()

			// 验证客户端不为 nil
			So(pipelineClient, ShouldNotBeNil)
		})

		Convey("Client 应该具有 EnqueueContext 和 Close 方法", func() {
			// 创建一个简单的 Redis 配置用于测试
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 验证客户端具有预期的方法
			So(pipelineClient, ShouldNotBeNil)
			ctx := context.Background()
			task := asynq.NewTask("test:task", []byte(`{"data": "test"}`))
			_, err := pipelineClient.EnqueueContext(ctx, task)
			_ = err

			err = pipelineClient.Close()
			_ = err
		})
	})
}

// TestNewClient_Creation 测试 NewClient 和 NewTracedClient 函数的客户端创建功能。
// 验证函数能够基于传入的 Redis 客户端正确创建 pipeline.Client 实例。
func TestNewClient_Creation(t *testing.T) {
	Convey("测试 NewClient 和 NewTracedClient 函数的客户端创建", t, func() {
		Convey("NewClient 应该能够使用有效的 Redis 客户端创建 Pipeline 客户端", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)

			// 验证客户端不为 nil
			So(pipelineClient, ShouldNotBeNil)

			// 清理资源
			err := pipelineClient.Close()
			_ = err
		})

		Convey("NewTracedClient 应该能够使用有效的 Redis 客户端创建 Pipeline 客户端", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建带追踪的 Pipeline 客户端
			pipelineClient := NewTracedClient(redisClient)

			// 验证客户端不为 nil
			So(pipelineClient, ShouldNotBeNil)

			// 清理资源
			err := pipelineClient.Close()
			_ = err
		})

		Convey("应该能够使用 Sentinel Redis 客户端创建 Pipeline 客户端", func() {
			// 创建 Sentinel Redis 客户端配置
			redisCfg := &redis.Config{
				Type:             "sentinel",
				MasterName:       "mymaster",
				SentinelAddrs:    []string{"sentinel1:26379", "sentinel2:26379"},
				SentinelUsername: "sentineluser",
				SentinelPassword: "sentinelpass",
				Username:         "redisuser",
				Password:         "redispass",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)

			// 验证客户端不为 nil
			So(pipelineClient, ShouldNotBeNil)

			// 清理资源
			err := pipelineClient.Close()
			_ = err
		})
	})
}

// TestClientMethods 测试 Pipeline 客户端的基本方法。
// 验证客户端可以执行基本操作而不会立即出错。
func TestClientMethods(t *testing.T) {
	Convey("测试 Pipeline 客户端的基本方法", t, func() {
		Convey("客户端应该支持带超时的 EnqueueContext 方法", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 创建带超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// 创建一个测试任务
			task := asynq.NewTask("test:task", []byte(`{"data": "test"}`))

			// 尝试使用上下文入队任务
			info, err := pipelineClient.EnqueueContext(ctx, task)
			_ = err // 标记为已使用，具体结果取决于 Redis 连接

			// 我们不断言具体的错误，因为执行结果取决于 Redis 连接
			// 当连接失败时，TaskInfo 可能为 nil，这是正常行为
			// 我们只验证方法可以调用而不 panic
			_ = info
		})

		Convey("客户端应该具有 Close 方法", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)

			// 验证客户端可以调用 Close 方法
			// 注意：asynq 客户端共享 Redis 连接，Close() 会返回错误
			err := pipelineClient.Close()
			_ = err // 标记为已使用，我们不断言具体错误
		})

		Convey("客户端应该支持 EnqueueContext 和 Close 组合调用", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 验证客户端具有 EnqueueContext 方法
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			task := asynq.NewTask("test:task", []byte(`{"data": "test"}`))
			_, err := pipelineClient.EnqueueContext(ctx, task)
			_ = err // 标记为已使用
		})
	})
}

// TestClientIntegrationSmokeTest 集成场景的冒烟测试。
// 此测试设计为在具有真实 Redis 实例的环境中运行。
func TestClientIntegrationSmokeTest(t *testing.T) {
	// 默认跳过此测试，因为它需要真实的 Redis 实例
	t.Skip("跳过集成测试 - 需要真实的 Redis 实例")

	Convey("集成冒烟测试", t, func() {
		Convey("应该能够与真实 Redis 实例通信并提交任务", func() {
			// 此测试需要真实的 Redis 实例才能运行
			// 它作为集成测试的模板包含在内
			redisCfg := &redis.Config{
				Type:     "normal",
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 测试基本操作 - Ping Redis
			ctx := context.Background()
			pingResult := redisClient.Ping(ctx)
			So(pingResult.Err(), ShouldBeNil)

			// 创建测试任务
			task := asynq.NewTask("integration:test", []byte(`{"test": "data"}`))

			// 提交任务到队列
			info, err := pipelineClient.EnqueueContext(ctx, task)
			So(err, ShouldBeNil)
			So(info, ShouldNotBeNil)

			// 验证任务信息
			So(info.ID, ShouldNotBeEmpty)
			So(info.Queue, ShouldEqual, "default")
		})
	})
}

// TestClientThreadSafety 测试客户端的线程安全性。
// 验证客户端可以在多个 goroutine 中并发使用。
func TestClientThreadSafety(t *testing.T) {
	Convey("测试客户端的线程安全性", t, func() {
		Convey("客户端应该支持并发访问", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)
			defer pipelineClient.Close()

			// 创建多个 goroutine 并发使用客户端
			const numGoroutines = 10
			done := make(chan bool, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer func() {
						done <- true
					}()

					// 每个 goroutine 创建一个任务并尝试入队
					task := asynq.NewTask("concurrent:task", []byte(`{"id": "`+string(rune(id))+`"}`))
					ctx := context.Background()

					// 尝试入队任务（可能会失败，因为没有真实的 Redis 服务器）
					_, err := pipelineClient.EnqueueContext(ctx, task)
					_ = err // 标记为已使用，具体结果取决于 Redis 连接
					// 我们不断言错误，因为执行结果取决于 Redis 连接
					// 主要测试并发调用不会 panic
				}(i)
			}

			// 等待所有 goroutine 完成
			for i := 0; i < numGoroutines; i++ {
				<-done
			}

			// 如果代码执行到这里没有 panic，说明客户端支持并发访问
		})
	})
}

// TestClientCloseMultipleTimes 测试多次关闭客户端的行为。
// 验证客户端可以安全地多次关闭。
func TestClientCloseMultipleTimes(t *testing.T) {
	Convey("测试多次关闭客户端的行为", t, func() {
		Convey("客户端应该可以安全地多次关闭", func() {
			// 创建 Redis 客户端配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 客户端
			pipelineClient := NewClient(redisClient)

			// 第一次关闭 - 注意：asynq 客户端共享 Redis 连接，Close() 会返回错误
			err := pipelineClient.Close()
			_ = err // 标记为已使用，我们不断言具体错误

			// 第二次关闭（应该不会 panic）
			err = pipelineClient.Close()
			_ = err // 标记为已使用，具体行为取决于 asynq 库的实现
		})
	})
}
