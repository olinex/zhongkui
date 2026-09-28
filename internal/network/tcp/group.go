// Package tcp 实现 TCP 流重组与协议检测路由：按元数据（五元组）维护重组会话，并把数据包路由到
// 对应的重组器，向上层交付重组后的流。
package tcp

import (
	"slices"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// groupEntry 表示一个元数据分组下的所有重组器节点。
// 使用 HashMap 实现 O(1) 节点查找，同时使用独立双向链表维护 LRU 顺序。
type groupEntry[P TCPPacket] struct {
	nodes map[string]*ReassemblerNode[P] // streamID → 节点映射
	oop   map[string][]P                 // streamID → 未序数据包队列
	head  *ReassemblerNode[P]            // LRU 链表头（最近使用）
	tail  *ReassemblerNode[P]            // LRU 链表尾（最久未使用）
}

func newGroupEntry[P TCPPacket]() *groupEntry[P] {
	return &groupEntry[P]{
		nodes: make(map[string]*ReassemblerNode[P]),
		oop:   make(map[string][]P),
	}
}

func (g *groupEntry[P]) len() int {
	return len(g.nodes)
}

func (g *groupEntry[P]) addOOP(streamID string, packet P) {
	packets, ok := g.oop[streamID]
	if !ok {
		packets = []P{packet}
	} else {
		packets = append(packets, packet)
	}
	g.oop[streamID] = packets
}

func (g *groupEntry[P]) flushOOP(n *ReassemblerNode[P]) error {
	if packets, ok := g.oop[n.ID]; ok {
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

func (g *groupEntry[P]) cleanup(noticeTimeout time.Duration, maxReassemblerSize int, kill bool) (removed, evicted, living int) {
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
