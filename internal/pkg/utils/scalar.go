package utils

import (
	// 标准库
	"math"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ScalarKind 是 JSON 标量值的类型标识，用于在「文本字面量」与「强类型值」之间做统一转换。
// 它是各层自有类型词汇表（参数类型、表单字段语义类型、JSON schema 类型）的最小公共内核：
// 各层保留自己的枚举，只把解析与格式化委托到本文件，避免同一种转换被实现多份而行为漂移。
type ScalarKind string

// 标量类型常量。
const (
	// ScalarKindString 字符串（不修剪空白，空白可能具有语义）
	ScalarKindString ScalarKind = "string"
	// ScalarKindInt 整数
	ScalarKindInt ScalarKind = "int"
	// ScalarKindFloat 浮点数
	ScalarKindFloat ScalarKind = "float"
	// ScalarKindBool 布尔值
	ScalarKindBool ScalarKind = "bool"
)

// ParseScalar 把文本字面量按 kind 解析为强类型值（string→string、int→int、
// float→float64、bool→bool）。数值与布尔类型先修剪首尾空白（宽容直接手改库内数据的情形），
// 字符串类型不修剪；解析失败或 kind 未知时返回英文错误。
func ParseScalar(kind ScalarKind, literal string) (any, error) {
	switch kind {
	case ScalarKindString:
		return literal, nil
	case ScalarKindInt:
		n, err := strconv.Atoi(strings.TrimSpace(literal))
		if err != nil {
			return nil, errors.Newf("%q is not an integer", literal)
		}
		return n, nil
	case ScalarKindFloat:
		f, err := strconv.ParseFloat(strings.TrimSpace(literal), 64)
		if err != nil {
			return nil, errors.Newf("%q is not a number", literal)
		}
		return f, nil
	case ScalarKindBool:
		b, err := strconv.ParseBool(strings.TrimSpace(literal))
		if err != nil {
			return nil, errors.Newf("%q is not a boolean", literal)
		}
		return b, nil
	default:
		return nil, errors.Newf("unknown scalar kind %q", string(kind))
	}
}

// CoerceScalar 把 JSON 解码得到的原生值（string、float64、bool）与 Go 原生 int 按 kind 收敛为
// 强类型值。字符串形式先按 ParseScalar 解析，但数值与布尔类型的空串（或纯空白）按该 kind 的
// 零值处理（表单清空数值/布尔字段的既有契约）；float64 收敛为 int 时要求其为整数；类型与 kind
// 不符时返回英文错误。
func CoerceScalar(kind ScalarKind, value any) (any, error) {
	switch v := value.(type) {
	case string:
		if kind != ScalarKindString && strings.TrimSpace(v) == "" {
			return zeroScalar(kind)
		}
		return ParseScalar(kind, v)
	case bool:
		if kind != ScalarKindBool {
			return nil, errors.Newf("a boolean is not a valid %s", string(kind))
		}
		return v, nil
	case int:
		switch kind {
		case ScalarKindInt:
			return v, nil
		case ScalarKindFloat:
			return float64(v), nil
		default:
			return nil, errors.Newf("an integer is not a valid %s", string(kind))
		}
	case float64:
		switch kind {
		case ScalarKindInt:
			if v != math.Trunc(v) {
				return nil, errors.Newf("%v is not an integer", v)
			}
			return int(v), nil
		case ScalarKindFloat:
			return v, nil
		default:
			return nil, errors.Newf("a number is not a valid %s", string(kind))
		}
	default:
		return nil, errors.Newf("unsupported value type %T for %s", value, string(kind))
	}
}

// FormatScalar 把值按 kind 格式化为稳定文本字面量（int→十进制、float→'g' 最短表示、
// bool→true/false、string→原样），作为 ParseScalar 的逆向操作。值先经 CoerceScalar 收敛，
// 因此可接受 JSON 原生值；类型与 kind 不符时返回英文错误。
func FormatScalar(kind ScalarKind, value any) (string, error) {
	coerced, err := CoerceScalar(kind, value)
	if err != nil {
		return "", errors.WithStack(err)
	}
	switch v := coerced.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", errors.Newf("unsupported coerced value type %T for %s", coerced, string(kind))
	}
}

// zeroScalar 返回 kind 的零值；未知 kind 返回错误。
func zeroScalar(kind ScalarKind) (any, error) {
	switch kind {
	case ScalarKindString:
		return "", nil
	case ScalarKindInt:
		return 0, nil
	case ScalarKindFloat:
		return 0.0, nil
	case ScalarKindBool:
		return false, nil
	default:
		return nil, errors.Newf("unknown scalar kind %q", string(kind))
	}
}
