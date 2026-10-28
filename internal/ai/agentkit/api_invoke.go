package agentkit

import (
	"encoding/json"
	"regexp"
	"strings"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
)

// apiInvokeMaxParamsBytes 是 api_invoke 请求体的字节上限，防止模型用超大载荷占满挂起现场。
const apiInvokeMaxParamsBytes = 64 << 10

// apiInvokePathParamPattern 限定路径参数允许的字符：只接受常见的标识符字符，拒绝斜杠、问号、
// 井号与空白，避免参数值改变请求路径或注入查询串。
var apiInvokePathParamPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// apiInvokeArgs 是模型给出的 api_invoke 原始入参。
type apiInvokeArgs struct {
	// Capability 能力 ID，必须来自能力地图。
	Capability string `json:"capability"`
	// Params 请求体 JSON 对象。
	Params json.RawMessage `json:"params"`
	// PathParams 路径占位符取值，键为占位符名。
	PathParams map[string]string `json:"pathParams"`
}

// apiInvokeCanonicalArgs 是后端解析后交给前端执行的规范入参：前端只按此处的 method/path 发起
// 请求，不再接受模型给出的任何 URL 或方法。
type apiInvokeCanonicalArgs struct {
	// Capability 能力 ID（留痕与展示用）。
	Capability string `json:"capability"`
	// Method 已校验的 HTTP 方法。
	Method string `json:"method"`
	// Path 已替换路径参数后的完整路径。
	Path string `json:"path"`
	// Params 请求体对象。
	Params json.RawMessage `json:"params"`
}

// prepareApiInvokeCall 校验一次 api_invoke 调用并返回规范入参；校验失败时返回英文错误文本，
// 由调用方按工具失败回灌给模型，不产生挂起现场。
func prepareApiInvokeCall(arguments string, capabilities []capability.Capability) (string, string) {
	var args apiInvokeArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", "api_invoke: the arguments are not a valid JSON object: " + err.Error()
	}
	id := strings.TrimSpace(args.Capability)
	if id == "" {
		return "", "api_invoke: a non-empty capability is required"
	}
	item, ok := capability.Lookup(capabilities, id)
	if !ok {
		return "", "api_invoke: capability " + quote(id) + " is not registered; use a capability ID from the capability map"
	}
	if item.Access != capability.AccessRead || !item.Invokable {
		return "", "api_invoke: capability " + quote(id) + " is not a read-only invokable capability"
	}
	params, errText := apiInvokeParams(args.Params)
	if errText != "" {
		return "", errText
	}
	resolvedPath, errText := apiInvokeResolvePath(item.Path, args.PathParams)
	if errText != "" {
		return "", errText
	}
	canonical, err := json.Marshal(apiInvokeCanonicalArgs{
		Capability: id,
		Method:     item.Method,
		Path:       resolvedPath,
		Params:     params,
	})
	if err != nil {
		return "", "api_invoke: cannot encode the resolved arguments: " + err.Error()
	}
	return string(canonical), ""
}

// apiInvokeParams 规范化请求体：缺省或 null 视为空对象，必须是合法 JSON 对象且不超过字节上限。
// 各列表接口的过滤字段契约由接口自身声明，本函数不做任何按接口的字段补齐或改写。
func apiInvokeParams(raw json.RawMessage) (json.RawMessage, string) {
	body := strings.TrimSpace(string(raw))
	if body == "" || body == "null" {
		return json.RawMessage(`{}`), ""
	}
	if len(raw) > apiInvokeMaxParamsBytes {
		return nil, "api_invoke: params exceed the 64 KiB limit"
	}
	if !json.Valid(raw) {
		return nil, "api_invoke: params are not valid JSON"
	}
	if !strings.HasPrefix(body, "{") {
		return nil, "api_invoke: params must be a JSON object"
	}
	return raw, ""
}

// apiInvokeResolvePath 用 pathParams 填充路径中的 :param 占位符：占位符必须全部提供、不得
// 提供未声明的键，取值必须匹配受限字符集。无占位符时 pathParams 必须为空。
func apiInvokeResolvePath(template string, pathParams map[string]string) (string, string) {
	// gin 还支持 *name catch-all 与 \: 转义，当前能力声明不使用这类模板；显式拒绝，避免把
	// 通配符原样留在最终 URL 中导致请求打到错误路径。
	if strings.ContainsAny(template, "*\\") {
		return "", "api_invoke: the endpoint path " + quote(template) + " uses unsupported wildcard syntax"
	}
	placeholders := apiInvokePathPlaceholders(template)
	if len(placeholders) == 0 {
		if len(pathParams) > 0 {
			return "", "api_invoke: pathParams is not accepted because the endpoint has no path parameters"
		}
		return template, ""
	}
	for name := range pathParams {
		if !containsPathParam(placeholders, name) {
			return "", "api_invoke: unknown path parameter " + quote(name)
		}
	}
	if len(pathParams) != len(placeholders) {
		return "", "api_invoke: the endpoint requires path parameters: " + strings.Join(placeholders, ", ")
	}
	for _, name := range placeholders {
		value, ok := pathParams[name]
		if !ok || strings.TrimSpace(value) == "" {
			return "", "api_invoke: missing path parameter " + quote(name)
		}
		if !apiInvokePathParamPattern.MatchString(value) {
			return "", "api_invoke: path parameter " + quote(name) + " contains characters that are not allowed"
		}
	}
	// 按整段替换而不是全串替换，避免 :id 命中 :idx 之类的前缀重叠。
	segments := strings.Split(template, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[i] = pathParams[segment[1:]]
		}
	}
	return strings.Join(segments, "/"), ""
}

// apiInvokePathPlaceholders 提取路径中以 : 开头的占位符名，保持出现顺序。
func apiInvokePathPlaceholders(path string) []string {
	var names []string
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			names = append(names, segment[1:])
		}
	}
	return names
}

// containsPathParam 报告路径占位符列表中是否包含指定名称。
func containsPathParam(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// quote 为面向模型的英文错误文本加双引号，保留原始 ID 便于定位。
func quote(value string) string {
	return `"` + value + `"`
}
