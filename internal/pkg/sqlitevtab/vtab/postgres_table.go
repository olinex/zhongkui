// Package vtab（本文件）提供 PostgreSQL 数据源实现。
// PostgresDatasource 把某实例内的一张业务表暴露为一张 vtable：构造期经
// information_schema.columns 自省表结构导出列名与 ValueType（映射见 derivePgValueType）
// 并据此构建 schema；Open 时借出 *sql.DB，把下推约束构建为参数化 SELECT（$n 占位符），
// 同步扫描全部行后立即归还连接。仅非 ValueTypeAny 的列参与下推。
//
// 下推谓词取舍：PostgreSQL 的 LIKE 大小写敏感（依赖 collation），而 sqlite LIKE 默认
// 大小写不敏感——若直接下推 LIKE 会漏掉大小写不一致的真实匹配行，违反超集契约，故
// OpLIKE 一律不在此下推，交由 sqlite 以 Omit=false 复查兜底；OpGLOB/OpMATCH 同样无
// 直接语义而跳过。EQ/NE/GT/GE/LT/LE 与 IS NULL/IS NOT NULL 在数值/布尔/文本（默认
// collation 下）上与 sqlite 语义一致，予以安全下推。
//
// This file provides the PostgreSQL datasource. PostgresDatasource exposes a business
// table of one instance as a vtable: at construction it introspects
// information_schema.columns to derive column names and ValueTypes (see
// derivePgValueType) and builds the schema; on Open it borrows a *sql.DB, translates
// pushdown constraints into a parameterized SELECT ($n placeholders), synchronously
// scans all rows and immediately returns the connection. Only non-ValueTypeAny columns
// participate in pushdown.
//
// Pushdown trade-off: PostgreSQL LIKE is case-sensitive (collation-dependent) while
// sqlite LIKE is case-insensitive by default — pushing LIKE down directly would drop
// true matches differing in case, violating the superset contract, so OpLIKE is never
// pushed here but left to sqlite's Omit=false re-check; OpGLOB/OpMATCH likewise have no
// direct semantics and are skipped. EQ/NE/GT/GE/LT/LE and IS NULL/IS NOT NULL align with
// sqlite under default collations and are safely pushed.
package vtab

