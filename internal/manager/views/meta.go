// 本文件提供接口结构体元信息的反射提取与 OPTIONS 接口暴露能力：
//   - FieldMeta/StructMeta/ApiMeta：描述字段与接口的元信息数据结构
//   - getStructFields/getStructMeta：通过反射提取结构体字段标签中的 JSON 字段名、
//     binding 校验规则、字段类型、语义 kind、required 与默认值；该提取是 OPTIONS
//     元信息与多态类型接口（如通知渠道配置）共用的唯一事实来源
//   - StructMetaGinHandler：泛型 Gin 处理器，配合 WithHeaderInput/WithHeaderOutput/
//     WithBodyInput/WithBodyOutput 四个可选配置项，将接口的请求/响应结构体元信息
//     提交给前端，用于动态渲染表单、表格与校验规则
//   - ListMetaGinHandler：列表接口专用处理器，在搜索选项与表格列元数据之外，
//     汇总各 HTTP 方法的结构体元信息
//
// 字段展示文案（label）不由本文件产出：前端依据字段名与业务上下文自行拼接 i18n key 翻译。
package views

import (
	// 标准库
	"net/http"
	"reflect"
	"strings"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// Kind 描述字段的语义类型，供前端选择相应的表单控件。
type Kind string

const (
	// KindString 单行文本
	KindString Kind = "string"
	// KindNumber 数值
	KindNumber Kind = "number"
	// KindPassword 密码输入
	KindPassword Kind = "password"
	// KindTextarea 多行文本
	KindTextarea Kind = "textarea"
	// KindObject 对象/结构化数据（通常以 JSON 文本编辑）
	KindObject Kind = "object"
	// KindColumns 列定义列表：形如 [{name, type}] 的手动 schema 编辑器（数据源接入用）
	KindColumns Kind = "columns"
	// KindBoolean 布尔开关
	KindBoolean Kind = "boolean"
	// KindCrypto 加密字段：值保存为 base64 密文（经 meta options 下发前端，明文需走明文权限接口）
	KindCrypto Kind = "crypto"
)

// FieldMeta 描述接口结构体中单个字段的元信息。
// 通过反射从结构体字段标签中提取，供前端动态渲染表单与校验规则。
// 字段展示文案（label）不由后端提供：前端依据字段名与业务上下文自行拼接 i18n key 翻译。
type FieldMeta struct {
	// Name JSON 序列化字段名，取自 json 标签（不含 omitempty 等选项）
	Name string `json:"name"`
	// Type 字段的 Go 数据类型名，指针类型取其元素类型名
	Type string `json:"type"`
	// Binding binding 校验规则，取自 binding 标签
	Binding string `json:"binding,omitempty"`
	// Kind 前端表单字段语义类型，取自 form 标签 kind（缺省时按 Go 类型推断）
	Kind Kind `json:"kind"`
	// Required 是否必填（binding 标签包含 required 时为 true）
	Required bool `json:"required"`
	// DefaultValue 默认值，取自 default 标签（按 Kind 解析），可选
	DefaultValue any `json:"defaultValue,omitempty"`
	// Enum 是否为可选枚举值字段：为 true 时前端应以下拉形式展示，并通过 EnumType 指定的枚举类获取条目
	Enum bool `json:"enum,omitempty"`
	// EnumType 该枚举字段的枚举类名（仅 Enum 为 true 时提供）；
	// 条目经统一枚举接口 /admin/enum/get 按类名获取（形状统一为 [{key, value}]），
	// label 由 key 经 enum.<类名>.<key> 翻译展示，value 为提交/存储值。
	EnumType string `json:"enumType,omitempty"`
}

// StructMeta 描述单个接口结构体的元信息。
// 包含结构体类型名与其所有字段的元信息列表。
type StructMeta struct {
	Name   string       `json:"name"`   // 结构体类型名
	Fields []*FieldMeta `json:"fields"` // 字段元信息列表
}

// MethodMeta 描述某个 HTTP 方法对应的接口元信息。
// 同一路径下不同方法（如 GET、POST）可能接收不同的入参并返回不同的出参。
type MethodMeta struct {
	HeaderInput  *StructMeta `json:"headerInput"`  // 请求头输入结构体元信息
	HeaderOutput *StructMeta `json:"headerOutput"` // 响应头输出结构体元信息
	BodyInput    *StructMeta `json:"bodyInput"`    // 请求体输入结构体元信息
	BodyOutput   *StructMeta `json:"bodyOutput"`   // 响应体输出结构体元信息
}

// ApiMeta 描述一个接口路径下所有 HTTP 方法的元信息。
// key 为 HTTP 方法名（如 GET、POST），value 为对应方法的接口元信息；
// 未指定结构体的部分在 JSON 序列化时输出为 null。
type ApiMeta map[string]*MethodMeta

// metadataOptions 保存元信息接口配置中可选的结构体类型集合。
type metadataOptions struct {
	headerInput  reflect.Type
	headerOutput reflect.Type
	bodyInput    reflect.Type
	bodyOutput   reflect.Type
	// methods 保存按 HTTP 方法分组的方法级配置，key 为 HTTP 方法名（如 GET、POST）
	methods map[string]*metadataOptions
}

// MetadataOption 元信息接口配置选项，用于指定接口各 HTTP 方法下的结构体类型。
type MetadataOption func(*metadataOptions)

// WithMethod 设置指定 HTTP 方法对应的接口结构体类型。
// 同一路径下存在多个方法（如 GET 与 POST）时，为每个方法分别调用一次；
// 该方法没有任何入参出参时可不传子选项。
//
// 典型用法：
//
//	adminGroup.OPTIONS("/user/:unionID/get", StructMetaGinHandler(
//		WithMethod(http.MethodPost, WithBodyOutput[UserDetailOut]()),
//	))
func WithMethod(method string, opts ...MetadataOption) MetadataOption {
	return func(o *metadataOptions) {
		if o.methods == nil {
			o.methods = make(map[string]*metadataOptions)
		}
		sub := &metadataOptions{}
		for _, opt := range opts {
			opt(sub)
		}
		o.methods[method] = sub
	}
}

// WithHeaderInput 设置接口的请求头输入结构体类型。
// 支持传入值类型或指针类型，如 WithHeaderInput[UserGroupIn]()。
func WithHeaderInput[T any]() MetadataOption {
	return func(o *metadataOptions) {
		o.headerInput = reflect.TypeFor[T]()
	}
}

// WithHeaderOutput 设置接口的响应头输出结构体类型。
// 支持传入值类型或指针类型。
func WithHeaderOutput[T any]() MetadataOption {
	return func(o *metadataOptions) {
		o.headerOutput = reflect.TypeFor[T]()
	}
}

// WithBodyInput 设置接口的请求体输入结构体类型。
// 支持传入值类型或指针类型，如 WithBodyInput[UserGroupIn]()。
func WithBodyInput[T any]() MetadataOption {
	return func(o *metadataOptions) {
		o.bodyInput = reflect.TypeFor[T]()
	}
}

// WithBodyOutput 设置接口的响应体输出结构体类型。
// 支持传入值类型或指针类型，如 WithBodyOutput[UserDetailOut]()。
func WithBodyOutput[T any]() MetadataOption {
	return func(o *metadataOptions) {
		o.bodyOutput = reflect.TypeFor[T]()
	}
}

// StructMetaGinHandler 创建一个返回接口结构体元信息的 Gin 处理器（配合 OPTIONS 方法使用）。
// 通过 WithMethod 按 HTTP 方法指定接口的请求头输入/输出、请求体输入/输出四类可选结构体，
// 同一路径下所有方法的结果汇总在一个 ApiMeta（map）中返回；
// 未注册任何方法时返回空 map，确保前端调用 OPTIONS 不会报错。
//
// 典型用法：
//
//	adminGroup.OPTIONS("/user/:unionID/get", StructMetaGinHandler(
//		WithMethod(http.MethodPost, WithBodyOutput[UserDetailOut]()),
//	))
func StructMetaGinHandler(opts ...MetadataOption) gin.HandlerFunc {
	options := &metadataOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return func(c *gin.Context) {
		response.SimpleOK.AbortWithGinW(c, buildApiMeta(options))
	}
}

// buildApiMeta 根据已应用的元信息选项生成 ApiMeta。
// 按 HTTP 方法分组汇总请求头输入/输出、请求体输入/输出四类结构体元信息；
// 未注册任何方法时返回空 map。
//
// 返回值：
//   - meta: 按 HTTP 方法分组的接口元信息
func buildApiMeta(options *metadataOptions) ApiMeta {
	meta := make(ApiMeta, len(options.methods))
	for method, sub := range options.methods {
		meta[method] = &MethodMeta{
			HeaderInput:  getStructMeta(sub.headerInput),
			HeaderOutput: getStructMeta(sub.headerOutput),
			BodyInput:    getStructMeta(sub.bodyInput),
			BodyOutput:   getStructMeta(sub.bodyOutput),
		}
	}
	return meta
}

// ListMeta 列表接口的自省信息响应结构。
// 在搜索选项（可排序/可过滤字段）与表格列元数据的基础上，
// 扩展各 HTTP 方法的结构体元信息，前端通过一次 OPTIONS 请求即可同时
// 获取搜索控件与表单/详情渲染所需的数据。
type ListMeta struct {
	utils.SearchOptions                 // 嵌入搜索选项：可排序字段（orderBy）与可过滤字段（filterBy）
	Headers             []*HeaderOption `json:"headers"` // 表格列元数据
	Meta                ApiMeta         `json:"meta"`    // 各 HTTP 方法的结构体元信息
}

// ListMetaGinHandler 创建一个返回列表接口自省信息的 Gin 处理器（配合 OPTIONS 方法使用）。
// 类型参数 M 为搜索输入结构体、N 为列表输出项结构体，用于生成搜索选项与表格列元数据；
// 并自动以 M/N 注册 POST 方法的请求体输入/输出元信息，可通过 WithMethod 补充其他
// 方法或请求头结构体，显式注册的 POST 方法优先级更高。
//
// 响应体结构：
//
//	{
//	  "code": 200,
//	  "data": {
//	    "orderBy": ["field1", "field2"],
//	    "filterBy": [{"name": "field1", "type": "string", "actions": ["eq", "ctn"]}],
//	    "headers": [{"name": "id", "type": "uint"}],
//	    "meta": {
//	      "POST": {"bodyInput": {"name": "SearchIn", "fields": [...]}, "bodyOutput": {...}}
//	    }
//	  }
//	}
func ListMetaGinHandler[M utils.SearcherIn, N utils.TotalOut](opts ...MetadataOption) gin.HandlerFunc {
	options := &metadataOptions{}
	for _, opt := range opts {
		opt(options)
	}
	// 列表接口的 POST 入参出参即搜索输入与列表输出结构体；显式注册的 POST 方法优先级更高
	if _, exists := options.methods[http.MethodPost]; !exists {
		WithMethod(http.MethodPost, WithBodyInput[M](), WithBodyOutput[N]())(options)
	}
	return func(c *gin.Context) {
		var in M
		var out N
		listMeta := &ListMeta{
			SearchOptions: *in.GetOptions(),
			Headers:       getJSONTagAndType(reflect.TypeOf(out)),
			Meta:          buildApiMeta(options),
		}
		response.SimpleOK.AbortWithGinW(c, listMeta)
	}
}

// getStructMeta 通过反射提取结构体的完整元信息。
// 未指定结构体类型时返回 nil；字段元信息交由 getStructFields 生成。
//
// 参数：
//   - t: 目标结构体的 reflect.Type，支持传入指针类型或 nil
//
// 返回值：
//   - meta: 提取到的结构体元信息
func getStructMeta(t reflect.Type) *StructMeta {
	if t == nil {
		return nil
	}
	// 如果传入的是指针类型，解引用获取实际类型名
	tt := t
	if tt.Kind() == reflect.Pointer {
		tt = tt.Elem()
	}
	return &StructMeta{
		Name:   tt.Name(),
		Fields: GetStructFields(t),
	}
}

// getStructFields 通过反射提取结构体的字段元信息列表（OPTIONS 元信息与多态类型
// 接口共用的唯一事实来源）。
// 自动解引用指针类型，递归展开匿名嵌入结构体的字段。
// 规则：
//   - 字段名：优先 json 标签（剥离 omitempty 等选项），无 json 标签时回退取
//     mapstructure 标签（搜索输入结构体使用），两者皆无或为 "-" 的字段被忽略。
//   - Kind：数值→number、bool→boolean、map/切片/结构体/数组→object、其余→string；
//     form 标签的 kind 可覆盖默认推断。
//   - Required：binding 标签包含 "required" 即 true。
//   - DefaultValue：取 default 标签，按 Kind 解析为数值/布尔/字符串。
//   - Type：字段 Go 类型字符串，指针类型取其元素类型。
//   - Enum/EnumType：按结构体类型名查 enumBindings 绑定表（typeName → field → 枚举类名），命中则标记为枚举字段并附带枚举类名。
//
// 未指定结构体类型或类型非结构体时返回 nil。
func GetStructFields(t reflect.Type) []*FieldMeta {
	if t == nil {
		return nil
	}
	tt := t
	if tt.Kind() == reflect.Pointer {
		tt = tt.Elem()
	}
	if tt.Kind() != reflect.Struct {
		return nil
	}
	// 枚举注册表按结构体类型名索引；匿名嵌入字段递归时沿用外层类型名，以保证
	// 注册表声明以具体（外层）结构体类型名为准。
	return getStructFieldsNamed(tt, tt.Name())
}

// getStructFieldsNamed 提取结构体字段元信息，并以 enumTypeName 为外键查询枚举注册表。
// 匿名嵌入字段递归展开时沿用传入的 enumTypeName（即外层结构体类型名）。
func getStructFieldsNamed(t reflect.Type, enumTypeName string) []*FieldMeta {
	if t == nil {
		return nil
	}
	tt := t
	if tt.Kind() == reflect.Pointer {
		tt = tt.Elem()
	}
	if tt.Kind() != reflect.Struct {
		return nil
	}

	classBindings := enumBindings[enumTypeName]
	fields := make([]*FieldMeta, 0, tt.NumField())
	for field := range tt.Fields() {
		// 处理匿名嵌入字段：递归展开其内部字段
		if field.Anonymous {
			embeddedType := field.Type
			// 嵌入字段为指针时，解引用获取实际类型
			if embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			// 仅对结构体类型的嵌入字段递归展开
			if embeddedType.Kind() == reflect.Struct {
				fields = append(fields, getStructFieldsNamed(embeddedType, enumTypeName)...)
				continue
			}
		}

		fieldKey := structFieldName(field)
		if fieldKey == "" {
			continue
		}

		binding := field.Tag.Get("binding")
		kv := parseFormTag(field.Tag.Get("form"))
		kind := kindForType(field.Type)
		if v, ok := kv["kind"]; ok && v != "" {
			kind = Kind(v)
		}
		var def any
		if v := field.Tag.Get("default"); v != "" {
			def = parseDefault(kind, v)
		}

		// 获取字段类型名称，指针类型取其元素类型名称
		fieldType := field.Type.String()
		if field.Type.Kind() == reflect.Pointer {
			fieldType = field.Type.Elem().String()
		}

		meta := &FieldMeta{
			Name:         fieldKey,
			Type:         fieldType,
			Binding:      binding,
			Kind:         kind,
			Required:     strings.Contains(binding, "required"),
			DefaultValue: def,
		}
		// 按 JSON 字段名查枚举类绑定表，命中则标注为枚举字段并附带枚举类名
		if className, ok := classBindings[fieldKey]; ok && className != "" {
			meta.Enum = true
			meta.EnumType = className
		}
		fields = append(fields, meta)
	}
	return fields
}

// structFieldName 提取结构体字段名：优先 json 标签，回退 mapstructure 标签，
// 两者皆无或为 "-" 时返回空串。
func structFieldName(field reflect.StructField) string {
	key := field.Tag.Get("json")
	if key == "" || key == "-" {
		if m := field.Tag.Get("mapstructure"); m != "" && m != "-" {
			key = m
		} else {
			return ""
		}
	}
	if i := strings.Index(key, ","); i != -1 {
		key = key[:i]
	}
	return key
}

// kindForType 依据 Go 类型推断字段的默认 kind。
func kindForType(t reflect.Type) Kind {
	switch t.Kind() {
	case reflect.Bool:
		return KindBoolean
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return KindNumber
	case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct:
		return KindObject
	default:
		return KindString
	}
}

// parseFormTag 解析 form 标签：";" 分隔的 k=v 列表。
func parseFormTag(tag string) map[string]string {
	kv := make(map[string]string)
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return kv
}

// parseDefault 依据 kind 将 form 标签的 default 字符串解析为数值/布尔/字符串。
func parseDefault(kind Kind, v string) any {
	switch kind {
	case KindNumber:
		if n, err := utils.ParseScalar(utils.ScalarKindInt, v); err == nil {
			return n
		}
		if f, err := utils.ParseScalar(utils.ScalarKindFloat, v); err == nil {
			return f
		}
		return v
	case KindBoolean:
		if b, err := utils.ParseScalar(utils.ScalarKindBool, v); err == nil {
			return b
		}
		return v
	default:
		return v
	}
}
