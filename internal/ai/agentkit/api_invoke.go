package agentkit

import (
	"encoding/json"
	"regexp"
	"strings"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// apiInvokeMaxParamsBytes 是 api_invoke 请求体的字节上限，防止模型用超大载荷占满挂起现场。
const apiInvokeMaxParamsBytes = 64 << 10

// apiInvokePathParamPattern 限定路径参数允许的字符：只接受常见的标识符字符，拒绝斜杠、问号、
// 井号与空白，避免参数值改变请求路径或注入查询串。
var apiInvokePathParamPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// apiInvokeArgs 是模型给出的 api_invoke 原始入参。
type apiInvokeArgs struct {
	// Capability 能力 ID，必须来自能力地图
	Capability string `json:"capability" binding:"required,notBlank"`
	// Params 请求体 JSON 对象。
	Params json.RawMessage `json:"params"`
	// PathParams 路径占位符取值，键为占位符名。
	PathParams map[string]string `json:"pathParams"`
	// Complete 是模型对「本次调用的全部参数已完整输出」的自声明。完整性由 JSON 解析结果判定：
	// 流式生成被中途截断时参数通常不是合法 JSON，json.Unmarshal 会直接失败；若模型在合法 JSON
	// 内主动省略后置字段且未置位，则该标志缺失或为 false。JSON 对象键序无语义，complete 可放在
	// 参数对象的任意位置，后端不依赖字段位置。该字段只用于完整性判定，不进入规范入参。
	Complete bool `json:"complete"`
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

// prepareApiInvokeCall 校验一次 api_invoke 调用并返回规范入参；校验失败时返回英文错误，
// 由调用方按工具失败回灌给模型，不产生挂起现场。只读与写入能力都允许调用：是否可调用由能力
// 的 Invokable 属性单点决定；写入能力同样以客户端工具挂起、由用户在审批卡片上确认后以本人
// 凭据执行。
func prepareApiInvokeCall(arguments string, capabilities []capability.Capability) (string, error) {
	var args apiInvokeArgs
	if err := validator.DecodeObject(arguments, &args); err != nil {
		return "", errors.Newf("api_invoke: the arguments are not a valid JSON object: %v", err)
	}
	if err := validator.Check(&args); err != nil {
		return "", errors.Newf("api_invoke: invalid arguments: %s", validator.Describe(err))
	}
	id := strings.TrimSpace(args.Capability)
	item, ok := capability.Lookup(capabilities, id)
	if !ok {
		return "", errors.Newf("api_invoke: capability %s is not registered; use a capability ID from the capability map", quote(id))
	}
	if !item.Invokable {
		return "", errors.Newf("api_invoke: capability %s is not an invokable capability", quote(id))
	}
	params, err := apiInvokeParams(args.Params)
	if err != nil {
		return "", err
	}
	resolvedPath, err := apiInvokeResolvePath(item.Path, args.PathParams)
	if err != nil {
		return "", err
	}
	if !args.Complete {
		return "", errors.New("api_invoke: the arguments are incomplete (the top-level \"complete\" flag is missing or false); " +
			"re-issue the same capability call, emit every field in full, and set \"complete\": true. " +
			"Do not split the call, omit content, or switch to another capability.")
	}
	canonical, err := json.Marshal(apiInvokeCanonicalArgs{
		Capability: id,
		Method:     item.Method,
		Path:       resolvedPath,
		Params:     params,
	})
	if err != nil {
		return "", errors.Newf("api_invoke: cannot encode the resolved arguments: %v", err)
	}
	return string(canonical), nil
}

// apiInvokeParams 规范化请求体：缺省或 null 视为空对象，必须是合法 JSON 对象且不超过字节上限。
// 各列表接口的过滤字段契约由接口自身声明，本函数不做任何按接口的字段补齐或改写。
func apiInvokeParams(raw json.RawMessage) (json.RawMessage, error) {
	body := strings.TrimSpace(string(raw))
	if body == "" || body == "null" {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > apiInvokeMaxParamsBytes {
		return nil, errors.New("api_invoke: params exceed the 64 KiB limit")
	}
	var object map[string]json.RawMessage
	if err := validator.DecodeObject(body, &object); err != nil {
		return nil, errors.Newf("api_invoke: params must be a JSON object: %v", err)
	}
	return raw, nil
}

// apiInvokeResolvePath 用 pathParams 填充路径中的 :param 占位符：占位符必须全部提供、不得
// 提供未声明的键，取值必须匹配受限字符集。无占位符时 pathParams 必须为空。
func apiInvokeResolvePath(template string, pathParams map[string]string) (string, error) {
	// gin 还支持 *name catch-all 与 \: 转义，当前能力声明不使用这类模板；显式拒绝，避免把
	// 通配符原样留在最终 URL 中导致请求打到错误路径。
	if strings.ContainsAny(template, "*\\") {
		return "", errors.Newf("api_invoke: the endpoint path %s uses unsupported wildcard syntax", quote(template))
	}
	placeholders := apiInvokePathPlaceholders(template)
	if len(placeholders) == 0 {
		if len(pathParams) > 0 {
			return "", errors.New("api_invoke: pathParams is not accepted because the endpoint has no path parameters")
		}
		return template, nil
	}
	for name := range pathParams {
		if !containsPathParam(placeholders, name) {
			return "", errors.Newf("api_invoke: unknown path parameter %s", quote(name))
		}
	}
	if len(pathParams) != len(placeholders) {
		return "", errors.Newf("api_invoke: the endpoint requires path parameters: %s", strings.Join(placeholders, ", "))
	}
	for _, name := range placeholders {
		value, ok := pathParams[name]
		if !ok || strings.TrimSpace(value) == "" {
			return "", errors.Newf("api_invoke: missing path parameter %s", quote(name))
		}
		if !apiInvokePathParamPattern.MatchString(value) {
			return "", errors.Newf("api_invoke: path parameter %s contains characters that are not allowed", quote(name))
		}
	}
	// 按整段替换而不是全串替换，避免 :id 命中 :idx 之类的前缀重叠。
	segments := strings.Split(template, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[i] = pathParams[segment[1:]]
		}
	}
	return strings.Join(segments, "/"), nil
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
