package key

import (
	"bytes"
	"testing"
)

// mockSecretKeyProvider 用于测试的密钥提供者桩，直接返回预设的密钥。
type mockSecretKeyProvider struct {
	key []byte
}

func (m *mockSecretKeyProvider) GetSecretKey() ([]byte, error) {
	return m.key, nil
}

func newTestPasswordBook(size int) *PasswordBook {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}
	return &PasswordBook{data: data}
}

func newTestKeyDerivation(parentKey []byte, book *PasswordBook) *KeyDerivation {
	return NewKeyDerivation(&mockSecretKeyProvider{key: parentKey}, book)
}

func TestDeriveChildKey_Deterministic(t *testing.T) {
	book := newTestPasswordBook(256)
	kd := newTestKeyDerivation([]byte("test-parent-key"), book)

	key1, err := kd.DeriveChildKey(10, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key2, err := kd.DeriveChildKey(10, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Fatal("DeriveChildKey should be deterministic: same inputs produced different outputs")
	}
}

func TestDeriveChildKey_DifferentOffsetDifferentKey(t *testing.T) {
	book := newTestPasswordBook(256)
	kd := newTestKeyDerivation([]byte("test-parent-key"), book)

	key1, err := kd.DeriveChildKey(10, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key2, err := kd.DeriveChildKey(20, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Fatal("different offsets should produce different keys")
	}
}

func TestDeriveChildKey_DifferentParentKeyDifferentKey(t *testing.T) {
	book := newTestPasswordBook(256)
	kd1 := newTestKeyDerivation([]byte("parent-key-one"), book)
	kd2 := newTestKeyDerivation([]byte("parent-key-two"), book)

	key1, err := kd1.DeriveChildKey(10, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key2, err := kd2.DeriveChildKey(10, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bytes.Equal(key1, key2) {
		t.Fatal("different parent keys should produce different derived keys")
	}
}

func TestDeriveChildKey_WrapAround(t *testing.T) {
	book := newTestPasswordBook(32)
	kd := newTestKeyDerivation([]byte("wrap-test-key"), book)

	// 偏移 24 + saltLen(17) = 41 > 32，会发生回绕
	key1, err := kd.DeriveChildKey(24, 17)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key2, err := kd.DeriveChildKey(24, 17)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Fatal("wrap-around derivation should be deterministic")
	}

	// 验证回绕产生的密钥与非回绕调用不同
	key3, err := kd.DeriveChildKey(0, 17)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bytes.Equal(key1, key3) {
		t.Fatal("wrap-around should produce different key than non-wrap call")
	}
}

func TestDeriveChildKey_KeyLengthVariants(t *testing.T) {
	book := newTestPasswordBook(256)
	kd := newTestKeyDerivation([]byte("keylen-test-key"), book)

	testCases := []int{17, 24, 32}
	for _, keyLen := range testCases {
		key, err := kd.DeriveChildKey(0, keyLen)
		if err != nil {
			t.Fatalf("unexpected error for keyLen=%d: %v", keyLen, err)
		}
		if len(key) != keyLen {
			t.Fatalf("expected key length %d, got %d", keyLen, len(key))
		}
	}
}

func TestDeriveChildKey_TooShortKeyLen(t *testing.T) {
	book := newTestPasswordBook(256)
	kd := newTestKeyDerivation([]byte("short-keylen-test"), book)

	_, err := kd.DeriveChildKey(0, 16)
	if err == nil {
		t.Fatal("expected error for keyLen=16 (not greater than MinKeyLength), got nil")
	}
}
