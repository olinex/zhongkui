// Package base 提供应用的基础定义：环境配置（Env）、全局常量（RBAC 策略、
// 数据库/日志/Redis 名称、HTTP 头）、语言类型及其解析与序列化、主机名工具，
// 以及语言与策略动作的枚举注册。
package base

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/creasty/defaults"
	"golang.org/x/text/language"
)

var (
	// LangSimplifiedChinese 简体中文
	LangSimplifiedChinese Language = *MustParseLanguage(language.SimplifiedChinese.String())
	// LangEnglish 英文
	LangEnglish Language = *MustParseLanguage(language.English.String())
	// SupportLanguages 应用支持的语言列表，与内置 locales 目录下的翻译文件一一对应
	SupportLanguages = []Language{
		LangSimplifiedChinese,
		LangEnglish,
	}
	// DefaultLanguage 应用默认语言，取自 Env.Lang（ZKI_LANG 环境变量，默认 zh-Hans）。
	// 必须在 init() 中赋值：包级变量初始化先于 init() 执行，
	// 若在此处直接引用 Env.Lang，会取到默认值尚未填充的空字符串。
	DefaultLanguage *Language
	// LanguageWildcard 通配所有语言的标记（String() 返回 "*"），用于模板等以
	// "*" 表示"对所有语言生效"的场景。
	LanguageWildcard = Language{wildcard: true}
)

// init 初始化 DefaultLanguage。
// environments.go 的 init() 按文件词法顺序先于本文件执行，
// 故此时 Env.Lang 已由 defaults.Set + envconfig.Process 填充默认值。
func init() {
	DefaultLanguage = MustParseLanguage(Env.Lang)
}

// Language 结构体封装了语言标签，提供了语言相关的操作方法
// 它使用 golang.org/x/text/language 包中的 Tag 类型作为内部表示
// 这个结构体实现了以下接口:
//   - encoding.TextMarshaler
//   - encoding.TextUnmarshaler
//   - encoding.JSONMarshaler
//   - encoding.JSONUnmarshaler
//   - defaults.Setter
//   - database/sql/driver.Valuer
//   - database/sql.Scanner
//
// 示例:
//
//	// 创建 Language 实例
//	lang, err := ParseLanguage("zh-Hans")
//	if err != nil {
//	    // 处理错误
//	}
//	fmt.Println(lang.String()) // 输出: "zh-Hans"
//	fmt.Println(lang.UnderscoreLocale()) // 输出: "zh_Hans"
type Language struct {
	// tag 内部语言标签，零值（und）表示未指定/平台默认。
	tag      language.Tag
	wildcard bool // true 表示通配所有语言（"*"）
}

// ParseLanguage 解析语言字符串并返回 Language 实例
// 空字符串表示"未指定/平台默认"（解析为零值 Language），"*" 表示通配所有语言，
// 其余按 golang.org/x/text/language 标准解析。
// 这个函数使用 golang.org/x/text/language 包的 Parse 方法进行解析
// 支持的语言格式包括:
//   - ISO 639-1 代码: "en", "zh", "fr", "ja", "ko"
//   - ISO 639-2 代码: "eng", "zho", "fra", "jpn", "kor"
//   - 语言-地区组合: "en-US", "zh-CN", "zh-TW", "fr-FR"
//   - 语言-脚本组合: "zh-Hans", "zh-Hant", "sr-Cyrl", "sr-Latn"
//   - 完整格式: "zh-Hans-CN", "en-US-u-ca-gregory"
//   - "*" 通配：所有语言
//   - "" 空字符串：未指定（平台默认）
//
// 参数:
//   - lang: 语言字符串，如 ""、"*"、"en"、"zh-Hans"、"zh-Hant"、"en-US" 等
//
// 返回值:
//   - *Language: 解析成功的 Language 实例
//   - error: 解析失败时返回的错误，包含堆栈信息
//
// 示例:
//
//	lang, err := ParseLanguage("zh-Hans")
//	if err != nil {
//	    // 处理错误
//	}
//	fmt.Println(lang.String()) // 输出: "zh-Hans"
func ParseLanguage(lang string) (*Language, error) {
	if lang == "" {
		return &Language{}, nil
	}
	if lang == "*" {
		return &Language{wildcard: true}, nil
	}
	tag, err := language.Parse(lang)
	if err != nil {
		return nil, fmt.Errorf("parse language %q: %w", lang, err)
	}
	return &Language{tag: tag}, nil
}

// MustParseLanguage 解析语言字符串并返回 Language 实例
// 如果解析失败，会 panic 并包含堆栈信息
func MustParseLanguage(lang string) *Language {
	l, err := ParseLanguage(lang)
	if err != nil {
		panic(err)
	}
	return l
}

