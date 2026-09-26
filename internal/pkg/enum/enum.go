// Package enum 提供全项目统一的枚举值注册表。
// 所有枚举值集合在此唯一登记：每个枚举类（Type）有且仅有一个注册点，且该注册点一律落在取值空间的
// 归属包（即取值空间的定义处），声明形态有两类——取值空间固定的类经 Register 在注册时快照条目
// （取值常量/清单就地声明在同一包内）；取值空间取自业务注册表的类（渠道类型、通知事件、数据源类型、
// 目标模型等）由所属业务包经 RegisterSource 声明条目来源，条目在读取时从业务注册表派生，避免包内
// init 顺序影响条目完整性。
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

// EntriesOf 把「key 与 value 相同」的取值序列转为枚举条目列表（多数枚举类即此形态），
// 使按类声明取值空间的枚举类在注册时直接复用同一份取值序列，无需在注册处重抄一遍字面量。
// EntriesOf converts a value sequence whose key equals its value (the shape of most enum classes)
// into enum entries, so an enum class declaring its value space in code reuses that very sequence
// at registration time instead of repeating the literals.
func EntriesOf[T ~string](values []T) []Entry {
	entries := make([]Entry, 0, len(values))
	for _, value := range values {
		entries = append(entries, Entry{Key: string(value), Value: string(value)})
	}
	return entries
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

// sources 惰性条目来源表：类名 → 条目派生函数（见 RegisterSource）。
// 仅在包 init 阶段写入，运行期只读，故无需加锁。
var sources = map[string]func() []Entry{}

// Register 注册枚举类，条目在注册时快照。
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
	validateEntries(t.Name, t.Entries)
	registry[t.Name] = &Type{Name: t.Name, Entries: t.Entries}
}

// RegisterSource 注册一个取值空间取自业务注册表的枚举类：条目不在注册时快照，而在每次读取
// （Get / List）时经 entries 从业务注册表实时派生。适用于来源注册表由同一包内多个 init() 依次
// 填充的场景（如渠道工厂、数据源插件），使条目完整性不取决于包内 init 的文件顺序；取值空间在
// 运行生命周期内仍固定（无动态枚举）。类名唯一性、非空与 entries 非 nil 的约束同 Register，
// 违规直接 panic。
// RegisterSource registers an enum class whose value space is derived from a business registry:
// the entries are not snapshotted at registration time but derived on every read (Get / List) via
// entries. It fits registries filled by several init() functions of one package (channel
// factories, datasource plugins), so entry completeness never depends on the in-package init file
// order; the value space stays fixed for the process lifetime (no dynamic enums). Name
// uniqueness/non-emptiness and a non-nil entries are enforced like Register, panicking on violation.
func RegisterSource(name string, entries func() []Entry) {
	if name == "" {
		panic("enum: register source with empty type name")
	}
	if entries == nil {
		panic(fmt.Sprintf("enum: type %q registered with a nil entries source", name))
	}
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("enum: duplicate type name %q", name))
	}
	registry[name] = &Type{Name: name}
	sources[name] = entries
}

// validateEntries 校验类内条目 key/value 非空且各自唯一，违规直接 panic。
// validateEntries validates that the keys and values of a class are non-empty and mutually
// unique, panicking on violation.
func validateEntries(name string, entries []Entry) {
	keys := make(map[string]struct{}, len(entries))
	values := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.Key == "" {
			panic(fmt.Sprintf("enum: type %q has an entry with empty key", name))
		}
		if _, ok := keys[e.Key]; ok {
			panic(fmt.Sprintf("enum: type %q has duplicate key %q", name, e.Key))
		}
		keys[e.Key] = struct{}{}
		if e.Value == "" {
			panic(fmt.Sprintf("enum: type %q has an entry with empty value", name))
		}
		if _, ok := values[e.Value]; ok {
			panic(fmt.Sprintf("enum: type %q has duplicate value %q", name, e.Value))
		}
		values[e.Value] = struct{}{}
	}
}

// resolve 返回类的有效条目：注册了条目来源的类经来源实时派生并校验，其余用注册时快照。
// resolve returns the effective entries of a class: classes with an entries source derive and
// validate them on the fly, the others use the registration-time snapshot.
func resolve(t *Type) *Type {
	src, ok := sources[t.Name]
	if !ok {
		return t
	}
	entries := src()
	validateEntries(t.Name, entries)
	return &Type{Name: t.Name, Entries: entries}
}

// Get 按类名返回枚举类（条目已就绪），未注册返回 false。
func Get(name string) (*Type, bool) {
	t, ok := registry[name]
	if !ok {
		return nil, false
	}
	return resolve(t), true
}

// Exists 判断枚举类是否已注册。
func Exists(name string) bool {
	_, ok := registry[name]
	return ok
}

// List 返回全部已注册枚举类（条目已就绪），按类名排序（输出顺序稳定）。
func List() []*Type {
	types := make([]*Type, 0, len(registry))
	for _, t := range registry {
		types = append(types, resolve(t))
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	return types
}
