// Package redis 提供Redis客户端创建和管理功能。
// 它支持独立Redis实例和Redis Sentinel集群，
// 提供全面的配置选项和错误处理。
package redis

import (
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Client 是 go-redis/v9 库中 redis.Client 的类型别名。
// 这提供了对所有Redis操作的直接访问，同时保持类型安全性和与底层go-redis库的兼容性。
//
// 使用示例:
//
//	client := redis.NewClient(config)
//	err := client.Ping(ctx).Err()
//	if err != nil {
//	    // 处理连接错误
//	}
type Client struct {
	*redis.Client
	// lock 保护 closed 标志的互斥锁，保证关闭幂等。
	lock sync.Mutex
	// closed 是否已关闭，避免重复关闭底层连接。
	closed bool
}

// NewClient 根据提供的配置创建一个新的Redis客户端。
// 这个工厂函数支持两种Redis部署模式:
//   - Normal (standalone): 单节点Redis实例
//   - Sentinel: Redis Sentinel高可用集群
//
// 参数:
//   - cfg: *Config - Redis配置，包含连接参数、认证详情和部署类型
//
// 返回值:
//   - *Client: 配置好的Redis客户端，可供使用
//
// 异常:
//   - 如果配置类型不是"normal"或"sentinel"，函数将panic并显示描述性错误信息。
//     这确保无效配置在应用启动时就能被捕获。
//
// 使用示例:
//
//	config := &redis.Config{
//	    Type:     redis.NORMAL_TYPE,
//	    Addr:     "localhost:6379",
//	    Password: "secret",
//	    DB:       0,
//	}
//	client := redis.NewClient(config)
//
// 注意事项:
//   - 对于普通Redis实例，函数使用redis.NewClient，并使用Config.ToOptions()转换的选项
//   - 对于Sentinel集群，函数使用redis.NewFailoverClient，并使用Config.ToSentinelOptions()转换的选项
//   - 返回的客户端在不再需要时应使用client.Close()正确关闭以释放资源
func NewClient(cfg *Config) *Client {
	switch cfg.Type {
	case NormalType:
		// 为独立实例创建标准Redis客户端
		return &Client{
			Client: redis.NewClient(cfg.ToOptions()),
			lock:   sync.Mutex{},
			closed: false,
		}
	case SentinelType:
		// 为Redis Sentinel集群创建具备故障转移感知的客户端
		return &Client{
			Client: redis.NewFailoverClient(cfg.ToSentinelOptions()),
			lock:   sync.Mutex{},
			closed: false,
		}
	default:
		// 无效的配置类型 - 触发panic以防止运行时错误
		panic(fmt.Sprintf("unsupport client type %s", cfg.Type))
	}
}

// Shutdown 关闭底层 Redis 客户端连接，重复调用安全（已关闭时直接返回 nil）。
func (c *Client) Shutdown() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.closed {
		return nil
	}
	if err := c.Client.Close(); err != nil {
		return errors.WithStack(err)
	}
	c.closed = true
	return nil
}
