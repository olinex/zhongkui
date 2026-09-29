package codes

import (
	"encoding/json"
	"fmt"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// iso6392Codes 是 ISO 639-2 语言代码的全局单例实例，通过 GetISO6392Codes 获取。
	iso6392Codes *ISO6392Codes
	// iso6392Once 用于确保 iso6392Codes 仅被初始化一次（sync.Once 惰性初始化）。
	iso6392Once sync.Once
)

// ISO6392Entry 表示 ISO 639-2 标准中的单条语言记录。
//
// ISO 639-2 定义了语言的 Alpha-3（三位字母）代码，部分语言同时拥有
// 对应 ISO 639-1 的 Alpha-2（两位字母）代码。
type ISO6392Entry struct {
	// Alpha2 对应 ISO 639-1 的两位字母代码，如 "zh"、"en"；部分语言无此代码则为空。
	Alpha2 string `json:"alpha_2,omitempty"`
	// Alpha3 对应 ISO 639-2/T 的三位字母代码，如 "zho"、"eng"。
	Alpha3 string `json:"alpha_3"`
	// Name 语言的英文标准名称，如 "Chinese"、"English"。
	Name string `json:"name"`
}

// ISO6392Data 表示 ISO 639-2 JSON 数据文件的顶层结构。
//
// JSON 文件以 "639-2" 为键，值为语言条目数组，例如：
//
//	{
//	  "639-2": [
//	    {"alpha_2": "zh", "alpha_3": "zho", "name": "Chinese"},
//	    ...
//	  ]
//	}
type ISO6392Data struct {
	// Languages 包含所有 ISO 639-2 语言条目。
	Languages []ISO6392Entry `json:"639-2"`
}

// ISO6392Codes 提供基于 ISO 639-2 标准的语言代码查询与转换能力。
//
// 内部维护三个索引映射表，分别以 Alpha-2 代码、Alpha-3 代码和语言名称为键，
// 支持从不同维度快速查找语言条目。所有映射均指向同一组 ISO6392Entry 指针，
// 保证通过不同方式查询到的同一语言指向相同对象。
//
// 该结构通过 GetISO6392Codes() 获取全局单例，不应直接构造。
type ISO6392Codes struct {
	// alpha2Index 以 ISO 639-1 Alpha-2 代码为键的索引，仅包含拥有 Alpha-2 代码的语言。
	alpha2Index map[string]*ISO6392Entry
	// alpha3Index 以 ISO 639-2 Alpha-3 代码为键的索引，包含所有语言。
	alpha3Index map[string]*ISO6392Entry
	// nameIndex 以语言英文标准名称为键的索引，包含所有语言。
	nameIndex map[string]*ISO6392Entry
}

// GetISO6392Codes 返回 ISO 639-2 语言代码查询器的全局单例。
//
// 首次调用时从嵌入的 JSON 数据文件中加载并构建索引，后续调用直接返回已缓存的实例。
// 使用 sync.Once 保证并发安全且仅初始化一次。
// 如果数据加载失败，将以 panic 终止程序，因为这属于不可恢复的启动错误。
func GetISO6392Codes() *ISO6392Codes {
	iso6392Once.Do(func() {
		codes, err := newISO6392Codes()
		if err != nil {
			panic(err)
		}
		iso6392Codes = codes
	})
	return iso6392Codes
}

// newISO6392Codes 从嵌入的 ISO 639-2 JSON 数据文件中加载语言数据，并构建索引映射表。
//
// 处理流程：
//  1. 从 embeddedStandards 中打开 iso_639-2.json 文件；
//  2. 读取文件内容并解析为 ISO6392Data 结构；
//  3. 遍历所有语言条目，分别构建 Alpha-2、Alpha-3 和名称三个索引；
//  4. 在构建索引过程中检测重复键，若发现重复则 panic（数据文件不应包含重复项）。
//
// 注意：对于没有 Alpha-2 代码的语言（如古希腊语 grc），仅将其加入 Alpha-3 和名称索引。
func newISO6392Codes() (*ISO6392Codes, error) {
	file, err := embeddedStandards.Open(Language)
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
	var data ISO6392Data
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a2Index := make(map[string]*ISO6392Entry, 0)
	a3Index := make(map[string]*ISO6392Entry, len(data.Languages))
	nIndex := make(map[string]*ISO6392Entry, len(data.Languages))
	for _, entry := range data.Languages {
		if entry.Alpha2 != "" {
			if _, ok := a2Index[entry.Alpha2]; ok {
				panic(fmt.Sprintf(`Alpha2 %s was duplicated`, entry.Alpha2))
			}
			a2Index[entry.Alpha2] = &entry
		}
		if _, ok := a3Index[entry.Alpha3]; ok {
			panic(fmt.Sprintf(`Alpha3 %s was duplicated`, entry.Alpha3))
		}
		a3Index[entry.Alpha3] = &entry
		if _, ok := nIndex[entry.Name]; ok {
			panic(fmt.Sprintf(`Name %s was duplicated`, entry.Name))
		}
		nIndex[entry.Name] = &entry
	}
	return &ISO6392Codes{
		alpha2Index: a2Index,
		alpha3Index: a3Index,
		nameIndex:   nIndex,
	}, nil
}

// ConvertFromAlpha2 根据 ISO 639-1 Alpha-2 代码（如 "zh"、"en"）查询对应的语言条目。
//
// 如果给定的 Alpha-2 代码不存在或语言没有 Alpha-2 代码，则返回 nil。
// 查询区分大小写。
func (c *ISO6392Codes) ConvertFromAlpha2(alpha2 string) *ISO6392Entry {
	return c.alpha2Index[alpha2]
}

// ConvertFromAlpha3 根据 ISO 639-2 Alpha-3 代码（如 "zho"、"eng"）查询对应的语言条目。
//
// 所有 ISO 639-2 语言均拥有 Alpha-3 代码，因此该索引最为完整。
// 如果给定的 Alpha-3 代码不存在，则返回 nil。
// 查询区分大小写。
func (c *ISO6392Codes) ConvertFromAlpha3(alpha3 string) *ISO6392Entry {
	return c.alpha3Index[alpha3]
}

// ConvertFromName 根据语言的英文标准名称（如 "Chinese"、"English"）查询对应的语言条目。
//
// 如果给定的名称不存在，则返回 nil。
// 查询区分大小写，名称必须与 ISO 标准完全一致。
func (c *ISO6392Codes) ConvertFromName(name string) *ISO6392Entry {
	return c.nameIndex[name]
}
