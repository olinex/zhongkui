package hash

import (
	"encoding/binary"
	"encoding/hex"
	"hash/maphash"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/utils"
)

var seed maphash.Seed = maphash.Seed{}

var pool = sync.Pool{
	New: func() any {
		h := new(maphash.Hash)
		h.SetSeed(seed)
		return h
	},
}

type StreamHasher struct {
	h *maphash.Hash
}

func Acquire() *StreamHasher {
	h := pool.Get().(*maphash.Hash)
	h.Reset()
	return &StreamHasher{h: h}
}

func (s *StreamHasher) Release() {
	pool.Put(s.h)
}

func (s *StreamHasher) WriteString(str string) {
	s.h.WriteString(str)
}

func (s *StreamHasher) WriteBytes(b []byte) {
	s.h.Write(b)
}

func (s *StreamHasher) WriteUint16(v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	s.h.Write(buf[:])
}

func (s *StreamHasher) Sum64() uint64 {
	return s.h.Sum64()
}

func (s *StreamHasher) SumHex() string {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], s.h.Sum64())
	return hex.EncodeToString(buf[:])
}

func init() {
	utils.SetField(&seed, "s", uint64(17))
}
