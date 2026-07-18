package key

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ConfigFileSecretKeyProvider 测试
// ---------------------------------------------------------------------------

func TestConfigFileSecretKeyProvider_GetSecretKey_Valid(t *testing.T) {
	expectedKey := []byte("test-secret-key-12345")
	provider := NewConfigFileSecretKeyProvider(string(expectedKey))

	key, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("GetSecretKey() returned error: %v", err)
	}
	if string(key) != string(expectedKey) {
		t.Fatalf("GetSecretKey() returned %q, want %q", key, expectedKey)
	}
}

func TestConfigFileSecretKeyProvider_GetSecretKey_Empty(t *testing.T) {
	provider := NewConfigFileSecretKeyProvider("")

	_, err := provider.GetSecretKey()
	if err == nil {
		t.Fatal("expected error for empty secret key, got nil")
	}
}

func TestConfigFileSecretKeyProvider_GetSecretKey_MultipleCalls(t *testing.T) {
	provider := NewConfigFileSecretKeyProvider("test-key")

	// 多次调用应该都能成功
	for i := 0; i < 3; i++ {
		key, err := provider.GetSecretKey()
		if err != nil {
			t.Fatalf("call %d: GetSecretKey() returned error: %v", i, err)
		}
		if string(key) != "test-key" {
			t.Fatalf("call %d: GetSecretKey() returned %q, want %q", i, key, "test-key")
		}
	}
}

// ---------------------------------------------------------------------------
// KMSSecretKeyProvider 测试
// ---------------------------------------------------------------------------

func TestKMSSecretKeyProvider_GetSecretKey_ReturnsError(t *testing.T) {
	provider := NewKMSSecretKeyProvider()

	_, err := provider.GetSecretKey()
	if err == nil {
		t.Fatal("expected error from KMS provider (not implemented), got nil")
	}
}

// ---------------------------------------------------------------------------
// NewSecretKeyProvider 工厂函数测试
// ---------------------------------------------------------------------------

func TestNewSecretKeyProvider_Config(t *testing.T) {
	provider, err := NewSecretKeyProvider(CRYPTO_SOURCE_CONFIG, "test-key")
	if err != nil {
		t.Fatalf("NewSecretKeyProvider() returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("NewSecretKeyProvider() returned nil")
	}

	// 验证返回的是 ConfigFileSecretKeyProvider
	if _, ok := provider.(*ConfigFileSecretKeyProvider); !ok {
		t.Fatal("expected *ConfigFileSecretKeyProvider")
	}
}

func TestNewSecretKeyProvider_KMS(t *testing.T) {
	provider, err := NewSecretKeyProvider(CRYPTO_SOURCE_KMS, "")
	if err != nil {
		t.Fatalf("NewSecretKeyProvider() returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("NewSecretKeyProvider() returned nil")
	}

	// 验证返回的是 KMSSecretKeyProvider
	if _, ok := provider.(*KMSSecretKeyProvider); !ok {
		t.Fatal("expected *KMSSecretKeyProvider")
	}
}

func TestNewSecretKeyProvider_InvalidSource(t *testing.T) {
	_, err := NewSecretKeyProvider("invalid-source", "key")
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestNewSecretKeyProvider_EmptySource(t *testing.T) {
	_, err := NewSecretKeyProvider("", "key")
	if err == nil {
		t.Fatal("expected error for empty source, got nil")
	}
}

// ---------------------------------------------------------------------------
// 常量测试
// ---------------------------------------------------------------------------

func TestCryptoSourceConstants(t *testing.T) {
	if CRYPTO_SOURCE_CONFIG != "config" {
		t.Fatalf("CRYPTO_SOURCE_CONFIG = %q, want %q", CRYPTO_SOURCE_CONFIG, "config")
	}
	if CRYPTO_SOURCE_KMS != "kms" {
		t.Fatalf("CRYPTO_SOURCE_KMS = %q, want %q", CRYPTO_SOURCE_KMS, "kms")
	}
}

// ---------------------------------------------------------------------------
// 集成测试：完整的密钥提供者使用流程
// ---------------------------------------------------------------------------

func TestSecretKeyProvider_Integration(t *testing.T) {
	testCases := []struct {
		name           string
		source         string
		parentKey      string
		newErr         bool // NewSecretKeyProvider 返回错误
		getKeyErr      bool // GetSecretKey 返回错误
	}{
		{
			name:      "valid config source",
			source:    CRYPTO_SOURCE_CONFIG,
			parentKey: "my-secret-key",
		},
		{
			name:      "empty config key",
			source:    CRYPTO_SOURCE_CONFIG,
			parentKey: "",
			getKeyErr: true,
		},
		{
			name:      "kms source (not implemented)",
			source:    CRYPTO_SOURCE_KMS,
			parentKey: "",
			getKeyErr: true,
		},
		{
			name:      "invalid source",
			source:    "invalid",
			parentKey: "key",
			newErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewSecretKeyProvider(tc.source, tc.parentKey)
			if tc.newErr {
				if err == nil {
					t.Fatal("expected error from NewSecretKeyProvider, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewSecretKeyProvider() returned error: %v", err)
			}

			key, err := provider.GetSecretKey()
			if tc.getKeyErr {
				if err == nil {
					t.Fatal("expected error from GetSecretKey, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetSecretKey() returned error: %v", err)
			}
			if len(key) == 0 {
				t.Fatal("GetSecretKey() returned empty key")
			}
		})
	}
}
