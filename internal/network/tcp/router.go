// router.go 实现TCP包重组路由器，用于管理多个TCP重组器并将TCP包路由到相关的重组器中。
// 路由器使用基于元数据的哈希映射和双向链表实现高效的流管理和LRU缓存策略。
//
// 主要功能包括：
//  1. TCP流的路由和分发
//  2. 重组器节点的生命周期管理
//  3. LRU缓存策略实现
//  4. 序列化和反序列化支持
//  5. 并发安全的goroutine管理
//
// 架构设计：
//  1. Router: 顶层路由器，管理多个TCP流分组
//  2. ReassemblerNode: 重组器节点，包含重组器实例和控制器
//  3. reassembleController: 重组器控制器，管理数据包通道
//  4. RouterAux: 路由器辅助序列化格式
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
package tcp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// ReassemblerNode 表示重组器节点，用于在双向链表中管理TCP流的重组器实例。
// 每个节点对应一个TCP连接流，包含重组器实例、控制器和链表指针。
// 节点使用双向链表结构实现LRU（最近最少使用）缓存策略，提高流查找效率。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
//
// 字段说明:
//   - ID: TCP流的唯一标识符，基于源/目的IP和端口计算得出
//   - Reassembler: TCP重组器实例，负责处理该TCP连接的数据包重组
//   - Prev: 指向前一个节点的指针，用于双向链表遍历
//   - Next: 指向下一个节点的指针，用于双向链表遍历
//   - controller: 重组器控制器，管理重组器的通信通道和生命周期
//   - stopped: 标记节点是否已停止，防止重复关闭
type ReassemblerNode[P TCPPacket] struct {
	ID          string          `json:"id"`          // TCP流的唯一标识符
	Reassembler *Reassembler[P] `json:"reassembler"` // TCP重组器实例
}

// linkedNode 是 LRU 双向链表的节点，包装 ReassemblerNode。
// 将链表指针从 ReassemblerNode 中分离出来，使得 ReassemblerNode 可以单独序列化，
// 同时链表操作不侵入业务数据结构。
type linkedNode[P TCPPacket] struct {
	node *ReassemblerNode[P]
	prev *linkedNode[P]
	next *linkedNode[P]
}

// groupEntry 表示一个元数据分组下的所有重组器节点。
// 使用 HashMap 实现 O(1) 节点查找，同时使用独立双向链表维护 LRU 顺序。
// 相比原始的 O(n) 链表遍历，大幅提升高并发下的查找效率。
type groupEntry[P TCPPacket] struct {
	nodes map[string]*linkedNode[P] // streamID → linkedNode 映射
	head  *linkedNode[P]            // LRU 链表头（最近使用）
	tail  *linkedNode[P]            // LRU 链表尾（最久未使用）
}

func (g *groupEntry[P]) len() int {
	return len(g.nodes)
}

func (g *groupEntry[P]) pushFront(ln *linkedNode[P]) {
	ln.prev = nil
	ln.next = g.head
	if g.head != nil {
		g.head.prev = ln
	}
	g.head = ln
	if g.tail == nil {
		g.tail = ln
	}
	g.nodes[ln.node.ID] = ln
}

func (g *groupEntry[P]) popTail() *linkedNode[P] {
	if g.tail == nil {
		return nil
	}
	ln := g.tail
	g.remove(ln)
	return ln
}

func (g *groupEntry[P]) moveToFront(ln *linkedNode[P]) {
	if g.head == ln {
		return
	}
	g.remove(ln)
	g.pushFront(ln)
}

func (g *groupEntry[P]) remove(ln *linkedNode[P]) {
	if ln.prev != nil {
		ln.prev.next = ln.next
	} else {
		g.head = ln.next
	}
	if ln.next != nil {
		ln.next.prev = ln.prev
	} else {
		g.tail = ln.prev
	}
	ln.prev = nil
	ln.next = nil
	delete(g.nodes, ln.node.ID)
}

