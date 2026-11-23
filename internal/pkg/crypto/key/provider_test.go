package key

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestKMSSecretKeyProvider_GetSecretKey(t *testing.T) {
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", `printf 'kms-secret-key'`}, time.Second)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	key, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("GetSecretKey() returned error: %v", err)
	}
	if string(key) != "kms-secret-key" {
		t.Fatalf("GetSecretKey() returned %q, want %q", key, "kms-secret-key")
	}
}

func TestKMSSecretKeyProvider_TrimsWhitespace(t *testing.T) {
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", `printf 'kms-secret-key\n\n'`}, time.Second)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	key, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("GetSecretKey() returned error: %v", err)
	}
	if string(key) != "kms-secret-key" {
		t.Fatalf("GetSecretKey() returned %q, want %q", key, "kms-secret-key")
	}
}

func TestKMSSecretKeyProvider_NonZeroExit(t *testing.T) {
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", `echo 'kms unavailable' >&2; exit 3`}, time.Second)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	_, err = provider.GetSecretKey()
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "kms unavailable") {
		t.Fatalf("error %q does not contain stderr output", err)
	}
}

func TestKMSSecretKeyProvider_EmptyOutput(t *testing.T) {
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", "true"}, time.Second)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	_, err = provider.GetSecretKey()
	if err == nil {
		t.Fatal("expected error for empty output, got nil")
	}
}

func TestKMSSecretKeyProvider_CachesResult(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	script := "printf x >> " + counter + "; printf 'kms-secret-key'"
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", script}, time.Second)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	first, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("first GetSecretKey() returned error: %v", err)
	}
	second, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("second GetSecretKey() returned error: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("cached key mismatch: first %q, second %q", first, second)
	}

	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read counter failed: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("command executed %d times, want 1", len(data))
	}
}

func TestKMSSecretKeyProvider_Timeout(t *testing.T) {
	provider, err := NewKMSSecretKeyProvider([]string{"sh", "-c", "exec sleep 2"}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewKMSSecretKeyProvider() returned error: %v", err)
	}

	_, err = provider.GetSecretKey()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error %q does not mention timeout", err)
	}
}

func TestNewKMSSecretKeyProvider_EmptyCommand(t *testing.T) {
	if _, err := NewKMSSecretKeyProvider(nil, time.Second); err == nil {
		t.Fatal("expected error for nil command, got nil")
	}
	if _, err := NewKMSSecretKeyProvider([]string{}, time.Second); err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

// ---------------------------------------------------------------------------
// NewSecretKeyProvider 工厂函数测试
// ---------------------------------------------------------------------------

func TestNewSecretKeyProvider_Config(t *testing.T) {
	provider, err := NewSecretKeyProvider(Config{Source: CryptoSourceConfig, ParentSecretKey: "test-key"})
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
	provider, err := NewSecretKeyProvider(Config{
		Source: CryptoSourceKms,
		KMS: KMSConfig{
			Command: []string{"sh", "-c", `printf 'kms-secret-key'`},
			Timeout: time.Second,
		},
	})
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

	key, err := provider.GetSecretKey()
	if err != nil {
		t.Fatalf("GetSecretKey() returned error: %v", err)
	}
	if string(key) != "kms-secret-key" {
		t.Fatalf("GetSecretKey() returned %q, want %q", key, "kms-secret-key")
	}
}

func TestNewSecretKeyProvider_KMSWithoutCommand(t *testing.T) {
	_, err := NewSecretKeyProvider(Config{Source: CryptoSourceKms})
	if err == nil {
		t.Fatal("expected error for kms source without command, got nil")
	}
}

func TestNewSecretKeyProvider_InvalidSource(t *testing.T) {
	_, err := NewSecretKeyProvider(Config{Source: "invalid-source", ParentSecretKey: "key"})
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestNewSecretKeyProvider_EmptySource(t *testing.T) {
	_, err := NewSecretKeyProvider(Config{Source: "", ParentSecretKey: "key"})
	if err == nil {
		t.Fatal("expected error for empty source, got nil")
	}
}

// ---------------------------------------------------------------------------
// 常量测试
// ---------------------------------------------------------------------------

func TestCryptoSourceConstants(t *testing.T) {
	if CryptoSourceConfig != "config" {
		t.Fatalf("CryptoSourceConfig = %q, want %q", CryptoSourceConfig, "config")
	}
	if CryptoSourceKms != "kms" {
		t.Fatalf("CryptoSourceKms = %q, want %q", CryptoSourceKms, "kms")
	}
}

// ---------------------------------------------------------------------------
// 集成测试：完整的密钥提供者使用流程
// ---------------------------------------------------------------------------

func TestSecretKeyProvider_Integration(t *testing.T) {
	testCases := []struct {
		name      string
		cfg       Config
		newErr    bool // NewSecretKeyProvider 返回错误
		getKeyErr bool // GetSecretKey 返回错误
	}{
		{
			name: "valid config source",
			cfg:  Config{Source: CryptoSourceConfig, ParentSecretKey: "my-secret-key"},
		},
		{
			name:      "empty config key",
			cfg:       Config{Source: CryptoSourceConfig},
			getKeyErr: true,
		},
		{
			name:   "kms source without command",
			cfg:    Config{Source: CryptoSourceKms},
			newErr: true,
		},
		{
			name: "kms source with command",
			cfg: Config{
				Source: CryptoSourceKms,
				KMS: KMSConfig{
					Command: []string{"sh", "-c", `printf 'kms-secret-key'`},
					Timeout: time.Second,
				},
			},
		},
		{
			name:   "invalid source",
			cfg:    Config{Source: "invalid", ParentSecretKey: "key"},
			newErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewSecretKeyProvider(tc.cfg)
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
