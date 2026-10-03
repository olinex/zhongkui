package utils

import (
	"reflect"
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// SetField 设置结构体字段的值（字段不可导出时经 unsafe 反射写入）。
func SetField(obj any, name string, value any) {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	field := rv.FieldByName(name)
	if !field.CanSet() {
		field = reflect.NewAt(field.Type(), field.Addr().UnsafePointer()).Elem()
	}
	field.Set(reflect.ValueOf(value))
}

// FindField 解析结构体字段路径并返回对应的 reflect.StructField 指针。
// 字段路径以 '.' 分隔，各段匹配字段的 Go 字段名或 nameTag 标签名：
//   - nameTag 指定字段名匹配所用的标签（如 "json"、"mapstructure"、"toml"），
//     取去除选项后的标签名（如 json:"payload,omitempty" 取 "payload"）；
//     传空字符串仅按 Go 字段名匹配；
//   - 单段（如 "payload"）：浅层优先递归查找，始终返回最外层（直接声明）的匹配字段，
//     与落库列一致（内嵌结构的同名嵌入字段被外层遮蔽，不参与列映射）；
//   - 多段（如 "PacketIPv6.Payload"）：按显式路径逐层下降，精确定位内层嵌入字段。
//
// model 为结构体实例或其指针；路径无法解析时返回 nil 与错误。
func FindField(model any, path, nameTag string) (*reflect.StructField, error) {
	t := structTypeOf(model)
	if t == nil {
		return nil, errors.Newf("reflection: %T is not a struct", model)
	}
	if !strings.Contains(path, ".") {
		return findOuterField(t, path, nameTag)
	}
	return findPathField(t, strings.Split(path, "."), nameTag)
}

// ExtractFieldTag 提取字段路径对应字段的指定标签值。
// 字段解析语义与 FindField 一致（nameTag 控制字段名匹配所用的标签，空值仅按 Go 字段名匹配）；
// 标签不存在时返回空字符串而非错误，路径无法解析时返回错误。
func ExtractFieldTag(model any, path, nameTag, tagKey string) (string, error) {
	f, err := FindField(model, path, nameTag)
	if err != nil {
		return "", err
	}
	return f.Tag.Get(tagKey), nil
}

// structTypeOf 返回模型的结构体反射类型（自动解引用指针）；非结构体返回 nil
func structTypeOf(model any) reflect.Type {
	t := reflect.TypeOf(model)
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	return t
}

// findOuterField 单段路径：按层（浅层优先）递归查找，返回最外层匹配字段的指针
func findOuterField(t reflect.Type, seg, nameTag string) (*reflect.StructField, error) {
	queue := []reflect.Type{t}
	for len(queue) > 0 {
		// 本层：只返回直接匹配的字段，保证浅层优先
		for _, cur := range queue {
			for f := range cur.Fields() {
				f := f
				if fieldMatches(f, seg, nameTag) {
					return &f, nil
				}
			}
		}
		// 本层无命中，下降一层继续
		var next []reflect.Type
		for _, cur := range queue {
			for field := range cur.Fields() {
				if ft := fieldStructType(field); ft != nil {
					next = append(next, ft)
				}
			}
		}
		queue = next
	}
	return nil, errors.Newf("reflection: field %q not found in %s", seg, t)
}

// findPathField 多段路径：按显式路径逐层下降定位字段
func findPathField(t reflect.Type, parts []string, nameTag string) (*reflect.StructField, error) {
	path := strings.Join(parts, ".")
	cur := t
	for i, seg := range parts {
		last := i == len(parts)-1
		var matched *reflect.StructField
		for f := range cur.Fields() {
			f := f
			if fieldMatches(f, seg, nameTag) {
				matched = &f
				break
			}
		}
		if matched == nil {
			return nil, errors.Newf("reflection: field path %q not found in %s", path, t)
		}
		if last {
			return matched, nil
		}
		ft := fieldStructType(*matched)
		if ft == nil {
			return nil, errors.Newf("reflection: field %q in path %q is not a struct, cannot descend", seg, path)
		}
		cur = ft
	}
	return nil, errors.Newf("reflection: field path %q not found in %s", path, t)
}

// fieldMatches 判断字段是否与路径段匹配：Go 字段名或 nameTag 标签名任一命中即可
func fieldMatches(f reflect.StructField, seg, nameTag string) bool {
	if f.Name == seg {
		return true
	}
	if nameTag == "" {
		return false
	}
	name, _, _ := strings.Cut(f.Tag.Get(nameTag), ",")
	return name != "" && name != "-" && name == seg
}

// fieldStructType 返回字段的底层结构体类型；非结构体字段返回 nil
func fieldStructType(f reflect.StructField) reflect.Type {
	ft := f.Type
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	if ft.Kind() == reflect.Struct {
		return ft
	}
	return nil
}
