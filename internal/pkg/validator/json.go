package validator

import (
	"bytes"
	"encoding/json"
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// JSONKind 是 JSON 规范定义的值类型名：string / number / boolean / object / array / null。
// 作为类型化枚举声明，供原始 JSON 值识别与类型匹配统一引用，避免调用方就地书写类型名字面量。
// JSONKindInvalid 是解析失败的哨兵值（不是 JSON 规范类型），零值 "" 仍表示「未返回类型」。
type JSONKind string

// JSON 规范值类型常量。
const (
	// JSONKindString JSON 字符串
	JSONKindString JSONKind = "string"
	// JSONKindNumber JSON 数字
	JSONKindNumber JSONKind = "number"
	// JSONKindBoolean JSON 布尔
	JSONKindBoolean JSONKind = "boolean"
	// JSONKindObject JSON 对象
	JSONKindObject JSONKind = "object"
	// JSONKindArray JSON 数组
	JSONKindArray JSONKind = "array"
	// JSONKindNull JSON null
	JSONKindNull JSONKind = "null"
	// JSONKindInvalid 不是合法 JSON 值（空值或语法错误）；由 JSONValueKind 在错误时返回，
	// 仅用于错误文案等展示场景，不参与类型匹配。
	JSONKindInvalid JSONKind = "invalid"
)

// JSONValueKind 返回原始 JSON 值的类型名（按 JSON 规范）：string / number / boolean /
// object / array / null；空值或非法 JSON 返回 JSONKindInvalid 与英文错误。识别只看首个
// 非空白字节，合法性由 json.Valid 兜底，因此缩进与首尾空白不影响结果。
// 调用方须以 error 判定成功与否：kind 为 JSONKindInvalid 时即使用于展示也代表解析失败。
func JSONValueKind(raw json.RawMessage) (JSONKind, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return JSONKindInvalid, errors.New("empty JSON value")
	}
	if !json.Valid(trimmed) {
		return JSONKindInvalid, errors.New("invalid JSON value")
	}
	switch trimmed[0] {
	case '"':
		return JSONKindString, nil
	case '{':
		return JSONKindObject, nil
	case '[':
		return JSONKindArray, nil
	case 't', 'f':
		return JSONKindBoolean, nil
	case 'n':
		return JSONKindNull, nil
	default:
		return JSONKindNumber, nil
	}
}

// JSONKindOrInvalid 返回原始 JSON 值的类型名；空值或非法 JSON 返回 JSONKindInvalid。
// 供错误文案等只需安全展示类型名的场景使用；需要判定合法性时请使用 JSONValueKind。
func JSONKindOrInvalid(raw json.RawMessage) JSONKind {
	kind, err := JSONValueKind(raw)
	if err != nil {
		return JSONKindInvalid
	}
	return kind
}

// DecodeObject 严格解析裸 JSON 对象：去除首尾空白后必须非空，且必须以 '{' 开头、以 '}' 结尾；
// 代码块围栏与前后解释文字均视为非法帧。非法帧或 JSON 语法错误时返回英文错误。
func DecodeObject(raw string, dst any) error {
	body := strings.TrimSpace(raw)
	if body == "" {
		return errors.New("json object is empty")
	}
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		return errors.New("json object must be a bare object: it has to start with '{' and end with '}', without explanations or code fences")
	}
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		return errors.Newf("json object is invalid: %v", err)
	}
	return nil
}

// DecodeObjectLenient 先剥离单层 ```json / ``` 代码块围栏，再按 DecodeObject 的严格规则解析。
// 适用于模型输出等可能被围栏包裹的裸 JSON 对象。
func DecodeObjectLenient(raw string, dst any) error {
	return DecodeObject(StripJSONFence(raw), dst)
}

// DecodeOptionalObject 解析可选的裸 JSON 对象：去首尾空白后为空或字面量 null 视为未提供，
// 返回 present=false 且不修改 dst；否则按 DecodeObject 的严格规则解析并返回 present=true。
// 该函数统一「空值 / null 等价于未提供」的对象解析语义，调用方不再各自书写字符串判断。
func DecodeOptionalObject(raw string, dst any) (present bool, err error) {
	if isAbsentJSON(raw) {
		return false, nil
	}
	if err := DecodeObject(raw, dst); err != nil {
		return false, err
	}
	return true, nil
}

// DecodeOptionalValue 解析可选的任意 JSON 值：去首尾空白后为空或字面量 null 视为未提供，
// 返回 present=false 且不修改 dst；否则把整段文本交给 json.Unmarshal 解析（bool / object /
// string / number / array 均可）。用于允许任意 JSON 类型的可选配置列。
func DecodeOptionalValue(raw string, dst any) (present bool, err error) {
	body := strings.TrimSpace(raw)
	if body == "" || body == "null" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		return false, errors.Newf("json value is invalid: %v", err)
	}
	return true, nil
}

// isAbsentJSON 判断可选 JSON 文本是否视为未提供：空串、纯空白与字面量 null 均视为未提供。
func isAbsentJSON(raw string) bool {
	body := strings.TrimSpace(raw)
	return body == "" || body == "null"
}

// StripJSONFence 去除可能包裹的 ```json / ``` 代码围栏与前后空白，返回裸 JSON 文本；
// 该函数是 JSON 围栏剥离的唯一实现，供对象解码与任意 JSON 载荷解析复用。
func StripJSONFence(content string) string {
	body := strings.TrimSpace(content)
	if !strings.HasPrefix(body, "```") {
		return body
	}
	if idx := strings.Index(body, "\n"); idx >= 0 {
		body = body[idx+1:]
	} else {
		// 只有一行（如 ```{"a":1}```）时去掉起始的 ``` 前缀
		body = strings.TrimPrefix(body, "```")
	}
	body = strings.TrimSpace(body)
	body = strings.TrimSuffix(body, "```")
	return strings.TrimSpace(body)
}
