package translate

import (
	"encoding/json"
	"strings"

	"github.com/creasty/defaults"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// DefaultLang 是默认的语言实例，使用英语作为默认语言
// 这个变量在应用程序启动时初始化，可以在需要默认语言的地方直接使用
// 示例:
//
//	lang := translate.DefaultLang
//	fmt.Println(lang.String()) // 输出: "en"
var (
	DefaultLang = &Language{tag: language.English}
)

// Language 结构体封装了语言标签，提供了语言相关的操作方法
// 它使用 golang.org/x/text/language 包中的 Tag 类型作为内部表示
// 这个结构体实现了以下接口:
//   - encoding.TextMarshaler
//   - encoding.TextUnmarshaler
//   - encoding.JSONMarshaler
//   - encoding.JSONUnmarshaler
//   - defaults.Setter
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
	tag language.Tag
}

// ParseLanguage 解析语言字符串并返回 Language 实例
// 这个函数使用 golang.org/x/text/language 包的 Parse 方法进行解析
// 支持的语言格式包括:
//   - ISO 639-1 代码: "en", "zh", "fr", "ja", "ko"
//   - ISO 639-2 代码: "eng", "zho", "fra", "jpn", "kor"
//   - 语言-地区组合: "en-US", "zh-CN", "zh-TW", "fr-FR"
//   - 语言-脚本组合: "zh-Hans", "zh-Hant", "sr-Cyrl", "sr-Latn"
//   - 完整格式: "zh-Hans-CN", "en-US-u-ca-gregory"
//
// 参数:
//   - lang: 语言字符串，如 "en", "zh-Hans", "zh-Hant", "en-US" 等
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
	tag, err := language.Parse(lang)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Language{tag: tag}, nil
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
//	    DefaultLang translate.Language `default:""`
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
//	    DefaultLang translate.Language `yaml:"default_lang"`
//	}
//
//	// 在 TOML 配置中
//	// default_lang = "zh-Hans"
//	var cfg struct {
//	    DefaultLang translate.Language `toml:"default_lang"`
//	}
func (l *Language) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		return nil
	} else if tag, err := language.Parse(string(data)); err != nil {
		return errors.WithStack(err)
	} else {
		l.tag = tag
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
//	    DefaultLang translate.Language `yaml:"default_lang"`
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
//	    Language translate.Language `json:"language"`
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
		return errors.WithStack(err)
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
//	    Language translate.Language `json:"language"`
//	}
//	cfg := Config{Language: *lang}
//	jsonData, _ := json.Marshal(cfg)
//	// 输出: {"language":"zh-Hans"}
func (l Language) MarshalJSON() ([]byte, error) {
	if raw, err := l.MarshalText(); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return json.Marshal(string(raw))
	}
}