// Normalize 将当前语言归一化为最近的支持语言（SupportLanguages 之一）。
//
// 该方法基于 BCP-47 匹配器计算与 SupportLanguages 的最近匹配：
//   - 完全/高度匹配时返回对应的支持语言（如 "zh-CN" → "zh-Hans"）
//   - 无法合理匹配时回退到 DefaultLanguage
//
// 返回值始终属于 SupportLanguages，保证后续 UniversalTranslator 查询、
// 语言文件查找等操作必然成功（不会因请求头携带未注册语言而 panic）。
//
// 使用场景:
//   - 中间件解析 Accept-Language 后，将请求语言归一化到受支持集合
func (l *Language) Normalize() *Language {
	// 空引用或零值（空）语言直接回退默认语言；通配 "*" 保持通配不变
	if l == nil {
		return DefaultLanguage
	}
	if l.wildcard {
		return &LanguageWildcard
	}
	if l.tag == language.Und {
		return DefaultLanguage
	}
	tags := make([]language.Tag, 0, len(SupportLanguages))
	for i := range SupportLanguages {
		tags = append(tags, SupportLanguages[i].StdTag())
	}
	matcher := language.NewMatcher(tags)
	_, idx, _ := matcher.Match(l.tag)
	return &SupportLanguages[idx]
}

// String 返回内部的语言标签字符串表示
// 这个方法实现了 fmt.Stringer 接口，使得 Language 实例可以直接用于字符串格式化
// 返回的字符串格式与输入到 ParseLanguage 的格式相同
//
// 返回值:
//   - string: 语言标签的字符串表示
//
// 示例:
//
//	lang, _ := ParseLanguage("zh-Hans")
//	fmt.Println(lang.String()) // 输出: "zh-Hans"
//	fmt.Printf("Language: %s\n", lang) // 输出: Language: zh-Hans
func (l *Language) String() string {
	if l == nil {
		return ""
	}
	if l.wildcard {
		return "*"
	}
	if l.tag == language.Und {
		return ""
	}
	return l.tag.String()
}

// StdTag 返回内部的语言标签
// 这个方法提供了对底层 language.Tag 的直接访问
// 当需要与 golang.org/x/text/language 包的其他功能交互时非常有用
//
// 返回值:
//   - language.Tag: 标准的语言标签
//
// 示例:
//
//	lang, _ := ParseLanguage("zh-Hans")
//	tag := lang.StdTag()
//	base, _ := tag.Base()
//	fmt.Println(base.String()) // 输出: "zh"
func (l *Language) StdTag() language.Tag {
	return l.tag
}

// UnderscoreLocale 将语言标签转换为下划线格式的字符串
// 这个方法主要用于文件系统路径或数据库字段名等场景
// 其中连字符可能不被支持或需要特殊处理
//
// 转换规则:
//   - 将语言标签中的所有连字符("-")替换为下划线("_")
//   - 例如: "zh-Hans" -> "zh_Hans", "en-US" -> "en_US"
//   - 例如: "zh-Hans-CN" -> "zh_Hans_CN"
//
// 返回值:
//   - string: 下划线格式的语言字符串
//
// 示例:
//
//	lang, _ := ParseLanguage("zh-Hans")
//	fmt.Println(lang.UnderscoreLocale()) // 输出: "zh_Hans"
//
//	lang, _ = ParseLanguage("en-US")
//	fmt.Println(lang.UnderscoreLocale()) // 输出: "en_US"
func (l *Language) UnderscoreLocale() string {
	return strings.ReplaceAll(l.tag.String(), "-", "_")
}

// SetDefaults 设置默认值，当使用 defaults 包时自动调用
// 这个方法实现了 github.com/creasty/defaults.Setter 接口
// 当使用 defaults.Set() 函数设置结构体默认值时，如果结构体包含 Language 字段
// 且该字段为零值，这个方法会被自动调用将其设置为英语
//
// 注意: 这个方法只在 Language 实例可以被更新时生效
// 即 Language 字段是结构体的一部分且为零值
//
// 示例:
//
//	type Config struct {
//	    DefaultLang base.Language `default:""`
//	}
//
//	var cfg Config
//	defaults.Set(&cfg)
//	fmt.Println(cfg.DefaultLang.String()) // 输出: "en"
func (l *Language) SetDefaults() {
	if defaults.CanUpdate(l) {
		l.tag = language.English
	}
}

// UnmarshalText 实现了 encoding.TextUnmarshaler 接口
// 用于从文本格式（如配置文件）反序列化 Language
// 这个方法使得 Language 类型可以用于 YAML、TOML 等配置文件的解析
// 以及 encoding.TextUnmarshaler 支持的其他文本格式
//
// 参数:
//   - data: 包含语言字符串的字节数组
//
// 返回值:
//   - error: 反序列化过程中的错误，包含堆栈信息
//
// 注意:
//   - 如果 data 为空字节数组，方法返回 nil 且不修改 Language 实例
//   - 如果 data 包含无效的语言字符串，返回解析错误
//
// 示例:
//
//	// 在 YAML 配置中
//	// default_lang: "zh-Hans"
//	var cfg struct {
//	    DefaultLang base.Language `yaml:"default_lang"`
//	}
//
//	// 在 TOML 配置中
//	// default_lang = "zh-Hans"
//	var cfg struct {
//	    DefaultLang base.Language `toml:"default_lang"`
//	}
func (l *Language) UnmarshalText(data []byte) error {
	text := string(data)
	if len(data) == 0 {
		return nil
	} else if text == "*" {
		l.tag = language.Und
		l.wildcard = true
		return nil
	} else if tag, err := language.Parse(text); err != nil {
		return fmt.Errorf("parse language %q: %w", text, err)
	} else {
		l.tag = tag
		l.wildcard = false
		return nil
	}
}

