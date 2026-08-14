package manager

import (
	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// LocalizerCtxName 是 go-i18n Localizer 在 gin.Context 中存储的键名
// 通过此键名从请求上下文中获取 Localizer 实例，用于本地化消息翻译
const LocalizerCtxName = "i18n-localizer"

// TranslatorCtxName 是 universal-translator Translator 在 gin.Context 中存储的键名
// 通过此键名从请求上下文中获取 Translator 实例，用于验证器错误消息翻译
const TranslatorCtxName = "i18n-translator"

// TranslateMiddleware 创建国际化中间件，为每个 HTTP 请求初始化本地化组件
//
// 该中间件在每个请求到达时执行以下操作：
//  1. 从请求的 Accept-Language 头部解析出客户端偏好的语言
//  2. 基于解析到的语言创建 go-i18n 的 Localizer 实例，用于本地化消息翻译
//  3. 获取 universal-translator 的 Translator 实例，用于验证器错误消息翻译
//  4. 将 Localizer 和 Translator 存入 gin.Context，供后续处理器使用
//
// 注意：如果获取 Translator 失败（例如请求的语言不在支持列表中），
// 中间件会记录警告日志但不会中断请求，后续调用 GetTranslator 将返回 nil
//
// 参数:
//   - i: 已初始化的 I18n 实例，包含 Bundle 和 UniversalTranslator
//
// 返回值:
//   - gin.HandlerFunc: 可直接注册到 Gin 路由的中间件函数
//
// 使用示例:
//
//	r := gin.New()
//	r.Use(TranslateMiddleware(i18nInstance))
func TranslateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := GetLanguage(c)
		localizer := translate.Localizer(lang)
		translator := validator.MustGetTranslator(lang)
		c.Set(LocalizerCtxName, localizer)
		c.Set(TranslatorCtxName, translator)
		c.Next()
	}
}

// GetLanguage 从 gin.Context 中提取客户端的语言偏好
//
// 该函数从 HTTP 请求的 Accept-Language 头部中解析语言信息：
//   - 如果头部存在且格式有效，返回对应的 Language 实例
//   - 如果头部不存在或格式无效，返回默认语言（英语）
//
// 参数:
//   - c: gin 上下文，用于获取 HTTP 请求头信息
//
// 返回值:
//   - *translate.Language: 解析得到的语言实例，保证不为 nil
//
// 使用示例:
//
//	lang := GetLanguage(c)
//	fmt.Println(lang.String()) // 输出: "zh-Hans" 或 "en"
func GetLanguage(c *gin.Context) *base.Language {
	acceptLanguage := c.GetHeader(base.HdrAcceptLanguage)
	if lang, err := base.ParseLanguage(acceptLanguage); err == nil {
		return lang
	} else {
		return base.DefaultLanguage
	}
}

// GetLocalizer 从 gin.Context 中获取 go-i18n 的 Localizer 实例
//
// 该函数从请求上下文中获取由 TranslateMiddleware 设置的 Localizer，
// 用于根据当前请求的语言偏好进行消息翻译。
//
// 注意：该函数必须在 TranslateMiddleware 之后调用，
// 否则会 panic，因为上下文中不存在 Localizer
//
// 参数:
//   - c: gin 上下文，用于获取存储的 Localizer
//
// 返回值:
//   - *i18n.Localizer: 当前请求的本地化器实例
//
// 异常:
//   - 如果上下文中不存在 Localizer，触发 panic
//
// 使用示例:
//
//	localizer := GetLocalizer(c)
//	msg, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: "welcome"})
func GetLocalizer(c *gin.Context) *i18n.Localizer {
	if localizer, ok := c.Get(LocalizerCtxName); ok {
		return localizer.(*i18n.Localizer)
	}
	panic("i18n localizer does not exists")
}

// GetTranslator 从 gin.Context 中获取 universal-translator 的 Translator 实例
//
// 该函数从请求上下文中获取由 TranslateMiddleware 设置的 Translator，
// 用于将验证器（validator）的错误消息翻译为当前请求的语言。
//
// 注意：该函数必须在 TranslateMiddleware 之后调用，
// 否则会 panic，因为上下文中不存在 Translator
//
// 参数:
//   - c: gin 上下文，用于获取存储的 Translator
//
// 返回值:
//   - ut.Translator: 当前请求的翻译器实例
//
// 异常:
//   - 如果上下文中不存在 Translator，触发 panic
//
// 使用示例:
//
//	translator := GetTranslator(c)
//	err := validator.Struct(data)
//	errs := err.(validator.ValidationErrors)
//	msg := errs[0].Translate(translator)
func GetTranslator(c *gin.Context) ut.Translator {
	if translator, ok := c.Get(TranslatorCtxName); ok {
		return translator.(ut.Translator)
	}
	panic("i18n translator does not exists")
}