// run 启动重组器节点的重组器实例。
// 该方法初始化重组器控制器，并在新的goroutine中启动重组器的运行。
// 如果节点已经启动，则返回错误。
//
// 参数:
//   - wg: errgroup.Group实例，用于管理goroutine的错误和等待
//   - dstChan: 目标通道，用于输出重组后的应用层数据
//   - packetChannelSize: 数据包通道的缓冲区大小
//
// 返回值:
//   - error: 如果节点已经启动，返回错误；否则返回nil
//
// 启动流程:
//  1. 检查控制器是否已初始化，避免重复启动
//  2. 创建新的重组器控制器
//  3. 在新的goroutine中启动重组器
//  4. 设置defer确保节点关闭时资源正确释放
func (n *ReassemblerNode[P]) run(
	ctx context.Context,
	logger *logging.Logger,
	meta *n.BaseMeta,
	wg *errgroupx.Group,
	dstChan chan<- any,
) {
	wg.Go(func() error {
		defer func() {
			if err := recover(); err != nil {
				logger.Ctx(ctx).Errorf(`reassemblerNode %s panic: %v`, n.ID, err)
			}
		}()
		err := n.Reassembler.parse(
			ctx,
			meta,
			dstChan,
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Ctx(ctx).Debugf(`reassemblerNode %s run error: %v`, n.ID, err)
		}
		return nil
	})
}

// RouterAux 是 Router 的 JSON 序列化辅助格式，用于持久化和恢复路由器状态。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
type RouterAux[P TCPPacket] struct {
	Metas                 []*n.BaseMeta           `json:"metas"`                     // 基础元数据列表
	Nodes                 [][]*ReassemblerNode[P] `json:"nodes"`                     // 节点集合列表
	MaxReassemblerSize    int                     `json:"max_reassembler_size"`      // 最大重组器缓冲区大小
	MaxOutOrderPacketSize int                     `json:"max_out_order_packet_size"` // 最大乱序包数量
	NoticeTimeout         time.Duration           `json:"notice_timeout"`            // 通知超时时间
}

// toRouter 将RouterAux转换为Router实例。
// 该方法重建路由器中的双向链表结构，恢复路由器的运行时状态。
// 转换过程包括：
//  1. 为每个元数据创建根节点映射
//  2. 重建每个链表中的节点连接关系
//  3. 恢复路由器的配置参数
//
// 返回值:
//   - *Router[P]: 重建完成的路由器实例
//
// 重建流程:
//  1. 遍历所有元数据，为每个元数据创建根节点映射
//  2. 对于每个节点列表，重建双向链表连接
//  3. 恢复路由器的配置参数（缓冲区大小等）
//  4. 返回重建完成的路由器实例
//
// 注意：序列化时节点指针被置为nil，反序列化时需要重新建立连接关系。
func (s *RouterAux[P]) toRouter() *Router[P] {
	roots := make(map[n.BaseMeta]*groupEntry[P], len(s.Metas))
	for index, meta := range s.Metas {
		if index >= len(s.Nodes) || len(s.Nodes[index]) == 0 {
			continue
		}
		group := &groupEntry[P]{
			nodes: make(map[string]*linkedNode[P], len(s.Nodes[index])),
		}
		for _, node := range s.Nodes[index] {
			ln := &linkedNode[P]{node: node}
			group.nodes[node.ID] = ln
			if group.tail == nil {
				group.head = ln
				group.tail = ln
			} else {
				group.tail.next = ln
				ln.prev = group.tail
				group.tail = ln
			}
		}
		roots[*meta] = group
	}
	return &Router[P]{
		roots:                 roots,
		maxReassemblerSize:    s.MaxReassemblerSize,
		maxOutOrderPacketSize: s.MaxOutOrderPacketSize,
		noticeTimeout:         s.NoticeTimeout,
	}
}

// Router 是TCP包重组路由器，用于管理多个TCP重组器并将TCP包路由到相关的重组器中。
// 路由器使用基于元数据的哈希映射和双向链表实现高效的流管理和LRU缓存策略。
// 主要功能包括：
//  1. TCP流的路由和分发
//  2. 重组器节点的生命周期管理
//  3. LRU缓存策略实现
//  4. 序列化和反序列化支持
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
//
// 字段说明:
//   - roots: 根节点映射，键为BaseMeta，值为重组器节点链表的头节点
//   - packetChannelSize: 数据包通道大小，影响并发处理能力
//   - maxReassemblerSize: 最大重组器数量，控制路由器中同时管理的TCP流数量
//   - maxOutOrderPacketSize: 最大乱序包数量，控制乱序包处理的容量
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

