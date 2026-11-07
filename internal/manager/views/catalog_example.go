// 本文件提供原生接口知识文档的 JSON 示例生成器：依据接口入参/出参结构体的 Go 类型，
// 递归构造一段确定性的 JSON 示例正文，供 renderApiKnowledgeDoc 产出「请求示例」「响应示例」小节。
//
// 确定性是硬要求：示例正文参与文档内容摘要（HMAC-SHA256）比对，逐字节不稳定会导致
// 每次同步都判定「内容已变更」而全量重嵌入。因此本文件一律按结构体字段声明顺序手工拼装，
// 禁止用 map 组装对象（Go map 遍历顺序随机）。
//
// 取值约定：不臆造业务样例数据，一律使用类型化占位——枚举字段取枚举类首个 value、
// 有 default 标签时取该默认值、字符串 "string"、数值 0、布尔 false、对象递归展开、
// 数组取单个元素示例。递归设深度上限并以类型名集合防环，避免自引用类型导致无限展开。
//
// 凭证类字段（如 password / secret / token）例外：示例值改为 <字段名> 形式。
// 这类字面量既更贴近占位符语义，也不会被密钥扫描器误判为真实泄露——例如 password
// 字段原先输出的普通字符串占位会被 Titus 的 np.generic.17（键名含 password 且值为
// 6~100 字符的普通字符串）命中，而 "<password>" 因含尖括号不匹配该正则。
package views

import (
	// 标准库
	"reflect"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// jsonExampleMaxDepth 是 JSON 示例的结构体递归展开深度上限（达到上限的嵌套对象以 {} 收口）。
const jsonExampleMaxDepth = 6

// jsonExampleTimeLiteral 是 time.Time 字段的示例字面量（该类型按标量呈现，见 IsScalarStructType）。
const jsonExampleTimeLiteral = `"1970-01-01T00:00:00Z"`

// BuildJSONExample 依据结构体类型构造 JSON 示例正文（不含代码围栏）；类型为空或非结构体时返回空串。
// 输出缩进 2 空格、字段顺序与结构体声明顺序一致，对同一类型连续调用结果逐字节相等。
func BuildJSONExample(t reflect.Type) string {
	tt := derefType(t)
	if tt == nil || tt.Kind() != reflect.Struct {
		return ""
	}
	return exampleValue(tt, "", 0, 0, map[string]bool{})
}

// exampleValue 渲染单个值或字段的 JSON 示例文本。enumClass 为该字段声明的枚举类名（可为空），
// depth 为当前递归深度，indent 为当前缩进层级（每层 2 空格），seen 为递归路径上已展开的类型名集合。
// 枚举类作用于值本身；切片/数组字段的枚举类下传到元素层（如 []string 枚举字段渲染为 ["首个取值"]）。
func exampleValue(t reflect.Type, enumClass string, depth, indent int, seen map[string]bool) string {
	tt := derefType(t)
	if tt == nil {
		return "{}"
	}
	if IsScalarStructType(tt) {
		return jsonExampleTimeLiteral
	}
	// json.RawMessage 与 any 无法推断结构，以空对象呈现；[]byte 在 JSON 中为 base64 字符串
	if isRawMessageType(tt) {
		return "{}"
	}
	if tt.Kind() == reflect.Slice || tt.Kind() == reflect.Array {
		if tt.Elem().Kind() == reflect.Uint8 {
			return `""`
		}
		if depth >= jsonExampleMaxDepth {
			return "[]"
		}
		elem := exampleValue(tt.Elem(), enumClass, depth+1, indent+1, seen)
		return wrapExampleArray(elem, indent)
	}
	if enumClass != "" {
		if value, ok := firstEnumValue(enumClass); ok {
			return strconv.Quote(value)
		}
	}
	switch tt.Kind() {
	case reflect.Bool:
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "0"
	case reflect.String:
		return `"string"`
	case reflect.Struct:
		return exampleObject(tt, depth, indent, seen)
	default:
		// map、接口、函数、通道等无法推断结构的值统一以空对象呈现
		return "{}"
	}
}

// exampleObject 渲染结构体对象的 JSON 示例：按字段声明顺序逐行输出，字段名经 structFieldName
// （json 标签优先、mapstructure 其次）解析；字段取值优先级为「default 标签 → 枚举类首个取值 →
// 类型化占位」。无可见字段、达到深度上限或类型已在递归路径上（防环）时返回 "{}"。
func exampleObject(t reflect.Type, depth, indent int, seen map[string]bool) string {
	if depth >= jsonExampleMaxDepth || seen[typeKeyOf(t)] {
		return "{}"
	}
	seen[typeKeyOf(t)] = true
	defer delete(seen, typeKeyOf(t))

	classBindings := enumBindings[t.Name()]
	pad := strings.Repeat("  ", indent+1)
	closePad := strings.Repeat("  ", indent)
	lines := make([]string, 0, t.NumField())
	for field := range t.Fields() {
		name := structFieldName(field)
		if name == "" {
			continue
		}
		value := exampleFieldValue(name, field, classBindings[name], depth, indent+1, seen)
		lines = append(lines, pad+strconv.Quote(name)+": "+value)
	}
	if len(lines) == 0 {
		return "{}"
	}
	return "{\n" + strings.Join(lines, ",\n") + "\n" + closePad + "}"
}

// exampleFieldValue 渲染单个字段的示例值：有 default 标签且可解析时优先使用该默认值，
// 否则按类型与枚举绑定推导（见 exampleValue）。凭证类字符串字段改用 <字段名> 占位。
func exampleFieldValue(name string, field reflect.StructField, enumClass string, depth, indent int, seen map[string]bool) string {
	if raw := field.Tag.Get("default"); raw != "" {
		if literal, ok := renderDefaultLiteral(kindForType(field.Type), raw); ok {
			return literal
		}
	}
	if tt := derefType(field.Type); tt != nil && tt.Kind() == reflect.String && isCredentialFieldName(name) {
		return strconv.Quote("<" + name + ">")
	}
	return exampleValue(field.Type, enumClass, depth+1, indent, seen)
}

// isCredentialFieldName 判断字段名是否为凭证语义（密码、密钥、令牌等）。
// 命中时示例值改用 <字段名> 占位，避免密钥扫描器把文档示例当作真实泄露。
// 只匹配明确的凭证名，避免把 maxTokens / contextTokens 这类计数误判。
func isCredentialFieldName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "password", "passwd", "pwd", "pass",
		"secret", "token", "authorization",
		"apikey", "api_key", "privatekey", "private_key",
		"credential", "credentials", "secretkey", "secret_key":
		return true
	}
	for _, suffix := range []string{"password", "passwd", "secret", "apikey", "privatekey"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	// token 只认单数结尾，排除 contextTokens / maxTokens 这类复数计数
	return strings.HasSuffix(lower, "token")
}

