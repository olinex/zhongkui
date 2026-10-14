package ai_test

import (
	// 标准库
	"os"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
)

// TestMain 初始化 ai 包测试的前置条件：生成文档的派生摘要与落库摘要共用 hash 字段的 HMAC 密钥，
// 纯逻辑测试也要能计算摘要，故先初始化全局密钥派生。
func TestMain(m *testing.M) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	os.Exit(m.Run())
}
