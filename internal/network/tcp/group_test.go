package tcp

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func newTestNode(id string) *ReassemblerNode[*mockTCPPacket] {
	return &ReassemblerNode[*mockTCPPacket]{
		ID: id,
	}
}

// TestGroupEntry_BasicOperations 验证 groupEntry 的基本操作：初始为空以及 pushFront 与 len 的正确性。
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

// TestGroupEntry_PopTail 验证 groupEntry 的 pick 尾节点行为：空组无操作、单元素清空及多元素返回尾节点。
func TestGroupEntry_PopTail(t *testing.T) {
	Convey("Test groupEntry popTail", t, func() {

		Convey("pick tail on empty group is a no-op", func() {
			g := &groupEntry[*mockTCPPacket]{}
			So(g.tail, ShouldBeNil)
		})

		Convey("pick tail on single element empties the group", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			g.pushFront(n1)

			g.pick(g.tail)
			So(g.len(), ShouldEqual, 0)
			So(g.head, ShouldBeNil)
			So(g.tail, ShouldBeNil)
		})

		Convey("pick tail on multiple elements returns tail", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("flow-1")
			n2 := newTestNode("flow-2")
			n3 := newTestNode("flow-3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)
			// 顺序：head=n3、n2，tail=n1

			popped := g.tail
			g.pick(g.tail)
			So(popped, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 2)
			So(g.tail, ShouldEqual, n2)
			So(g.head, ShouldEqual, n3)
			_, exists := g.nodes["flow-1"]
			So(exists, ShouldBeFalse)
		})

	})
}

// TestGroupEntry_MoveToFront 验证 groupEntry 的 moveToFront：头节点无操作、尾节点与中间节点移动到头部。
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
			// 头节点为 n2，尾节点为 n1

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
			// 头节点为 n3、n2，尾节点为 n1

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
			// 头节点为 n3、n2，尾节点为 n1

			g.moveToFront(n2)
			So(g.head, ShouldEqual, n2)
			So(g.tail, ShouldEqual, n1)
			So(g.len(), ShouldEqual, 3)
			So(g.head.next, ShouldEqual, n3)
			So(g.head.next.next, ShouldEqual, n1)
		})

	})
}

// TestGroupEntry_Remove 验证 groupEntry 的 pick 删除行为：删头、删尾、删中间节点以及逐个删空。
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
			// 头节点为 n2，尾节点为 n1

			g.pick(n2)
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
			// 头节点为 n2，尾节点为 n1

			g.pick(n1)
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
			// 头节点为 n3、n2，尾节点为 n1

			g.pick(n2)
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

			g.pick(n1)
			g.pick(n2)
			So(g.len(), ShouldEqual, 0)
			So(g.head, ShouldBeNil)
			So(g.tail, ShouldBeNil)
		})

	})
}

// TestGroupEntry_OOPCapacityLimit 验证无 SYN 流的暂存队列容量上限：
// 超过 maxSize 的新包被丢弃，已暂存的前序包保留；maxSize<=0 表示不限制。
func TestGroupEntry_OOPCapacityLimit(t *testing.T) {
	Convey("Test groupEntry oop capacity limit", t, func() {

		Convey("addOOP drops packets beyond max size", func() {
			g := newGroupEntry[*mockTCPPacket]()
			now := time.Now()
			for i := 0; i < 5; i++ {
				g.addOOP("flow-1", &mockTCPPacket{seqNum: uint32(i)}, 2, now)
			}
			So(len(g.oop["flow-1"].packets), ShouldEqual, 2)
			So(g.oop["flow-1"].packets[0].seqNum, ShouldEqual, 0)
			So(g.oop["flow-1"].packets[1].seqNum, ShouldEqual, 1)
		})

		Convey("addOOP with non-positive max size does not limit", func() {
			g := newGroupEntry[*mockTCPPacket]()
			now := time.Now()
			for i := 0; i < 5; i++ {
				g.addOOP("flow-1", &mockTCPPacket{seqNum: uint32(i)}, 0, now)
			}
			So(len(g.oop["flow-1"].packets), ShouldEqual, 5)
		})

		Convey("multiple streams are capped independently", func() {
			g := newGroupEntry[*mockTCPPacket]()
			now := time.Now()
			for i := 0; i < 3; i++ {
				g.addOOP("flow-1", &mockTCPPacket{seqNum: uint32(i)}, 1, now)
				g.addOOP("flow-2", &mockTCPPacket{seqNum: uint32(i)}, 1, now)
			}
			So(len(g.oop["flow-1"].packets), ShouldEqual, 1)
			So(len(g.oop["flow-2"].packets), ShouldEqual, 1)
		})

	})
}

