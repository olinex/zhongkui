// Package codes 提供基于国际标准（ISO、RFC 等）的代码查询与转换能力。
package codes

import "strings"

// RFC1766Entry 表示符合 RFC 1766 标准的区域设置条目。
//
// RFC 1766 定义了语言标识标签的格式，由语言代码（ISO 639）和国家/地区代码（ISO 3166-1）组成，
// 用于标识特定国家/地区的语言变体，例如 "zh_CN" 表示中国大陆使用的中文，"en_US" 表示美国英语。
type RFC1766Entry struct {
	// Language 语言条目（ISO 639-2）。
	Language *ISO6392Entry
	// Country 国家/地区条目（ISO 3166-1）。
	Country *ISO31661Entry
}

// RFC1766Codes 提供基于 RFC 1766 标准的区域设置代码查询与转换能力。
//
// 内部组合了 ISO 639-2 语言代码查询器和 ISO 3166-1 国家/地区代码查询器，
// 支持将 "语言_国家" 格式的区域设置名称（如 "zh_CN"、"en-US"）解析为
// 包含完整语言和国家/地区信息的 RFC1766Entry。
//
// 支持的分隔符：
//   - 下划线 "_"：如 "zh_CN"、"en_US"
//   - 连字符 "-"：如 "zh-CN"、"en-US"
//
// 该结构通过 GetRFC1766Codes() 获取实例，不应直接构造。
type RFC1766Codes struct {
	// language ISO 639-2 语言代码查询器。
	language *ISO6392Codes
	// country ISO 3166-1 国家/地区代码查询器。
	country *ISO31661Codes
}

// GetRFC1766Codes 创建并返回 RFC 1766 区域设置代码查询器实例。
//
// 内部分别调用 GetISO6392Codes() 和 GetISO31661Codes() 获取语言和国家/地区
// 代码查询器的全局单例，组合为 RFC1766Codes 实例。
func GetRFC1766Codes() *RFC1766Codes {
	return &RFC1766Codes{
		language: GetISO6392Codes(),
		country:  GetISO31661Codes(),
	}
}

// ConvertFromName 根据区域设置名称查询对应的 RFC 1766 条目。
//
// 将输入的区域设置名称按分隔符（下划线 "_" 或连字符 "-"）拆分为语言代码和国家/地区代码两部分，
// 然后分别通过 ISO 639-2 Alpha-2 和 ISO 3166-1 Alpha-2 索引进行查询。
// 仅当两部分均成功匹配时，才返回组合后的 RFC1766Entry。
//
// 参数：
//   - name: 区域设置名称，格式为 "语言Alpha2_国家Alpha2" 或 "语言Alpha2-国家Alpha2"，
//     例如 "zh_CN"、"en-US"、"ja_JP"
//
// 返回值：
//   - 匹配到的 RFC1766Entry 指针；若名称格式无效、语言或国家/地区代码不存在则返回 nil
func (c *RFC1766Codes) ConvertFromName(name string) *RFC1766Entry {
	var langAlpha2, cnyAlpha2 string
	if parts := strings.Split(name, "_"); len(parts) == 2 {
		langAlpha2 = parts[0]
		cnyAlpha2 = parts[1]
	} else if parts = strings.Split(name, "-"); len(parts) == 2 {
		langAlpha2 = parts[0]
		cnyAlpha2 = parts[1]
	} else {
		return nil
	}
	if lanEntry := c.language.ConvertFromAlpha2(langAlpha2); lanEntry == nil {
		return nil
	} else if cnyEntry := c.country.ConvertFromAlpha2(cnyAlpha2); cnyEntry == nil {
		return nil
	} else {
		return &RFC1766Entry{
			Language: lanEntry,
			Country:  cnyEntry,
		}
	}
}

// RFC4646Entry 表示符合 RFC 4646 标准的语言脚本条目。
//
// RFC 4646（后修订为 RFC 5646 / BCP 47）定义了语言标识标签的格式，
// 由语言代码（ISO 639）和文字脚本代码（ISO 15924）组成，
// 用于标识使用特定文字书写系统的语言，例如 "zh_Hans" 表示简体中文，
// "ja_Jpan" 表示使用日文书写系统（汉字+平假名+片假名）的日语。
type RFC4646Entry struct {
	// Language 语言条目（ISO 639-2）。
	Language *ISO6392Entry
	// Script 文字脚本条目（ISO 15924）。
	Script *ISO15924Entry
}

// RFC4646Codes 提供基于 RFC 4646 标准的语言脚本代码查询与转换能力。
//
// 内部组合了 ISO 639-2 语言代码查询器和 ISO 15924 文字脚本代码查询器，
// 支持将 "语言_脚本" 格式的语言标签名称（如 "zh_Hans"、"en-Latn"）解析为
// 包含完整语言和脚本信息的 RFC4646Entry。
//
// 支持的分隔符：
//   - 下划线 "_"：如 "zh_Hans"、"en_Latn"
//   - 连字符 "-"：如 "zh-Hans"、"en-Latn"
//
// 该结构通过 GetRFC4646Codes() 获取实例，不应直接构造。
type RFC4646Codes struct {
	// language ISO 639-2 语言代码查询器。
	language *ISO6392Codes
	// script ISO 15924 文字脚本代码查询器。
	script *ISO15924Codes
}

// GetRFC4646Codes 创建并返回 RFC 4646 语言脚本代码查询器实例。
//
// 内部分别调用 GetISO6392Codes() 和 GetISO15924Codes() 获取语言和文字脚本
// 代码查询器的全局单例，组合为 RFC4646Codes 实例。
func GetRFC4646Codes() *RFC4646Codes {
	return &RFC4646Codes{
		language: GetISO6392Codes(),
		script:   GetISO15924Codes(),
	}
}

// ConvertFromName 根据语言标签名称查询对应的 RFC 4646 条目。
//
// 将输入的语言标签名称按分隔符（下划线 "_" 或连字符 "-"）拆分为语言代码和脚本代码两部分，
// 然后分别通过 ISO 639-2 Alpha-2 和 ISO 15924 Alpha-4 索引进行查询。
// 仅当两部分均成功匹配时，才返回组合后的 RFC4646Entry。
//
// 参数：
//   - name: 语言标签名称，格式为 "语言Alpha2_脚本Alpha4" 或 "语言Alpha2-脚本Alpha4"，
//     例如 "zh_Hans"、"en-Latn"、"ja_Jpan"
//
// 返回值：
//   - 匹配到的 RFC4646Entry 指针；若名称格式无效、语言或脚本代码不存在则返回 nil
func (c *RFC4646Codes) ConvertFromName(name string) *RFC4646Entry {
	var langAlpha2, sptAlpha4 string
	if parts := strings.Split(name, "_"); len(parts) == 2 {
		langAlpha2 = parts[0]
		sptAlpha4 = parts[1]
	} else if parts = strings.Split(name, "-"); len(parts) == 2 {
		langAlpha2 = parts[0]
		sptAlpha4 = parts[1]
	} else {
		return nil
	}
	if lanEntry := c.language.ConvertFromAlpha2(langAlpha2); lanEntry == nil {
		return nil
	} else if sptEntry := c.script.ConvertFromAlpha4(sptAlpha4); sptEntry == nil {
		return nil
	} else {
		return &RFC4646Entry{
			Language: lanEntry,
			Script:   sptEntry,
		}
	}
}
