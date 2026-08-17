// 本文件提供接口结构体元信息的反射提取与 OPTIONS 接口暴露能力：
//   - FieldMeta/StructMeta/ApiMeta：描述字段与接口的元信息数据结构
//   - getStructMeta：通过反射提取结构体字段标签中的 JSON 字段名、binding 校验规则
//     与字段数据类型，并经 i18n 翻译 key 解析字段 comment（支持多语言）
//   - StructMetaGinHandler：泛型 Gin 处理器，配合 WithHeaderInput/WithHeaderOutput/
//     WithBodyInput/WithBodyOutput 四个可选配置项，将接口的请求/响应结构体元信息
//     提交给前端，用于动态渲染表单、表格与校验规则
//   - ListMetaGinHandler：列表接口专用处理器，在搜索选项与表格列元数据之外，
//     汇总各 HTTP 方法的结构体元信息
package routes

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// FieldMeta 描述接口结构体中单个字段的元信息。
// 通过反射从结构体字段标签中提取，供前端动态渲染表单与校验规则。
type FieldMeta struct {
	Name    string `json:"name"`    // JSON 序列化字段名，取自 json 标签（不含 omitempty 等选项）
	Type    string `json:"type"`    // 字段的 Go 数据类型名，指针类型取其元素类型名
	Binding string `json:"binding"` // binding 校验规则，取自 binding 标签
	Comment string `json:"comment"` // 字段注释，经 i18n 按 key "model.{StructName}.field.{jsonName}" 翻译得到
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
// 字段 comment 按当前请求语言（Accept-Language）翻译。
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
		manager.SimpleOK.AbortWithGinW(c, buildApiMeta(options, manager.GetLocalizer(c)))
	}
}

// buildApiMeta 根据已应用的元信息选项生成 ApiMeta。
// 按 HTTP 方法分组汇总请求头输入/输出、请求体输入/输出四类结构体元信息；
// 未注册任何方法时返回空 map。
//
// 参数：
//   - options: 已应用全部元信息选项的配置
//   - lc: 当前请求的 i18n Localizer，用于按请求语言翻译字段 comment
//
// 返回值：
//   - meta: 按 HTTP 方法分组的接口元信息
func buildApiMeta(options *metadataOptions, lc *i18n.Localizer) ApiMeta {
	meta := make(ApiMeta, len(options.methods))
	for method, sub := range options.methods {
		meta[method] = &MethodMeta{
			HeaderInput:  getStructMeta(sub.headerInput, lc),
			HeaderOutput: getStructMeta(sub.headerOutput, lc),
			BodyInput:    getStructMeta(sub.bodyInput, lc),
			BodyOutput:   getStructMeta(sub.bodyOutput, lc),
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
			Meta:          buildApiMeta(options, manager.GetLocalizer(c)),
		}
		manager.SimpleOK.AbortWithGinW(c, listMeta)
	}
}

// getStructMeta 通过反射提取结构体的完整元信息。
// 自动解引用指针类型，递归展开匿名嵌入结构体的字段。
// 字段 key 优先取 json 标签（剥离 omitempty 等选项），无 json 标签时回退取
// mapstructure 标签（搜索输入结构体使用），两者皆无或为 "-" 的字段被忽略。
// 字段 comment 经 i18n 翻译 key "model.{StructName}.field.{key}" 解析，
// 其中 StructName 为声明该字段的结构体类型名（嵌入字段取嵌入结构体类型名），
// 翻译 key 未命中时回退返回 key 本身。
// 未指定结构体类型时返回 nil。
//
// 参数：
//   - t: 目标结构体的 reflect.Type，支持传入指针类型或 nil
//   - lc: 当前请求的 i18n Localizer，用于按请求语言翻译字段 comment
//
// 返回值：
//   - meta: 提取到的结构体元信息
func getStructMeta(t reflect.Type, lc *i18n.Localizer) *StructMeta {
	if t == nil {
		return nil
	}
	// 如果传入的是指针类型，解引用获取实际类型
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	meta := &StructMeta{
		Name:   t.Name(),
		Fields: make([]*FieldMeta, 0),
	}

	for field := range t.Fields() {
		// 处理匿名嵌入字段：递归展开其内部字段
		if field.Anonymous {
			embeddedType := field.Type
			// 嵌入字段为指针时，解引用获取实际类型
			if embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			// 仅对结构体类型的嵌入字段递归展开
			if embeddedType.Kind() == reflect.Struct {
				meta.Fields = append(meta.Fields, getStructMeta(embeddedType, lc).Fields...)
				continue
			}
		}

		// 获取字段 key：优先 json 标签，回退 mapstructure 标签，
		// 忽略无标签或显式标记为 "-" 的字段
		fieldKey := field.Tag.Get("json")
		if fieldKey == "" || fieldKey == "-" {
			if mapTag := field.Tag.Get("mapstructure"); mapTag != "" && mapTag != "-" {
				fieldKey = mapTag
			} else {
				continue
			}
		}

		// 处理 json 标签中的选项部分（如 "name,omitempty"），只取逗号前的名称
		if commaIdx := strings.Index(fieldKey, ","); commaIdx != -1 {
			fieldKey = fieldKey[:commaIdx]
		}

		// 获取字段类型名称，指针类型取其元素类型名称
		fieldType := field.Type.String()
		if field.Type.Kind() == reflect.Pointer {
			fieldType = field.Type.Elem().String()
		}

		meta.Fields = append(meta.Fields, &FieldMeta{
			Name:    fieldKey,
			Type:    fieldType,
			Binding: field.Tag.Get("binding"),
			Comment: localizeFieldComment(lc, t.Name(), fieldKey),
		})
	}
	return meta
}

// fieldCommentKey 生成字段 comment 的 i18n 翻译 key。
// 格式为 "model.{StructName}.field.{jsonName}"，如 model.UserListOut.field.username。
// 翻译文案维护在 internal/pkg/translate/locales/ 的 zh-Hans.toml 与 en.toml。
func fieldCommentKey(structName, jsonName string) string {
	return fmt.Sprintf("model.%s.field.%s", structName, jsonName)
}

// localizeFieldComment 按请求语言的 Localizer 翻译字段 comment。
// 翻译 key 未命中时回退返回 key 本身，便于前端识别未提供翻译的字段。
//
// 参数：
//   - lc: 当前请求的 i18n Localizer
//   - structName: 声明该字段的结构体类型名
//   - jsonName: 字段的 JSON 序列化名称
//
// 返回值：
//   - string: 翻译后的字段注释
func localizeFieldComment(lc *i18n.Localizer, structName, jsonName string) string {
	key := fieldCommentKey(structName, jsonName)
	if msg, err := lc.Localize(&i18n.LocalizeConfig{MessageID: key}); err == nil {
		return msg
	}
	return key
}
