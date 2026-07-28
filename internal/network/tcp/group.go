package tcp

// groupEntry 表示一个元数据分组下的所有重组器节点。
// 使用 HashMap 实现 O(1) 节点查找，同时使用独立双向链表维护 LRU 顺序。
type groupEntry[P TCPPacket] struct {
	nodes map[string]*ReassemblerNode[P] // streamID → 节点映射
	head  *ReassemblerNode[P]            // LRU 链表头（最近使用）
	tail  *ReassemblerNode[P]            // LRU 链表尾（最久未使用）
}

func (g *groupEntry[P]) len() int {
	return len(g.nodes)
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

func (g *groupEntry[P]) popTail() *ReassemblerNode[P] {
	if g.tail == nil {
		return nil
	}
	n := g.tail
	g.remove(n)
	return n
}

func (g *groupEntry[P]) moveToFront(n *ReassemblerNode[P]) {
	if g.head == n {
		return
	}
	g.remove(n)
	g.pushFront(n)
}

func (g *groupEntry[P]) remove(n *ReassemblerNode[P]) {
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
