package translate

import (
	"embed"
	"fmt"

	en "github.com/go-playground/locales/en"                                 // 英语本地化器
	zhHans "github.com/go-playground/locales/zh_Hans"                        // 简体中文本地化器
	ut "github.com/go-playground/universal-translator"                       // 通用翻译器
	en_translations "github.com/go-playground/validator/v10/translations/en" // 英语验证器翻译
	zh_translations "github.com/go-playground/validator/v10/translations/zh" // 中文验证器翻译
	"github.com/nicksnyder/go-i18n/v2/i18n"                                  // go-i18n 国际化库
	"github.com/pelletier/go-toml/v2"                                        // TOML 解析库
	"golang.org/x/text/language"                                             // 语言标签处理
	"zhongkui.com/zhongkui/internal/pkg/errors"                              // 错误处理
	"zhongkui.com/zhongkui/internal/pkg/logging"                             // 日志记录
	"zhongkui.com/zhongkui/internal/pkg/validator"                           // 验证器
)

// embeddedLocales 嵌入的本地化文件系统
// 包含内置的本地化文件（如 en.toml, zh-Hans.toml）
//
//go:embed locales/*
var embeddedLocales embed.FS

// I18n 结构体封装了国际化相关的核心组件
//
// I18n 提供了完整的国际化解决方案，包括：
// 1. 本地化消息管理（通过 go-i18n 的 Bundle）
// 2. 验证器错误消息翻译（通过 universal-translator）
// 3. 多语言支持配置
//
// 使用示例：
//
//	cfg := &translate.Config{
//	    DefaultLocale:   translate.MustParseLanguage("zh-Hans"),
//	    SupportLocales:  []translate.Language{translate.MustParseLanguage("zh-Hans"), translate.MustParseLanguage("en")},
//	    RootPath:        "locales",
//	}
//	logger := logging.NewLogger()
//	i18n := translate.InitI18n(cfg, logger)
//
// 主要功能：
//   - 加载和管理多语言资源文件
//   - 提供验证器错误消息的本地化
//   - 支持动态语言切换
//   - 提供统一的翻译接口
type I18n struct {
	Bundle *i18n.Bundle            // 本地化消息包，管理所有语言的翻译资源
	Uni    *ut.UniversalTranslator // 通用翻译器，提供验证器错误消息的翻译
}

