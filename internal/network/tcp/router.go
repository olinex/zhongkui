// router.go 实现TCP包重组路由器，用于管理多个TCP重组器并将TCP包路由到相关的重组器中。
// 路由器使用基于元数据的哈希映射和双向链表实现高效的流管理和LRU缓存策略。
//
// 主要功能：
//  1. TCP流的路由和分发
//  2. 重组器节点的生命周期管理
//  3. LRU缓存策略实现
//  4. 并发安全的goroutine管理
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
package tcp

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// Router 是TCP包重组路由器，用于管理多个TCP重组器并将TCP包路由到相关的重组器中。
// 路由器使用基于元数据的哈希映射和双向链表实现高效的流管理和LRU缓存策略。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
type Router[P TCPPacket] struct {
	roots                 map[n.BaseMeta]*groupEntry[P] // 元数据分组映射
	maxReassemblerSize    int                           // 最大重组器数量
	maxOutOrderPacketSize int                           // 最大乱序包数量
	noticeTimeout         time.Duration                 // 通知超时时间
}

// NewRouter 创建并初始化一个新的TCP包重组路由器。
// 该函数创建一个空的路由器实例，初始化所有配置参数。
//
// 参数:
//   - packetChannelSize: 数据包通道大小，影响goroutine间的通信效率
//   - maxReassemblerSize: 最大重组器数量，控制路由器容量
//   - maxOutOrderPacketSize: 最大乱序包数量，影响乱序包处理能力
//
// 返回值:
//   - Router[P]: 初始化完成的路由器实例
//
// 配置参数说明:
//   - packetChannelSize: 建议值在10-100之间，根据数据包流量调整
//   - maxReassemblerSize: 根据系统内存和并发连接数设置
//   - maxOutOrderPacketSize: 根据网络延迟和乱序程度设置
func NewRouter[P TCPPacket](maxReassemblerSize, maxOutOrderPacketSize int, noticeTimeout time.Duration) Router[P] {
	InitMetrics()
	return Router[P]{
		roots:                 make(map[n.BaseMeta]*groupEntry[P], 32),
		maxReassemblerSize:    maxReassemblerSize,
		maxOutOrderPacketSize: maxOutOrderPacketSize,
		noticeTimeout:         noticeTimeout,
	}
}

// addPacket 将TCP数据包添加到路由器中，路由到对应的重组器节点。
// 这是路由器的核心方法，负责TCP流的管理、重组器节点的创建和LRU缓存策略。
// 方法执行流程：
//  1. 根据数据包元数据和流ID查找对应的重组器节点
//  2. 如果找到节点，将数据包路由到该节点
//  3. 如果未找到节点，创建新的重组器节点（仅对SYN包）
//  4. 实施LRU缓存策略，将活跃节点移动到链表头部
//  5. 管理节点生命周期，完成时移除节点
//
// 参数:
//   - ctx: 上下文，用于控制goroutine的生命周期
//   - logger: 日志记录器，用于记录运行时信息
//   - packet: TCP数据包，实现了TCPPacket接口
//   - wg: errgroup.Group实例，用于管理goroutine的错误和等待
//   - dstChan: 目标通道，用于输出重组后的应用层数据
//
// 返回值:
//   - error: 处理过程中发生的错误，包括：
//   - 创建重组器失败
//   - 启动重组器节点失败
//   - 数据包路由失败
//
// 算法细节:
//   - 使用元数据和流ID进行两级查找：先按元数据分组，再在组内按流ID查找
//   - 实现LRU缓存：每次访问节点时将其移动到链表头部，保持最近使用的节点在前面
//   - 容量管理：超过最大重组器数量时，丢弃链表尾部的节点（最近最少使用）
//   - 生命周期管理：TCP流完成后（请求和响应负载都完成）自动移除节点
//   - SYN包处理：只有SYN包会创建新的重组器节点，非SYN包会被忽略（如果找不到对应节点）
//
// 性能考虑:
//   - 时间复杂度：O(n) 在最坏情况下需要遍历整个链表
//   - 空间复杂度：O(m×n) m为元数据分组数，n为每个分组的节点数
//   - 并发安全：通过errgroup管理goroutine，确保错误传播和资源清理
func (r *Router[P]) addPacket(ctx context.Context, logger *logging.Logger, packet P, wg *errgroupx.Group, dstChan chan<- any) (err error) {
	ctx, span := startRouterSpan(ctx, packet)
	defer endSpan(span, &err)
	defer recordPacket(ctx, packet)
	streamID := packet.StreamID()
	meta := packet.Meta()

	group := r.roots[*meta]
	if group == nil {
		group = &groupEntry[P]{
			nodes: make(map[string]*ReassemblerNode[P], r.maxReassemblerSize),
		}
		r.roots[*meta] = group
	}

	node := group.nodes[streamID]

	if node != nil && node.Reassembler.isCompleted() {
		group.remove(node)
		node.Reassembler.stop()
		node = nil
	}

	if node == nil {
		if !packet.IsSyn() {
			return nil
		}
		rsm, err := newReassembler(
			ctx,
			packet,
			r.maxOutOrderPacketSize,
		)
		if err != nil {
			return errors.WithStack(err)
		}
		node = &ReassemblerNode[P]{
			ID:          streamID,
			Reassembler: rsm,
		}
		node.run(ctx, logger, meta, wg, dstChan)
		group.pushFront(node)
	} else if err := node.Reassembler.addPacket(ctx, packet); err != nil {
		return errors.WithStack(err)
	}

	node.Reassembler.notice(ctx)
	group.moveToFront(node)

	return nil
}

