package response

import (
	// 第三方
	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// LocalizerCtxName 是 go-i18n Localizer 在 gin.Context 中存储的键名
// 通过此键名从请求上下文中获取 Localizer 实例，用于本地化消息翻译
const LocalizerCtxName = "i18n-localizer"

// TranslatorCtxName 是 universal-translator Translator 在 gin.Context 中存储的键名
// 通过此键名从请求上下文中获取 Translator 实例，用于验证器错误消息翻译
const TranslatorCtxName = "i18n-translator"

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
