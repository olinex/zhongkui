package codes

import (
	"encoding/json"
	"fmt"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// iso15924Codes 是 ISO 15924 书写系统代码的全局单例实例，通过 sync.Once 保证仅初始化一次。
	iso15924Codes *ISO15924Codes
	// iso15924Once 用于控制 iso15924Codes 的延迟初始化，确保并发安全。
	iso15924Once sync.Once
)

// ISO15924Entry 表示 ISO 15924 标准中一种书写系统（script）的完整信息条目。
//
// ISO 15924 是国际标准化组织（ISO）定义的书写系统代码标准，
// 每个条目包含该书写系统的四位字母代码、数字代码及名称。
//
// 字段说明：
//   - Alpha4: 四位字母代码（如 "Hans"、"Latn"、"Cyrl"），是使用最广泛的书写系统编码形式
//   - Name: 书写系统的英文名称（如 "Han (Simplified variant)"、"Latin"、"Cyrillic"）
//   - Numeric: 三位数字代码（如 "501"、"215"、"220"），与联合国数字代码一致
type ISO15924Entry struct {
	// Alpha4 四位字母代码（Alpha-4），如 "Hans"、"Latn"。
	Alpha4 string `json:"alpha_4"`
	// Name 书写系统的英文名称，如 "Latin"、"Cyrillic"。
	Name string `json:"name"`
	// Numeric 三位数字代码，如 "501"（简体汉字）、"215"（拉丁文）。
	Numeric string `json:"numeric"`
}

// ISO15924Data 表示 ISO 15924 标准数据文件的顶层结构，
// 用于从嵌入的 JSON 文件中反序列化书写系统条目列表。
//
// JSON 文件以 "15924" 为键，值为书写系统条目数组，例如：
//
//	{
//	  "15924": [
//	    {"alpha_4": "Hans", "name": "Han (Simplified variant)", "numeric": "501"},
//	    ...
//	  ]
//	}
type ISO15924Data struct {
	// ScriptLanguages 包含所有 ISO 15924 书写系统条目。
	ScriptLanguages []ISO15924Entry `json:"15924"`
}

// ISO15924Codes 提供 ISO 15924 书写系统代码的多种维度快速查询能力。
//
// 内部维护了三个索引映射表，分别以 Alpha4、Name、Numeric 作为键，
// 支持从任意一种编码形式快速定位到完整的书写系统信息条目。
// 所有索引均在初始化时一次性构建，查询操作的时间复杂度为 O(1)。
type ISO15924Codes struct {
	// alpha4Index 以四位字母代码（Alpha-4）为键的索引，如 "Hans" -> 简体汉字条目。
	alpha4Index map[string]*ISO15924Entry
	// nameIndex 以书写系统英文名称为键的索引，如 "Latin" -> 拉丁文字条目。
	nameIndex map[string]*ISO15924Entry
	// numericIndex 以三位数字代码为键的索引，如 "501" -> 简体汉字条目。
	numericIndex map[string]*ISO15924Entry
}

// GetISO15924Codes 获取 ISO 15924 书写系统代码查询器的全局单例。
//
// 首次调用时会从嵌入的 JSON 数据文件中加载并解析所有书写系统数据，
// 构建各维度索引。后续调用直接返回已缓存的实例，不再重复加载。
// 使用 sync.Once 保证并发安全且仅初始化一次。
// 如果加载过程中发生错误，将触发 panic，因为这是不可恢复的初始化错误。
func GetISO15924Codes() *ISO15924Codes {
	iso15924Once.Do(func() {
		codes, err := newISO15924Codes()
		if err != nil {
			panic(err)
		}
		iso15924Codes = codes
	})
	return iso15924Codes
}

// newISO15924Codes 从嵌入的 ISO 15924 标准数据文件中加载书写系统数据，
// 并构建 Alpha4、Name、Numeric 三个维度的索引映射表。
//
// 处理流程：
//  1. 从 embeddedStandards 中打开 iso_15924.json 文件；
//  2. 读取文件内容并解析为 ISO15924Data 结构；
//  3. 遍历所有书写系统条目，分别构建 Alpha4、Name 和 Numeric 三个索引；
//  4. 在构建索引过程中检测重复键，若发现重复则 panic（数据文件不应包含重复项）。
func newISO15924Codes() (*ISO15924Codes, error) {
	file, err := embeddedStandards.Open(ScriptLanguage)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	raw := make([]byte, info.Size())
	_, err = file.Read(raw)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var data ISO15924Data
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a4Index := make(map[string]*ISO15924Entry, len(data.ScriptLanguages))
	nIndex := make(map[string]*ISO15924Entry, len(data.ScriptLanguages))
	nmIndex := make(map[string]*ISO15924Entry, len(data.ScriptLanguages))
	for _, entry := range data.ScriptLanguages {
		if _, ok := a4Index[entry.Alpha4]; ok {
			panic(fmt.Sprintf(`Alpha4 %s was duplicated`, entry.Alpha4))
		}
		a4Index[entry.Alpha4] = &entry
		if _, ok := nIndex[entry.Name]; ok {
			panic(fmt.Sprintf(`Name %s was duplicated`, entry.Name))
		}
		nIndex[entry.Name] = &entry
		if _, ok := nmIndex[entry.Numeric]; ok {
			panic(fmt.Sprintf(`Numeric %s was duplicated`, entry.Numeric))
		}
		nmIndex[entry.Numeric] = &entry
	}
	return &ISO15924Codes{
		alpha4Index:  a4Index,
		nameIndex:    nIndex,
		numericIndex: nmIndex,
	}, nil
}

// ConvertFromAlpha4 根据四位字母代码（Alpha-4）查询书写系统信息。
//
// 参数：
//   - alpha4: ISO 15924 Alpha-4 代码，如 "Hans"、"Latn"、"Cyrl"
//
// 返回值：
//   - 匹配到的书写系统条目指针；若未找到则返回 nil
func (c *ISO15924Codes) ConvertFromAlpha4(alpha4 string) *ISO15924Entry {
	return c.alpha4Index[alpha4]
}

// ConvertFromName 根据书写系统英文名称查询书写系统信息。
//
// 参数：
//   - name: 书写系统的英文名称，如 "Latin"、"Cyrillic"、"Arabic"
//
// 返回值：
//   - 匹配到的书写系统条目指针；若未找到则返回 nil
func (c *ISO15924Codes) ConvertFromName(name string) *ISO15924Entry {
	return c.nameIndex[name]
}

// ConvertFromNumeric 根据三位数字代码查询书写系统信息。
//
// 参数：
//   - numeric: ISO 15924 数字代码，如 "501"（简体汉字）、"215"（拉丁文）、"220"（西里尔文）
//
// 返回值：
//   - 匹配到的书写系统条目指针；若未找到则返回 nil
func (c *ISO15924Codes) ConvertFromNumeric(numeric string) *ISO15924Entry {
	return c.numericIndex[numeric]
}
