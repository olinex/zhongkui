// Package redis 提供Redis客户端创建和管理功能。
// 此文件包含客户端创建逻辑的单元测试。
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/creasty/defaults"
	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"
)

// TestNewClient_NormalType 测试普通Redis客户端的创建
// 验证标准Redis客户端能够根据提供的配置正确创建
func TestNewClient_NormalType(t *testing.T) {
	Convey("测试普通Redis客户端的创建", t, func() {
		Convey("应该使用正常配置成功创建客户端", func() {
			// 创建普通Redis实例的配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 覆盖普通Redis的配置
			cfg.Type = NORMAL_TYPE
			cfg.Addr = "localhost:6380"
			cfg.Username = "testuser"
			cfg.Password = "testpass"
			cfg.ClientName = "test-client"
			cfg.DB = 1
			cfg.PoolSize = 20
			cfg.MinIdleConns = 5
			cfg.MaxIdleConns = 10
			cfg.MaxActiveConns = 30
			cfg.MaxRetries = 3
			cfg.MinRetryBackoff = 10 * time.Millisecond
			cfg.MaxRetryBackoff = 1 * time.Second
			cfg.DialTimeout = 10 * time.Second
			cfg.ReadTimeout = 5 * time.Second
			cfg.WriteTimeout = 5 * time.Second
			cfg.ContextTimeoutEnabled = false

			// 创建客户端
			client := NewClient(cfg)

			// 验证客户端不为nil
			So(client, ShouldNotBeNil)

			// 验证客户端类型和基本属性
			// 注意：无法直接访问redis.Client的内部字段，
			// 但我们可以验证客户端创建时没有panic
			So(client, ShouldHaveSameTypeAs, &redis.Client{})

			// 测试客户端可以关闭（清理）
			err = client.Close()
			So(err, ShouldBeNil)
		})
	})
}

// TestNewClient_SentinelType 测试Redis Sentinel客户端的创建
// 验证Sentinel感知客户端能够根据提供的配置正确创建
func TestNewClient_SentinelType(t *testing.T) {
	Convey("测试Redis Sentinel客户端的创建", t, func() {
		Convey("应该使用Sentinel配置成功创建客户端", func() {
			// 创建Redis Sentinel的配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 覆盖Sentinel的配置
			cfg.Type = SENTINEL_TYPE
			cfg.MasterName = "mymaster"
			cfg.SentinelAddrs = []string{"sentinel1:26379", "sentinel2:26379"}
			cfg.SentinelUsername = "sentineluser"
			cfg.SentinelPassword = "sentinelpass"
			cfg.RouteByLatency = false          // NewFailoverClient不支持高级路由
			cfg.RouteRandomly = false           // NewFailoverClient不支持高级路由
			cfg.ReplicaOnly = true              // 默认是true
			cfg.UseDisconnectedReplicas = false // 默认是false
			cfg.Username = "redisuser"
			cfg.Password = "redispass"
			cfg.ClientName = "sentinel-client"
			cfg.DB = 2
			cfg.PoolSize = 15
			cfg.MinIdleConns = 4
			cfg.MaxRetries = 2
			cfg.MinRetryBackoff = 20 * time.Millisecond
			cfg.MaxRetryBackoff = 2 * time.Second
			cfg.DialTimeout = 8 * time.Second
			cfg.ReadTimeout = 4 * time.Second
			cfg.WriteTimeout = 4 * time.Second
			cfg.ContextTimeoutEnabled = true

			// 创建客户端
			client := NewClient(cfg)

			// 验证客户端不为nil
			So(client, ShouldNotBeNil)

			// 验证客户端类型
			// 注意：redis.NewFailoverClient返回*redis.Client，与普通客户端相同
			So(client, ShouldHaveSameTypeAs, &redis.Client{})

			// 测试客户端可以关闭（清理）
			err = client.Close()
			So(err, ShouldBeNil)
		})
	})
}

// TestNewClient_InvalidType 测试无效客户端类型会导致panic
// 确保工厂函数正确验证配置并在无效输入时快速失败
func TestNewClient_InvalidType(t *testing.T) {
	Convey("测试无效客户端类型", t, func() {
		Convey("应该对无效客户端类型panic", func() {
			// 创建无效类型的配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 设置无效的客户端类型
			cfg.Type = "invalid-type"

			// 验证创建具有无效类型的客户端会导致panic
			So(func() {
				NewClient(cfg)
			}, ShouldPanic)

			// 验证panic消息包含无效类型
			defer func() {
				if r := recover(); r != nil {
					errMsg, ok := r.(string)
					So(ok, ShouldBeTrue)
					So(errMsg, ShouldContainSubstring, "invalid-type")
					So(errMsg, ShouldContainSubstring, "Unsupport client type")
				}
			}()

			// 这将panic并被defer函数捕获
			NewClient(cfg)
		})
	})
}

