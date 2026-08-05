package hash

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestNewConsistentHash 测试构造函数创建合法的哈希环
func TestNewConsistentHash(t *testing.T) {
	Convey("测试 ConsistentHash 构造函数", t, func() {
		Convey("NewConsistentHash(0) 应创建 vnodes=0 的环", func() {
			ch := NewConsistentHash(0)
			So(ch, ShouldNotBeNil)
			So(ch.vnodes, ShouldEqual, 0)
			So(ch.MemberCount(), ShouldEqual, 0)
		})

		Convey("NewConsistentHashWithDefaultVnodes 应使用 DefaultVnodes(128)", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			So(ch, ShouldNotBeNil)
			So(ch.vnodes, ShouldEqual, DefaultVnodes)
			So(ch.vnodes, ShouldEqual, 128)
			So(ch.MemberCount(), ShouldEqual, 0)
		})

		Convey("新创建的环 MemberCount 应为 0", func() {
			ch := NewConsistentHash(64)
			So(ch.MemberCount(), ShouldEqual, 0)
		})
	})
}

// TestConsistentHash_Update 测试成员更新
func TestConsistentHash_Update(t *testing.T) {
	Convey("测试 Update 成员更新", t, func() {
		Convey("Update 单个成员后 MemberCount 应为 1", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"server-a"})
			So(ch.MemberCount(), ShouldEqual, 1)
		})

		Convey("Update 多个成员后 MemberCount 应为正确数量", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"node-1", "node-2", "node-3"})
			So(ch.MemberCount(), ShouldEqual, 3)
		})

		Convey("Update 空切片应清空环", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"server-a", "server-b"})
			So(ch.MemberCount(), ShouldEqual, 2)

			ch.Update([]string{})
			So(ch.MemberCount(), ShouldEqual, 0)
			So(ch.Get("any-key"), ShouldEqual, "")
		})

		Convey("多次 Update 应替换而非追加", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"a", "b"})
			So(ch.MemberCount(), ShouldEqual, 2)

			ch.Update([]string{"c", "d", "e"})
			So(ch.MemberCount(), ShouldEqual, 3)

			// 旧成员不应该存在
			result := ch.Get("some-key")
			So(result, ShouldNotEqual, "")
			So(result == "a" || result == "b", ShouldBeFalse)
		})
	})
}

// TestConsistentHash_Get 测试键查找
func TestConsistentHash_Get(t *testing.T) {
	Convey("测试 Get 键查找", t, func() {
		Convey("空环上 Get 返回空字符串", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			So(ch.Get("anything"), ShouldEqual, "")
		})

		Convey("单成员环上 Get 始终返回该成员", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"only-node"})

			So(ch.Get("key-a"), ShouldEqual, "only-node")
			So(ch.Get("key-b"), ShouldEqual, "only-node")
			So(ch.Get("key-c"), ShouldEqual, "only-node")
			So(ch.Get(""), ShouldEqual, "only-node")
		})

		Convey("多成员环上同一 key 始终映射到同一成员（确定性）", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"alpha", "beta", "gamma"})

			results := make(map[string]string)
			for _, key := range []string{"k1", "k2", "k3", "k4", "k5"} {
				for i := 0; i < 100; i++ {
					result := ch.Get(key)
					if prev, ok := results[key]; ok {
						So(result, ShouldEqual, prev)
					} else {
						results[key] = result
					}
				}
			}
		})

		Convey("多成员环上不同 key 应分布到至少 2 个不同成员（100 个随机 key，3 个成员）", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"node-a", "node-b", "node-c"})

			memberSet := make(map[string]struct{})
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("random-key-%d", i)
				memberSet[ch.Get(key)] = struct{}{}
			}

			So(len(memberSet), ShouldBeGreaterThanOrEqualTo, 2)
			So(len(memberSet), ShouldBeLessThanOrEqualTo, 3)
		})
	})
}