import (
	// stdlib
	"context"
	"database/sql"
	"strconv"
	"strings"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// pgColumn 描述自省得到的一列：列名与 ValueType。
// pgColumn describes one introspected column: its name and ValueType.
type pgColumn struct {
	name string
	vt   sqlitevtab.ValueType
}

// PostgresDatasource 是基于 PostgreSQL 一张表的 vtable 数据源。
// PostgresDatasource is the vtable datasource over one PostgreSQL table.
type PostgresDatasource struct {
	pgBase
	tableRef string
}

// NewPostgresDatasource 自省 pool 指向实例中的 table 表结构并创建数据源。构造会借出
// 一次连接执行 information_schema 查询（随后立即归还）。table 必须是合法 SQL 标识符；
// 返回的数据源经 SQLitePool.RegisterDatasource 注册（schema 用 ds.datasourceSchema()）
// 后即可查询。表限定于当前默认 schema（current_schema()），以匹配 pool 绑定的 DBName。
// NewPostgresDatasource introspects the structure of table in the instance behind pool
// and creates a datasource. Construction borrows a connection once, runs an
// information_schema query and immediately returns it. table must be a valid SQL
// identifier; the returned datasource is queryable once registered via
// SQLitePool.RegisterDatasource (schema via ds.datasourceSchema()). The table is scoped
// to the current schema (current_schema()) to match the DBName the pool is bound to.
func NewPostgresDatasource(ctx context.Context, pool *PostgresPool, table string) (*PostgresDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("postgres: invalid table identifier %q", table)
	}
	db, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	// 自省失败的分支都需先归还连接再上抛
	const q = `SELECT column_name, udt_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`
	qrows, err := db.QueryContext(ctx, q, table)
	if err != nil {
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	var cols []pgColumn
	for qrows.Next() {
		var name, udt string
		if err := qrows.Scan(&name, &udt); err != nil {
			qrows.Close()
			pool.Release(ctx, db)
			return nil, errors.WithStack(err)
		}
		cols = append(cols, pgColumn{name: name, vt: derivePgValueType(udt)})
	}
	if err := qrows.Err(); err != nil {
		qrows.Close()
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	qrows.Close()
	pool.Release(ctx, db)
	if len(cols) == 0 {
		return nil, errors.Newf("postgres: table %q has no columns", table)
	}
	return newPostgresDatasource(pool, pool.db, table, cols), nil
}

// NewPostgresDatasourceWithSchema 以手动指定的列构建数据源，跳过 information_schema 自省。
// 语义同 NewMySQLDatasourceWithSchema。
// NewPostgresDatasourceWithSchema builds a datasource with manually-specified columns,
// skipping information_schema introspection. Same semantics as NewMySQLDatasourceWithSchema.
func NewPostgresDatasourceWithSchema(pool *PostgresPool, table string, columns []sqlitevtab.Column) (*PostgresDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("postgres: manual schema: invalid table identifier %q", table)
	}
	if len(columns) == 0 {
		return nil, errors.New("postgres: manual schema requires at least one column")
	}
	cols := make([]pgColumn, len(columns))
	for i, c := range columns {
		cols[i] = pgColumn{name: c.Name, vt: c.ValueType}
	}
	return newPostgresDatasource(pool, pool.db, table, cols), nil
}

// newPostgresDatasource 以给定列构建数据源（也供测试直接注入列，无需真实 PostgreSQL）。
// newPostgresDatasource builds a datasource with the given columns (also used by tests to
// inject columns directly without a real PostgreSQL).
func newPostgresDatasource(pool *PostgresPool, db, table string, cols []pgColumn) *PostgresDatasource {
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
	// Filterable is written back into the schema so sqlitevtab's BestIndex emits and
	// pushes down the constraints.
	schema.Filterable = filterable
	return &PostgresDatasource{
		pgBase:   pgBase{pool: pool, schema: schema, valueTypes: valueTypes, filterable: filterable},
		tableRef: quotePgIdent(db) + "." + quotePgIdent(table),
	}
}

// derivePgValueType 将 PostgreSQL 列类型（udt_name）映射为 sqlitevtab.ValueType。
// udt_name 来自 information_schema，为不含长度/参数的简化类型名；未知类型映射为
// ValueTypeAny（declare 省略类型、按值推断，且不参与下推）。
// derivePgValueType maps a PostgreSQL column type (udt_name) to sqlitevtab.ValueType.
// udt_name from information_schema is a simplified type name without length/params;
// unknown types map to ValueTypeAny (no declared type, inferred from values, and not
// pushdown-filterable).
func derivePgValueType(udtName string) sqlitevtab.ValueType {
	switch udtName {
	case "int2", "int4", "int8", "oid", "smallint", "integer", "bigint",
		"smallserial", "serial", "bigserial":
		return sqlitevtab.ValueTypeInt64
	case "float4", "float8", "real", "double precision", "numeric", "decimal", "money":
		return sqlitevtab.ValueTypeFloat64
	case "bool", "boolean":
		return sqlitevtab.ValueTypeBool
	case "char", "bpchar", "varchar", "text", "name", "citext",
		"date", "time", "timetz", "timestamp", "timestamptz", "interval",
		"uuid", "json", "jsonb", "inet", "cidr", "macaddr", "xml":
		return sqlitevtab.ValueTypeString
	case "bytea":
		return sqlitevtab.ValueTypeBlob
	default:
		return sqlitevtab.ValueTypeAny
	}
}

// Open 实现 Datasource.Open：借出连接，依据下推约束构建参数化查询、同步取回全部行
// 并返回内存游标；连接作用域到 fetch 结束即归还。
// Open implements Datasource.Open: it borrows a connection, builds a parameterized
// query from the pushdown constraints, synchronously fetches all rows and returns an
// in-memory cursor; the connection is released once fetch is done.
func (d *PostgresDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	db, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	query, args := d.buildQuery(pushdown)
	rows, err := d.fetch(ctx, db, query, args)
	// fetch 已同步取回全部行，连接作用域到此结束，即时归还以便复用
	d.pool.Release(ctx, db)
	if err != nil {
		return nil, err
	}
	return newMySQLRows(rows), nil
}

// buildQuery 依据下推约束构建参数化 SELECT 与绑定参数（$n 占位符）。仅对可下推（非 Any）
// 列合成谓词；OpLIKE/OpGLOB/OpMATCH 因与 sqlite 语义不一致或无可直接语义而跳过（见
// 文件头注释）；无约束时返回全表 SELECT（宽松超集）。
// buildQuery builds a parameterized SELECT and its bound args from the pushdown
// constraints ($n placeholders). Predicates are synthesized only for filterable (non-Any)
// columns; OpLIKE/OpGLOB/OpMATCH are skipped due to sqlite-semantics divergence or a lack
// of a direct translation (see the file header comment); with no constraints a full-table
// SELECT is returned (a loose superset).
func (d *PostgresDatasource) buildQuery(pushdown []sqlitevtab.Constraint) (string, []any) {
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
	argIdx := 1
	for _, cst := range pushdown {
		if !containsInt(d.filterable, cst.Column) {
			continue
		}
		name := quotePgIdent(d.schema.Columns[cst.Column].Name)
		ph := "$" + strconv.Itoa(argIdx)
		switch cst.Op {
		case sqlitevtab.OpEQ:
			where = append(where, name+" = "+ph)
			args = append(args, cst.Value)
			argIdx++
		case sqlitevtab.OpNE:
			where = append(where, name+" != "+ph)
			args = append(args, cst.Value)
			argIdx++
		case sqlitevtab.OpGT:
			where = append(where, name+" > "+ph)
			args = append(args, cst.Value)
			argIdx++
		case sqlitevtab.OpGE:
			where = append(where, name+" >= "+ph)
			args = append(args, cst.Value)
			argIdx++
		case sqlitevtab.OpLT:
			where = append(where, name+" < "+ph)
			args = append(args, cst.Value)
			argIdx++
		case sqlitevtab.OpLE:
			where = append(where, name+" <= "+ph)
			args = append(args, cst.Value)
			argIdx++
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

// fetch 执行查询并把全部行扫描进内存（按 valueTypes 归一化，NULL→nil）。返回的行
// 顺序即驱动返回顺序。行游标复用包内通用预先行集游标 newMySQLRows（纯内存集，与
// MySQL 无耦合）。
// fetch executes the query and scans all rows into memory (normalized per valueTypes,
// NULL→nil). The row order is the driver's returned order. The row-set cursor reuses the
// package-wide precomputed in-memory cursor newMySQLRows (a plain in-memory set, not
// MySQL-coupled).
func (d *PostgresDatasource) fetch(ctx context.Context, db *sql.DB, query string, args []any) ([][]any, error) {
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
// scanRow scans one row against the schema columns and normalizes each cell via
// convertCell.
func (d *PostgresDatasource) scanRow(qrows *sql.Rows) ([]any, error) {
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