// newBundle 创建并初始化 go-i18n 的消息包
//
// 该函数负责：
// 1. 创建基于默认语言的消息包
// 2. 注册 TOML 格式的解析函数
// 3. 加载所有支持语言的本地化文件
//
// 文件加载策略（优先级从高到低）：
// 1. 自定义文件：从 cfg.RootPath 目录加载
// 2. 内置文件：从嵌入的 locales 目录加载
// 3. 如果两种文件都不存在，则 panic
//
// 参数:
//   - cfg: 国际化配置，包含默认语言、支持语言列表和文件根路径
//   - logger: 日志记录器，用于记录加载过程
//
// 返回值:
//   - *i18n.Bundle: 初始化完成的消息包，包含所有加载的翻译资源
//
// 异常:
//   - 如果任何支持语言的本地化文件都无法加载，函数会 panic
//
// 示例文件结构:
//
//	locales/
//	├── en.toml      # 英语翻译文件
//	├── zh-Hans.toml # 简体中文翻译文件
//	└── ja.toml      # 日语翻译文件
func newBundle(cfg *Config, logger *logging.Logger) *i18n.Bundle {
	// 创建新的消息包，使用默认语言作为基础语言
	// 基础语言决定了消息包的默认语言和回退机制
	bundle := i18n.NewBundle(cfg.DefaultLocale.StdTag())

	// 注册 TOML 格式的解析函数
	// TOML 格式比 JSON 更易读，适合翻译文件
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// 遍历所有支持的语言，加载对应的翻译文件
	for _, locale := range cfg.SupportLocales {
		// 获取语言的标准标签和字符串表示
		tag := locale.StdTag()
		lang := tag.String()

		// 构建文件路径
		customPath := fmt.Sprintf("%s/%s.toml", cfg.RootPath, lang) // 自定义文件路径
		buildinPath := fmt.Sprintf("locales/%s.toml", lang)         // 内置文件路径

		// 优先尝试加载自定义本地化文件
		// 自定义文件允许用户覆盖内置翻译
		if _, err := bundle.LoadMessageFile(customPath); err == nil {
			logger.Infof("load custom locale %s from %s", lang, customPath)
			continue
		} else if _, err := bundle.LoadMessageFileFS(embeddedLocales, buildinPath); err == nil {
			// 如果自定义文件不存在，尝试加载内置文件
			// 内置文件提供默认翻译
			logger.Infof("load buildin locale %s from %s", lang, buildinPath)
			continue
		} else {
			// 如果两种文件都不存在，则抛出异常
			// 这通常表示配置错误或缺少必要的翻译文件
			panic(err)
		}
	}
	return bundle
}

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
func newUniversalTranslator(cfg *Config, bundle *i18n.Bundle, logger *logging.Logger) *ut.UniversalTranslator {
	// 获取全局验证器实例
	// 验证器是单例模式，确保所有翻译使用同一个验证器实例
	var uni *ut.UniversalTranslator

	// 创建英语和简体中文的本地化器
	// 这些本地化器用于创建通用翻译器
	enT, zhHansT := en.New(), zhHans.New()

	// 根据默认语言创建通用翻译器
	// 通用翻译器的参数：主要语言、备用语言、支持的语言列表...
	switch cfg.DefaultLocale.StdTag() {
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
	for _, locale := range cfg.SupportLocales {
		// 获取对应语言的翻译器
		// 翻译器用于将验证器错误消息转换为目标语言
		trans := MustGetTranslator(uni, &locale)
		tag := locale.StdTag()
		lang := tag.String()

		// 根据语言类型注册相应的验证器翻译
		// 目前仅支持英语和简体中文的验证器翻译
		switch tag {
		case language.SimplifiedChinese:
			logger.Infof("register zh validator translation for %s", lang)
			// 注册中文验证器翻译（默认规则）
			err := zh_translations.RegisterDefaultTranslations(validator.Validate, trans)
			if err != nil {
				panic(err)
			}
			// 注册自定义验证器翻译（用户定义规则）
			err = validator.TranslationRegister(trans, bundle, lang)
			if err != nil {
				panic(err)
			}
		case language.English:
			logger.Infof("register en validator translation for %s", lang)
			// 注册英文验证器翻译（默认规则）
			err := en_translations.RegisterDefaultTranslations(validator.Validate, trans)
			if err != nil {
				panic(err)
			}
			// 注册自定义验证器翻译（用户定义规则）
			err = validator.TranslationRegister(trans, bundle, lang)
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
//   - uni: 通用翻译器实例，包含所有已注册语言的翻译器
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
func GetTranslator(uni *ut.UniversalTranslator, lang *Language) (ut.Translator, error) {
	// 使用下划线格式的语言标签查找翻译器
	// 通用翻译器内部使用下划线格式存储翻译器
	trans, found := uni.GetTranslator(lang.UnderscoreLocale())
	if !found {
		// 返回详细的错误信息，帮助调试配置问题
		return trans, errors.Newf("Could not find %s in universal translator", lang.StdTag())
	}
	return trans, nil
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
func MustGetTranslator(uni *ut.UniversalTranslator, lang *Language) ut.Translator {
	trans, err := GetTranslator(uni, lang)
	if err != nil {
		// 在初始化阶段，配置错误应该立即暴露
		panic(err)
	}
	return trans
}

// InitI18n 初始化国际化系统
//
// 该函数是整个国际化模块的入口点，负责：
// 1. 初始化 go-i18n 消息包，加载所有翻译资源
// 2. 初始化通用翻译器，注册验证器错误消息翻译
// 3. 创建并返回完整的 I18n 实例
//
// 初始化流程：
// 1. 创建消息包（newBundle）：加载所有支持语言的翻译文件
// 2. 创建通用翻译器（newUniversalTranslator）：注册验证器翻译
// 3. 组装 I18n 实例：包含消息包和通用翻译器
//
// 参数:
//   - cfg: 国际化配置，必须包含有效的默认语言和支持语言列表
//   - logger: 日志记录器，用于记录初始化过程和调试信息
//
// 返回值:
//   - *I18n: 初始化完成的国际化实例，包含所有必要的组件
//
// 异常:
//   - 如果配置无效或文件加载失败，初始化过程会 panic
//   - 建议在应用程序启动时调用此函数，确保错误尽早暴露
//
// 使用示例:
//
//	func main() {
//	    // 创建配置
//	    cfg := &translate.Config{
//	        DefaultLocale:   translate.MustParseLanguage("zh-Hans"),
//	        SupportLocales:  []translate.Language{
//	            translate.MustParseLanguage("zh-Hans"),
//	            translate.MustParseLanguage("en"),
//	        },
//	        RootPath: "locales",
//	    }
//
//	    // 创建日志记录器
//	    logger := logging.NewLogger()
//
//	    // 初始化国际化系统
//	    i18n := translate.InitI18n(cfg, logger)
//
//	    // 使用国际化系统
//	    // ...
//	}
//
// 最佳实践:
//   - 在应用程序启动时尽早初始化国际化系统
//   - 使用单例模式或依赖注入管理 I18n 实例
//   - 确保所有支持语言的翻译文件都存在
//   - 在生产环境中监控初始化错误
func InitI18n(cfg *Config, logger *logging.Logger) *I18n {
	// 创建消息包：加载所有翻译资源文件
	// 消息包管理应用程序的所有本地化消息
	bundle := newBundle(cfg, logger)

	// 创建通用翻译器：注册验证器错误消息翻译
	// 通用翻译器处理验证器规则的本地化
	uni := newUniversalTranslator(cfg, bundle, logger)

	// 创建并返回 I18n 实例
	// I18n 实例是国际化系统的核心，提供统一的接口
	i := &I18n{
		Bundle: bundle,
		Uni:    uni,
	}

	// 记录初始化完成信息
	logger.Infof("i18n system initialized with default locale: %s", cfg.DefaultLocale.String())

	return i
}
