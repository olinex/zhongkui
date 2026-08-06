package hash

import (
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
)

const (
	// DefaultVnodes 每个成员对应的虚拟节点数。
	// 128 个虚拟节点在负载均衡和迁移成本之间取得平衡。
	DefaultVnodes = 128
)

// ConsistentHash 基于虚拟节点的一致性哈希环。
//
// 每个成员在环上对应 vnodes 个虚拟节点，通过 hash(memberID + vnodeIndex) 计算位置。
// 查找 key 时，计算 hash(key) 并在环上顺时针找到第一个虚拟节点，其所属成员即为目标。
type ConsistentHash struct {
	vnodes   int               // 每成员虚拟节点数
	members  []string          // 有序成员列表（去重）
	ring     []uint32          // 有序哈希值列表
	ownerMap map[uint32]string // hash → memberID
}

// NewConsistentHash 创建新的一致性哈希环。
func NewConsistentHash(vnodes int) *ConsistentHash {
	return &ConsistentHash{
		vnodes:   vnodes,
		ownerMap: make(map[uint32]string),
	}
}

func NewConsistentHashWithDefaultVnodes() *ConsistentHash {
	return NewConsistentHash(DefaultVnodes)
}

// Update 用新的成员列表重建哈希环。
// members 必须已排序。
func (ch *ConsistentHash) Update(members []string) {
	ch.members = members
	ch.ring = make([]uint32, 0, len(members)*ch.vnodes)
	ch.ownerMap = make(map[uint32]string, len(members)*ch.vnodes)

	for _, member := range members {
		for i := 0; i < ch.vnodes; i++ {
			h := ch.HashVNode(member, i)
			ch.ring = append(ch.ring, h)
			ch.ownerMap[h] = member
		}
	}
	slices.Sort(ch.ring)
}

// Get 在哈希环上查找 key 对应的成员。
// 如果环为空，返回空字符串。
func (ch *ConsistentHash) Get(key string) string {
	if len(ch.ring) == 0 {
		return ""
	}
	h := ch.HashKey(key)
	// 二分查找第一个 >= h 的位置
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= h
	})
	if idx == len(ch.ring) {
		idx = 0 // 环状，回到开头
	}
	return ch.ownerMap[ch.ring[idx]]
}

// MemberCount 返回当前成员数量。
func (ch *ConsistentHash) MemberCount() int {
	return len(ch.members)
}

// HashKey 计算 key 的 32 位 FNV-1a 哈希值。
func (ch *ConsistentHash) HashKey(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}

// HashVNode 计算成员第 i 个虚拟节点的哈希值。
func (ch *ConsistentHash) HashVNode(member string, i int) uint32 {
	return ch.HashKey(fmt.Sprintf("%s#%d", member, i))
}

// HashesOf 返回指定成员在环上所有虚拟节点的哈希值列表（升序）。
// 哈希值为 32-bit FNV-1a 结果，范围 [0, 2^32)。
// 成员不存在时返回空切片。
//
// 该方法通过 HashVNode 实时计算，无需额外存储。
// 常用于调试环分布、计算迁移影响或可视化。
func (ch *ConsistentHash) HashesOf(member string) []uint32 {
	hashes := make([]uint32, 0, ch.vnodes)
	for i := 0; i < ch.vnodes; i++ {
		hashes = append(hashes, ch.HashVNode(member, i))
	}
	slices.Sort(hashes)
	return hashes
}

// VNodeIndicesOf 返回指定成员在数据分片中所属的虚拟节点索引列表（升序）。
//
// 与 HashesOf 返回环上哈希位置不同，该方法返回可用于取模分片的索引值
// （范围 [0, vnodes)）。索引分配采用轮询策略：成员在有序列表中位于
// position 时，所属索引为 {position, position+N, position+2N, ...}，
// 其中 N = 成员总数。该方案确保数据均匀分布且与 ClickHouse 的取模哈希兼容。
//
// 成员不存在时返回空切片。
func (ch *ConsistentHash) VNodeIndicesOf(member string) []int {
	if len(ch.members) == 0 {
		return nil
	}
	position := ch.memberPosition(member)
	if position < 0 {
		return nil
	}
	totalMembers := len(ch.members)
	indices := make([]int, 0, ch.vnodes/totalMembers+1)
	for i := position; i < ch.vnodes; i += totalMembers {
		indices = append(indices, i)
	}
	return indices
}

// memberPosition 返回成员在有序成员列表中的位置，不存在时返回 -1。
func (ch *ConsistentHash) memberPosition(member string) int {
	for i, m := range ch.members {
		if m == member {
			return i
		}
	}
	return -1
}