// TestConsistentHash_HashKey 测试哈希函数
func TestConsistentHash_HashKey(t *testing.T) {
	Convey("测试 HashKey 哈希函数", t, func() {
		ch := NewConsistentHashWithDefaultVnodes()

		Convey("相同 key 产生相同哈希值", func() {
			h1 := ch.HashKey("test-key")
			h2 := ch.HashKey("test-key")
			So(h1, ShouldEqual, h2)
		})

		Convey("不同 key 产生不同哈希值（极大概率）", func() {
			h1 := ch.HashKey("apple")
			h2 := ch.HashKey("banana")
			So(h1, ShouldNotEqual, h2)
		})

		Convey("空字符串产生合法的 uint32 值", func() {
			h := ch.HashKey("")
			// uint32 总是合法的，这里验证不会 panic
			So(h, ShouldBeGreaterThanOrEqualTo, uint32(0))
		})
	})
}

// TestConsistentHash_HashVNode 测试虚拟节点哈希
func TestConsistentHash_HashVNode(t *testing.T) {
	Convey("测试 HashVNode 虚拟节点哈希", t, func() {
		ch := NewConsistentHashWithDefaultVnodes()

		Convey("同一成员的不同的 vnode 索引产生不同哈希值", func() {
			h0 := ch.HashVNode("server-1", 0)
			h1 := ch.HashVNode("server-1", 1)
			h2 := ch.HashVNode("server-1", 127)

			So(h0, ShouldNotEqual, h1)
			So(h1, ShouldNotEqual, h2)
			So(h0, ShouldNotEqual, h2)
		})

		Convey("相同成员和索引产生相同哈希值", func() {
			h1 := ch.HashVNode("server-x", 42)
			h2 := ch.HashVNode("server-x", 42)
			So(h1, ShouldEqual, h2)
		})
	})
}

// TestConsistentHash_HashesOf 测试虚拟节点哈希值列表
func TestConsistentHash_HashesOf(t *testing.T) {
	Convey("测试 HashesOf 虚拟节点哈希值列表", t, func() {
		vnodes := 64
		ch := NewConsistentHash(vnodes)
		ch.Update([]string{"member-a", "member-b"})

		Convey("存在成员的 HashesOf 返回 vnodes 数量的哈希值", func() {
			hashes := ch.HashesOf("member-a")
			So(len(hashes), ShouldEqual, vnodes)
		})

		Convey("哈希值列表是升序排列的", func() {
			hashes := ch.HashesOf("member-a")
			So(len(hashes), ShouldEqual, vnodes)

			for i := 1; i < len(hashes); i++ {
				So(hashes[i], ShouldBeGreaterThanOrEqualTo, hashes[i-1])
			}
		})

		Convey("所有哈希值都是合法的 uint32", func() {
			hashes := ch.HashesOf("member-a")
			for _, h := range hashes {
				So(h, ShouldBeGreaterThanOrEqualTo, uint32(0))
			}
		})
	})
}

// TestConsistentHash_VNodeIndicesOf 测试虚拟节点索引分配
func TestConsistentHash_VNodeIndicesOf(t *testing.T) {
	Convey("测试 VNodeIndicesOf 虚拟节点索引分配", t, func() {
		Convey("空环上 VNodeIndicesOf 返回 nil", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			indices := ch.VNodeIndicesOf("anything")
			So(indices, ShouldBeNil)
		})

		Convey("不存在的成员返回 nil", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"real-member"})
			indices := ch.VNodeIndicesOf("fake-member")
			So(indices, ShouldBeNil)
		})

		Convey("单成员时应拥有全部索引 [0, vnodes-1]", func() {
			vnodes := 16
			ch := NewConsistentHash(vnodes)
			ch.Update([]string{"solo"})

			indices := ch.VNodeIndicesOf("solo")
			So(len(indices), ShouldEqual, vnodes)

			for i := 0; i < vnodes; i++ {
				So(indices[i], ShouldEqual, i)
			}
		})

		Convey("N 个成员时每个成员按轮询策略获得索引 {position, position+N, position+2N, ...}", func() {
			vnodes := 12
			ch := NewConsistentHash(vnodes)
			// 有序成员列表
			ch.Update([]string{"a", "b", "c"})

			// position 0: indices = {0, 3, 6, 9}
			indicesA := ch.VNodeIndicesOf("a")
			So(indicesA, ShouldResemble, []int{0, 3, 6, 9})

			// position 1: indices = {1, 4, 7, 10}
			indicesB := ch.VNodeIndicesOf("b")
			So(indicesB, ShouldResemble, []int{1, 4, 7, 10})

			// position 2: indices = {2, 5, 8, 11}
			indicesC := ch.VNodeIndicesOf("c")
			So(indicesC, ShouldResemble, []int{2, 5, 8, 11})
		})

		Convey("所有成员的索引互不相交且覆盖全部 vnodes", func() {
			vnodes := 128
			members := []string{"alpha", "beta", "gamma", "delta"}
			ch := NewConsistentHash(vnodes)
			ch.Update(members)

			allIndices := make(map[int]struct{})
			for _, m := range members {
				indices := ch.VNodeIndicesOf(m)
				for _, idx := range indices {
					// 每个索引应该只出现一次
					_, exists := allIndices[idx]
					So(exists, ShouldBeFalse)
					allIndices[idx] = struct{}{}
				}
			}

			// 应该覆盖全部 vnodes
			So(len(allIndices), ShouldEqual, vnodes)
			for i := 0; i < vnodes; i++ {
				_, ok := allIndices[i]
				So(ok, ShouldBeTrue)
			}
		})
	})
}

