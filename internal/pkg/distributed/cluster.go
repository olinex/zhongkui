package distributed

import (
	"context"
	"time"
)

// ClusterHandler 定义分布式集群处理管道的业务逻辑接口。
//
// 实现方通过该接口定制数据拉取、处理和水位提取逻辑。
// 编排器（Orchestrator）负责集群成员管理、水位持久化及主循环调度，
// 不进行分片操作——分片由实现方在 Poll 中通过 vnodeIndices 自行处理。
//
// 类型参数:
//   - Item: 数据项类型
type ClusterHandler[Item any] interface {
	// Poll 从数据源拉取数据项并发送到 items 通道。
	//
	// 实现方应在此方法内完成数据查询和过滤：
	//   - prevWatermark: 上次处理到的水位，作为查询起点
	//   - curWatermark: 当前处理到的水位，作为查询终点
	//   - vnodeIndices: 当前成员在分片中所属的虚拟节点索引列表，
	//     可用于 ClickHouse 侧按 cityHash64(key) % VNodeCount IN (indices) 预过滤
	//   - items: 将匹配的数据项发送到此通道，方法返回前应关闭
	//
	// 该方法阻塞直到数据拉取完成或 ctx 取消。
	Poll(
		ctx context.Context,
		prevWatermark, curWatermark time.Time,
		vnodeIndices []int,
		items chan<- Item,
	) error

	// Watermark 从数据项中提取水位时间戳。
	// 编排器使用最后处理项的时间戳作为下一次 Poll 的 prevWatermark。
	Watermark(item Item) time.Time

	// Start 运行处理循环。
	//
	// 编排器将所有分发给本进程的数据项汇总到 items 通道，
	// 并在该方法内统一处理。实现方自行决定处理逻辑（如单 Router、
	// 多 Router 分片等），编排器不做任何分片操作。
	//
	// 该方法阻塞直到 ctx 取消或 items 通道关闭（编排器退出时关闭）。
	Start(ctx context.Context, items <-chan Item) error

	// Shutdown 优雅关闭处理逻辑，清理资源。
	Shutdown()
}
