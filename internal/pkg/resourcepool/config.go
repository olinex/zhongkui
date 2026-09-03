package resourcepool

import (
	// stdlib
	"time"

	// third-party
	gpool "github.com/jolestar/go-commons-pool/v2"
)

// Config 资源池配置，是 go-commons-pool 的 ObjectPoolConfig 面的友好包装，
// 提供贴合本项目需求的默认值并映射到底层引擎配置。
// Config is a friendly wrapper over go-commons-pool's ObjectPoolConfig that
// provides project-appropriate defaults and maps onto the underlying engine config.
type Config struct {
	// MaxTotal 单个 Key 池可分配（借出 + 闲置）对象总数上限；负值表示无上限
	// MaxTotal the cap on objects (checked out + idle) per Key pool; negative for no limit
	MaxTotal int
	// MaxIdle 单个 Key 池闲置对象上限；超限即销毁最旧的闲置对象
	// MaxIdle the cap on idle objects per Key pool; exceeding it destroys the oldest idle object
	MaxIdle int
	// MinIdle 期望维持的闲置对象下限（仅当驱逐线程运行时生效）
	// MinIdle the minimum idle objects to maintain (effective only while the evictor runs)
	MinIdle int
	// TestOnCreate 新建对象后是否立即校验（Validate）
	// TestOnCreate whether to validate newly created objects
	TestOnCreate bool
	// TestOnBorrow 借出前是否校验（Validate）；false 视为失效并销毁重建，应对连接被服务端断开
	// TestOnBorrow whether to validate before borrowing; false means stale and is rebuilt
	TestOnBorrow bool
	// TestOnReturn 归还时是否校验；校验失败则销毁而非回池
	// TestOnReturn whether to validate on return; a failed validation destroys instead of pooling
	TestOnReturn bool
	// BlockWhenExhausted 池耗尽时是否阻塞等待；false 则立即返回错误
	// BlockWhenExhausted whether to block when the pool is exhausted; false returns an error immediately
	BlockWhenExhausted bool
	// MinEvictableIdleTime 闲置对象可被驱逐的最小时长；非正值禁用闲置驱逐
	// MinEvictableIdleTime the minimum idle duration before an object is evictable; non-positive disables idle eviction
	MinEvictableIdleTime time.Duration
	// TimeBetweenEvictionRuns 驱逐线程运行的间隔；非正值禁用驱逐线程
	// TimeBetweenEvictionRuns the interval between eviction runs; non-positive disables the evictor
	TimeBetweenEvictionRuns time.Duration
}

// NewDefaultConfig 返回适合长连接复用的默认资源池配置：
// 开启 testOnBorrow/testOnCreate/blockWhenExhausted，设定合理的连接上限与
// 闲置驱逐阈值，使空闲连接在超时后被定时回收、失效连接在借出时自动重建。
// NewDefaultConfig returns a default pool config suited to long-connection reuse:
// testOnBorrow/testOnCreate/blockWhenExhausted enabled, sensible connection caps,
// and idle-eviction thresholds so idle connections are reclaimed periodically and
// stale connections are rebuilt on borrow.
func NewDefaultConfig() *Config {
	return &Config{
		MaxTotal:               8,
		MaxIdle:                4,
		MinIdle:                0,
		TestOnCreate:           true,
		TestOnBorrow:           true,
		TestOnReturn:           false,
		BlockWhenExhausted:     true,
		MinEvictableIdleTime:   30 * time.Second,
		TimeBetweenEvictionRuns: 10 * time.Second,
	}
}

// toPoolConfig 将资源池配置映射为 go-commons-pool 的 ObjectPoolConfig。
// toPoolConfig maps the pool Config onto go-commons-pool's ObjectPoolConfig.
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