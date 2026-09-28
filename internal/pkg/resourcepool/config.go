package resourcepool

import (
	// 标准库
	"time"

	// 第三方
	gpool "github.com/jolestar/go-commons-pool/v2"
)

// Config 资源池配置，是 go-commons-pool 的 ObjectPoolConfig 面的友好包装，
// 提供贴合本项目需求的默认值并映射到底层引擎配置。
type Config struct {
	// MaxTotal 单个 Key 池可分配（借出 + 闲置）对象总数上限；负值表示无上限
	MaxTotal int
	// MaxIdle 单个 Key 池闲置对象上限；超限即销毁最旧的闲置对象
	MaxIdle int
	// MinIdle 期望维持的闲置对象下限（仅当驱逐线程运行时生效）
	MinIdle int
	// TestOnCreate 新建对象后是否立即校验（Validate）
	TestOnCreate bool
	// TestOnBorrow 借出前是否校验（Validate）；false 视为失效并销毁重建，应对连接被服务端断开
	TestOnBorrow bool
	// TestOnReturn 归还时是否校验；校验失败则销毁而非回池
	TestOnReturn bool
	// BlockWhenExhausted 池耗尽时是否阻塞等待；false 则立即返回错误
	BlockWhenExhausted bool
	// MinEvictableIdleTime 闲置对象可被驱逐的最小时长；非正值禁用闲置驱逐
	MinEvictableIdleTime time.Duration
	// TimeBetweenEvictionRuns 驱逐线程运行的间隔；非正值禁用驱逐线程
	TimeBetweenEvictionRuns time.Duration
}

// NewDefaultConfig 返回适合长连接复用的默认资源池配置：
// 开启 testOnBorrow/testOnCreate/blockWhenExhausted，设定合理的连接上限与
// 闲置驱逐阈值，使空闲连接在超时后被定时回收、失效连接在借出时自动重建。
func NewDefaultConfig() *Config {
	return &Config{
		MaxTotal:                8,
		MaxIdle:                 4,
		MinIdle:                 0,
		TestOnCreate:            true,
		TestOnBorrow:            true,
		TestOnReturn:            false,
		BlockWhenExhausted:      true,
		MinEvictableIdleTime:    30 * time.Second,
		TimeBetweenEvictionRuns: 10 * time.Second,
	}
}

// toPoolConfig 将资源池配置映射为 go-commons-pool 的 ObjectPoolConfig。
func (c *Config) toPoolConfig() *gpool.ObjectPoolConfig {
	cfg := gpool.NewDefaultPoolConfig()
	cfg.MaxTotal = c.MaxTotal
	cfg.MaxIdle = c.MaxIdle
	cfg.MinIdle = c.MinIdle
	cfg.TestOnCreate = c.TestOnCreate
	cfg.TestOnBorrow = c.TestOnBorrow
	cfg.TestOnReturn = c.TestOnReturn
	cfg.BlockWhenExhausted = c.BlockWhenExhausted
	cfg.MinEvictableIdleTime = c.MinEvictableIdleTime
	cfg.TimeBetweenEvictionRuns = c.TimeBetweenEvictionRuns
	return cfg
}
