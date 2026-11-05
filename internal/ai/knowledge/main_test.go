package knowledge

import (
	// 标准库
	"os"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
)

// TestMain 初始化知识库测试的前置条件：生成文档的派生摘要与落库摘要共用 hash 字段的 HMAC 密钥，
// 纯逻辑测试也要能计算摘要，故先初始化全局密钥派生。
func TestMain(m *testing.M) {
	aitest.InitKeys()
	os.Exit(m.Run())
}
