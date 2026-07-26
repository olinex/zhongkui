// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import "time"

// Param TCP 流组包处理器的配置参数。
// 支持从 Viper 配置中通过 mapstructure tag 加载。
type Param struct {
	// MaxReassemblerSize 每个 BaseMeta 分组最大并发 TCP 流数
	MaxReassemblerSize int `mapstructure:"maxReassemblerSize" binding:"min=1,max=65536" default:"1024"`
	// MaxOutOrderPacketSize 每个方向最大乱序包缓冲数
	MaxOutOrderPacketSize int `mapstructure:"maxOutOrderPacketSize" binding:"min=1,max=65536" default:"256"`
	// NoticeTimeout TCP 流无活动超时时间（用于 cleanup 和 RingBuffer 超时）
	NoticeTimeout time.Duration `mapstructure:"noticeTimeout" binding:"min=1s,max=5m" default:"30s"`
	// PollInterval ClickHouse 轮询间隔
	PollInterval time.Duration `mapstructure:"pollInterval" binding:"min=1s,max=1m" default:"5s"`
	// PollOffset Clickhouse 轮询时的警戒偏移，避免数据在写入时被查询而导致遗漏
	PollOffset time.Duration `mapstructure:"pollOffset" binding:"min=1m,max=24h" default:"5m"`
	// VNodeCount 虚拟节点数量
	VNodeCount int `mapstructure:"vNodeCount" binding:"min=0,max=1024" default:"128"`
	// BatchSize BatchWriter 缓冲区大小阈值，达到后触发 flush
	BatchSize int `mapstructure:"batchSize" binding:"min=100,max=10000" default:"1000"`
	// FlushInterval BatchWriter 定时 flush 间隔
	FlushInterval time.Duration `mapstructure:"flushInterval" binding:"min=5s,max=10m" default:"1m"`
	// SaveInterval 水位保存到 etcd 的间隔
	SaveInterval time.Duration `mapstructure:"saveInterval" binding:"min=10s,max=5m" default:"30s"`
	// ReplayWindow 重放窗口，用于回放旧数据
	ReplayWindow time.Duration `mapstructure:"replayWindow" binding:"min=1m,max=24h" default:"5m"`
}