// renderDefaultLiteral 把 default 标签字面量按字段 kind 解析后渲染为 JSON 文本；
// 解析失败时返回 ok=false，由调用方回退到类型化占位。数值默认值的解析结果为 int/float64
// （见 parseDefault 与 utils.ParseScalar），故两者都要处理（int 缺失会让整数默认值静默退回 0）。
func renderDefaultLiteral(kind Kind, raw string) (string, bool) {
	value := parseDefault(kind, raw)
	switch v := value.(type) {
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	case string:
		return strconv.Quote(v), true
	default:
		return "", false
	}
}

// wrapExampleArray 把元素示例包装为数组示例：元素为单行时输出 "[elem]"，
// 元素含换行（嵌套对象）时按缩进层级展开，保证示例可读且确定。
func wrapExampleArray(elem string, indent int) string {
	if !strings.Contains(elem, "\n") {
		return "[" + elem + "]"
	}
	pad := strings.Repeat("  ", indent+1)
	closePad := strings.Repeat("  ", indent)
	return "[\n" + pad + strings.TrimLeft(elem, " ") + "\n" + closePad + "]"
}

// firstEnumValue 返回枚举类首个条目的存储值；枚举类未注册或无条目时返回 ok=false
// （离线生成期归属包未触发 init 注册时即此情形，调用方降级为类型化占位）。
func firstEnumValue(className string) (string, bool) {
	t, ok := enum.Get(className)
	if !ok || len(t.Entries) == 0 {
		return "", false
	}
	return t.Entries[0].Value, true
}

// typeKeyOf 返回类型的全局唯一标识（包路径 + 类型名），用于递归防环。
func typeKeyOf(t reflect.Type) string {
	return t.PkgPath() + "." + t.Name()
}

// isRawMessageType 判断类型是否为 json.RawMessage（无法推断结构，示例以空对象呈现）。
func isRawMessageType(t reflect.Type) bool {
	return t.PkgPath() == "encoding/json" && t.Name() == "RawMessage"
}
