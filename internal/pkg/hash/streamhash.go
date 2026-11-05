package hash

import (
	"encoding/binary"
	"encoding/hex"
	"hash/maphash"
	"sync"

	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

var seed maphash.Seed = maphash.Seed{}

var pool = sync.Pool{
	New: func() any {
		h := new(maphash.Hash)
		h.SetSeed(seed)
		return h
	},
}

// StreamHasher 基于 maphash 的流式哈希器，用于对多个字段进行顺序拼接哈希。
type StreamHasher struct {
	// h 从对象池借出的 maphash 哈希器实例，承载累积哈希状态。
	h *maphash.Hash
}

// Acquire 从对象池中获取一个 StreamHasher，并重置其内部哈希状态。
func Acquire() *StreamHasher {
	h := pool.Get().(*maphash.Hash)
	h.Reset()
	return &StreamHasher{h: h}
}

// Release 将 StreamHasher 归还对象池以便复用。
func (s *StreamHasher) Release() {
	pool.Put(s.h)
}

// WriteString 将字符串写入哈希器。
func (s *StreamHasher) WriteString(str string) {
	s.h.WriteString(str)
}

// WriteBytes 将字节切片写入哈希器。
func (s *StreamHasher) WriteBytes(b []byte) {
	s.h.Write(b)
}

// WriteUint16 以小端序将 uint16 写入哈希器。
func (s *StreamHasher) WriteUint16(v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	s.h.Write(buf[:])
}

// Sum64 返回当前累积内容的 64 位哈希值。
func (s *StreamHasher) Sum64() uint64 {
	return s.h.Sum64()
}

// SumHex 返回当前累积内容哈希值的 16 进制字符串表示。
func (s *StreamHasher) SumHex() string {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], s.h.Sum64())
	return hex.EncodeToString(buf[:])
}

func init() {
	utils.SetField(&seed, "s", uint64(17))
}
