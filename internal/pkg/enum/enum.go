// Package enum 提供全项目统一的枚举值注册表。
//
// 所有枚举值集合在此唯一登记：每个枚举类（Type）有且仅有一个注册点，且该注册点一律落在取值空间的
// 归属包（即取值空间的定义处）。
//
// 统一契约：
//   - 类名（Type.Name）全局唯一；
//   - 类内 key 互相唯一、value 互相唯一；
//   - 数据库字段归属某枚举类时，存储 value（不可存 key）；
//   - i18n 可读 label 按 key 翻译，键统一为 `enum.{类名}.{key}`（共享 enum. 前缀）。
//
// 注册冲突直接 panic（启动期快速失败），避免运行期出现未知值。
//
// ## 一、三类枚举与注册形态
//
// 按取值空间的归属，枚举分三类：
//
//  1. 取值空间固定的纯声明类：取值空间就地声明在归属包内（类型化取值常量或包内既有清单），在包内
//     init() 经 enum.EntriesOf + enum.Register 在注册时快照条目。若包内已有取值序列（如
//     base.SupportLanguages、base.EffectAllow），直接复用为条目来源，不重复声明常量。示例见
//     internal/pkg/base/enum.go（language / policyEffect）。
//  2. 取值空间取自业务注册表的类：值空间随业务注册表在代码内固定，由所属业务包经
//     enum.RegisterSource(类名, func() []Entry) 声明条目来源；条目不在注册时快照，而在每次读取
//     （Get / List）时从业务注册表实时派生，避免包内多 init() 的文件顺序影响条目完整性。示例见
//     internal/pkg/message/enum.go（channelType 取自渠道工厂 Types()）、internal/pkg/sqlitevtab/vtab/
//     enum.go（datasourceType 取自插件注册表 DatasourceTypeKeys()）、internal/notify/enum.go
//     （notificationEvent 取自事件模板注册表 TemplateSpecs()）。
//  3. 跨层共用枚举：不写进 search.toml 的 [[enums]]，也不写进业务包或 pkg 域包，由消费方 import
//     归属包引用其常量（如 tagengine 的模型键）。
//
// 归属判据是「取值空间的拥有者」而非存储位置：取值空间仅作为某模型列的值域存在时归该模型包，
// 由某个非 model 子系统定义/使用的词汇归该子系统包，禁止因「存在同名模型列」而迁入 model。
//
// ## 二、分步清单：新增一个枚举类
//
//  1. 确定归属包（取值空间的定义处），在该包内声明取值常量/清单，或复用包内既有清单。
//
//  2. 在包内 init() 注册，两种形态二选一：
//
//     // 取值空间固定：注册时快照条目
//     enum.Register(enum.Type{Name: "policyEffect", Entries: enum.EntriesOf(policyEffectValues)})
//     // 业务注册表来源：每次读取时从业务注册表实时派生
//     enum.RegisterSource("channelType", func() []enum.Entry { return enum.EntriesOf(Types()) })
//
//     类名须全局唯一；类内 key/value 各自非空且唯一，违规 panic。
//
//  3. 补双语 label：在 internal/pkg/translate/locales/zh-Hans.toml 与 en.toml 补
//     `[enum.{类名}.{key}]`，中英结构一一对应；只允许经 key 翻译，禁止经 value 翻译。
//
//  4. 若字段需要下发 enumType 供前端定位枚举类，按下节把「结构体字段 → 枚举类名」写入 search.toml。
//
//  5. 用 go test ./internal/manager/views/ -run TestEnumClassesHaveBilingualLabels 验证。
//
// ## 三、视图层「字段 → 枚举类名」绑定（生成器）
//
// 枚举取值空间不在视图层声明：search.toml 的 [[enums]] 只声明「视图结构体类型 + 字段 JSON 名 →
// 枚举类名」，经 search-generate 生成 `internal/manager/views/enum_bindings.go` 的 enumBindings 表；
// meta.go 反射 In/Out 字段时输出 FieldMeta.EnumType，前端据此定位枚举类并渲染 `enum.{类名}.{key}`
// 文案。该生成由 make build 自动触发（见 Makefile 中 search.go / enum_bindings.go 的生成规则），
// 等价命令为：
//
//	go run cmd/template/main.go search-generate \
//	    --packageName="views" \
//	    --templatePath="cmd/template/base.tmpl" \
//	    --configPath="internal/manager/views/search.toml" \
//	    --outputPath="internal/manager/views/search.go" \
//	    --enumBindingsTemplatePath="cmd/template/enum_bindings.tmpl" \
//	    --enumBindingsOutputPath="internal/manager/views/enum_bindings.go"
//
// [[enums]] 的 set 仅为类名引用，不携带可选项——可选项唯一来源是本注册表。无法写进 toml 的运行时
// 字段（如 NotificationTemplateCreateIn.event）经 views.AddEnumBinding(typeName, field, className)
// 在对应模块 init() 补充（见 internal/manager/views/enum_registry.go）。
//
// ## 四、读取接口与一致性守卫
//
// 读取接口：enum.Get(name) 取单类（条目已就绪）、enum.List() 取全部（按类名排序，输出稳定）、
// enum.Exists(name) 判存在；Type 上提供 LabelKey / EntryByKey / EntryByValue / Resolve（优先按
// value、再按 key 双解析）。统一枚举接口 POST /admin/enum/get|list 出参条目为 {key, value, label}，
// label 跟随请求语言。
//
// 一致性由 `internal/manager/views/enum_i18n_test.go` 的 TestEnumClassesHaveBilingualLabels 兜底：
// 遍历 enum.List() 断言每个类每个条目的 `enum.{类名}.{key}` 在每种支持语言下均可解析（不等于 key），
// 并断言注册表非空、每类至少一条目。该测试经 blank import 归属包触发其 init() 注册，新增归属包
// 必须同步加入该测试的 import 清单，否则其枚举类会静默缺失。
package enum

import (
	// 标准库
	"fmt"
	"sort"
)

// registry 包级枚举类注册表：类名 → 枚举类。
var registry = map[string]*Type{}

// Entry 枚举条目：key 为类内唯一标识（i18n 定位依据），value 为数据库存储值。
// key 与 value 可以相同（多数枚举），也可以在"富枚举"场景下分离
// （如目标模型类：key=httpTransport、value=完整类型键）。
type Entry struct {
	// Key 类内唯一标识，作为 i18n label 的定位依据
	Key string `json:"key"`
	// Value 数据库存储值
	Value string `json:"value"`
}

// Type 枚举类：name 全局唯一，Entries 内 key/value 各自唯一。
type Type struct {
	// Name 枚举类名，全局唯一
	Name string
	// Entries 类内枚举条目列表，key/value 各自唯一
	Entries []Entry
}

// EntriesOf 把「key 与 value 相同」的取值序列转为枚举条目列表（多数枚举类即此形态），
// 使按类声明取值空间的枚举类在注册时直接复用同一份取值序列，无需在注册处重抄一遍字面量。
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
