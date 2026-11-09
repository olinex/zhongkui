package config

import (
	"bytes"
	"encoding"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

var (
	durationType      = reflect.TypeOf(time.Duration(0))
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	bareKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// MarshalTOML 将配置结构体实例序列化为带注释的 TOML 文本。
//
// 序列化规则与项目配置加载链路保持一致：
//   - 字段名取自 mapstructure 标签（其次 toml/json，最后回退字段名），"-" 表示忽略；
//   - comment 标签输出为该字段/表的行前注释；
//   - 结构体（未实现 encoding.TextMarshaler）输出为独立子表；
//   - 匿名嵌入结构体（如 grpc.BaseConfig）内联到父表；
//   - 实现 encoding.TextMarshaler 的类型（DurationNS/BibyteSize/CIDR 等）按文本值输出；
//   - time.Duration 输出为可读时长字符串（如 "1m"）；
//   - map[string]结构体 输出为逐个键的子表，其余 map 输出为内联表。
//
// 这样默认配置的字段集合、层级与注释完全由结构体定义驱动，新增字段会自动出现在
// 生成的配置中，避免手写模板与代码定义漂移。
func MarshalTOML(v any) ([]byte, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, errors.New("MarshalTOML: config instance is nil")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, errors.Newf("MarshalTOML: expected struct, got %s", rv.Kind())
	}
	table, err := buildTOMLTable(rv, "", "")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	table.render(&buf, true)
	return buf.Bytes(), nil
}

type tomlTable struct {
	header  string
	comment string
	lines   []tomlLine
	tables  []*tomlTable
}

type tomlLine struct {
	comment string
	key     string
	value   string
}

func (t *tomlTable) empty() bool {
	return len(t.lines) == 0 && len(t.tables) == 0
}

func (t *tomlTable) render(buf *bytes.Buffer, root bool) {
	if !root {
		// 仅作命名空间的中间表（自身无标量）不输出空表头
		if len(t.lines) == 0 {
			for _, sub := range t.tables {
				buf.WriteString("\n")
				sub.render(buf, false)
			}
			return
		}
		if t.comment != "" {
			writeTOMLComment(buf, t.comment)
		}
		buf.WriteString("[" + t.header + "]\n")
	}
	for _, line := range t.lines {
		if line.comment != "" {
			writeTOMLComment(buf, line.comment)
		}
		buf.WriteString(line.key + " = " + line.value + "\n")
	}
	for _, sub := range t.tables {
		buf.WriteString("\n")
		sub.render(buf, false)
	}
}

func writeTOMLComment(buf *bytes.Buffer, comment string) {
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			buf.WriteString("#\n")
			continue
		}
		buf.WriteString("# " + line + "\n")
	}
}

func buildTOMLTable(v reflect.Value, path, comment string) (*tomlTable, error) {
	table := &tomlTable{header: path, comment: comment}
	for _, field := range flattenFields(v) {
		key := tomlFieldKey(field.field)
		if key == "" {
			continue
		}
		val := indirect(field.val)
		if !val.IsValid() {
			continue
		}
		fieldComment := field.field.Tag.Get("comment")

		// map[string]结构体：按排序后的键输出为独立子表
		if val.Kind() == reflect.Map && isTOMLTableType(val.Type().Elem()) {
			keys := val.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for idx, mk := range keys {
				subComment := ""
				if idx == 0 {
					subComment = fieldComment
				}
				sub, err := buildTOMLTable(indirect(val.MapIndex(mk)), joinTOMLPath(path, key)+"."+quoteTOMLKey(mk.String()), subComment)
				if err != nil {
					return nil, err
				}
				if !sub.empty() {
					table.tables = append(table.tables, sub)
				}
			}
			continue
		}

		// map[string]map：每个值展开为独立子表，避免超长内联表
		if mapValuesAreTables(val) {
			keys := val.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for idx, mk := range keys {
				subComment := ""
				if idx == 0 {
					subComment = fieldComment
				}
				sub, err := buildTOMLMapTable(indirect(val.MapIndex(mk)), joinTOMLPath(path, key)+"."+quoteTOMLKey(mk.String()), subComment)
				if err != nil {
					return nil, err
				}
				if !sub.empty() {
					table.tables = append(table.tables, sub)
				}
			}
			continue
		}

		// 普通结构体：独立子表
		if isTOMLTableType(val.Type()) {
			sub, err := buildTOMLTable(val, joinTOMLPath(path, key), fieldComment)
			if err != nil {
				return nil, err
			}
			if !sub.empty() {
				table.tables = append(table.tables, sub)
			}
			continue
		}

		value, err := formatTOMLValue(val)
		if err != nil {
			return nil, err
		}
		table.lines = append(table.lines, tomlLine{comment: fieldComment, key: quoteTOMLKey(key), value: value})
	}
	return table, nil
}

