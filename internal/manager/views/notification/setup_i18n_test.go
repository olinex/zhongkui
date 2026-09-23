package notification

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	uv "github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
	"golang.org/x/text/language"

	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/logging"
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

// newTagRuleTestLogger 创建测试用日志记录器
// （原定义于 tag 子系统测试，本测试包需自包含）
func newTagRuleTestLogger() *logging.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	zapLogger, _ := config.Build()
	return &logging.Logger{SugaredLogger: zapLogger.Sugar()}
}

// newNotificationTestLocalizer 返回测试专用的 go-i18n Localizer
// 内置本包测试所需的最小错误消息键，供错误响应渲染使用（缺失时回退为 key 本身）
// （基座 views 包的 newBaseTestLocalizer 未导出，无法跨包引用，故在本测试包内自包含）
func newNotificationTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.1000 = "unknown error"
code.1002 = "bad body"
code.1003 = "bad query"
code.1004 = "bad url param"
code.1009 = "namespace not exist"
code.1010 = "cannot delete cause relate record"
code.1111 = "notification send failed"
code.200 = "ok"
code.404 = "not found"
error.emptyUrlParam = "empty url param: %s"
error.badUrlParam = "bad url param: %s"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newNotificationTestTranslator 返回测试专用的 universal-translator
func newNotificationTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回一个注入 Localizer 和 Translator 的 Gin 中间件
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newNotificationTestLocalizer())
		c.Set(response.TranslatorCtxName, newNotificationTestTranslator())
		c.Next()
	}
}