// MarshalJSON 序列化Router实例为JSON格式的字节数组。
// 该方法将路由器的运行时状态转换为可序列化的存储格式，支持持久化和传输。
// 序列化过程包括：
//  1. 将路由器转换为RouterAux格式
//  2. 使用标准JSON库进行序列化
//  3. 处理序列化过程中的错误
//
// 返回值:
//   - []byte: JSON格式的字节数组
//   - error: 序列化过程中发生的错误，包含堆栈信息
//
// 序列化内容:
//   - 所有重组器节点的ID和重组器状态
//   - 路由器的配置参数（缓冲区大小等）
//   - 基础元数据信息
//
// 注意：节点之间的指针关系（Prev和Next）不会被序列化，需要在反序列化时重建。
func (r Router[P]) MarshalJSON() ([]byte, error) {
	store := r.toStore()
	if raw, err := json.Marshal(store); err != nil {
		return raw, errors.WithStack(err)
	} else {
		return raw, nil
	}
}

// UnmarshalJSON 反序列化JSON格式的字节数组为Router实例。
// 该方法从持久化存储中恢复路由器的状态，重建所有重组器节点和链表结构。
// 反序列化过程包括：
//  1. 解析JSON数据到RouterAux结构
//  2. 将RouterAux转换为Router实例
//  3. 重建双向链表结构
//
// 参数:
//   - data: JSON格式的字节数组，包含路由器的序列化状态
//
// 返回值:
//   - error: 反序列化过程中发生的错误，包含堆栈信息
//
// 反序列化流程:
//  1. 使用标准JSON库解析数据到RouterAux
//  2. 调用asRouter方法重建路由器实例
//  3. 恢复路由器的根节点映射
//
// 注意：反序列化后需要重新启动重组器节点，因为控制器状态无法序列化。
func (r *Router[P]) UnmarshalJSON(data []byte) error {
	initMetrics()
	var store RouterAux[P]
	if err := json.Unmarshal(data, &store); err != nil {
		return errors.WithStack(err)
	} else {
		*r = *store.toRouter()
	}
	return nil
}

// toStore 将Router实例转换为可序列化的存储格式。
// 该方法准备路由器的状态用于序列化，包括断开节点间的指针连接以避免循环引用。
// 转换过程包括：
//  1. 收集所有基础元数据
//  2. 遍历每个链表，收集所有节点
//  3. 断开节点的Prev和Next指针，避免序列化时的循环引用
//  4. 保存路由器的配置参数
//
// 返回值:
//   - *RouterAux[P]: 准备就绪的辅助格式实例
//
// 转换流程:
//  1. 初始化元数据和节点集合的切片
//  2. 遍历根节点映射，处理每个链表
//  3. 对于每个链表，遍历所有节点，断开指针连接并收集节点
//  4. 保存配置参数到辅助格式中
//
// 注意：该方法会修改节点的Prev和Next指针，将其设置为nil。
// 这是必要的，因为JSON序列化无法处理循环引用，且指针关系可以在反序列化时重建。
func (r *Router[P]) toStore() *RouterAux[P] {
	// 先收集所有 meta 及其对应的 nodes，然后按 meta 排序保证输出确定性
	type entry struct {
		meta  *n.BaseMeta
		nodes []*ReassemblerNode[P]
	}
	entries := make([]entry, 0, len(r.roots))
	for meta, group := range r.roots {
		m := meta
		sets := make([]*ReassemblerNode[P], 0, len(group.nodes))
		for ln := group.head; ln != nil; ln = ln.next {
			sets = append(sets, ln.node)
		}
		entries = append(entries, entry{meta: &m, nodes: sets})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if a.meta.Namespace != b.meta.Namespace {
			if a.meta.Namespace < b.meta.Namespace {
				return -1
			}
			return 1
		}
		if a.meta.Capture != b.meta.Capture {
			if a.meta.Capture < b.meta.Capture {
				return -1
			}
			return 1
		}
		if a.meta.Version != b.meta.Version {
			if a.meta.Version < b.meta.Version {
				return -1
			}
			return 1
		}
		return strings.Compare(a.meta.SessionID.String(), b.meta.SessionID.String())
	})

	metas := make([]*n.BaseMeta, len(entries))
	nodes := make([][]*ReassemblerNode[P], len(entries))
	for i, e := range entries {
		metas[i] = e.meta
		nodes[i] = e.nodes
	}
	return &RouterAux[P]{
		Metas:                 metas,
		Nodes:                 nodes,
		MaxReassemblerSize:    r.maxReassemblerSize,
		MaxOutOrderPacketSize: r.maxOutOrderPacketSize,
		NoticeTimeout:         r.noticeTimeout,
	}
}