func (r *Router[P]) cleanup(ctx context.Context, force bool) {
	ctx, span := startCleanupSpan(ctx, force)
	var removedCount, evictedCount int
	defer func() {
		span.SetAttributes(
			attribute.Int("tcp.cleanup.removed", removedCount),
			attribute.Int("tcp.cleanup.evicted", evictedCount),
		)
		recordCleanupFlowsRemoved(ctx, removedCount)
		recordCleanupFlowsEvicted(ctx, evictedCount)
		endSpan(span, nil)
	}()
	for _, group := range r.roots {
		if group == nil {
			continue
		}
		// 清理已完成的或明显异常的 tcp 流
		current := group.head
		for current != nil {
			next := current.next
			if force || current.Reassembler.isCompleted() ||
				current.Reassembler.lastNotice().Add(r.noticeTimeout).Before(time.Now()) {
				group.remove(current)
				current.Reassembler.stop()
				removedCount++
			}
			current = next
		}
		// 清理超出缓存的 tcp 流
		for group.len() > r.maxReassemblerSize {
			evicted := group.popTail()
			if evicted != nil {
				evicted.Reassembler.stop()
				evictedCount++
			}
		}
	}
}

// Run 启动路由器的主循环，处理来自源通道的TCP数据包。
// 这是路由器的入口方法，负责监听数据包通道、协调goroutine和处理错误。
// 方法执行流程：
//  1. 创建errgroup用于管理所有重组器goroutine
//  2. 设置最大并发限制，防止资源耗尽
//  3. 进入主循环，监听上下文取消和数据包到达
//  4. 将接收到的数据包路由到对应的重组器节点
//  5. 等待所有goroutine完成，处理错误
//
// 参数:
//   - ctx: 上下文，用于控制路由器的主循环生命周期
//   - logger: 日志记录器，用于记录运行时信息
//   - srcChan: 源通道，输入TCP数据包
//   - dstChan: 目标通道，输出重组后的应用层数据
//   - limit: 最大并发数，用于限制同时处理的数据包数量
//
// 返回值:
//   - error: 运行过程中发生的错误，包括：
//   - 上下文取消
//   - 数据包处理错误
//   - 重组器goroutine错误
//   - 源通道提前关闭
//
// 运行模式:
//   - 阻塞模式：持续监听数据包直到上下文取消
//   - 错误处理：使用errgroup收集所有goroutine的错误
//   - 资源管理：确保所有goroutine在退出前正确关闭
//   - 并发控制：通过limit参数限制同时处理的数据包数量
//
// 注意：该方法会阻塞直到上下文取消或发生错误。
// 调用者应该在一个单独的goroutine中运行此方法。
//
// 错误处理策略:
//  1. 源通道提前关闭：返回错误，通知调用者
//  2. 数据包处理错误：立即返回错误，终止路由器
//  3. 重组器goroutine错误：等待所有goroutine完成后返回错误
//  4. 上下文取消：正常退出，返回nil
//
// 资源清理:
//  1. 上下文取消时，所有goroutine会自动收到取消信号
//  2. errgroup会等待所有goroutine完成
//  3. 每个重组器节点有自己的清理逻辑，确保资源释放
func (r *Router[P]) Run(ctx context.Context, logger *logging.Logger, srcChan <-chan P, dstChan chan<- any) error {
	wg, ctx := errgroupx.WithContext(ctx)
	ticker := time.NewTicker(r.noticeTimeout)
	defer ticker.Stop()
	// 进入主循环
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case <-ticker.C:
			r.cleanup(ctx, false)
		case packet, open := <-srcChan:
			if !open {
				break LOOP
			}
			// 处理数据包
			err := r.addPacket(ctx, logger, packet, wg, dstChan)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	// 退出主循环后，强制清理所有残留的流，确保所有重组器 goroutine 收到 stop 信号
	r.cleanup(ctx, true)
	ctx, cancel := context.WithTimeout(ctx, r.noticeTimeout)
	defer cancel()
	// 有活跃流时，等待一个 noticeTimeout 让它们完成或超时
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