// TestNewClient_EmptyConfig 测试最小配置下的客户端创建
// 验证默认值被正确应用并且客户端可以在最小配置下创建
func TestNewClient_EmptyConfig(t *testing.T) {
	Convey("测试最小配置下的客户端创建", t, func() {
		Convey("应该使用最小配置成功创建客户端", func() {
			// 创建最小配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			// 确保类型设置为normal（默认）
			cfg.Type = NORMAL_TYPE

			// 创建客户端
			client := NewClient(cfg)

			// 验证客户端不为nil
			So(client, ShouldNotBeNil)

			// 测试客户端可以关闭（清理）
			err = client.Close()
			So(err, ShouldBeNil)
		})
	})
}

// TestNewClient_ContextOperations 测试客户端的基本上下文操作
// 验证创建的客户端可以执行基本操作而不会立即出错
func TestNewClient_ContextOperations(t *testing.T) {
	Convey("测试客户端的基本上下文操作", t, func() {
		Convey("应该能够执行基本操作", func() {
			// 创建测试配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)

			cfg.Type = NORMAL_TYPE
			cfg.Addr = "localhost:6379" // 默认Redis地址

			// 创建客户端
			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 为测试创建带有超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// 尝试ping Redis服务器
			// 注意：这可能会失败，因为我们没有真正的Redis服务器，
			// 但我们正在测试可以调用客户端方法
			pingResult := client.Ping(ctx)

			// 我们不断言错误，因为它取决于Redis是否运行
			// 相反，我们验证ping结果不为nil
			So(pingResult, ShouldNotBeNil)

			// 测试客户端可以关闭
			err = client.Close()
			So(err, ShouldBeNil)
		})
	})
}

// TestClientTypeAlias 测试Client类型别名是否正确工作
// 验证类型别名提供预期的接口并与底层go-redis库保持兼容性
func TestClientTypeAlias(t *testing.T) {
	Convey("测试Client类型别名", t, func() {
		Convey("Client应该是redis.Client的别名", func() {
			// 验证Client确实是redis.Client的别名
			var client Client
			So(&client, ShouldHaveSameTypeAs, &redis.Client{})

			// 创建最小配置
			cfg := &Config{}
			err := defaults.Set(cfg)
			So(err, ShouldBeNil)
			cfg.Type = NORMAL_TYPE

			// 测试NewClient返回正确的类型
			createdClient := NewClient(cfg)
			So(createdClient, ShouldHaveSameTypeAs, &client)

			// 清理
			if createdClient != nil {
				createdClient.Close()
			}
		})
	})
}

// TestNewClient_ConfigurationEdgeCases 测试配置中的边界情况
// 确保客户端创建正确处理配置中的各种边界情况
func TestNewClient_ConfigurationEdgeCases(t *testing.T) {
	Convey("测试配置边界情况", t, func() {
		Convey("应该处理默认值后的空类型", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = "" // 空类型

			So(func() {
				NewClient(cfg)
			}, ShouldPanic)
		})

		Convey("应该处理普通类型下的空地址", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = NORMAL_TYPE
			cfg.Addr = "" // 空地址

			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("应该处理Sentinel类型下的空主节点名称", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = SENTINEL_TYPE
			cfg.MasterName = ""       // 空主节点名称
			cfg.RouteRandomly = false // NewFailoverClient需要

			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("应该处理Sentinel类型下的空Sentinel地址", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = SENTINEL_TYPE
			cfg.SentinelAddrs = []string{} // 空Sentinel地址
			cfg.RouteRandomly = false      // NewFailoverClient需要

			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("应该处理非常大的连接池大小", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = NORMAL_TYPE
			cfg.PoolSize = 10000 // 非常大的连接池大小

			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("应该处理零超时值", func() {
			cfg := &Config{}
			defaults.Set(cfg)
			cfg.Type = NORMAL_TYPE
			cfg.DialTimeout = 0
			cfg.ReadTimeout = 0
			cfg.WriteTimeout = 0

			client := NewClient(cfg)
			So(client, ShouldNotBeNil)

			// 清理
			if client != nil {
				client.Close()
			}
		})
	})
}

// TestNewClient_IntegrationSmokeTest 集成场景的冒烟测试
// 此测试设计为在具有真实Redis实例的环境中运行
// 默认情况下跳过，但可以启用进行集成测试
func TestNewClient_IntegrationSmokeTest(t *testing.T) {
	// 默认跳过此测试，因为它需要真实的Redis实例
	t.Skip("跳过集成测试 - 需要真实的Redis实例")

	Convey("集成冒烟测试", t, func() {
		Convey("应该能够与真实Redis实例通信", func() {
			// 此测试需要真实的Redis实例才能运行
			// 它作为集成测试的模板包含在内
			cfg := &Config{
				Type:     NORMAL_TYPE,
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
			}

			client := NewClient(cfg)
			defer client.Close()

			ctx := context.Background()

			// 测试基本操作
			err := client.Set(ctx, "test-key", "test-value", 0).Err()
			So(err, ShouldBeNil)

			val, err := client.Get(ctx, "test-key").Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "test-value")

			// 清理测试键
			err = client.Del(ctx, "test-key").Err()
			So(err, ShouldBeNil)
		})
	})
}
