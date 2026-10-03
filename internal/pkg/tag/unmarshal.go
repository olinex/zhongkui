// Package tag 提供结构体标签（struct tag）的解析能力：把形如 `mode="aes-gcm", length=32`
// 的标签字符串解析为键值对，并经反射填充到目标结构体。
package tag

import (
	"reflect"
	"strconv"

	"github.com/alecthomas/participle/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

var parser = participle.MustBuild[Tag]()

// Unmarshal 解析 tagStr 标签字符串，并通过反射将解析结果填充到 target 指向的结构体中。
// target 必须是一个非空的结构体指针。
// 默认使用结构体字段名匹配标签键名，也可以通过 struct tag `tag:"keyname"` 指定映射的键名。
//
// 示例:
//
//	type Config struct {
//	    Mode   string `tag:"mode"`
//	    Length int    `tag:"length"`
//	}
//	var cfg Config
//	err := tag.Unmarshal(`mode="aes-gcm", length=32`, &cfg)
func Unmarshal(tagStr string, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.Newf("target must be a non-nil pointer to struct, got %T", target)
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return errors.Newf("target must be a pointer to struct, got %T", target)
	}

	ast, err := parser.ParseString("", tagStr)
	if err != nil {
		return errors.Newf("parse tag: %w", err)
	}

	pairMap := make(map[string]*Pair, len(ast.Pairs))
	for _, p := range ast.Pairs {
		pairMap[p.Key] = p
	}

	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldVal := rv.Field(i)

		if !field.IsExported() {
			continue
		}

		key := field.Name
		if tagKey := field.Tag.Get("tag"); tagKey != "" {
			key = tagKey
		}

		pair, ok := pairMap[key]
		if !ok {
			continue
		}

		if err := setValue(fieldVal, pair.Value); err != nil {
			return errors.Newf("field %s: %w", field.Name, err)
		}
	}

	return nil
}

func setValue(fieldVal reflect.Value, value *Value) error {
	switch fieldVal.Kind() {
	case reflect.String:
		if value.String == nil {
			return errors.Newf("expected string value")
		}
		fieldVal.SetString(unquoteString(*value.String))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := getInt64(value)
		if err != nil {
			return err
		}
		fieldVal.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := getInt64(value)
		if err != nil {
			return err
		}
		fieldVal.SetUint(uint64(v))
	case reflect.Float32, reflect.Float64:
		if value.String != nil {
			f, err := strconv.ParseFloat(unquoteString(*value.String), 64)
			if err != nil {
				return errors.Newf("cannot convert %q to float", *value.String)
			}
			fieldVal.SetFloat(f)
		} else if value.Int != nil {
			fieldVal.SetFloat(float64(*value.Int))
		} else {
			return errors.Newf("no value present")
		}
	case reflect.Bool:
		if value.String != nil {
			b, err := strconv.ParseBool(unquoteString(*value.String))
			if err != nil {
				return errors.Newf("cannot convert %q to bool", *value.String)
			}
			fieldVal.SetBool(b)
		} else if value.Int != nil {
			fieldVal.SetBool(*value.Int != 0)
		} else {
			return errors.Newf("no value present")
		}
	default:
		return errors.Newf("unsupported type %s", fieldVal.Kind())
	}
	return nil
}

func getInt64(value *Value) (int64, error) {
	if value.Int != nil {
		return int64(*value.Int), nil
	}
	if value.String != nil {
		v, err := strconv.ParseInt(unquoteString(*value.String), 10, 64)
		if err != nil {
			return 0, errors.Newf("cannot convert %q to int", *value.String)
		}
		return v, nil
	}
	return 0, errors.Newf("no value present")
}

// unquoteString 移除 participle @String 解析器捕获的字符串两侧的引号
func unquoteString(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
