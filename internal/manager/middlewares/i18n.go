package middlewares

import (
	// third-party
	"github.com/gin-gonic/gin"

	// internal

	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

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
//   - 无（Localizer/Translator 键名定义于 response 包，经 response.LocalizerCtxName 等读取）
//
// 返回值:
//   - gin.HandlerFunc: 可直接注册到 Gin 路由的中间件函数
//
// 使用示例:
//
//	r := gin.New()
//	r.Use(TranslateMiddleware())
func TranslateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := GetLanguage(c)
		localizer := translate.Localizer(lang)
		translator := validator.MustGetTranslator(lang)
		c.Set(response.LocalizerCtxName, localizer)
		c.Set(response.TranslatorCtxName, translator)
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
//   - *base.Language: 解析得到的语言实例，保证不为 nil
//
// 使用示例:
//
//	lang := GetLanguage(c)
//	fmt.Println(lang.String()) // 输出: "zh-Hans" 或 "en"
func GetLanguage(c *gin.Context) *base.Language {
	acceptLanguage := c.GetHeader(base.HdrAcceptLanguage)
	// 将请求语言归一化到受支持语言集合（如 "zh-CN" → "zh-Hans"），
	// 避免未注册语言导致 MustGetTranslator 查找失败而 panic
	if lang, err := base.ParseLanguage(acceptLanguage); err == nil {
		return lang.Normalize()
	} else {
		return base.DefaultLanguage
	}
}
