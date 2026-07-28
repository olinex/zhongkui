package tcp

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func newTestNode(id string) *ReassemblerNode[*mockTCPPacket] {
	return &ReassemblerNode[*mockTCPPacket]{
		ID: id,
	}
}

func TestGroupEntry_BasicOperations(t *testing.T) {
	Convey("Test groupEntry basic operations", t, func() {

		Convey("Initial state should be empty", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			So(g.len(), ShouldEqual, 0)
			So(g.head, ShouldBeNil)
			So(g.tail, ShouldBeNil)
		})

		Convey("pushFront and len should work correctly", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			g.pushFront(n1)
			So(g.len(), ShouldEqual, 1)
			So(g.head, ShouldEqual, n1)
			So(g.tail, ShouldEqual, n1)

			n2 := newTestNode("flow-2")
			g.pushFront(n2)
			So(g.len(), ShouldEqual, 2)
			So(g.head, ShouldEqual, n2)
			So(g.tail, ShouldEqual, n1)
		})

	})
}

func TestGroupEntry_PopTail(t *testing.T) {
	Convey("Test groupEntry popTail", t, func() {

		Convey("popTail on empty group returns nil", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			So(g.popTail(), ShouldBeNil)
		})

		Convey("popTail on single element empties the group", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			g.pushFront(n1)

			popped := g.popTail()
			So(popped, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 0)
			So(g.head, ShouldBeNil)
			So(g.tail, ShouldBeNil)
		})

		Convey("popTail on multiple elements returns tail", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			n3 := newTestNode("flow-3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)
			// order: head=n3, n2, tail=n1

			popped := g.popTail()
			So(popped, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 2)
			So(g.tail, ShouldEqual, n2)
			So(g.head, ShouldEqual, n3)
			_, exists := g.nodes["flow-1"]
			So(exists, ShouldBeFalse)
		})

	})
}

func TestGroupEntry_MoveToFront(t *testing.T) {
	Convey("Test groupEntry moveToFront", t, func() {

		Convey("moveToFront on head is a no-op", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			g.pushFront(n1)
			g.pushFront(n2)
			// head=n2, tail=n1

			g.moveToFront(n2)
			So(g.head, ShouldEqual, n2)
			So(g.tail, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 2)
		})

		Convey("moveToFront moves tail to head", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			n3 := newTestNode("flow-3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)
			// head=n3, n2, tail=n1

			g.moveToFront(n1)
			So(g.head, ShouldEqual, n1)
			So(g.tail, ShouldEqual, n2)
			So(g.len(), ShouldEqual, 3)
			So(g.head.next, ShouldEqual, n3)
		})

		Convey("moveToFront moves middle node to head", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			n3 := newTestNode("flow-3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)
			// head=n3, n2, tail=n1

			g.moveToFront(n2)
			So(g.head, ShouldEqual, n2)
			So(g.tail, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 3)
			So(g.head.next, ShouldEqual, n3)
			So(g.head.next.next, ShouldEqual, n1)
		})

	})
}

func TestGroupEntry_Remove(t *testing.T) {
	Convey("Test groupEntry remove", t, func() {

		Convey("remove head should update head", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			g.pushFront(n1)
			g.pushFront(n2)
			// head=n2, tail=n1

			g.remove(n2)
			So(g.head, ShouldEqual, n1)
			So(g.tail, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 1)
			_, exists := g.nodes["flow-2"]
			So(exists, ShouldBeFalse)
		})

		Convey("remove tail should update tail", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			g.pushFront(n1)
			g.pushFront(n2)
			// head=n2, tail=n1

			g.remove(n1)
			So(g.head, ShouldEqual, n2)
			So(g.tail, ShouldEqual, n2)
			So(g.len(), ShouldEqual, 1)
			_, exists := g.nodes["flow-1"]
			So(exists, ShouldBeFalse)
		})

		Convey("remove middle node links prev and next", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			n3 := newTestNode("flow-3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)
			// head=n3, n2, tail=n1

			g.remove(n2)
			So(g.head, ShouldEqual, n3)
			So(g.tail, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 2)
			So(g.head.next, ShouldEqual, n1)
			So(n2.prev, ShouldBeNil)
			So(n2.next, ShouldBeNil)
			_, exists := g.nodes["flow-2"]
			So(exists, ShouldBeFalse)
		})

		Convey("remove all nodes one by one empties the group", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			g.pushFront(n1)
			g.pushFront(n2)

			g.remove(n1)
			g.remove(n2)
			So(g.len(), ShouldEqual, 0)
			So(g.head, ShouldBeNil)
			So(g.tail, ShouldBeNil)
		})

	})
}

func TestGroupEntry_LRUOrdering(t *testing.T) {
	Convey("Test groupEntry LRU ordering", t, func() {

		Convey("pushFront inserts at head, popTail removes tail (FIFO-like after LRU)", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("1")
			n2 := newTestNode("2")
			n3 := newTestNode("3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)

			// 按访问顺序 popTail 应返回最旧的
			So(g.popTail().ID, ShouldEqual, "1")
			So(g.popTail().ID, ShouldEqual, "2")
			So(g.popTail().ID, ShouldEqual, "3")
		})

		Convey("moveToFront promotes node, changing eviction order", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("1")
			n2 := newTestNode("2")
			n3 := newTestNode("3")
			g.pushFront(n1) // head=1
			g.pushFront(n2) // head=2, tail=1
			g.pushFront(n3) // head=3, 2, tail=1

			// 访问最旧的 n1，提升到头部
			g.moveToFront(n1) // head=1, 3, tail=2

			// 现在 popTail 应返回 n2（新的最旧）
			So(g.popTail().ID, ShouldEqual, "2")
			So(g.popTail().ID, ShouldEqual, "3")
			So(g.popTail().ID, ShouldEqual, "1")
		})
	})
}
