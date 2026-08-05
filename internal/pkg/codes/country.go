package codes

import (
	"encoding/json"
	"fmt"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// iso31661Codes 是 ISO 3166-1 国家/地区代码的全局单例实例，通过 sync.Once 保证仅初始化一次。
	iso31661Codes *ISO31661Codes
	// iso31661Once 用于控制 iso31661Codes 的延迟初始化，确保并发安全。
	iso31661Once sync.Once
)

// ISO31661Entry 表示 ISO 3166-1 标准中一个国家/地区的完整信息条目。
//
// ISO 3166-1 是国际标准化组织（ISO）定义的国家/地区代码标准，
// 每个条目包含该国家/地区的多种编码形式、名称及旗帜标识。
//
// 字段说明：
//   - Alpha2: 两位字母代码（如 "CN"、"US"），是使用最广泛的编码形式
//   - Alpha3: 三位字母代码（如 "CHN"、"USA"），比 Alpha2 更具可读性
//   - Flag: 该国家/地区对应的 emoji 旗帜符号（如 "🇨🇳"、"🇺🇸"）
//   - Name: 国家/地区的英文名称（如 "China"、"United States"）
//   - Numeric: 三位数字代码（如 "156"、"840"），与联合国统计司的数字代码一致
//   - OfficialName: 国家/地区的官方全称（如 "People's Republic of China"），部分条目可能为空
type ISO31661Entry struct {
	Alpha2       string `json:"alpha_2"`
	Alpha3       string `json:"alpha_3"`
	Flag         string `json:"flag"`
	Name         string `json:"name"`
	Numeric      string `json:"numeric"`
	OfficialName string `json:"official_name,omitempty"`
}

// ISO31661Data 表示 ISO 3166-1 标准数据文件的顶层结构，
// 用于从嵌入的 JSON 文件中反序列化国家/地区条目列表。
type ISO31661Data struct {
	// Countries 包含所有 ISO 3166-1 国家/地区条目。
	Countries []ISO31661Entry `json:"3166-1"`
}

// ISO31661Codes 提供 ISO 3166-1 国家/地区代码的多种维度快速查询能力。
//
// 内部维护了四个索引映射表，分别以 Alpha2、Alpha3、Name、Numeric 作为键，
// 支持从任意一种编码形式快速定位到完整的国家/地区信息条目。
// 所有索引均在初始化时一次性构建，查询操作的时间复杂度为 O(1)。
type ISO31661Codes struct {
	// alpha2Index 以两位字母代码（Alpha-2）为键的索引，如 "CN" -> China 条目。
	alpha2Index map[string]*ISO31661Entry
	// alpha3Index 以三位字母代码（Alpha-3）为键的索引，如 "CHN" -> China 条目。
	alpha3Index map[string]*ISO31661Entry
	// nameIndex 以国家/地区英文名称为键的索引，如 "China" -> China 条目。
	nameIndex map[string]*ISO31661Entry
	// numericIndex 以三位数字代码为键的索引，如 "156" -> China 条目。
	numericIndex map[string]*ISO31661Entry
}

// GetISO31661Codes 获取 ISO 3166-1 国家/地区代码查询器的全局单例。
//
// 首次调用时会从嵌入的 JSON 数据文件中加载并解析所有国家/地区数据，
// 构建各维度索引。后续调用直接返回已缓存的实例，不再重复加载。
// 如果加载过程中发生错误，将触发 panic，因为这是不可恢复的初始化错误。
func GetISO31661Codes() *ISO31661Codes {
	iso31661Once.Do(func() {
		codes, err := newISO31661Codes()
		if err != nil {
			panic(err)
		}
		iso31661Codes = codes
	})
	return iso31661Codes
}

// newISO31661Codes 从嵌入的 ISO 3166-1 标准数据文件中加载国家/地区数据，
// 并构建 Alpha2、Alpha3、Name、Numeric 四个维度的索引映射表。
//
// 在构建索引的过程中，会检测数据中是否存在重复的键值，
// 若发现重复将直接 panic，以保证索引数据的唯一性和正确性。
//
// 注意：Alpha2 索引允许条目的 Alpha2 为空（某些特殊地区可能没有 Alpha2 代码），
// 但 Alpha3、Name、Numeric 均为必填字段，重复时将触发 panic。
func newISO31661Codes() (*ISO31661Codes, error) {
	file, err := embeddedStandards.Open(CountryArea)
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
	var data ISO31661Data
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a2Index := make(map[string]*ISO31661Entry, 0)
	a3Index := make(map[string]*ISO31661Entry, len(data.Countries))
	nIndex := make(map[string]*ISO31661Entry, len(data.Countries))
	nmIndex := make(map[string]*ISO31661Entry, len(data.Countries))
	for _, entry := range data.Countries {
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
		if _, ok := nmIndex[entry.Numeric]; ok {
			panic(fmt.Sprintf(`Numeric %s was duplicated`, entry.Numeric))
		}
		nmIndex[entry.Numeric] = &entry
	}
	return &ISO31661Codes{
		alpha2Index:  a2Index,
		alpha3Index:  a3Index,
		nameIndex:    nIndex,
		numericIndex: nmIndex,
	}, nil
}

// ConvertFromAlpha2 根据两位字母代码（Alpha-2）查询国家/地区信息。
//
// 参数：
//   - alpha2: ISO 3166-1 Alpha-2 代码，如 "CN"、"US"、"JP"
//
// 返回值：
//   - 匹配到的国家/地区条目指针；若未找到则返回 nil
func (c *ISO31661Codes) ConvertFromAlpha2(alpha2 string) *ISO31661Entry {
	return c.alpha2Index[alpha2]
}

// ConvertFromAlpha3 根据三位字母代码（Alpha-3）查询国家/地区信息。
//
// 参数：
//   - alpha3: ISO 3166-1 Alpha-3 代码，如 "CHN"、"USA"、"JPN"
//
// 返回值：
//   - 匹配到的国家/地区条目指针；若未找到则返回 nil
func (c *ISO31661Codes) ConvertFromAlpha3(alpha3 string) *ISO31661Entry {
	return c.alpha3Index[alpha3]
}

// ConvertFromName 根据国家/地区英文名称查询国家/地区信息。
//
// 参数：
//   - name: 国家/地区的英文名称，如 "China"、"United States"、"Japan"
//
// 返回值：
//   - 匹配到的国家/地区条目指针；若未找到则返回 nil
func (c *ISO31661Codes) ConvertFromName(name string) *ISO31661Entry {
	return c.nameIndex[name]
}

// ConvertFromNumeric 根据三位数字代码查询国家/地区信息。
//
// 参数：
//   - numeric: ISO 3166-1 数字代码，如 "156"（中国）、"840"（美国）、"392"（日本）
//
// 返回值：
//   - 匹配到的国家/地区条目指针；若未找到则返回 nil
func (c *ISO31661Codes) ConvertFromNumeric(numeric string) *ISO31661Entry {
	return c.numericIndex[numeric]
}
