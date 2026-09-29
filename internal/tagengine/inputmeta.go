// 本文件实现输入文档字段元信息的自动推导（InputMetadata），
// 供前端 Rego 编辑器自动补全与字段面板使用。字段集合/类型/敏感标记
// 由类型反射与 json tag 推导；字段描述（Description）不解析源码注释，
// 而是由 tagengine 以 i18n 消息键（input.<typeKey>.<field>）标注，
// 翻译文案维护在 internal/pkg/translate/locales 下，由接口层按请求语言翻译。

package tagengine

import (
	// 标准库
	"reflect"
	"sort"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// InputFieldMeta 输入文档单个字段的元信息。
// 供前端 Rego 编辑器自动补全与字段面板使用：Field 为输入文档中的扁平化字段名
// （规则中以 input.<field> 引用），Type 为 JSON 类型，GoType 为 Go 类型名，
// Description 为字段含义（由接口层按请求语言翻译 DescriptionKey 后填充，可为空），
// Sensitive 标记敏感字段（secure tag 或 serializer.Crypto 类型），
// Items 在 Type 为 array 时给出元素类型。
type InputFieldMeta struct {
	// Field 输入文档字段名（input.<field>）
	Field string `json:"field"`
	// Type JSON 类型：string/number/boolean/object/array
	Type string `json:"type"`
	// GoType Go 类型名（含包名短名，如 string、int32、layers.TCPPort、serializer.IPAddr）
	GoType string `json:"goType"`
	// Description 字段含义描述（接口层按请求语言翻译 DescriptionKey 后填充，可为空）
	Description string `json:"description"`
	// DescriptionKey 字段描述的 i18n 消息键（input.<typeKey>.<field>），
	// 翻译文案维护在 internal/pkg/translate/locales 下，注册模型须为每个字段提供翻译
	DescriptionKey string `json:"-"`
	// Sensitive 是否敏感字段（secure tag 或 serializer.Crypto 类型）
	Sensitive bool `json:"sensitive"`
	// Items Type==array 时的元素类型信息
	Items *InputItemMeta `json:"items,omitempty"`
}

// InputItemMeta 数组元素类型信息。
type InputItemMeta struct {
	// Type 元素 JSON 类型
	Type string `json:"type"`
	// GoType 元素 Go 类型名
	GoType string `json:"goType"`
}

// InputMetadata 返回已注册模型输入文档的扁平化字段元信息列表（按字段名排序）。
// 字段集合与 BuildInput 零值实例文档键一致；未注册模型返回 ErrModelNotRegistered。
// Description 字段不在此处填充，由接口层按 DescriptionKey 按请求语言翻译后输出。
func InputMetadata(typeKey string) ([]InputFieldMeta, error) {
	target, ok := TargetInfo(typeKey)
	if !ok {
		return nil, errors.WithStack(ErrModelNotRegistered)
	}
	// BuildInput 零值实例文档的键集合为权威字段集合（与引擎实际评估输入一致）；
	// nil slice / 指针字段在零值文档中序列化为 null，但键仍然存在。
	input, err := BuildInput(target.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	keys := make(map[string]struct{}, len(input))
	for key := range input {
		keys[key] = struct{}{}
	}
	metas, err := deriveFieldMetas(typeKey, target.Type)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 仅保留出现在评估输入键集合中的字段，保证与 BuildInput 字段集合严格一致
	result := make([]InputFieldMeta, 0, len(keys))
	for key, meta := range metas {
		if _, ok := keys[key]; !ok {
			continue
		}
		result = append(result, meta)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Field < result[j].Field })
	return result, nil
}

// deriveFieldMetas 通过反射递归遍历目标类型（含匿名嵌入字段提升），推导扁平化字段元信息。
// 按层 BFS 遍历保证多级嵌入同名字段"浅层优先"（与 json.Marshal 提升语义一致）；
// 每个字段的 DescriptionKey 由 typeKey 与字段名推导（input.<typeKey>.<field>），
// 描述文案由接口层按请求语言从 i18n 翻译文件中提取。
func deriveFieldMetas(typeKey string, target any) (map[string]InputFieldMeta, error) {
	typ := reflect.TypeOf(target)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, errors.Newf("target type is not a struct: %s", typ)
	}
	metas := make(map[string]InputFieldMeta)
	// pending 描述待展开结构体：typ 为当前结构体类型，index 为从目标类型到该结构体的反射路径
	type pending struct {
		typ   reflect.Type
		index []int
	}
	queue := []pending{{typ: typ}}
	for len(queue) > 0 {
		level := queue
		queue = nil
		for _, p := range level {
			for i := 0; i < p.typ.NumField(); i++ {
				sf := p.typ.Field(i)
				// 非导出字段不参与 JSON 序列化（含非导出的匿名嵌入字段）
				if sf.PkgPath != "" {
					continue
				}
				jsonName := jsonNameOf(sf)
				index := append(append([]int(nil), p.index...), i)
				if sf.Anonymous && jsonName == "" {
					// 匿名嵌入结构体且无 json tag：提升展开，下一层处理
					ft := sf.Type
					for ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if ft.Kind() == reflect.Struct {
						queue = append(queue, pending{typ: ft, index: index})
						continue
					}
					// 匿名嵌入的非结构体类型（本项目无此场景）不提升，跳过
					continue
				}
				if jsonName == "" {
					continue
				}
				// 浅层优先：同名字段已记录时跳过深层字段
				if _, exists := metas[jsonName]; exists {
					continue
				}
				jsonType, items, sensitive := classifyType(sf.Type, sf.Tag)
				metas[jsonName] = InputFieldMeta{
					Field:          jsonName,
					Type:           jsonType,
					GoType:         sf.Type.String(),
					DescriptionKey: "input." + typeKey + "." + jsonName,
					Sensitive:      sensitive,
					Items:          items,
				}
			}
		}
	}
	return metas, nil
}

