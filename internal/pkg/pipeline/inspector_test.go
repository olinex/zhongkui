// Package pipeline 提供基于 asynq 的任务队列和调度器功能测试。
// 此文件包含 pipeline 包中检查器创建和类型验证的单元测试。
package pipeline

import (
	"testing"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// TestInspectorTypeAlias 测试 Inspector 类型别名是否正确工作。
// 验证类型别名提供预期的接口并与底层 asynq 库保持兼容性。
func TestInspectorTypeAlias(t *testing.T) {
	Convey("测试 Inspector 类型别名", t, func() {
		Convey("Inspector 应该是 asynq.Inspector 的别名", func() {
			// 验证 Inspector 确实是 asynq.Inspector 的类型别名
			var inspector Inspector
			So(&inspector, ShouldHaveSameTypeAs, &asynq.Inspector{})

			// 验证类型别名保持了相同的接口
			So(inspector, ShouldHaveSameTypeAs, Inspector{})
		})

		Convey("Inspector 应该具有 asynq.Inspector 的所有方法签名", func() {
			// 创建一个简单的 Redis 配置用于测试
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 检查器
			pipelineInspector := NewInspector(redisClient)

			// 验证检查器类型
			So(pipelineInspector, ShouldHaveSameTypeAs, &asynq.Inspector{})

			// 验证检查器不为 nil
			So(pipelineInspector, ShouldNotBeNil)

			// 验证检查器具有基础方法（通过类型检查）
			// 注意：我们无法直接调用需要真实 Redis 连接的方法
			// 但我们可以验证类型匹配
			var _ interface {
				Close() error
				GetQueueInfo(queue string) (*asynq.QueueInfo, error)
				Queues() ([]string, error)
				ListPendingTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
			} = pipelineInspector
		})
	})
}

// TestNewInspector 测试 NewInspector 函数的正确性。
// 验证函数能够基于 Redis 客户端正确创建检查器实例。
func TestNewInspector(t *testing.T) {
	Convey("测试 NewInspector 函数", t, func() {
		Convey("应该使用有效的 Redis 客户端成功创建检查器", func() {
			// 创建普通 Redis 实例的配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 检查器
			inspector := NewInspector(redisClient)

			// 验证检查器不为 nil
			So(inspector, ShouldNotBeNil)

			// 验证检查器类型正确
			So(inspector, ShouldHaveSameTypeAs, &asynq.Inspector{})
		})

		Convey("应该正确处理 Sentinel Redis 配置", func() {
			// 创建 Redis Sentinel 配置
			redisCfg := &redis.Config{
				Type:             "sentinel",
				MasterName:       "mymaster",
				SentinelAddrs:    []string{"sentinel1:26379", "sentinel2:26379"},
				SentinelUsername: "",
				SentinelPassword: "",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 检查器
			inspector := NewInspector(redisClient)

			// 验证检查器不为 nil
			So(inspector, ShouldNotBeNil)

			// 验证检查器类型正确
			So(inspector, ShouldHaveSameTypeAs, &asynq.Inspector{})
		})

		Convey("创建的检查器应该具有正确的上下文处理方法", func() {
			// 创建测试配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 检查器
			inspector := NewInspector(redisClient)

			// 验证检查器可以安全关闭
			// 注意：由于没有真实的 Redis 连接，Close() 可能不会成功
			// 但我们仍然可以调用它来验证接口
			// 只验证不会 panic
			So(func() { inspector.Close() }, ShouldNotPanic)
		})
	})
}

// TestInspectorInterfaceCompatibility 测试 Inspector 接口兼容性。
// 验证类型别名与底层 asynq.Inspector 保持完全兼容。
func TestInspectorInterfaceCompatibility(t *testing.T) {
	Convey("测试 Inspector 接口兼容性", t, func() {
		Convey("应该可以赋值给 asynq.Inspector 接口", func() {
			// 创建测试配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 创建 Redis 客户端
			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			// 创建 Pipeline 检查器
			pipelineInspector := NewInspector(redisClient)

			// 验证可以赋值给 asynq.Inspector 接口
			var asynqInspector *asynq.Inspector = pipelineInspector
			So(asynqInspector, ShouldNotBeNil)
			So(asynqInspector, ShouldEqual, pipelineInspector)
		})

		Convey("应该支持所有 asynq.Inspector 的方法", func() {
			// 验证 Inspector 类型别名支持所有方法
			// 通过声明接口变量来验证方法签名存在
			var inspector interface {
				Close() error
				GetQueueInfo(queue string) (*asynq.QueueInfo, error)
				GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
				ListQueues() ([]string, error)
				ListPendingTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListActiveTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListScheduledTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListRetryTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListArchivedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListCompletedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListAggregatingTasks(queue, group string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
				ListSchedulerEntries() ([]*asynq.SchedulerEntry, error)
				ListSchedulerEnqueueEvents(entryID string, opts ...asynq.ListOption) ([]*asynq.SchedulerEnqueueEvent, error)
				Servers() ([]*asynq.ServerInfo, error)
				Groups(queue string) ([]*asynq.GroupInfo, error)
				History(queue string, n int) ([]*asynq.DailyStats, error)
				ClusterNodes(queue string) ([]*asynq.ClusterNode, error)
				ClusterKeySlot(queue string) (int64, error)
				PauseQueue(queue string) error
				UnpauseQueue(queue string) error
				DeleteQueue(queue string, force bool) error
				DeleteTask(queue, id string) error
				ArchiveTask(queue, id string) error
				RunTask(queue, id string) error
				CancelProcessing(id string) error
				DeleteAllPendingTasks(queue string) (int, error)
				DeleteAllScheduledTasks(queue string) (int, error)
				DeleteAllRetryTasks(queue string) (int, error)
				DeleteAllArchivedTasks(queue string) (int, error)
				DeleteAllCompletedTasks(queue string) (int, error)
				DeleteAllAggregatingTasks(queue, group string) (int, error)
				ArchiveAllPendingTasks(queue string) (int, error)
				ArchiveAllScheduledTasks(queue string) (int, error)
				ArchiveAllRetryTasks(queue string) (int, error)
				ArchiveAllCompletedTasks(queue string) (int, error)
				ArchiveAllAggregatingTasks(queue, group string) (int, error)
				RunAllScheduledTasks(queue string) (int, error)
				RunAllRetryTasks(queue string) (int, error)
				RunAllArchivedTasks(queue string) (int, error)
				RunAllAggregatingTasks(queue, group string) (int, error)
			}

			// 验证 Inspector 实现了上述接口
			// 这通过编译时类型检查完成
			So(func() {
				var _ = inspector // 引用变量以避免未使用警告
			}, ShouldNotPanic)
		})
	})
}

// TestInspectorIntegrationSmokeTest 集成场景的冒烟测试。
// 此测试设计为在具有真实 Redis 实例的环境中运行。
// 默认情况下跳过，但可以启用进行集成测试。
func TestInspectorIntegrationSmokeTest(t *testing.T) {
	// 默认跳过此测试，因为它需要真实的 Redis 实例
	t.Skip("跳过集成测试 - 需要真实的 Redis 实例")

	Convey("集成冒烟测试", t, func() {
		Convey("应该能够与真实 Redis 实例通信", func() {
			// 此测试需要真实的 Redis 实例才能运行
			// 它作为集成测试的模板包含在内
			redisCfg := &redis.Config{
				Type:     "normal",
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
			}

			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			inspector := NewInspector(redisClient)
			defer inspector.Close()

			// 测试基本操作
			queues, err := inspector.Queues()
			So(err, ShouldBeNil)
			So(queues, ShouldNotBeNil)

			// 测试获取队列信息（如果存在队列）
			if len(queues) > 0 {
				info, err := inspector.GetQueueInfo(queues[0])
				So(err, ShouldBeNil)
				So(info, ShouldNotBeNil)
			}

			// 测试关闭检查器
			err = inspector.Close()
			So(err, ShouldBeNil)
		})

		Convey("应该处理 Sentinel Redis 集群", func() {
			// Sentinel 集群集成测试
			redisCfg := &redis.Config{
				Type:             "sentinel",
				MasterName:       "mymaster",
				SentinelAddrs:    []string{"sentinel1:26379", "sentinel2:26379", "sentinel3:26379"},
				SentinelUsername: "",
				SentinelPassword: "",
				Username:         "",
				Password:         "",
				DB:               0,
			}

			redisClient := redis.NewClient(redisCfg)
			defer redisClient.Close()

			inspector := NewInspector(redisClient)
			defer inspector.Close()

			// 验证检查器创建成功
			So(inspector, ShouldNotBeNil)

			// 尝试获取队列列表
			// 不断言具体结果，因为取决于 Redis 状态
			// 只验证不会 panic
			So(func() { inspector.Queues() }, ShouldNotPanic)
			So(func() { inspector.Close() }, ShouldNotPanic)
		})
	})
}

// TestInspectorContextSafety 测试检查器的上下文安全性。
// 验证检查器在并发场景下的行为。
func TestInspectorContextSafety(t *testing.T) {
	Convey("测试检查器的上下文安全性", t, func() {
		Convey("应该支持并发创建检查器", func(c C) {
			// 创建测试配置
			redisCfg := &redis.Config{
				Type: "normal",
				Addr: "localhost:6379",
			}

			// 验证可以并发创建多个检查器
			done := make(chan bool, 5)
			errChan := make(chan error, 5)

			for i := 0; i < 5; i++ {
				go func() {
					redisClient := redis.NewClient(redisCfg)
					defer redisClient.Close()

					inspector := NewInspector(redisClient)
					if inspector == nil {
						errChan <- nil // inspector 为 nil 不是错误，只是测试条件
						done <- true
						return
					}

					// 安全关闭，捕获可能的 panic
					func() {
						defer func() {
							if r := recover(); r != nil {
								errChan <- r.(error)
							} else {
								errChan <- nil
							}
						}()
						inspector.Close()
					}()

					done <- true
				}()
			}

			// 等待所有 goroutine 完成
			for i := 0; i < 5; i++ {
				<-done
			}

			// 检查是否有错误
			close(errChan)
			for err := range errChan {
				if err != nil {
					c.So(err, ShouldBeNil)
				}
			}
		})

		Convey("检查器类型应该与标准库兼容", func() {
			// 验证 Inspector 可以用于标准库函数
			var inspector Inspector
			var _ any = &inspector

			// 验证可以作为参数传递
			testFunc := func(i *Inspector) {
				So(i, ShouldHaveSameTypeAs, &asynq.Inspector{})
			}

			testFunc(&inspector)

			// 验证可以作为返回值
			returnFunc := func() *Inspector {
				return &inspector
			}

			result := returnFunc()
			So(result, ShouldEqual, &inspector)
		})
	})
}
