package distributed

import (
	"time"
)

// ClusterConfig 单个分布式处理器的运行时配置参数。
type ClusterConfig struct {
	// VNodeCount 虚拟节点总数，用于计算分片索引
	VNodeCount int
	// ChanSize items 通道的缓冲区大小
	ChanSize int
	// PollInterval 轮询间隔
	PollInterval time.Duration
	// SaveInterval 水位持久化间隔
	SaveInterval time.Duration
	// ReplayWindow 回放窗口，首次启动或水位丢失时回溯的时间范围
	ReplayWindow time.Duration
}
