package codes

import (
	"encoding/json"
	"fmt"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// iso4217Codes 是 ISO 4217 货币代码的全局单例实例，通过 sync.Once 保证仅初始化一次。
	iso4217Codes *ISO4217Codes
	// iso4217Once 用于控制 iso4217Codes 的延迟初始化，确保并发安全。
	iso4217Once sync.Once
)

// ISO4217Entry 表示 ISO 4217 标准中一种货币的完整信息条目。
//
// ISO 4217 是国际标准化组织（ISO）定义的货币代码标准，
// 每个条目包含该货币的字母代码、数字代码及名称。
//
// 字段说明：
//   - Alpha3: 三位字母代码（如 "CNY"、"USD"、"EUR"），是使用最广泛的货币编码形式
//   - Name: 货币的英文名称（如 "Yuan Renminbi"、"US Dollar"、"Euro"）
//   - Numeric: 三位数字代码（如 "156"、"840"、"978"），与联合国数字代码一致
type ISO4217Entry struct {
	// Alpha3 三位字母代码（Alpha-3），如 "CNY"、"USD"。
	Alpha3 string `json:"alpha_3"`
	// Name 货币的英文名称，如 "Yuan Renminbi"、"US Dollar"。
	Name string `json:"name"`
	// Numeric 三位数字代码，如 "156"（人民币）、"840"（美元）。
	Numeric string `json:"numeric"`
}

// ISO4217Data 表示 ISO 4217 标准数据文件的顶层结构，
// 用于从嵌入的 JSON 文件中反序列化货币条目列表。
type ISO4217Data struct {
	// Currencies 包含所有 ISO 4217 货币条目。
	Currencies []ISO4217Entry `json:"4217"`
}

// ISO4217Codes 提供 ISO 4217 货币代码的多种维度快速查询能力。
//
// 内部维护了三个索引映射表，分别以 Alpha3、Name、Numeric 作为键，
// 支持从任意一种编码形式快速定位到完整的货币信息条目。
// 所有索引均在初始化时一次性构建，查询操作的时间复杂度为 O(1)。
type ISO4217Codes struct {
	// alpha3Index 以三位字母代码（Alpha-3）为键的索引，如 "CNY" -> 人民币条目。
	alpha3Index map[string]*ISO4217Entry
	// nameIndex 以货币英文名称为键的索引，如 "Yuan Renminbi" -> 人民币条目。
	nameIndex map[string]*ISO4217Entry
	// numericIndex 以三位数字代码为键的索引，如 "156" -> 人民币条目。
	numericIndex map[string]*ISO4217Entry
}

// GetISO4217Codes 获取 ISO 4217 货币代码查询器的全局单例。
//
// 首次调用时会从嵌入的 JSON 数据文件中加载并解析所有货币数据，
// 构建各维度索引。后续调用直接返回已缓存的实例，不再重复加载。
// 如果加载过程中发生错误，将触发 panic，因为这是不可恢复的初始化错误。
func GetISO4217Codes() *ISO4217Codes {
	iso4217Once.Do(func() {
		codes, err := newISO4217Codes()
		if err != nil {
			panic(err)
		}
		iso4217Codes = codes
	})
	return iso4217Codes
}

// newISO4217Codes 从嵌入的 ISO 4217 标准数据文件中加载货币数据，
// 并构建 Alpha3、Name、Numeric 三个维度的索引映射表。
//
// 在构建索引的过程中，会检测数据中是否存在重复的键值，
// 若发现重复将直接 panic，以保证索引数据的唯一性和正确性。
func newISO4217Codes() (*ISO4217Codes, error) {
	file, err := embeddedStandards.Open(Currency)
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
	var data ISO4217Data
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a3Index := make(map[string]*ISO4217Entry, len(data.Currencies))
	nIndex := make(map[string]*ISO4217Entry, len(data.Currencies))
	nmIndex := make(map[string]*ISO4217Entry, len(data.Currencies))
	for _, entry := range data.Currencies {
		if _, ok := a3Index[entry.Alpha3]; ok {
			panic(fmt.Sprintf(`Alpha3 %s was duplicated`, entry.Alpha3))
		}
		a3Index[entry.Alpha3] = &entry
		if _, ok := nIndex[entry.Name]; ok {
			panic(fmt.Sprintf(`Name %s was duplicated`, entry.Name))
		}
		nIndex[entry.Name] = &entry
		if _, ok := nmIndex[entry.Numeric]; ok {
			panic(fmt.Sprintf(`Numeric %s was duplicated`, entry.Numeric))
		}
		nmIndex[entry.Numeric] = &entry
	}
	return &ISO4217Codes{
		alpha3Index:  a3Index,
		nameIndex:    nIndex,
		numericIndex: nmIndex,
	}, nil
}

// ConvertFromAlpha3 根据三位字母代码（Alpha-3）查询货币信息。
//
// 参数：
//   - alpha3: ISO 4217 Alpha-3 代码，如 "CNY"、"USD"、"EUR"
//
// 返回值：
//   - 匹配到的货币条目指针；若未找到则返回 nil
func (c *ISO4217Codes) ConvertFromAlpha3(alpha3 string) *ISO4217Entry {
	return c.alpha3Index[alpha3]
}

// ConvertFromName 根据货币英文名称查询货币信息。
//
// 参数：
//   - name: 货币的英文名称，如 "Yuan Renminbi"、"US Dollar"、"Euro"
//
// 返回值：
//   - 匹配到的货币条目指针；若未找到则返回 nil
func (c *ISO4217Codes) ConvertFromName(name string) *ISO4217Entry {
	return c.nameIndex[name]
}

// ConvertFromNumeric 根据三位数字代码查询货币信息。
//
// 参数：
//   - numeric: ISO 4217 数字代码，如 "156"（人民币）、"840"（美元）、"978"（欧元）
//
// 返回值：
//   - 匹配到的货币条目指针；若未找到则返回 nil
func (c *ISO4217Codes) ConvertFromNumeric(numeric string) *ISO4217Entry {
	return c.numericIndex[numeric]
}
