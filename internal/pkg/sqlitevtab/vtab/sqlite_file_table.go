// Package vtab（本文件）提供 SQLite 文件数据源实现。
// SQLiteFileDatasource 把一个 .db 文件内的一张业务表暴露为一张 vtable：构造期经
// PRAGMA table_info 自省表结构导出列名与类型亲和（见 deriveSQLiteValueType）并据此
// 构建 schema；Open 时借出 *sql.DB，把下推约束构建为参数化 SELECT（? 占位符），同步
// 扫描全部行后立即归还连接。仅非 ValueTypeAny 的列参与下推。
//
// 下推取舍：源与 sqlitevtab 同为 SQLite，LIKE/GLOB 语义完全一致，故 OpLIKE 也可安全
// 下推（区别于 MySQL/PG/ClickHouse 的大小写敏感问题）；EQ/NE/GT/GE/LT/LE/IS NULL 自然
// 一致下推。OpGLOB/OpMATCH 需绑定非参数化的模式匹配形式，为简洁起见仍跳过、交由
// sqlite 复查（语义一致，复不复查结果相同）。

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// sqColumn 描述自省得到的一列：列名与 ValueType。
type sqColumn struct {
	name string
	vt   sqlitevtab.ValueType
}

// SQLiteFileDatasource 是一个 SQLite 文件的一张表的 vtable 数据源。
type SQLiteFileDatasource struct {
	// pool 数据源连接池
	pool *SQLiteFilePool
	// schema 注册 schema
	schema sqlitevtab.Schema
	// valueTypes 列 ValueType，扫描行时归一化
	valueTypes []sqlitevtab.ValueType
	// filterable 可下推（非 Any）列索引
	filterable []int
	// tableRef 引用的表名（已引号包裹）
	tableRef string
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
func (d *SQLiteFileDatasource) datasourceSchema() sqlitevtab.Schema {
	return d.schema
}

// NewSQLiteFileDatasource 自省 pool 指向文件中的 table 表结构并创建数据源。table 必须
// 是合法 SQL 标识符。构造借出一次连接执行 PRAGMA 后立即归还。
func NewSQLiteFileDatasource(ctx context.Context, pool *SQLiteFilePool, table string) (*SQLiteFileDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("sqlite_file: invalid table identifier %q", table)
	}
	db, err := pool.Acquire(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	q := "PRAGMA table_info(" + quotePgIdent(table) + ")"
	qrows, err := db.QueryContext(ctx, q)
	if err != nil {
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	var cols []sqColumn
	for qrows.Next() {
		var cid, name, typ, notnull, dflt any
		var pk int
		if err := qrows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			qrows.Close()
			pool.Release(ctx, db)
			return nil, errors.WithStack(err)
		}
		_ = cid
		_ = notnull
		_ = dflt
		_ = pk
		n, _ := name.(string)
		t, _ := typ.(string)
		cols = append(cols, sqColumn{name: n, vt: deriveSQLiteValueType(t)})
	}
	if err := qrows.Err(); err != nil {
		qrows.Close()
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	qrows.Close()
	pool.Release(ctx, db)
	if len(cols) == 0 {
		return nil, errors.Newf("sqlite_file: table %q has no columns", table)
	}
	return newSQLiteFileDatasource(pool, table, cols), nil
}

// NewSQLiteFileDatasourceWithSchema 以手动指定的列构建数据源，跳过 PRAGMA table_info 自省。
// 语义同 NewMySQLDatasourceWithSchema。
func NewSQLiteFileDatasourceWithSchema(pool *SQLiteFilePool, table string, columns []sqlitevtab.Column) (*SQLiteFileDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("sqlite_file: manual schema: invalid table identifier %q", table)
	}
	if len(columns) == 0 {
		return nil, errors.New("sqlite_file: manual schema requires at least one column")
	}
	cols := make([]sqColumn, len(columns))
	for i, c := range columns {
		cols[i] = sqColumn{name: c.Name, vt: c.ValueType}
	}
	return newSQLiteFileDatasource(pool, table, cols), nil
}

// newSQLiteFileDatasource 以给定列构建数据源（也供测试直接注入列）。
func newSQLiteFileDatasource(pool *SQLiteFilePool, table string, cols []sqColumn) *SQLiteFileDatasource {
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
	return &SQLiteFileDatasource{
		pool: pool, schema: schema, valueTypes: valueTypes, filterable: filterable,
		tableRef: quotePgIdent(table),
	}
}

// deriveSQLiteValueType 将 SQLite 列类型声明映射为 sqlitevtab.ValueType。SQLite 类型按
// 类型亲和（type affinity）判定：INTEGER 亲和→Int64，REAL 亲和→Float64，BOOLEAN→Bool，
// BLOB 亲和→Blob，其余（TEXT/NUMERIC/空）→String。未知声明保守映射为 String
// （sqlite 可灵活存储，String 在 sqlitevtab 复查时最宽容）。
func deriveSQLiteValueType(decl string) sqlitevtab.ValueType {
	t := strings.ToUpper(strings.TrimSpace(decl))
	switch {
	case t == "BOOL" || t == "BOOLEAN":
		return sqlitevtab.ValueTypeBool
	case strings.Contains(t, "INT") || t == "INTEGER":
		return sqlitevtab.ValueTypeInt64
	case strings.Contains(t, "REAL") || strings.Contains(t, "FLOA") || strings.Contains(t, "DOUB") || t == "NUMERIC":
		return sqlitevtab.ValueTypeFloat64
	case t == "BLOB" || strings.Contains(t, "BINARY") || strings.Contains(t, "BLOB"):
		return sqlitevtab.ValueTypeBlob
	default:
		return sqlitevtab.ValueTypeString
	}
}

// Open 实现 Datasource.Open：借出连接、按下推约束构建参数化查询、同步取回全部行并
// 返回内存游标；连接作用域到 fetch 结束即归还。
func (d *SQLiteFileDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	db, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	query, args := d.buildQuery(pushdown)
	rows, err := d.fetch(ctx, db, query, args)
	d.pool.Release(ctx, db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newMySQLRows(rows), nil
}

// buildQuery 依据下推约束构建参数化 SELECT（? 占位符）与绑定参数。Only 非 Any 列合成
// 谓词；OpGLOB/OpMATCH 跳过（语义与 sqlite 一致，复查无副作用）；无约束返回全表 SELECT。
func (d *SQLiteFileDatasource) buildQuery(pushdown []sqlitevtab.Constraint) (string, []any) {
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
		case sqlitevtab.OpLIKE:
			where = append(where, name+" LIKE "+ph+" ESCAPE '\\'")
			args = append(args, cst.Value)
		case sqlitevtab.OpISNULL:
			where = append(where, name+" IS NULL")
		case sqlitevtab.OpISNOTNULL:
			where = append(where, name+" IS NOT NULL")
		default:
			// OpGLOB / OpMATCH / 其他：不下推，交给 sqlite 复查
		}
	}
	if len(where) > 0 {
		sel.WriteString(" WHERE ")
		sel.WriteString(strings.Join(where, " AND "))
	}
	return sel.String(), args
}

// fetch 执行查询并把全部行扫描进内存（按 valueTypes 归一化，NULL→nil）。
func (d *SQLiteFileDatasource) fetch(ctx context.Context, db *sql.DB, query string, args []any) ([][]any, error) {
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
func (d *SQLiteFileDatasource) scanRow(qrows *sql.Rows) ([]any, error) {
	dest := make([]any, len(d.schema.Columns))
	ptrs := make([]any, len(dest))
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	if err := qrows.Scan(ptrs...); err != nil {
		return nil, errors.WithStack(err)
	}
	row := make([]any, len(dest))
	for i, v := range dest {
		row[i] = convertCell(v, d.valueTypes[i])
	}
	return row, nil
}
