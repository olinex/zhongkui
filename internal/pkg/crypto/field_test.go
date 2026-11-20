package crypto

import (
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/mode"
)

// dispatchTestMode 测试用加密模式：仅原样返回数据，用于验证 ModeForTag 经注册表分发。
type dispatchTestMode struct{}

// Encrypt 原样返回输入，满足 mode.Mode 接口。
func (dispatchTestMode) Encrypt(p []byte) ([]byte, error) { return p, nil }

// Decrypt 原样返回输入，满足 mode.Mode 接口。
func (dispatchTestMode) Decrypt(p []byte) ([]byte, error) { return p, nil }

// TestModeForTag_RegistryDispatch 验证 ModeForTag 按标签 mode 从注册表构造加密模式：
// 注册新模式的实现可被直接解析，未注册的模式返回明确错误。
func TestModeForTag_RegistryDispatch(t *testing.T) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		t.Fatalf("load password book: %v", err)
	}
	key.InitGlobalKeyDerivation(key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!"), pb)

	mode.Register("test-dispatch", func(cryptoKey, additionalData []byte) (mode.Mode, error) {
		return dispatchTestMode{}, nil
	})

	m, err := ModeForTag(CryptoTag{Mode: "test-dispatch", Offset: 0, Length: 32})
	if err != nil {
		t.Fatalf("ModeForTag(test-dispatch) failed: %v", err)
	}
	if _, ok := m.(dispatchTestMode); !ok {
		t.Fatalf("unexpected mode type: %T", m)
	}

	if _, err = ModeForTag(CryptoTag{Mode: "not-registered", Offset: 0, Length: 32}); err == nil {
		t.Fatal("expected error for unregistered mode, got nil")
	} else if !strings.Contains(err.Error(), "unknown crypto mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
