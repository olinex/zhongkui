// Package enum 提供全项目统一的枚举值注册表。
// 所有枚举值集合在此唯一登记：每个枚举类（Type）有且仅有一个注册点，来源可以是
// 配置文件代码生成（search.toml → search-generate）或 Go 业务注册表代码字面量
// （message / notify / vtab / tagengine）。
//
// 统一契约：
//   - 类名（Type.Name）全局唯一；
//   - 类内 key 互相唯一、value 互相唯一；
//   - 数据库字段归属某枚举类时，存储 value（不可存 key）；
//   - i18n 可读 label 按 key 翻译，键统一为 enum.<类名>.<key>（共享 enum. 前缀）。
//
// 注册冲突直接 panic（启动期快速失败），避免运行期出现未知值。
package enum

import (
	// stdlib
	"fmt"
	"sort"
)

// registry 包级枚举类注册表：类名 → 枚举类。
var registry = map[string]*Type{}

// Entry 枚举条目：key 为类内唯一标识（i18n 定位依据），value 为数据库存储值。
// key 与 value 可以相同（多数枚举），也可以在"富枚举"场景下分离
// （如目标模型类：key=httpTransport、value=完整类型键）。
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Type 枚举类：name 全局唯一，Entries 内 key/value 各自唯一。
type Type struct {
	Name    string
	Entries []Entry
}

// LabelKey 返回类内 key 对应的 i18n label 键（enum.<类名>.<key>）。
// label 键共享顶层 enum. 前缀，同一类的键集中在 enum.<类名>. 下。
func (t *Type) LabelKey(key string) string {
	return "enum." + t.Name + "." + key
}

// EntryByKey 按 key 查找条目，未命中返回 false。
func (t *Type) EntryByKey(key string) (Entry, bool) {
	for _, e := range t.Entries {
		if e.Key == key {
			return e, true
		}
	}
	return Entry{}, false
}

// EntryByValue 按 value 查找条目，未命中返回 false。
func (t *Type) EntryByValue(value string) (Entry, bool) {
	for _, e := range t.Entries {
		if e.Value == value {
			return e, true
		}
	}
	return Entry{}, false
}

// Resolve 将 key 或 value 解析为条目：优先按 value 匹配（数据库存储语义），
// 其次按 key 匹配，两者均未命中返回 false。展示侧按"value → entry → key → label"
// 双解析，key 与 value 均可定位到同一 label。
func (t *Type) Resolve(kv string) (Entry, bool) {
	if e, ok := t.EntryByValue(kv); ok {
		return e, true
	}
	return t.EntryByKey(kv)
}

// Register 注册枚举类。
// 校验规则（违规直接 panic，错误信息指明冲突项）：
//   - 类名全局唯一；
//   - 类名非空；
//   - 类内 key 非空且互相唯一；
//   - 类内 value 非空且互相唯一。
func Register(t Type) {
	if t.Name == "" {
		panic("enum: register with empty type name")
	}
	if _, ok := registry[t.Name]; ok {
		panic(fmt.Sprintf("enum: duplicate type name %q", t.Name))
	}
	keys := make(map[string]struct{}, len(t.Entries))
	values := make(map[string]struct{}, len(t.Entries))
	for _, e := range t.Entries {
		if e.Key == "" {
			panic(fmt.Sprintf("enum: type %q has an entry with empty key", t.Name))
		}
		if _, ok := keys[e.Key]; ok {
			panic(fmt.Sprintf("enum: type %q has duplicate key %q", t.Name, e.Key))
		}
		keys[e.Key] = struct{}{}
		if e.Value == "" {
			panic(fmt.Sprintf("enum: type %q has an entry with empty value", t.Name))
		}
		if _, ok := values[e.Value]; ok {
			panic(fmt.Sprintf("enum: type %q has duplicate value %q", t.Name, e.Value))
		}
		values[e.Value] = struct{}{}
	}
	registry[t.Name] = &Type{Name: t.Name, Entries: t.Entries}
}

// Get 按类名返回枚举类，未注册返回 false。
func Get(name string) (*Type, bool) {
	t, ok := registry[name]
	return t, ok
}

// Exists 判断枚举类是否已注册。
func Exists(name string) bool {
	_, ok := registry[name]
	return ok
}

// List 返回全部已注册枚举类，按类名排序（输出顺序稳定）。
func List() []*Type {
	types := make([]*Type, 0, len(registry))
	for _, t := range registry {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	return types
}