// MarshalText 实现了 encoding.TextMarshaler 接口
// 用于将 Language 序列化为文本格式
// 这个方法使得 Language 类型可以序列化为 YAML、TOML 等配置文件格式
// 以及 encoding.TextMarshaler 支持的其他文本格式
//
// 返回值:
//   - []byte: 语言标签的字节数组表示
//   - error: 序列化过程中的错误，总是返回 nil
//
// 示例:
//
//	lang, _ := ParseLanguage("zh-Hans")
//	data, err := lang.MarshalText()
//	if err != nil {
//	    // 处理错误（实际上不会发生）
//	}
//	fmt.Println(string(data)) // 输出: "zh-Hans"
//
//	// 在 YAML 序列化中
//	type Config struct {
//	    DefaultLang base.Language `yaml:"default_lang"`
//	}
//	cfg := Config{DefaultLang: *lang}
//	yamlData, _ := yaml.Marshal(cfg)
//	// 输出包含: default_lang: zh-Hans
func (l Language) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalJSON 实现了 encoding.JSONUnmarshaler 接口
// 用于从 JSON 格式反序列化 Language
// 这个方法使得 Language 类型可以直接从 JSON 数据中解析
// JSON 数据应该是一个包含语言标签的字符串
//
// 参数:
//   - data: 包含语言字符串的 JSON 字节数组
//
// 返回值:
//   - error: 反序列化过程中的错误，包含堆栈信息
//
// 注意:
//   - JSON 数据必须是有效的字符串格式
//   - 字符串内容必须是有效的语言标签
//   - 如果 JSON 数据为空或 null，Language 实例不会被修改
//
// 示例:
//
//	// JSON 数据: {"language": "zh-Hans"}
//	type Config struct {
//	    Language base.Language `json:"language"`
//	}
//
//	var cfg Config
//	jsonData := []byte(`{"language": "zh-Hans"}`)
//	if err := json.Unmarshal(jsonData, &cfg); err != nil {
//	    // 处理错误
//	}
//	fmt.Println(cfg.Language.String()) // 输出: "zh-Hans"
func (l *Language) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal language json: %w", err)
	}
	return l.UnmarshalText([]byte(raw))
}

// MarshalJSON 实现了 encoding.JSONMarshaler 接口
// 用于将 Language 序列化为 JSON 格式
// 这个方法使得 Language 类型可以直接序列化为 JSON 数据
// 序列化结果为包含语言标签的 JSON 字符串
//
// 返回值:
//   - []byte: 语言标签的 JSON 字节数组表示
//   - error: 序列化过程中的错误，包含堆栈信息
//
// 注意:
//   - 序列化结果是一个 JSON 字符串
//   - 如果 MarshalText 方法返回错误，该错误会被包装并返回
//
// 示例:
//
//	lang, _ := ParseLanguage("zh-Hans")
//	jsonData, err := lang.MarshalJSON()
//	if err != nil {
//	    // 处理错误
//	}
//	fmt.Println(string(jsonData)) // 输出: "\"zh-Hans\""
//
//	// 在结构体序列化中
//	type Config struct {
//	    Language base.Language `json:"language"`
//	}
//	cfg := Config{Language: *lang}
//	jsonData, _ := json.Marshal(cfg)
//	// 输出: {"language":"zh-Hans"}
func (l Language) MarshalJSON() ([]byte, error) {
	if raw, err := l.MarshalText(); err != nil {
		return nil, fmt.Errorf("marshal language json: %w", err)
	} else {
		return json.Marshal(string(raw))
	}
}

// Value 实现 driver.Valuer，数据库以字符串（VARCHAR）存储语言标签。
// 未设置（零值 und）时返回 nil，数据库存储 NULL。
func (l Language) Value() (driver.Value, error) {
	if l.wildcard {
		return "*", nil
	}
	if l.tag == language.Und {
		return nil, nil
	}
	return l.String(), nil
}

// Scan 实现 sql.Scanner，兼容字符串语言标签、字节数组与 NULL。
// NULL 将 Language 重置为零值（und）。
func (l *Language) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		l.tag = language.Und
		l.wildcard = false
		return nil
	case string:
		return l.UnmarshalText([]byte(v))
	case []byte:
		return l.UnmarshalText(v)
	default:
		return fmt.Errorf("unsupported language value type: %T", value)
	}
}
