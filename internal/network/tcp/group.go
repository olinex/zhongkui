// Package tcp 实现 TCP 流重组与协议检测路由：按元数据（五元组）维护重组会话，并把数据包路由到
// 对应的重组器，向上层交付重组后的流。
package tcp

import (
	"slices"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// groupEntry 表示一个元数据分组下的所有重组器节点。
// 使用 HashMap 实现 O(1) 节点查找，同时使用独立双向链表维护 LRU 顺序。
type groupEntry[P TCPPacket] struct {
	nodes map[string]*ReassemblerNode[P] // streamID → 节点映射
	oop   map[string]*oopEntry[P]        // streamID → 未建流前的暂存数据包队列
	head  *ReassemblerNode[P]            // LRU 链表头（最近使用）
	tail  *ReassemblerNode[P]            // LRU 链表尾（最久未使用）
}

// oopEntry 表示一条尚未观测到 SYN 的流的暂存数据包队列。
// 重组器只能由 SYN 触发创建，先于 SYN 到达的包需要暂存等待；
// firstSeen 记录该流首个暂存包的到达时间，供 cleanup 按 noticeTimeout
// 丢弃永远等不到 SYN 的流，避免其无限堆积。
type oopEntry[P TCPPacket] struct {
	packets   []P       // 按到达顺序暂存的数据包，flush 时按 SeqNum 排序
	firstSeen time.Time // 首个暂存包的到达时间
}

func newGroupEntry[P TCPPacket]() *groupEntry[P] {
	return &groupEntry[P]{
		nodes: make(map[string]*ReassemblerNode[P]),
		oop:   make(map[string]*oopEntry[P]),
	}
}

func (g *groupEntry[P]) len() int {
	return len(g.nodes)
}

// addOOP 暂存一条尚未创建重组器的流的数据包，等待其 SYN 到达后按序注入。
// maxSize 限制单条流的暂存包数量（<=0 表示不限制）：达到上限后丢弃新包，
// 保留已暂存的前序包，避免无 SYN 的流无限占用内存。
// now 作为该流首个暂存包的到达时间，供 cleanup 判定是否超时。
func (g *groupEntry[P]) addOOP(streamID string, packet P, maxSize int, now time.Time) {
	entry, ok := g.oop[streamID]
	if !ok {
		entry = &oopEntry[P]{firstSeen: now}
		g.oop[streamID] = entry
	}
	if maxSize > 0 && len(entry.packets) >= maxSize {
		return
	}
	entry.packets = append(entry.packets, packet)
}

func (g *groupEntry[P]) flushOOP(n *ReassemblerNode[P]) error {
	if entry, ok := g.oop[n.ID]; ok {
		packets := entry.packets
		slices.SortFunc(packets, func(a, b P) int {
			aSeq, bSeq := a.SeqNum(), b.SeqNum()
			if aSeq < bSeq {
				return -1
			}
			if aSeq > bSeq {
				return 1
			}
			return 0
		})
		for _, packet := range packets {
			if err := n.reassembler.addPacket(n.ctx, packet); err != nil {
				return errors.WithStack(err)
			}
		}
		delete(g.oop, n.ID)
	}
	return nil
}

func (g *groupEntry[P]) pushFront(n *ReassemblerNode[P]) {
	n.prev = nil
	n.next = g.head
	if g.head != nil {
		g.head.prev = n
	}
	g.head = n
	if g.tail == nil {
		g.tail = n
	}
	g.nodes[n.ID] = n
}

func (g *groupEntry[P]) moveToFront(n *ReassemblerNode[P]) {
	if g.head == n {
		return
	}
	g.pick(n)
	g.pushFront(n)
}

func (g *groupEntry[P]) pick(n *ReassemblerNode[P]) {
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		g.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		g.tail = n.prev
	}
	n.prev = nil
	n.next = nil
	delete(g.nodes, n.ID)
}

func (g *groupEntry[P]) remove(n *ReassemblerNode[P], kill bool) {
	g.pick(n)
	n.stop(kill)
	delete(g.oop, n.ID)
}

func (g *groupEntry[P]) cleanup(noticeTimeout time.Duration, maxReassemblerSize int, kill bool) (removed, evicted, living, oopDropped int) {
	now := time.Now()
	// 清理永远等不到 SYN 的暂存流：自首个包到达起超过 noticeTimeout 仍未建流，
	// 说明该流的握手不在采集窗口内，继续保留只会无限占用内存。
	for streamID, entry := range g.oop {
		if kill || now.Sub(entry.firstSeen) >= noticeTimeout {
			delete(g.oop, streamID)
			oopDropped++
		}
	}
	current := g.head
	// 清理已完成的或明显异常的 tcp 流
	for current != nil {
		next := current.next
		if kill || current.clearable(noticeTimeout) {
			g.remove(current, kill)
			removed++
		} else {
			living++
		}
		current = next
	}
	// 清理超出缓存的 tcp 流
	for g.len() > maxReassemblerSize {
		if g.tail == nil {
			break
		}
		g.remove(g.tail, true)
		evicted++
		living--
	}
	return
}
