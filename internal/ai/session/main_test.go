package session

import (
	// 标准库
	"os"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
)

// TestMain 初始化会话包测试的前置条件：消息正文与摘要使用字段级加密与 HMAC 摘要，
// 故先初始化全局密钥派生。
func TestMain(m *testing.M) {
	aitest.InitKeys()
	os.Exit(m.Run())
}

// testUserID 是会话测试使用的属主用户 ID。
const testUserID uint = 1

// testOtherUserID 是跨用户隔离用例的另一个属主用户 ID。
const testOtherUserID uint = 2
