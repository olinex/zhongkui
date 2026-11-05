package validator

import (
	"fmt"

	en "github.com/go-playground/locales/en"           // 英语本地化器
	zhHans "github.com/go-playground/locales/zh_Hans"  // 简体中文本地化器
	ut "github.com/go-playground/universal-translator" // 通用翻译器
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en" // 英语验证器翻译
	zh_translations "github.com/go-playground/validator/v10/translations/zh" // 中文验证器翻译
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// UniversalTranslator 全局通用翻译器实例。
var (
	UniversalTranslator *ut.UniversalTranslator
)

// newUniversalTranslator 创建并初始化通用翻译器
//
// 该函数负责：
// 1. 创建通用翻译器实例，根据默认语言设置主要语言
// 2. 为每个支持的语言注册验证器错误消息的翻译
// 3. 集成自定义验证器翻译规则
//
// 翻译注册流程：
// 1. 获取全局验证器实例
// 2. 为每种语言获取对应的翻译器
// 3. 注册默认验证器翻译（如 required、email、min 等规则）
// 4. 注册自定义验证器翻译（从 go-i18n 消息包加载）
//
// 参数:
//   - cfg: 国际化配置，包含默认语言和支持语言列表
//   - bundle: go-i18n 消息包，包含自定义验证器翻译规则
//   - logger: 日志记录器，用于记录注册过程
//
// 返回值:
//   - *ut.UniversalTranslator: 初始化完成的通用翻译器，支持所有配置的语言
//
// 异常:
//   - 如果无法注册验证器翻译，函数会 panic
//
// 支持的验证器翻译类型：
//   - 默认翻译：内置的验证规则翻译（如 required、email、min 等）
//   - 自定义翻译：从 TOML 文件加载的用户自定义验证规则翻译
//
// 注意:
//   - 目前仅支持英语（en）和简体中文（zh-Hans）的验证器翻译
//   - 其他语言会跳过验证器翻译注册，但仍可用于消息翻译
func newUniversalTranslator() *ut.UniversalTranslator {
	// 获取全局验证器实例
	// 验证器是单例模式，确保所有翻译使用同一个验证器实例
	var uni *ut.UniversalTranslator

	// 创建英语和简体中文的本地化器
	// 这些本地化器用于创建通用翻译器
	enT, zhHansT := en.New(), zhHans.New()

	// 根据默认语言创建通用翻译器
	// 通用翻译器的参数：主要语言、备用语言、支持的语言列表...
	switch base.DefaultLanguage.StdTag() {
	case language.SimplifiedChinese:
		// 如果默认语言是简体中文，将中文作为主要语言
		// 参数：主要语言、备用语言、支持的语言...
		uni = ut.New(zhHansT, enT, zhHansT)
	case language.English:
		fallthrough
	default:
		// 默认情况下，将英语作为主要语言
		// 参数：主要语言、备用语言、支持的语言...
		uni = ut.New(enT, enT, zhHansT)
	}

	// 为每个支持的语言注册验证器翻译
	// 这确保验证器错误消息能够正确本地化
	for _, locale := range base.SupportLanguages {
		// 获取对应语言的翻译器
		// 翻译器用于将验证器错误消息转换为目标语言
		trans := mustGetTranslator(uni, &locale)
		tag := locale.StdTag()

		// 根据语言类型注册相应的验证器翻译
		// 目前仅支持英语和简体中文的验证器翻译
		switch tag {
		case language.SimplifiedChinese:
			// 注册中文验证器翻译（默认规则）
			err := zh_translations.RegisterDefaultTranslations(Validate, trans)
			if err != nil {
				panic(err)
			}
		case language.English:
			// 注册英文验证器翻译（默认规则）
			err := en_translations.RegisterDefaultTranslations(Validate, trans)
			if err != nil {
				panic(err)
			}
		default:
			// 对于其他语言，暂时跳过验证器翻译注册
			// 这些语言仍可用于消息翻译，但验证器错误消息可能为英文
			continue
		}
	}
	return uni
}

// GetTranslator 从通用翻译器中获取指定语言的翻译器
//
// 该函数是国际化系统的核心工具函数，用于：
// 1. 根据语言获取对应的翻译器实例
// 2. 提供错误处理机制，当语言不支持时返回错误
// 3. 支持语言标签格式转换（将 "zh-Hans" 转换为 "zh_Hans"）
//
// 参数:
//   - lang: 目标语言，必须是已注册的支持语言之一
//
// 返回值:
//   - ut.Translator: 找到的翻译器，可用于验证器错误消息翻译
//   - error: 如果找不到对应语言的翻译器，返回详细的错误信息
//
// 使用场景:
//   - 在验证过程中获取当前语言的翻译器
//   - 在中间件中根据请求头设置翻译器
//   - 在测试中模拟不同语言的翻译环境
//
// 注意:
//   - 语言必须使用下划线格式（如 "zh_Hans"）进行查找
//   - 如果语言未在通用翻译器中注册，函数会返回错误
func getTranslator(uni *ut.UniversalTranslator, lang *base.Language) (ut.Translator, error) {
	// 使用下划线格式的语言标签查找翻译器
	// 通用翻译器内部使用下划线格式存储翻译器
	trans, found := uni.GetTranslator(lang.UnderscoreLocale())
	if !found {
		// 返回详细的错误信息，帮助调试配置问题
		return trans, errors.Newf("Could not find %s in universal translator", lang.StdTag())
	}
	return trans, nil
}

// GetTranslator 从通用翻译器中获取指定语言的翻译器。
func GetTranslator(lang *base.Language) ut.Translator {
	return mustGetTranslator(UniversalTranslator, lang)
}

// MustGetTranslator 从通用翻译器中获取指定语言的翻译器，如果找不到则 panic
//
// 该函数是 GetTranslator 的简化版本，适用于以下场景：
// 1. 初始化阶段，配置错误应该立即暴露
// 2. 确定语言一定存在的情况（如默认语言）
// 3. 简化错误处理逻辑，当错误不可恢复时直接 panic
//
// 参数:
//   - uni: 通用翻译器实例，包含所有已注册语言的翻译器
//   - lang: 目标语言，必须是已注册的支持语言之一
//
// 返回值:
//   - ut.Translator: 找到的翻译器，如果找不到会 panic
//
// 使用场景:
//   - 在初始化函数中获取默认语言的翻译器
//   - 在测试中确保语言配置正确
//   - 在确定语言一定存在的情况下简化代码
//
// 注意:
//   - 该函数会 panic，因此只应在错误不可恢复的情况下使用
//   - 在生产代码中，通常应该使用 GetTranslator 并处理错误
func mustGetTranslator(uni *ut.UniversalTranslator, lang *base.Language) ut.Translator {
	trans, err := getTranslator(uni, lang)
	if err != nil {
		// 在初始化阶段，配置错误应该立即暴露
		panic(err)
	}
	return trans
}

// MustGetTranslator 从通用翻译器中获取指定语言的翻译器，如果找不到则 panic
func MustGetTranslator(lang *base.Language) ut.Translator {
	return mustGetTranslator(UniversalTranslator, lang)
}

// RegisterTranslation 注册验证标签的国际化翻译
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
func RegisterTranslation(tag string, lang *base.Language) error {
	trans, err := getTranslator(UniversalTranslator, lang)
	if err != nil {
		return errors.WithStack(err)
	}
	// 注册翻译到验证器
	return Validate.RegisterTranslation(tag, trans,
		// 注册翻译文本
		func(trans ut.Translator) error {
			// 从i18n消息包中获取本地化消息
			// 消息ID格式为 "validator.{tag}"，例如 "validator.strongPassword"
			return trans.Add(
				tag,
				translate.
					Localizer(lang).
					MustLocalizeMessage(&i18n.Message{
						ID: fmt.Sprintf("validator.%s", tag),
					}),
				true,
			)
		},
		// 错误格式化函数
		func(trans ut.Translator, fErr validator.FieldError) string {
			// 获取翻译后的错误消息
			// 参数为验证标签和字段名
			t, err := trans.T(tag, fErr.Field())
			if err != nil {
				// 翻译缺失时回退到字段名，避免返回空消息掩盖校验失败原因。
				return fErr.Field()
			}
			return t
		})
}

func init() {
	UniversalTranslator = newUniversalTranslator()
}
