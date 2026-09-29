// Package vtab（本文件）提供 ClickHouse 数据源实现。
// ClickHouseDatasource 把某实例内的一张业务表暴露为一张 vtable：构造期经
// DESCRIBE TABLE 自省表结构导出列名与 ValueType（映射见 deriveCHValueType）并据此
// 构建 schema；Open 时借出 *sql.DB，把下推约束构建为参数化 SELECT（? 占位符），同步
// 扫描全部行后立即归还连接。仅非 ValueTypeAny 的列参与下推。
//
// 下推取舍：ClickHouse 的 LIKE 大小写敏感（与 sqlite 不敏感不一致），直接下推会漏掉
// 大小写不一致的真实匹配行，违反超集契约，故 OpLIKE 一律不在此下推、交由 sqlite 以
// Omit=false 复查；OpGLOB/OpMATCH 无直接语义同样跳过。EQ/NE/GT/GE/LT/LE/IS NULL 与
// sqlite 语义一致，予以安全下推（同 MySQL/PostgreSQL 的取舍）。

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// chColumn 描述自省得到的一列：列名与 ValueType。
type chColumn struct {
	name string
	vt   sqlitevtab.ValueType
}

// ClickHouseDatasource 是基于 ClickHouse 一张表的 vtable 数据源。
type ClickHouseDatasource struct {
	chBase
	tableRef string
}

