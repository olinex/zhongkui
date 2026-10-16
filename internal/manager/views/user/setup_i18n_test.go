package user

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
	"golang.org/x/text/language"

	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
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

	v, ok := binding.Validator.Engine().(*uv.Validate)
	if ok {
		for tag, fn := range validator.Funcs() {
			_ = v.RegisterValidation(tag, fn)
		}
	}

	os.Exit(m.Run())
}

// newUserTestLocalizer 返回测试专用的 go-i18n Localizer
// 内置本包测试所需的最小错误消息键，供错误响应渲染使用（缺失时回退为 key 本身）
// （基座 views 包的 newBaseTestLocalizer 未导出，无法跨包引用，故在本测试包内自包含）
func newUserTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.1000 = "unknown error"
code.1001 = "user not exist"
code.1002 = "bad body"
code.1003 = "bad query"
code.1004 = "bad url param"
code.1005 = "bad jwt token"
code.1006 = "timeout jwt token"
code.1007 = "authorize blank full path"
code.1008 = "authorize failed"
code.1009 = "namespace not exist"
code.1010 = "cannot delete cause relate record"
code.1100 = "username or password check failed"
code.1101 = "old password check failed"
code.1102 = "username or password check failed"
code.1103 = "username or email already exist"
code.1104 = "too many access key"
code.1105 = "tag event rule invalid"
code.1106 = "tag rule runtime invalid"
code.1107 = "tag rule target model not registered"
code.1108 = "frontend config conflict"
code.1109 = "frontend config sync disabled"
code.1110 = "tag rule sample input invalid"
code.1111 = "notification send failed"
code.1112 = "scheduler task config invalid"
code.1113 = "password reset unavailable"
code.1114 = "password reset token invalid"
code.1123 = "too many requests"
code.200 = "ok"
code.404 = "not found"
error.emptyUrlParam = "empty url param: %s"
error.badUrlParam = "bad url param: %s"
error.schedulerScheduleConflict = "cronSpec and interval are mutually exclusive, set only one"
error.schedulerScheduleEmpty = "either cronSpec or interval must be set"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newUserTestTranslator 返回测试专用的 universal-translator
func newUserTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回一个注入 Localizer 和 Translator 的 Gin 中间件
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newUserTestLocalizer())
		c.Set(response.TranslatorCtxName, newUserTestTranslator())
		c.Next()
	}
}
