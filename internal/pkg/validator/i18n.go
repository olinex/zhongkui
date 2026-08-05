package validator

import (
	"fmt"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// translationRegister 注册验证标签的国际化翻译
//
// 该函数负责将验证标签与国际化消息绑定，使得验证错误消息可以根据用户的语言偏好进行本地化。
// 它使用 go-i18n 包来管理多语言消息，并使用 go-playground/validator 的翻译系统。
//
// 参数：
//   - trans: universal-translator 的翻译器实例，用于管理特定语言的翻译
//   - bundle: i18n 消息包，包含所有语言的翻译消息
//   - tag: 验证标签名称，如 "strongPassword"
//   - lang: 目标语言代码，如 "zh-CN"、"en-US"
//
// 返回值：
//   - error: 如果注册过程中出现错误则返回错误，否则返回 nil
//
// 工作流程：
// 1. 获取全局验证器实例
// 2. 创建对应语言的本地化器
// 3. 注册翻译，包括翻译文本和错误格式化函数
// 4. 将验证标签与国际化消息ID关联（格式为 "validator.{tag}"）
//
// 注意事项：
// - 消息ID必须与i18n消息文件中的键名一致
// - 如果本地化消息不存在，MustLocalizeMessage会触发panic
// - 翻译器会缓存翻译结果以提高性能
func translationRegister(trans ut.Translator, bundle *i18n.Bundle, tag, lang string) error {
	// 获取全局验证器实例
	validate := mustValidate()

	// 创建对应语言的本地化器
	localizer := i18n.NewLocalizer(bundle, lang)

	// 注册翻译到验证器
	return validate.RegisterTranslation(tag, trans,
		// 注册翻译文本
		func(trans ut.Translator) error {
			// 从i18n消息包中获取本地化消息
			// 消息ID格式为 "validator.{tag}"，例如 "validator.strongPassword"
			return trans.Add(tag, localizer.MustLocalizeMessage(&i18n.Message{
				ID: fmt.Sprintf("validator.%s", tag),
			}), true)
		},
		// 错误格式化函数
		func(trans ut.Translator, fErr validator.FieldError) string {
			// 获取翻译后的错误消息
			// 参数为验证标签和字段名
			t, _ := trans.T(tag, fErr.Field())
			return t
		})
}

// TranslationRegister 注册强密码验证标签的国际化翻译
//
// 这是 translationRegister 函数的包装器，专门用于注册强密码验证标签的翻译。
// 它简化了调用过程，用户只需提供翻译器、消息包和语言代码即可。
//
// 参数：
//   - trans: universal-translator 的翻译器实例
//   - bundle: i18n 消息包
//   - lang: 目标语言代码
//
// 返回值：
//   - error: 如果注册过程中出现错误则返回错误，否则返回 nil
//
// 使用示例：
//
//	// 初始化翻译器
//	zh := ut.New("zh", "zh-CN").GetFallback()
//
//	// 加载i18n消息包
//	bundle := i18n.NewBundle(language.Chinese)
//	bundle.LoadMessageFile("active.zh-CN.json")
//
//	// 注册翻译
//	err := validator.TranslationRegister(zh, bundle, "zh-CN")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// 消息文件示例 (active.zh-CN.json):
//
//	{
//	  "validator.strongPassword": "密码必须包含大小写字母、数字和特殊字符"
//	}
func TranslationRegister(trans ut.Translator, bundle *i18n.Bundle, lang string) error {
	return translationRegister(trans, bundle, TagStrongPassword, lang)
}
