package memory

import (
	// 标准库
	"os"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/config"
)

// TestMain 初始化记忆包测试的前置条件：正文摘要使用 HMAC 密钥，故先初始化全局密钥派生。
func TestMain(m *testing.M) {
	aitest.InitKeys()
	os.Exit(m.Run())
}

// testParams 构造测试用参数读取器（实现见 aitest.Params）。
func testParams(t *testing.T, overrides map[string]string) *config.Params {
	t.Helper()
	return aitest.Params(t, overrides)
}
