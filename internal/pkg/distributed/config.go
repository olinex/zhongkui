package distributed

import (
	"time"
)

// ClusterConfig 单个分布式处理器的运行时配置参数。
type ClusterConfig struct {
	// VNodeCount 虚拟节点总数，用于分布式分片计算。
	// 每个集群成员通过虚拟节点数来确定自己负责处理的分片范围，
	// 配合一致性哈希确保数据均匀分布在集群成员之间。
	VNodeCount int

	// ChanSize items 通道的缓冲区大小，即 Poll 与 Start 之间传递数据项的
	// 缓冲通道容量。当消费者处理速度跟不上生产者时，缓冲区可减少阻塞。
	ChanSize int

	// PollInterval 轮询间隔，控制 Poll 方法被调用的频率。
	// 每次 Poll 从数据源拉取一批新数据项，间隔越短则处理延迟越低，
	// 但会增加数据源和系统的负载。
	PollInterval time.Duration

	// SaveInterval 水位持久化间隔，控制将当前处理水位写入 etcd 的频率。
	// 频繁持久化可在故障恢复时减少重复处理，但会增加 etcd 写入压力。
	SaveInterval time.Duration

	// ReplayWindow 回放窗口，首次启动或水位丢失时回溯的时间范围。
	// 当从 etcd 无法获取上次处理水位时，系统将从当前时间减去此窗口
	// 作为起始水位，确保不会遗漏关键数据。
	ReplayWindow time.Duration
}
