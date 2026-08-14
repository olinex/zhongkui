package translate

import (
	"embed"
	"fmt"

	"github.com/nicksnyder/go-i18n/v2/i18n" // go-i18n 国际化库
	"github.com/pelletier/go-toml/v2"       // TOML 解析库
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// embeddedLocales 嵌入的本地化文件系统
// 包含内置的本地化文件（如 en.toml, zh-Hans.toml）
//
//go:embed locales/*
var embeddedLocales embed.FS

var (
	Bundle *i18n.Bundle
)

// NewBundle 创建并初始化 go-i18n 的消息包
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
//	└── zh-Hans.toml # 简体中文翻译文件
func newBundle() *i18n.Bundle {
	// 创建新的消息包，使用默认语言作为基础语言
	// 基础语言决定了消息包的默认语言和回退机制
	// ZKI_LANG 未设置或非法时回退到简体中文
	lang := base.DefaultLanguage
	bundle := i18n.NewBundle(lang.StdTag())

	// 注册 TOML 格式的解析函数
	// TOML 格式比 JSON 更易读，适合翻译文件
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// 遍历所有支持的语言，加载对应的翻译文件
	for _, lang := range base.SupportLanguages {
		// 获取语言的标准标签和字符串表示
		tag := lang.StdTag()
		lang := tag.String()

		// 构建文件路径
		customPath := fmt.Sprintf("%s/%s.toml", base.Env.LocaleDirPath, lang) // 自定义文件路径
		buildinPath := fmt.Sprintf("locales/%s.toml", lang)                   // 内置文件路径

		// 优先尝试加载自定义本地化文件
		// 自定义文件允许用户覆盖内置翻译
		if _, err := bundle.LoadMessageFile(customPath); err == nil {
			continue
		} else if _, err := bundle.LoadMessageFileFS(embeddedLocales, buildinPath); err == nil {
			// 自定义文件不存在时回退到内置文件
			continue
		} else {
			// 如果两种文件都不存在，则抛出异常
			// 这通常表示配置错误或缺少必要的翻译文件
			panic(err)
		}
	}
	return bundle
}

func init() {
	Bundle = newBundle()
}
