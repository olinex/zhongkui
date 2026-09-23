package admin

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin/binding"
	uv "github.com/go-playground/validator/v10"

	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// TestMain 初始化全局 KeyDerivation 与 Gin binding 自定义验证规则。
// 原由基座 views 包的 TestMain 完成（package 拆分后本测试包需自包含）。
func TestMain(m *testing.M) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	if v, ok := binding.Validator.Engine().(*uv.Validate); ok {
		for tag, fn := range map[string]uv.Func{
			validator.TagStrongPassword:         validator.ValidateStrongPassword,
			validator.TagWindowsFileName:        validator.ValidateWindowsFileName,
			validator.TagUnixFileName:           validator.ValidateUnixFileName,
			validator.TagFileName:               validator.ValidateFileName,
			validator.TagStrictFileName:         validator.ValidateStrictFileName,
			validator.TagDirpath:                validator.ValidateDirPath,
			validator.TagAlphanumSpecial:        validator.ValidateAlphanumSpecial,
			validator.TagGreaterThanCurrentTime: validator.ValidateGtCurrentTime,
			validator.TagLesserThanCurrentTime:  validator.ValidateLtCurrentTime,
		} {
			_ = v.RegisterValidation(tag, fn)
		}
	}

	os.Exit(m.Run())
}