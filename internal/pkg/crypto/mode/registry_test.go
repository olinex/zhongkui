package mode

import (
	"strings"
	"testing"
)

// registryTestMode 测试用加密模式：仅原样返回数据，用于验证注册表分发。
type registryTestMode struct{}

// Encrypt 原样返回输入，满足 Mode 接口。
func (registryTestMode) Encrypt(p []byte) ([]byte, error) { return p, nil }

// Decrypt 原样返回输入，满足 Mode 接口。
func (registryTestMode) Decrypt(p []byte) ([]byte, error) { return p, nil }

// TestNew_AesGcmRegistered 验证内置 AES-GCM 已注册且可正常加解密。
func TestNew_AesGcmRegistered(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	m, err := New(AesGcm, key, nil)
	if err != nil {
		t.Fatalf("New(%q) failed: %v", AesGcm, err)
	}
	ciphertext, err := m.Encrypt([]byte("hello mode registry"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	plaintext, err := m.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(plaintext) != "hello mode registry" {
		t.Fatalf("roundtrip mismatch: %q", plaintext)
	}
}

// TestNew_UnknownMode 验证未注册的模式返回错误。
func TestNew_UnknownMode(t *testing.T) {
	_, err := New("not-registered", []byte("0123456789abcdef"), nil)
	if err == nil {
		t.Fatal("expected error for unregistered mode, got nil")
	}
	if !strings.Contains(err.Error(), "unknown crypto mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRegister_CustomMode 验证自定义模式注册后经 New 分发。
func TestRegister_CustomMode(t *testing.T) {
	Register("test-mode", func(key, additionalData []byte) (Mode, error) {
		return registryTestMode{}, nil
	})
	m, err := New("test-mode", nil, nil)
	if err != nil {
		t.Fatalf("New(test-mode) failed: %v", err)
	}
	if _, ok := m.(registryTestMode); !ok {
		t.Fatalf("unexpected mode type: %T", m)
	}
}

// TestRegister_DuplicatePanics 验证重复注册在注册期直接 panic。
func TestRegister_DuplicatePanics(t *testing.T) {
	Register("test-duplicate", func(key, additionalData []byte) (Mode, error) {
		return registryTestMode{}, nil
	})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register("test-duplicate", func(key, additionalData []byte) (Mode, error) {
		return registryTestMode{}, nil
	})
}

// TestRegister_EmptyNamePanics 验证空名称注册直接 panic。
func TestRegister_EmptyNamePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on empty mode name")
		}
	}()
	Register("", func(key, additionalData []byte) (Mode, error) {
		return registryTestMode{}, nil
	})
}
