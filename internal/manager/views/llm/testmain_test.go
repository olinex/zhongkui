package llm

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin/binding"
	uv "github.com/go-playground/validator/v10"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// TestMain 初始化全局 KeyDerivation（crypto serializer 落库/读取所需）与 Gin binding 自定义验证规则。
// 本测试包自包含——基座 views 包的 TestMain 不被子包继承，故各子包各自注册。本包 In 结构体使用了
// trimmed / enum=mcpTransport / httpUrl / jsonObject 等自定义 tag，且 LlmModel.APIKey 与
// McpServer.Headers 为加密列，缺此初始化会让相关用例以「未注册规则」或密文异常的形式失败。
func TestMain(m *testing.M) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	if v, ok := binding.Validator.Engine().(*uv.Validate); ok {
		for tag, fn := range validator.Funcs() {
			_ = v.RegisterValidation(tag, fn)
		}
	}

	os.Exit(m.Run())
}