// chBase 是 ClickHouse vtable 数据源的共享基座。
type chBase struct {
	pool       *ClickHousePool
	schema     sqlitevtab.Schema
	valueTypes []sqlitevtab.ValueType
	filterable []int
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
func (b *chBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// NewClickHouseDatasource 自省 pool 指向实例中的 table 表结构并创建数据源。table 必须
// 是合法 SQL 标识符；表限定于 pool 绑定的 DBName。构造借出一次连接执行 DESCRIBE 后立即归还。
func NewClickHouseDatasource(ctx context.Context, pool *ClickHousePool, table string) (*ClickHouseDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("clickhouse: invalid table identifier %q", table)
	}
	db, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	// DESCRIBE TABLE 返回 (name, type, default_type, default_expression, comment)，
	// 仅需前两列
	q := "DESCRIBE TABLE " + quotePgIdent(table)
	qrows, err := db.QueryContext(ctx, q)
	if err != nil {
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	var cols []chColumn
	for qrows.Next() {
		var name, typ string
		var ignored string
		if err := qrows.Scan(&name, &typ, &ignored, &ignored, &ignored); err != nil {
			qrows.Close()
			pool.Release(ctx, db)
			return nil, errors.WithStack(err)
		}
		_ = ignored
		cols = append(cols, chColumn{name: name, vt: deriveCHValueType(typ)})
	}
	if err := qrows.Err(); err != nil {
		qrows.Close()
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	qrows.Close()
	pool.Release(ctx, db)
	if len(cols) == 0 {
		return nil, errors.Newf("clickhouse: table %q has no columns", table)
	}
	return newClickHouseDatasource(pool, table, cols), nil
}

// NewClickHouseDatasourceWithSchema 以手动指定的列构建数据源，跳过 DESCRIBE TABLE 自省。
// 语义同 NewMySQLDatasourceWithSchema。
func NewClickHouseDatasourceWithSchema(pool *ClickHousePool, table string, columns []sqlitevtab.Column) (*ClickHouseDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("clickhouse: manual schema: invalid table identifier %q", table)
	}
	if len(columns) == 0 {
		return nil, errors.New("clickhouse: manual schema requires at least one column")
	}
	cols := make([]chColumn, len(columns))
	for i, c := range columns {
		cols[i] = chColumn{name: c.Name, vt: c.ValueType}
	}
	return newClickHouseDatasource(pool, table, cols), nil
}

// newClickHouseDatasource 以给定列构建数据源（也供测试直接注入列，无需真实 ClickHouse）。
func newClickHouseDatasource(pool *ClickHousePool, table string, cols []chColumn) *ClickHouseDatasource {
	schema := sqlitevtab.Schema{Columns: make([]sqlitevtab.Column, 0, len(cols))}
	valueTypes := make([]sqlitevtab.ValueType, 0, len(cols))
	var filterable []int
	for i, c := range cols {
		schema.Columns = append(schema.Columns, sqlitevtab.Column{Name: c.name, ValueType: c.vt})
		valueTypes = append(valueTypes, c.vt)
		if c.vt != sqlitevtab.ValueTypeAny {
			filterable = append(filterable, i)
		}
	}
	// Filterable 也要写回 schema，sqlitevtab 的 BestIndex 才会据此生成并下推约束。
	schema.Filterable = filterable
	return &ClickHouseDatasource{
		chBase:   chBase{pool: pool, schema: schema, valueTypes: valueTypes, filterable: filterable},
		tableRef: quotePgIdent(table),
	}
}

// deriveCHValueType 将 ClickHouse 列类型映射为 sqlitevtab.ValueType。类型串可能内嵌参数
// 或包装（Nullable/LowCardinality），先剥离包装再取括号前基础名判断。未识别类型映射为
// ValueTypeAny（declare 省略类型、按值推断且不参与下推）。
func deriveCHValueType(typ string) sqlitevtab.ValueType {
	t := typ
	for {
		switch {
		case strings.HasPrefix(t, "Nullable("):
			t = strings.TrimSuffix(t[9:], ")")
		case strings.HasPrefix(t, "LowCardinality("):
			t = strings.TrimSuffix(t[15:], ")")
		default:
			goto base
		}
	}
base:
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = t[:i]
	}
	switch t {
	case "Int8", "Int16", "Int32", "Int64", "Int128", "Int256",
		"UInt8", "UInt16", "UInt32", "UInt64", "UInt128", "UInt256":
		return sqlitevtab.ValueTypeInt64
	case "Float32", "Float64", "Decimal", "Decimal32", "Decimal64", "Decimal128", "Decimal256":
		return sqlitevtab.ValueTypeFloat64
	case "Bool":
		return sqlitevtab.ValueTypeBool
	case "String", "FixedString", "Date", "Date32", "DateTime", "DateTime64",
		"Enum8", "Enum16", "UUID", "IPv4", "IPv6", "JSON":
		return sqlitevtab.ValueTypeString
	default:
		return sqlitevtab.ValueTypeAny
	}
}

// Open 实现 Datasource.Open：借出连接、按下推约束构建参数化查询、同步取回全部行并
// 返回内存游标；连接作用域到 fetch 结束即归还。
func (d *ClickHouseDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	db, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	query, args := d.buildQuery(pushdown)
	rows, err := d.fetch(ctx, db, query, args)
	d.pool.Release(ctx, db)
	if err != nil {
		return nil, err
	}
	return newMySQLRows(rows), nil
}

// buildQuery 依据下推约束构建参数化 SELECT（? 占位符）与绑定参数。Only non-Any 列合成
// 谓词；OpLIKE/OpGLOB/OpMATCH 因与 sqlite 语义不一致或无可直接语义而跳过（见文件头
// 注释）；无约束时返回全表 SELECT（宽松超集）。
func (d *ClickHouseDatasource) buildQuery(pushdown []sqlitevtab.Constraint) (string, []any) {
	var sel strings.Builder
	sel.WriteString("SELECT ")
	for i, col := range d.schema.Columns {
		if i > 0 {
			sel.WriteString(", ")
		}
		sel.WriteString(quotePgIdent(col.Name))
	}
	sel.WriteString(" FROM ")
	sel.WriteString(d.tableRef)

	var where []string
	var args []any
	for _, cst := range pushdown {
		if !containsInt(d.filterable, cst.Column) {
			continue
		}
		name := quotePgIdent(d.schema.Columns[cst.Column].Name)
		ph := "?"
		switch cst.Op {
		case sqlitevtab.OpEQ:
			where = append(where, name+" = "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpNE:
			where = append(where, name+" != "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpGT:
			where = append(where, name+" > "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpGE:
			where = append(where, name+" >= "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpLT:
			where = append(where, name+" < "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpLE:
			where = append(where, name+" <= "+ph)
			args = append(args, cst.Value)
		case sqlitevtab.OpISNULL:
			where = append(where, name+" IS NULL")
		case sqlitevtab.OpISNOTNULL:
			where = append(where, name+" IS NOT NULL")
		default:
			// OpLIKE / OpGLOB / OpMATCH / 其他：不下推，交给 sqlite 复查
		}
	}
	if len(where) > 0 {
		sel.WriteString(" WHERE ")
		sel.WriteString(strings.Join(where, " AND "))
	}
	return sel.String(), args
}

// fetch 执行查询并把全部行扫描进内存（按 valueTypes 归一化，NULL→nil）。
func (d *ClickHouseDatasource) fetch(ctx context.Context, db *sql.DB, query string, args []any) ([][]any, error) {
	qrows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer qrows.Close()
	var out [][]any
	for qrows.Next() {
		row, err := d.scanRow(qrows)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		out = append(out, row)
	}
	if err := qrows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}

// scanRow 将一行按 schema 列扫描并逐列经 convertCell 归一化。
func (d *ClickHouseDatasource) scanRow(qrows *sql.Rows) ([]any, error) {
	dest := make([]any, len(d.schema.Columns))
	ptrs := make([]any, len(dest))
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	if err := qrows.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := make([]any, len(dest))
	for i, v := range dest {
		row[i] = convertCell(v, d.valueTypes[i])
	}
	return row, nil
}