// jsonNameOf 返回字段的 JSON 键名。
// 本项目目标模型字段均有显式 json tag，直接取 tag 名（逗号后选项如 omitempty 忽略）；
// 无 tag 或 tag 为 "-" 时返回空串（对应字段跳过）。
func jsonNameOf(sf reflect.StructField) string {
	tag := sf.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

// classifyType 推导字段的 JSON 类型、数组元素信息与敏感标记。
// 命名类型先解引用指针、再依据基础 kind 与已知特判类型分类：
// time.Time 与 serializer.IPAddr/IPv4Addr/HexUUID 序列化为 string，
// datatypes.JSON（本质 []byte）为 object，serializer.Crypto 为 object 且敏感；
// struct tag 含 secure（crypto.SecurityTagKey）或类型为 serializer.Crypto 时 Sensitive=true。
func classifyType(typ reflect.Type, tag reflect.StructTag) (jsonType string, items *InputItemMeta, sensitive bool) {
	// struct tag 含 secure 即视为敏感字段
	if tag.Get(crypto.SecurityTagKey) != "" {
		sensitive = true
	}
	// 指针解引用（如 *serializer.Crypto → serializer.Crypto）
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	// 命名类型特判优先于基础 kind（其底层可能是 slice/array/struct）
	switch typ.String() {
	case "time.Time":
		return "string", nil, sensitive
	case "serializer.IPAddr", "serializer.IPv4Addr", "serializer.HexUUID":
		return "string", nil, sensitive
	case "datatypes.JSON":
		return "object", nil, sensitive
	case "serializer.Crypto":
		return "object", nil, true
	}
	switch typ.Kind() {
	case reflect.String:
		return "string", nil, sensitive
	case reflect.Bool:
		return "boolean", nil, sensitive
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64:
		return "number", nil, sensitive
	case reflect.Slice, reflect.Array:
		elemType := typ.Elem()
		elemJSON, _, _ := classifyType(elemType, "")
		return "array", &InputItemMeta{Type: elemJSON, GoType: elemType.String()}, sensitive
	case reflect.Struct:
		return "object", nil, sensitive
	default:
		// map、chan、func、interface 等：JSON 序列化语义不明，按 object 兜底
		return "object", nil, sensitive
	}
}
