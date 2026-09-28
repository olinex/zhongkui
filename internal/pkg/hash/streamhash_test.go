package hash

import (
	"testing"
)

// TestStreamHasherConsistency 验证相同输入序列产生一致的哈希值。
func TestStreamHasherConsistency(t *testing.T) {
	parts := []string{"8080", "443", "192.168.1.100", "10.0.0.1"}

	h1 := Acquire()
	for _, p := range parts {
		h1.WriteString(p)
		h1.WriteString(":")
	}
	sum1 := h1.SumHex()
	h1.Release()

	h2 := Acquire()
	for _, p := range parts {
		h2.WriteString(p)
		h2.WriteString(":")
	}
	sum2 := h2.SumHex()
	h2.Release()

	if sum1 != sum2 {
		t.Fatalf("inconsistent hash: %s != %s", sum1, sum2)
	}
	if len(sum1) != 16 {
		t.Fatalf("expected 16-char hex, got %d: %s", len(sum1), sum1)
	}
}

// TestStreamHasherDifferentInput 验证不同输入产生不同的哈希值。
func TestStreamHasherDifferentInput(t *testing.T) {
	h1 := Acquire()
	h1.WriteString("192.168.1.100:8080:10.0.0.1:443")
	s1 := h1.SumHex()
	h1.Release()

	h2 := Acquire()
	h2.WriteString("192.168.1.100:8080:10.0.0.1:444")
	s2 := h2.SumHex()
	h2.Release()

	if s1 == s2 {
		t.Fatal("different inputs produced same hash")
	}
}

// TestStreamHasherUint16 验证按小端序写入 uint16 的哈希结果一致。
func TestStreamHasherUint16(t *testing.T) {
	h1 := Acquire()
	h1.WriteUint16(8080)
	h1.WriteUint16(443)
	h1.WriteUint16(30000)
	s1 := h1.SumHex()
	h1.Release()

	h2 := Acquire()
	h2.WriteUint16(8080)
	h2.WriteUint16(443)
	h2.WriteUint16(30000)
	s2 := h2.SumHex()
	h2.Release()

	if s1 != s2 {
		t.Fatalf("inconsistent uint16 hash: %s != %s", s1, s2)
	}
}

// BenchmarkStreamHasher 基准测试流式哈希器的拼接与求和性能。
func BenchmarkStreamHasher(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h := Acquire()
		h.WriteUint16(8080)
		h.WriteString(":")
		h.WriteUint16(443)
		h.WriteString(":")
		h.WriteString("192.168.1.100")
		h.WriteString(":")
		h.WriteString("10.0.0.1")
		_ = h.SumHex()
		h.Release()
	}
}
