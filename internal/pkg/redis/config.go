// Package redis 提供Redis连接的配置和客户端创建功能。
// 它支持独立Redis和Redis Sentinel，并提供详细的配置选项。
package redis

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

const (
	Prefix       = "redis"    // 配置前缀，用于在配置文件中标识Redis配置
	NormalType   = "normal"   // 普通Redis连接类型，用于单节点Redis
	SentinelType = "sentinel" // Redis Sentinel连接类型，用于高可用Redis集群
)

// Config 定义了Redis客户端的配置参数
// 该结构体支持两种Redis连接模式：普通单节点模式和Sentinel高可用模式
// 配置可以通过Viper从配置文件、环境变量或命令行参数加载
type Config struct {
	// Type 指定Redis连接类型，可选值为"normal"或"sentinel"
	// - "normal": 单节点Redis连接
	// - "sentinel": Redis Sentinel高可用连接
	// 默认值: "normal"
	Type string `mapstructure:"type" default:"normal" binding:"required,oneof=normal sentinel" comment:"Redis连接类型：'normal'用于独立Redis，'sentinel'用于Redis Sentinel (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#NewClient)"`

	// MasterName 指定Redis Sentinel中的主节点名称
	// 仅在Type为"sentinel"时有效，用于Sentinel识别要连接的主节点
	// 默认值: "-" (空字符串)
	MasterName string `mapstructure:"masterName" default:"-" comment:"Redis Sentinel的主节点名称，当Type为sentinel时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-MasterName)"`

	// SentinelAddrs 指定Redis Sentinel节点的地址列表
	// 格式: ["host1:port1", "host2:port2", ...]
	// 仅在Type为"sentinel"时有效，用于连接Sentinel集群
	// 默认值: [] (空数组)
	SentinelAddrs []string `mapstructure:"sentinelAddrs" default:"[]" comment:"Sentinel地址列表，当Type为sentinel时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-SentinelAddrs)"`

	// SentinelUsername 指定Redis Sentinel认证的用户名
	// 仅在Type为"sentinel"时有效，用于Sentinel节点认证
	// 默认值: "-" (空字符串)
	SentinelUsername string `mapstructure:"sentinelUsername" default:"-" comment:"Sentinel认证用户名，当Type为sentinel时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-SentinelUsername)"`

	// SentinelPassword 指定Redis Sentinel认证的密码
	// 仅在Type为"sentinel"时有效，用于Sentinel节点认证
	// 默认值: "-" (空字符串)
	SentinelPassword string `mapstructure:"sentinelPassword" default:"-" comment:"Sentinel认证密码，当Type为sentinel时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-SentinelPassword)"`

	// RouteByLatency 指定是否根据延迟路由命令到副本
	// 仅在Type为"sentinel"时有效，当设置为true时，命令会被路由到延迟最低的副本
	// 默认值: false
	RouteByLatency bool `mapstructure:"routeByLatency" default:"false" comment:"按延迟将命令路由到副本，当 Type 为 sentinel 时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-RouteByLatency)"`

	// RouteRandomly 指定是否随机路由命令到副本
	// 仅在Type为"sentinel"时有效，当设置为true时，命令会被随机路由到副本
	// 默认值: true
	RouteRandomly bool `mapstructure:"routeRandomly" default:"true" comment:"随机将命令路由到副本，当 Type 为 sentinel 时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-RouteRandomly)"`

	// ReplicaOnly 指定是否只将读命令路由到副本
	// 仅在Type为"sentinel"时有效，当设置为true时，写命令会被发送到主节点，读命令会被发送到副本
	// 默认值: true
	ReplicaOnly bool `mapstructure:"replicaOnly" default:"true" comment:"仅将读命令路由到副本，当 Type 为 sentinel 时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-ReplicaOnly)"`

	// UseDisconnectedReplicas 指定是否使用已断开连接的副本
	// 仅在Type为"sentinel"时有效，当设置为true时，即使副本断开连接也会尝试使用
	// 默认值: false
	UseDisconnectedReplicas bool `mapstructure:"useDisconnectedReplicas" default:"false" comment:"即使副本断开连接也使用，当 Type 为 sentinel 时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#FailoverOptions-UseDisconnectedReplicas)"`

	// Network 指定网络类型，可选值为"tcp"或"unix"
	// 用于建立Redis连接的网络协议
	// 默认值: "tcp"
	Network string `mapstructure:"network" default:"tcp" comment:"网络类型：tcp或unix (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-Network)"`

	// Addr 指定Redis服务器地址
	// 格式: "host:port"，仅在Type为"normal"时有效
	// 默认值: "localhost:6379"
	Addr string `mapstructure:"addr" default:"localhost:6379" comment:"Redis服务器地址，当Type为normal时使用 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-Addr)"`

	// Username 指定Redis AUTH认证的用户名
	// 用于Redis 6.0及以上版本的ACL认证
	// 默认值: "-" (空字符串)
	Username string `mapstructure:"username" default:"-" comment:"Redis AUTH 认证用户名 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-Username)"`

	// Password 指定Redis AUTH认证的密码
	// 用于Redis认证，如果Redis未设置密码，可以留空
	// 默认值: "-" (空字符串)
	Password string `mapstructure:"password" default:"-" comment:"Redis AUTH 认证密码 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-Password)"`

	// ClientName 指定客户端名称
	// 通过CLIENT SETNAME命令设置，用于在Redis中识别客户端
	// 默认值: "-" (空字符串)
	ClientName string `mapstructure:"clientName" default:"-" comment:"客户端名称，通过 CLIENT SETNAME 设置 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-ClientName)"`

	// DB 指定Redis数据库编号
	// Redis支持多个逻辑数据库，编号从0开始
	// 默认值: 0
	DB int `mapstructure:"db" default:"0" comment:"数据库编号 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-DB)"`

	// PoolSize 指定连接池的最大连接数
	// 控制同时可以建立的Redis连接数量
	// 默认值: 10
	PoolSize int `mapstructure:"poolSize" default:"10" comment:"连接池的最大连接数 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-PoolSize)"`

	// MinIdleConns 指定连接池的最小空闲连接数
	// 保持的最小空闲连接数，用于快速响应请求
	// 默认值: 3
	MinIdleConns int `mapstructure:"minIdleConns" default:"3" comment:"连接池的最小空闲连接数 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MinIdleConns)"`

	// MaxIdleConns 指定连接池的最大空闲连接数
	// 保持的最大空闲连接数，超过此数量的空闲连接会被关闭
	// 默认值: 5
	MaxIdleConns int `mapstructure:"maxIdleConns" default:"5" comment:"连接池的最大空闲连接数 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MaxIdleConns)"`

	// MaxActiveConns 指定连接池的最大活动连接数
	// 0表示无限制，控制同时活跃的连接数量
	// 默认值: 10
	MaxActiveConns int `mapstructure:"maxActiveConns" default:"10" comment:"连接池的最大活动连接数，0 表示无限制 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MaxActiveConns)"`

	// MaxRetries 指定最大重试次数
	// 命令执行失败时的最大重试次数，0表示不重试
	// 默认值: 5
	MaxRetries int `mapstructure:"maxRetries" default:"5" comment:"最大重试次数 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MaxRetries)"`

	// MinRetryBackoff 指定最小重试退避时间
	// 重试之间的最小等待时间，用于实现指数退避
	// 默认值: 8ms
	MinRetryBackoff time.Duration `mapstructure:"minRetryBackoff" default:"8ms" comment:"重试之间的最小退避时间 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MinRetryBackoff)"`

	// MaxRetryBackoff 指定最大重试退避时间
	// 重试之间的最大等待时间，用于实现指数退避
	// 默认值: 512ms
	MaxRetryBackoff time.Duration `mapstructure:"maxRetryBackoff" default:"512ms" comment:"重试之间的最大退避时间 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-MaxRetryBackoff)"`

	// DialTimeout 指定连接建立超时时间
	// 建立Redis连接的最大等待时间
	// 默认值: 5s
	DialTimeout time.Duration `mapstructure:"dialTimeout" default:"5s" comment:"建立连接的超时时间 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-DialTimeout)"`

	// ReadTimeout 指定读取操作超时时间
	// 从Redis读取数据的最大等待时间
	// 默认值: 3s
	ReadTimeout time.Duration `mapstructure:"readTimeout" default:"3s" comment:"读取操作的超时时间 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-ReadTimeout)"`

	// WriteTimeout 指定写入操作超时时间
	// 向Redis写入数据的最大等待时间
	// 默认值: 3s
	WriteTimeout time.Duration `mapstructure:"writeTimeout" default:"3s" comment:"写入操作的超时时间 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-WriteTimeout)"`

	// ContextTimeoutEnabled 指定是否启用上下文超时
	// 当设置为true时，连接池操作会使用上下文超时
	// 默认值: true
	ContextTimeoutEnabled bool `mapstructure:"contextTimeoutEnabled" default:"true" comment:"连接池操作是否使用上下文超时 (ref: https://pkg.go.dev/github.com/redis/go-redis/v9#Options-ContextTimeoutEnabled)"`
}

