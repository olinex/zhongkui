// Package sqlitevtab 提供"通用 sqlite vtable 外部数据源接入"设施。
// 第三方数据源只需实现本包的 Datasource 标准接口（Open 返回行游标 Rows），
// 即可在任意时机经 *SQLitePool 动态注册为 sqlite 虚拟表（vtable），并通过
// SQL 语句查询；vtable 的 schema 声明、下推约束（BestIndex/xFilter）与
// 结果列输出（xColumn）均由本包代理，数据源实现无需关心 sqlite 细节。
// 数据源实例与 sqlite 连接的生命周期统一由 internal/pkg/resourcepool 管理：
// sqlite 连接按固定 Key 池化复用（每个连接独占一个内存库），数据源按
// 数据源名池化复用；vtable 回调在查询 goroutine 上同步执行，查询上下文
// 经连接对象透传给数据源。
//
// 包内错误统一使用 internal/pkg/errors 创建；查询为只读语义（连接级
// authorizer 拒绝一切写操作）；单次查询默认最多返回 1000 行，超出即报错。
package sqlitevtab

import (
	// 标准库
	"context"
	"regexp"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ValueType 描述数据源列到 SQLite 类型的亲和性映射：
// String→TEXT、Int64→INTEGER、Float64→REAL、Bool→INTEGER、Blob→BLOB，
// Any→无类型（declare 时省略类型声明，交由 SQLite 按值推断）。
type ValueType uint8

const (
	// ValueTypeAny 无固定类型；declare 时省略类型声明。
	ValueTypeAny ValueType = iota
	// ValueTypeString 字符串类型，映射 SQLite TEXT。
	ValueTypeString
	// ValueTypeInt64 有符号 64 位整数，映射 SQLite INTEGER。
	ValueTypeInt64
	// ValueTypeFloat64 双精度浮点数，映射 SQLite REAL。
	ValueTypeFloat64
	// ValueTypeBool 布尔值，映射 SQLite INTEGER（0/1）。
	ValueTypeBool
	// ValueTypeBlob 二进制数据，映射 SQLite BLOB。
	ValueTypeBlob
)

// Column 描述 vtable 中的一个列。
type Column struct {
	// Name 列名，必须是合法 SQL 标识符（^[A-Za-z_][A-Za-z0-9_]*$）。
	Name string
	// ValueType 列值类型。
	ValueType ValueType
}

// Schema 描述一个数据源的 vtable schema。
type Schema struct {
	// Columns 列定义，必须非空且列名唯一。
	Columns []Column
	// Filterable 支持下推筛选的列索引集合；nil 表示不支持下推（全表扫描）。
	Filterable []int
}

// Op 是下推约束的操作符，与 sqlite 的 IndexConstraintOp 一一映射。
type Op uint8

const (
	// OpEQ 等于（=）。
	OpEQ Op = iota
	// OpNE 不等于（!= / <>）。
	OpNE
	// OpGT 大于（>）。
	OpGT
	// OpGE 大于等于（>=）。
	OpGE
	// OpLT 小于（<）。
	OpLT
	// OpLE 小于等于（<=）。
	OpLE
	// OpLIKE LIKE 模式匹配。
	OpLIKE
	// OpGLOB GLOB 模式匹配。
	OpGLOB
	// OpMATCH MATCH 全文匹配。
	OpMATCH
	// OpISNULL IS NULL 判断。
	OpISNULL
	// OpISNOTNULL IS NOT NULL 判断。
	OpISNOTNULL
)

// Constraint 是一条下推筛选约束。
type Constraint struct {
	// Column 约束列在 schema.Columns 中的索引。
	Column int
	// Op 约束操作符。
	Op Op
	// Value 约束的右操作数（绑定值），是按列 ValueType 转换后的 Go 值；
	// ISNULL/ISNOTNULL 无右操作数，该字段为 nil。
	Value any
}

// Rows 是数据源返回的行游标。
type Rows interface {
	// Next 返回下一行，迭代结束时返回 io.EOF。
	// 返回的切片仅在下一次 Next 调用前有效，调用方如需保留须自行拷贝。
	Next() ([]any, error)
	// Close 释放游标持有的资源；调用方保证最终调用一次。
	Close() error
}

// Datasource 是第三方数据源需实现的通用标准接口。
// Open 依据下推约束返回一个行游标；实现方应尽力在数据源头过滤，但过滤
// 仅是优化：sqlite 侧以 Omit=false 复查每个返回行，因此过滤结果必须是
// 真实匹配行的超集，禁止漏掉任何真实匹配的行（漏行无法被复查找回）。
type Datasource interface {
	Open(ctx context.Context, pushdown []Constraint) (Rows, error)
}

// identifierPattern 合法 SQL 标识符模式。
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateDatasourceName 校验数据源名是合法 SQL 标识符且非 SQLite 保留关键字。
func validateDatasourceName(name string) error {
	if !identifierPattern.MatchString(name) {
		return errors.Newf("%w: %q", ErrInvalidDatasourceName, name)
	}
	if _, reserved := sqliteKeywords[strings.ToLower(name)]; reserved {
		return errors.Newf("%w: reserved keyword %q", ErrInvalidDatasourceName, name)
	}
	return nil
}

// validateSchema 校验 schema：列名合法、非空且不重复，Filterable 索引在界内。
func validateSchema(schema Schema) error {
	if len(schema.Columns) == 0 {
		return errors.Newf("%w: schema has no columns", ErrInvalidColumnName)
	}
	seen := make(map[string]struct{}, len(schema.Columns))
	for _, col := range schema.Columns {
		if !identifierPattern.MatchString(col.Name) {
			return errors.Newf("%w: %q", ErrInvalidColumnName, col.Name)
		}
		if _, dup := seen[col.Name]; dup {
			return errors.Newf("%w: %q", ErrDuplicateColumn, col.Name)
		}
		seen[col.Name] = struct{}{}
	}
	for _, idx := range schema.Filterable {
		if idx < 0 || idx >= len(schema.Columns) {
			return errors.Newf("%w: %d", ErrColumnIndexOutOfRange, idx)
		}
	}
	return nil
}