// TestGroupEntry_CleanupOOP 验证无 SYN 的暂存流按 noticeTimeout 清理：
// 超过超时仍未建流的条目被丢弃，未超时的保留；kill 模式清空全部暂存。
func TestGroupEntry_CleanupOOP(t *testing.T) {
	Convey("Test groupEntry cleanup oop", t, func() {

		Convey("cleanup drops entries older than noticeTimeout", func() {
			g := newGroupEntry[*mockTCPPacket]()
			timeout := 30 * time.Second
			g.addOOP("stale", &mockTCPPacket{seqNum: 1}, 0, time.Now().Add(-2*timeout))
			g.addOOP("fresh", &mockTCPPacket{seqNum: 1}, 0, time.Now())

			_, _, _, oopDropped := g.cleanup(timeout, 1024, false)
			So(oopDropped, ShouldEqual, 1)
			_, staleExists := g.oop["stale"]
			So(staleExists, ShouldBeFalse)
			_, freshExists := g.oop["fresh"]
			So(freshExists, ShouldBeTrue)
		})

		Convey("kill cleanup drops every oop entry", func() {
			g := newGroupEntry[*mockTCPPacket]()
			g.addOOP("flow-1", &mockTCPPacket{seqNum: 1}, 0, time.Now())
			g.addOOP("flow-2", &mockTCPPacket{seqNum: 1}, 0, time.Now())

			_, _, _, oopDropped := g.cleanup(time.Minute, 1024, true)
			So(oopDropped, ShouldEqual, 2)
			So(len(g.oop), ShouldEqual, 0)
		})

		Convey("cleanup does not touch completed node lifecycle counts", func() {
			g := newGroupEntry[*mockTCPPacket]()
			g.addOOP("fresh", &mockTCPPacket{seqNum: 1}, 0, time.Now())

			removed, evicted, living, oopDropped := g.cleanup(time.Minute, 1024, false)
			So(removed, ShouldEqual, 0)
			So(evicted, ShouldEqual, 0)
			So(living, ShouldEqual, 0)
			So(oopDropped, ShouldEqual, 0)
		})

	})
}

// TestGroupEntry_LRUOrdering 验证 groupEntry 的 LRU 顺序：pushFront 入头部、pick 尾部淘汰最旧，
// 以及 moveToFront 提升节点后改变淘汰顺序。
func TestGroupEntry_LRUOrdering(t *testing.T) {
	Convey("Test groupEntry LRU ordering", t, func() {

		Convey("pushFront inserts at head, pick tail removes oldest (FIFO-like after LRU)", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("1")
			n2 := newTestNode("2")
			n3 := newTestNode("3")
			g.pushFront(n1)
			g.pushFront(n2)
			g.pushFront(n3)

			// 按访问顺序 pick tail 应返回最旧的
			So(g.tail.ID, ShouldEqual, "1")
			g.pick(g.tail)
			So(g.tail.ID, ShouldEqual, "2")
			g.pick(g.tail)
			So(g.tail.ID, ShouldEqual, "3")
			g.pick(g.tail)
		})

		Convey("moveToFront promotes node, changing eviction order", func() {
			g := &groupEntry[*mockTCPPacket]{
				nodes: make(map[string]*ReassemblerNode[*mockTCPPacket]),
			}
			n1 := newTestNode("1")
			n2 := newTestNode("2")
			n3 := newTestNode("3")
			g.pushFront(n1) // 头节点为 1
			g.pushFront(n2) // 头节点为 2，尾节点为 1
			g.pushFront(n3) // 头节点为 3、2，尾节点为 1

			// 访问最旧的 n1，提升到头部
			g.moveToFront(n1) // 头节点为 1、3，尾节点为 2

			// 现在 pick tail 应返回 n2（新的最旧）
			So(g.tail.ID, ShouldEqual, "2")
			g.pick(g.tail)
			So(g.tail.ID, ShouldEqual, "3")
			g.pick(g.tail)
			So(g.tail.ID, ShouldEqual, "1")
			g.pick(g.tail)
		})
	})
}
