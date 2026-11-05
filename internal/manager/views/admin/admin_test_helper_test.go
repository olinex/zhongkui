package admin

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"

	"zhongkuisec.com/zhongkui/internal/manager/response"
)

// frontendConfigHash 计算前端配置内容的 SHA-256 十六进制摘要
// 与 user 子包的实现保持一致，供前端配置重置测试构造种子用户使用
// （user.frontendConfigHash 未导出，无法跨包引用，故在本测试包内自包含）
func frontendConfigHash(config string) string {
	sum := sha256.Sum256([]byte(config))
	return hex.EncodeToString(sum[:])
}

// newAdminTestLocalizer 返回测试专用的 go-i18n Localizer
// 内置本包测试所需的最小错误消息键，供错误响应渲染使用（缺失时回退为 key 本身）
func newAdminTestLocalizer() *i18n.Localizer {
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
code.1200 = "too many access key"
code.1201 = "tag event rule invalid"
code.1202 = "tag rule runtime invalid"
code.1203 = "tag rule target model not registered"
code.1400 = "frontend config conflict"
code.1401 = "frontend config sync disabled"
code.1204 = "tag rule sample input invalid"
code.1402 = "notification send failed"
code.1403 = "scheduler task config invalid"
code.1104 = "password reset unavailable"
code.1105 = "password reset token invalid"
code.200 = "ok"
code.404 = "not found"
error.emptyUrlParam = "empty url param: %s"
error.badUrlParam = "bad url param: %s"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newAdminTestTranslator 返回测试专用的 universal-translator
func newAdminTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回一个注入 Localizer 和 Translator 的 Gin 中间件
// 该辅助函数在本测试包内自包含，供各路由测试的请求绑定与错误响应渲染使用
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newAdminTestLocalizer())
		c.Set(response.TranslatorCtxName, newAdminTestTranslator())
		c.Next()
	}
}