type structFieldValue struct {
	field reflect.StructField
	val   reflect.Value
}

// mapValuesAreTables 判断 map 的每个值是否都是 map，用于将嵌套映射展开为独立子表。
func mapValuesAreTables(v reflect.Value) bool {
	if v.Kind() != reflect.Map || v.Len() == 0 {
		return false
	}
	for _, key := range v.MapKeys() {
		if indirect(v.MapIndex(key)).Kind() != reflect.Map {
			return false
		}
	}
	return true
}

// buildTOMLMapTable 将 map 输出为子表；嵌套 map 继续展开为更深层子表，其余值输出为键值对。
func buildTOMLMapTable(v reflect.Value, path, comment string) (*tomlTable, error) {
	table := &tomlTable{header: path, comment: comment}
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, mk := range keys {
		item := indirect(v.MapIndex(mk))
		if !item.IsValid() {
			continue
		}
		itemKey := quoteTOMLKey(mk.String())
		if item.Kind() == reflect.Map {
			sub, err := buildTOMLMapTable(item, joinTOMLPath(path, itemKey), "")
			if err != nil {
				return nil, err
			}
			if !sub.empty() {
				table.tables = append(table.tables, sub)
			}
			continue
		}
		value, err := formatTOMLValue(item)
		if err != nil {
			return nil, err
		}
		table.lines = append(table.lines, tomlLine{key: itemKey, value: value})
	}
	return table, nil
}

// flattenFields 按声明顺序展开结构体字段，匿名嵌入结构体的字段内联到当前层级。
func flattenFields(v reflect.Value) []structFieldValue {
	typ := v.Type()
	fields := make([]structFieldValue, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		val := v.Field(i)
		if field.Anonymous {
			inner := indirect(val)
			if inner.IsValid() && isTOMLTableType(inner.Type()) {
				fields = append(fields, flattenFields(inner)...)
				continue
			}
		}
		fields = append(fields, structFieldValue{field: field, val: val})
	}
	return fields
}

// tomlFieldKey 解析字段对应的 TOML 键，返回空字符串表示忽略该字段。
func tomlFieldKey(field reflect.StructField) string {
	for _, tag := range []string{"mapstructure", "toml", "json"} {
		value := field.Tag.Get(tag)
		if value == "" {
			continue
		}
		key, _, _ := strings.Cut(value, ",")
		key = strings.TrimSpace(key)
		if key == "-" {
			return ""
		}
		if key != "" {
			return key
		}
	}
	return field.Name
}

// isTOMLTableType 判断类型是否应输出为独立子表。
func isTOMLTableType(t reflect.Type) bool {
	t = derefType(t)
	if t == durationType {
		return false
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return false
	}
	return t.Kind() == reflect.Struct
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func indirect(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func joinTOMLPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func quoteTOMLKey(key string) string {
	if key != "" && bareKeyPattern.MatchString(key) {
		return key
	}
	quoted, err := tomlQuoteString(key)
	if err != nil {
		return strconv.Quote(key)
	}
	return quoted
}

func formatTOMLValue(v reflect.Value) (string, error) {
	v = indirect(v)
	if !v.IsValid() {
		return "", errors.New("formatTOMLValue: invalid value")
	}
	if v.Type() == durationType {
		return tomlQuoteString(v.Interface().(time.Duration).String())
	}
	if marshaler, ok := v.Interface().(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return "", errors.Wrapf(err, "failed to marshal %s", v.Type())
		}
		return tomlQuoteString(string(text))
	}
	switch v.Kind() {
	case reflect.String:
		return tomlQuoteString(v.String())
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64), nil
	case reflect.Slice, reflect.Array:
		items := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := formatTOMLValue(v.Index(i))
			if err != nil {
				return "", err
			}
			items = append(items, item)
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	case reflect.Map:
		return formatTOMLInlineTable(v)
	default:
		return "", errors.Newf("formatTOMLValue: unsupported type %s", v.Type())
	}
}

// formatTOMLInlineTable 将 map 输出为内联表，如 { retention = "168h" }。
func formatTOMLInlineTable(v reflect.Value) (string, error) {
	if v.Len() == 0 {
		return "{}", nil
	}
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		value, err := formatTOMLValue(v.MapIndex(key))
		if err != nil {
			return "", err
		}
		items = append(items, quoteTOMLKey(key.String())+" = "+value)
	}
	return "{ " + strings.Join(items, ", ") + " }", nil
}

// tomlQuoteString 借助 go-toml 生成标准 TOML 基本字符串，确保转义规则一致。
func tomlQuoteString(s string) (string, error) {
	encoded, err := toml.Marshal(map[string]any{"k": s})
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal TOML string")
	}
	line := strings.TrimSpace(string(encoded))
	return strings.TrimPrefix(line, "k = "), nil
}
