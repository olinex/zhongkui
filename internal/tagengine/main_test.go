package tagengine

import (
	"os"
	"testing"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
)

// TestMain 初始化全局密钥派生器，使 BuildInput 的敏感字段加密与
// crypto.reveal 解密在全部 tagengine 测试中可用。
// 该包内、外部测试共享同一测试二进制，故在此统一初始化一次。
func TestMain(m *testing.M) {
	passwordBook, err := key.LoadPasswordBook("")
	if err != nil {
		panic("load password book: " + err.Error())
	}
	key.InitGlobalKeyDerivation(key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!"), passwordBook)
	os.Exit(m.Run())
}