// Default 返回一个使用当前路由器配置参数的新路由器实例。
// 该方法创建一个新的路由器实例，继承当前实例的所有配置参数，但使用空的节点映射。
// 这可以用于创建具有相同配置的新路由器，或者作为工厂方法使用。
//
// 返回值:
//   - any: 新的路由器实例，类型为Router[P]
//
// 使用场景:
//  1. 需要创建多个相同配置的路由器实例时
//  2. 作为工厂方法，根据现有配置创建新实例
//  3. 测试时创建独立的测试实例
//
// 注意：返回的路由器实例是全新的，不包含任何重组器节点。
// 配置参数（缓冲区大小等）从当前实例复制。
func (r *Router[P]) Default() any {
	return NewRouter[P](
		r.maxReassemblerSize,
		r.maxOutOrderPacketSize,
		r.noticeTimeout,
	)
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
			nodes: make(map[string]*linkedNode[P], r.maxReassemblerSize),
		}
		r.roots[*meta] = group
	}

	ln := group.nodes[streamID]

	if ln != nil && ln.node.Reassembler.isCompleted() {
		group.remove(ln)
		ln.node.Reassembler.stop()
		ln = nil
	}

	if ln == nil {
		if !packet.IsSyn() {
			return nil
		}
		rsm, err := newReassembler(
			ctx,
			packet,
			r.maxOutOrderPacketSize,
			r.noticeTimeout,
		)
		if err != nil {
			return errors.WithStack(err)
		}
		node := &ReassemblerNode[P]{
			ID:          streamID,
			Reassembler: rsm,
		}
		ln = &linkedNode[P]{
			node: node,
		}
		node.run(ctx, logger, meta, wg, dstChan)
		group.pushFront(ln)
	} else if err := ln.node.Reassembler.addPacket(ctx, packet); err != nil {
		return errors.WithStack(err)
	}

	ln.node.Reassembler.notice(ctx)
	group.moveToFront(ln)

	return nil
}

func (r *Router[P]) activeFlowCount() int {
	count := 0
	for _, group := range r.roots {
		if group == nil {
			continue
		}
		count += group.len()
	}
	return count
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
			if force || current.node.Reassembler.isCompleted() ||
				current.node.Reassembler.lastNoticeTime.Add(r.noticeTimeout).Before(time.Now()) {
				group.remove(current)
				current.node.Reassembler.stop()
				removedCount++
			}
			current = next
		}
		// 清理超出缓存的 tcp 流
		for group.len() > r.maxReassemblerSize {
			evicted := group.popTail()
			if evicted != nil {
				evicted.node.Reassembler.stop()
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
	ctx, cancel := context.WithCancel(ctx)
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
				cancel()
				return errors.WithStack(err)
			}
		}
	}
	// 最后执行一次清理，强制清理所有超时流
	r.cleanup(ctx, false)
	// 如果没有活跃的流，立即退出，避免不必要的等待
	if r.activeFlowCount() == 0 {
		cancel()
		if err := wg.Wait(); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	// 有活跃流时，等待一个 noticeTimeout 让它们完成或超时
	select {
	case <-ctx.Done():
	case <-time.After(r.noticeTimeout):
	}
	cancel()
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