// ToOptions 将Config转换为redis.Options
// 用于创建普通Redis客户端连接
// 返回值: *redis.Options - 配置好的Redis选项，可以直接用于创建Redis客户端
// 注意: 此方法仅适用于Type为"normal"的配置
func (c *Config) ToOptions() *redis.Options {
	return &redis.Options{
		Network:               c.Network,
		Addr:                  c.Addr,
		Username:              c.Username,
		Password:              c.Password,
		ClientName:            c.ClientName,
		DB:                    c.DB,
		PoolSize:              c.PoolSize,
		MinIdleConns:          c.MinIdleConns,
		MaxIdleConns:          c.MaxIdleConns,
		MaxActiveConns:        c.MaxActiveConns,
		MaxRetries:            c.MaxRetries,
		MinRetryBackoff:       c.MinRetryBackoff,
		MaxRetryBackoff:       c.MaxRetryBackoff,
		DialTimeout:           c.DialTimeout,
		ReadTimeout:           c.ReadTimeout,
		WriteTimeout:          c.WriteTimeout,
		ContextTimeoutEnabled: c.ContextTimeoutEnabled,
	}
}

// ToSentinelOptions 将Config转换为redis.FailoverOptions
// 用于创建Redis Sentinel高可用客户端连接
// 返回值: *redis.FailoverOptions - 配置好的Redis Sentinel选项，可以直接用于创建Redis Sentinel客户端
// 注意: 此方法仅适用于Type为"sentinel"的配置
func (c *Config) ToSentinelOptions() *redis.FailoverOptions {
	return &redis.FailoverOptions{
		MasterName:              c.MasterName,
		SentinelAddrs:           c.SentinelAddrs,
		ClientName:              c.ClientName,
		SentinelUsername:        c.SentinelUsername,
		SentinelPassword:        c.SentinelPassword,
		RouteByLatency:          c.RouteByLatency,
		RouteRandomly:           c.RouteRandomly,
		ReplicaOnly:             c.ReplicaOnly,
		UseDisconnectedReplicas: c.UseDisconnectedReplicas,
		Username:                c.Username,
		Password:                c.Password,
		DB:                      c.DB,
		PoolSize:                c.PoolSize,
		MinIdleConns:            c.MinIdleConns,
		MaxRetries:              c.MaxRetries,
		MinRetryBackoff:         c.MinRetryBackoff,
		MaxRetryBackoff:         c.MaxRetryBackoff,
		DialTimeout:             c.DialTimeout,
		ReadTimeout:             c.ReadTimeout,
		WriteTimeout:            c.WriteTimeout,
		ContextTimeoutEnabled:   c.ContextTimeoutEnabled,
	}
}

// DescribeConfig 返回Redis配置的描述信息
// 参数:
//   - names: 配置名称列表，用于指定要描述的配置项
//
// 返回值: config.ConfigDescriptions - 配置描述信息
func DescribeConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[Config](fmt.Sprintf(`%s.{%s}`, Prefix, strings.Join(names, ",")))
}

// ExtractConfig 从Viper配置中提取Redis配置
// 参数:
//   - v: *viper.Viper - Viper配置实例
//   - name: string - 配置名称
//
// 返回值: (*Config, error) - 提取的Redis配置和可能的错误
func ExtractConfig(viper *viper.Viper, validate *validator.Validate, name string) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, fmt.Sprintf(`%s.%s`, Prefix, name))
}