// TestConsistentHash_Rebalance 测试成员变更时的环重平衡
func TestConsistentHash_Rebalance(t *testing.T) {
	Convey("测试成员变更时的环重平衡", t, func() {
		const numKeys = 1000

		Convey("移除 1 个成员时，迁移量应合理（不超过约 50%）", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			allMembers := []string{"node-x", "node-y", "node-z"}
			ch.Update(allMembers)

			// 记录 1000 个 key 的映射
			keyMapping := make(map[string]string, numKeys)
			for i := 0; i < numKeys; i++ {
				key := fmt.Sprintf("key-%06d", i)
				keyMapping[key] = ch.Get(key)
			}

			// 移除中间成员 "node-y"
			ch.Update([]string{"node-x", "node-z"})

			// 统计变更
			changed := 0
			for key, oldMember := range keyMapping {
				newMember := ch.Get(key)
				if newMember != oldMember {
					changed++
				}
			}

			// 至少有一些 key 发生了迁移（孤儿 key 被重新分配）
			So(changed, ShouldBeGreaterThan, 0)
			// 迁移量不应超过 50%
			So(changed, ShouldBeLessThanOrEqualTo, numKeys/2)
		})

		Convey("未被移除的成员上的 key 应尽量保持不变", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"server-1", "server-2", "server-3"})

			// 记录每个 key 的映射
			keyMapping := make(map[string]string, numKeys)
			for i := 0; i < numKeys; i++ {
				key := fmt.Sprintf("data-%06d", i)
				keyMapping[key] = ch.Get(key)
			}

			// 移除 "server-2"
			ch.Update([]string{"server-1", "server-3"})

			// 原本在 server-1 或 server-3 上的 key 应该尽量保持不变
			stayedSame := 0
			wasOnRemaining := 0
			for key, oldMember := range keyMapping {
				if oldMember == "server-2" {
					continue // 跳过被移除成员上的 key
				}
				wasOnRemaining++
				newMember := ch.Get(key)
				if newMember == oldMember {
					stayedSame++
				}
			}

			// 剩余成员上的 key 大多数应保持不变
			So(wasOnRemaining, ShouldBeGreaterThan, 0)
			So(stayedSame, ShouldBeGreaterThan, 0)
			// 至少 85% 的剩余成员 key 保持不变
			stayRatio := float64(stayedSame) / float64(wasOnRemaining)
			So(stayRatio, ShouldBeGreaterThan, 0.85)
		})
	})
}

// TestConsistentHash_MemberCount 测试成员数量跟踪
func TestConsistentHash_MemberCount(t *testing.T) {
	Convey("测试 MemberCount 成员数量跟踪", t, func() {
		Convey("新环的成员数为 0", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			So(ch.MemberCount(), ShouldEqual, 0)
		})

		Convey("Update 5 个成员后 MemberCount 返回 5", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"a", "b", "c", "d", "e"})
			So(ch.MemberCount(), ShouldEqual, 5)
		})

		Convey("Update 0 个成员后 MemberCount 返回 0", func() {
			ch := NewConsistentHashWithDefaultVnodes()
			ch.Update([]string{"a", "b", "c"})
			So(ch.MemberCount(), ShouldEqual, 3)

			ch.Update([]string{})
			So(ch.MemberCount(), ShouldEqual, 0)
		})
	})
}
