// Package vtab 提供 sqlitevtab 标准之上可直接复用的数据源实现。
// StaticDatasource 是内存行集的参考数据源实现，演示 Datasource 契约与下推
// 约束匹配语义，同时可作为测试基准；接入方可参考其模式实现自有数据源。

package vtab

import (
	// 标准库
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// StaticDatasource 是内存行集的参考数据源实现，演示 Datasource 契约与下推
// 约束匹配语义，同时可作为测试基准。
type StaticDatasource struct {
	schema sqlitevtab.Schema
	data   [][]any
}

// NewStaticDatasource 创建静态数据源。data 每行长度应与 schema.Columns 对齐；
// 越界行值按 nil 处理。
func NewStaticDatasource(schema sqlitevtab.Schema, data [][]any) *StaticDatasource {
	return &StaticDatasource{schema: schema, data: data}
}

// Open 实现 Datasource.Open：返回逐行匹配全部下推约束的行游标。
func (d *StaticDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	rules := make([]staticRule, 0, len(pushdown))
	for _, cst := range pushdown {
		rule := staticRule{cst: cst}
		switch cst.Op {
		case sqlitevtab.OpLIKE:
			if pattern, ok := textPattern(cst.Value); ok {
				if re, err := regexp.Compile("(?i)" + likeToRegex(pattern)); err == nil {
					rule.re = re
				}
			}
		case sqlitevtab.OpGLOB:
			if pattern, ok := textPattern(cst.Value); ok {
				if re, err := regexp.Compile(globToRegex(pattern)); err == nil {
					rule.re = re
				}
			}
		}
		rules = append(rules, rule)
	}
	return &staticRows{data: d.data, rules: rules}, nil
}

// staticRows 是 StaticDatasource 的行游标。
type staticRows struct {
	data  [][]any
	rules []staticRule
	idx   int
}

// Next 返回下一行命中行（行值的副本），迭代结束返回 io.EOF。
func (r *staticRows) Next() ([]any, error) {
	for r.idx < len(r.data) {
		row := r.data[r.idx]
		r.idx++
		if r.matches(row) {
			return append([]any(nil), row...), nil
		}
	}
	return nil, io.EOF
}

// Close 释放游标引用（幂等）。
func (r *staticRows) Close() error {
	r.data = nil
	r.rules = nil
	return nil
}

// matches 逐条应用全部下推约束。
func (r *staticRows) matches(row []any) bool {
	for _, rule := range r.rules {
		if !rule.matches(row) {
			return false
		}
	}
	return true
}

// staticRule 是一条约束及其（可选）预编译正则。
type staticRule struct {
	cst sqlitevtab.Constraint
	re  *regexp.Regexp
}

// matches 判断一行是否满足该约束。对不匹配类型的数据（如 String 列收到
// 数值绑定）按不匹配处理（返回 false），最终正确性由 sqlite 侧 Omit=false
// 复查兜底。
func (r *staticRule) matches(row []any) bool {
	value := rowValue(row, r.cst.Column)
	switch r.cst.Op {
	case sqlitevtab.OpEQ:
		return eqValues(value, r.cst.Value)
	case sqlitevtab.OpNE:
		return !eqValues(value, r.cst.Value)
	case sqlitevtab.OpGT:
		cmp, ok := compareValues(value, r.cst.Value)
		return ok && cmp > 0
	case sqlitevtab.OpGE:
		cmp, ok := compareValues(value, r.cst.Value)
		return ok && cmp >= 0
	case sqlitevtab.OpLT:
		cmp, ok := compareValues(value, r.cst.Value)
		return ok && cmp < 0
	case sqlitevtab.OpLE:
		cmp, ok := compareValues(value, r.cst.Value)
		return ok && cmp <= 0
	case sqlitevtab.OpLIKE:
		if r.re == nil {
			return false
		}
		s, ok := textPattern(value)
		return ok && r.re.MatchString(s)
	case sqlitevtab.OpGLOB:
		if r.re == nil {
			return false
		}
		s, ok := textPattern(value)
		return ok && r.re.MatchString(s)
	case sqlitevtab.OpMATCH:
		return matchContains(value, r.cst.Value)
	case sqlitevtab.OpISNULL:
		return value == nil
	case sqlitevtab.OpISNOTNULL:
		return value != nil
	default:
		return false
	}
}

// rowValue 返回一行中指定列的值；越界按 nil 处理。
func rowValue(row []any, col int) any {
	if col < 0 || col >= len(row) {
		return nil
	}
	return row[col]
}

// textPattern 将值规整为字符串；支持 string 与 []byte。
func textPattern(v any) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case []byte:
		return string(s), true
	default:
		return "", false
	}
}

// eqValues 按值语义判断相等：字符串按字符串、整数按 int64、浮点按 float64、
// 布尔按布尔、二进制按 bytes.Equal；值可为 nil 时仅 nil==nil 匹配，
// 类型不匹配视为不相等。
func eqValues(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case []byte:
		bv, ok := b.([]byte)
		return ok && bytes.Equal(av, bv)
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
		if bv, ok := b.(int64); ok {
			return int64(av) == bv
		}
		return false
	case int64:
		if bv, ok := b.(int64); ok {
			return av == bv
		}
		if bv, ok := b.(int); ok {
			return av == int64(bv)
		}
		return false
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	default:
		return a == b
	}
}

// compareValues 按数值或字符串语义比较两值，返回 (比较结果, 是否可比较)。
// 任一值为 nil 或类型不匹配时不可比较。
func compareValues(a, b any) (int, bool) {
	if a == nil || b == nil {
		return 0, false
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		if !ok {
			return 0, false
		}
		return strings.Compare(av, bv), true
	case []byte:
		bv, ok := b.([]byte)
		if !ok {
			return 0, false
		}
		return bytes.Compare(av, bv), true
	case bool:
		return 0, false
	case int:
		switch bv := b.(type) {
		case int:
			return cmpInt64(int64(av), int64(bv)), true
		case int64:
			return cmpInt64(int64(av), bv), true
		default:
			return 0, false
		}
	case int64:
		switch bv := b.(type) {
		case int64:
			return cmpInt64(av, bv), true
		case int:
			return cmpInt64(av, int64(bv)), true
		default:
			return 0, false
		}
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return 0, false
		}
		switch {
		case av < bv:
			return -1, true
		case av > bv:
			return 1, true
		default:
			return 0, true
		}
	default:
		return 0, false
	}
}

// cmpInt64 比较两个 int64。
func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// matchContains 按大小写不敏感的子串包含实现 MATCH 匹配。
func matchContains(a, b any) bool {
	as, ok1 := textPattern(a)
	bs, ok2 := textPattern(b)
	if !ok1 || !ok2 || bs == "" {
		return false
	}
	return strings.Contains(strings.ToLower(as), strings.ToLower(bs))
}

// likeToRegex 将 sqlite LIKE 通配模式转为正则：%→.*、_→.，其余字符转义；
// 大小写不敏感由调用方加 (?i) 前缀。
func likeToRegex(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteByte('$')
	return b.String()
}

// globToRegex 将 sqlite GLOB 通配模式转为正则：*→.*、?→.、[..] 字符类
// 原样透传，其余字符转义；大小写敏感。
func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			if end := strings.IndexByte(pattern[i+1:], ']'); end >= 0 {
				end += i + 1
				b.WriteString(pattern[i : end+1])
				i = end
			} else {
				b.WriteString(`\[`)
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteByte('$')
	return b.String()
}
