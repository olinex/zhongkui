package translate

import (
	"embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n" // go-i18n 国际化库
	"github.com/pelletier/go-toml/v2"       // TOML 解析库
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// embeddedLocales 嵌入的本地化文件系统
// 包含内置的本地化文件（en.toml, zh-Hans.toml）及其引用的外部模板文件
// （locales/emails/*.html 等），支持 TOML 通过 "@路径" 引用外部长文本模板。
//
//go:embed locales
var embeddedLocales embed.FS

// Bundle 全局 i18n 消息包，供 Localize 等函数经其取回各语言文案。
var Bundle *i18n.Bundle

// extRefRe 匹配 TOML 中形如 `key = "@相对路径"` 的外部文件引用行。
// 命中后会将整个行的字符串值替换为该外部文件的内容，从而避免在 TOML 中
// 直接编写/维护超长模板文本。
var extRefRe = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*=\s*"@([A-Za-z0-9_./-]+)"\s*$`)

// readFile 抽象按路径读取本地化相关文件：
//   - getFile(key)：key 为 "lang.toml" 或外部引用路径，返回原始字节
//   - 对自定义与内置两类来源统一收敛，供 resolveExternalLocales 使用
type readFile func(key string) ([]byte, bool)

// resolveExternalLocales 将 TOML 数据中的外部文件引用（`key = "@path"`）就地展开为
// TOML 多行字面量字符串，返回处理后的字节。外部文件以 TOML 引用路径声明（相对
// locales 目录），由 getFile 回调读取——自定义目录命中时优先，否则回退内置嵌入文件。
//
// 展开规则：命中行的值替换为 `”' <文件内容> ”'`（TOML 字面量字符串不处理转义，
// 适合承载 HTML 等含引号/换行的大文本）。无法读取引用文件时返回错误。
func resolveExternalLocales(data []byte, getFile readFile) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		m := extRefRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		content, ok := getFile(m[2])
		if !ok {
			return nil, errors.Newf("translate: external locale file %q not found", m[2])
		}
		lines[i] = fmt.Sprintf("%s = '''%s'''", m[1], string(content))
	}
	return []byte(strings.Join(lines, "\n")), nil
}

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
// 每个语言文件在解析前会经 resolveExternalLocales 展开其中 `key = "@path"` 的
// 外部模板引用，使超长模板（如邮件 HTML / 通知正文）可直接维护在独立文件而非 TOML 内。
// 模板字符串（含占位符）作为普通 i18n 消息加载进 Bundle；需要渲染最终文案时，由
// 调用方将参数直接传入 Localize（见 localize.go）渲染，无需事后取回原始模板串。
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
//	├── en.toml            # 英语翻译文件（可经 "@" 引用外部模板）
//	├── zh-Hans.toml       # 简体中文翻译文件
//	└── emails/
//	    └── password-reset-content.zh-Hans.html
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
		langStr := tag.String()

		// 构建内置文件路径（仅用于命名解析，避免 TOML 格式误判）
		buildinPath := fmt.Sprintf("locales/%s.toml", langStr)

		// 统一文件读取：自定义目录优先，回退内置嵌入文件。key 为相对 locales 目录的路径。
		getFile := func(key string) ([]byte, bool) {
			if data, err := os.ReadFile(fmt.Sprintf("%s/%s", base.Env.LocaleDirPath, key)); err == nil {
				return data, true
			}
			data, err := embeddedLocales.ReadFile(fmt.Sprintf("locales/%s", key))
			return data, err == nil
		}

		// 读取本地化文件（自定义优先）；两者都不存在则此语言无法加载，属于启动期严重错误
		data, ok := getFile(langStr + ".toml")
		if !ok {
			panic(errors.Newf("translate: no locale file found for language %q", langStr))
		}
		// 展开外部模板引用（`key = "@path"`）后解析进消息包
		expanded, err := resolveExternalLocales(data, getFile)
		if err != nil {
			panic(err)
		}
		// 遍历 ParseMessageFileBytes 返回的 MessageFile 仅用于解析进消息包；
		// 枚举 label 键（enum.*）无需另行收集：键形如 enum.<类名>.<key>，可直接经
		// 枚举注册表（internal/pkg/enum）推导，由 EnumLabel 按需 Localize。
		if _, err := bundle.ParseMessageFileBytes(expanded, buildinPath); err != nil {
			panic(err)
		}
	}
	return bundle
}

func init() {
	Bundle = newBundle()
}
