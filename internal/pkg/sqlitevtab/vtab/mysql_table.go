// Package vtab（本文件）提供 MySQL/MariaDB 数据源实现。
// MySQLDatasource 把某实例内的一张业务表暴露为一张 vtable：构造期经
// SHOW COLUMNS 自省表结构导出列名与 ValueType（映射见 deriveValueType）并据此构建
// schema；Open 时借出 *sql.DB，把下推约束构建为参数化 SELECT，同步扫描全部行后
// 立即归还连接。仅非 ValueTypeAny 的列参与下推，GLOB/MATCH 与类型不确定的谓词
// 交由 sqlite 以 Omit=false 复查兜底，保证下推结果为真实匹配行的超集。
package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"slices"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// mysqlColumn 描述自省得到的一列：列名与 ValueType。
type mysqlColumn struct {
	name string
	vt   sqlitevtab.ValueType
}

// MySQLDatasource 是基于 MySQL/MariaDB 一张表的 vtable 数据源。
type MySQLDatasource struct {
	mysqlBase
	tableRef string
}

// NewMySQLDatasource 自省 pool 指向实例中的 table 表结构并创建数据源。构造会借出
// 一次连接执行 SHOW COLUMNS（随后立即归还）。table 必须是合法 SQL 标识符；返回的
// 数据源经 SQLitePool.RegisterDatasource 注册（schema 用 ds.datasourceSchema()）
// 后即可查询。
func NewMySQLDatasource(ctx context.Context, pool *MySQLPool, table string) (*MySQLDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("mysql: invalid table identifier %q", table)
	}
	db, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	// 自省失败的分支都需先归还连接再上抛
	q := "SHOW COLUMNS FROM " + quoteIdent(pool.db) + "." + quoteIdent(table)
	qrows, err := db.QueryContext(ctx, q)
	if err != nil {
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	var cols []mysqlColumn
	for qrows.Next() {
		var field, typ, null, key, extra string
		var def any
		if err := qrows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			qrows.Close()
			pool.Release(ctx, db)
			return nil, errors.WithStack(err)
		}
		cols = append(cols, mysqlColumn{name: field, vt: deriveValueType(typ)})
	}
	if err := qrows.Err(); err != nil {
		qrows.Close()
		pool.Release(ctx, db)
		return nil, errors.WithStack(err)
	}
	qrows.Close()
	pool.Release(ctx, db)
	if len(cols) == 0 {
		return nil, errors.Newf("mysql: table %q has no columns", table)
	}
	return newMySQLDatasource(pool, pool.db, table, cols), nil
}

// NewMySQLDatasourceWithSchema 以手动指定的列构建数据源，跳过 SHOW COLUMNS 自省。
// 适用于接入账号无表结构查看权限、由客户自行提供数据结构的场景；table 仍须为合法 SQL
// 标识符，columns 至少一列（名称/类型键已在 registerInto 经 parseDatasourceColumns 校验）。
func NewMySQLDatasourceWithSchema(pool *MySQLPool, table string, columns []sqlitevtab.Column) (*MySQLDatasource, error) {
	if !mysqlIdentPattern.MatchString(table) {
		return nil, errors.Newf("mysql: invalid table identifier %q", table)
	}
	if len(columns) == 0 {
		return nil, errors.New("mysql: manual schema requires at least one column")
	}
	cols := make([]mysqlColumn, len(columns))
	for i, c := range columns {
		cols[i] = mysqlColumn{name: c.Name, vt: c.ValueType}
	}
	return newMySQLDatasource(pool, pool.db, table, cols), nil
}

// newMySQLDatasource 以给定列构建数据源（也供测试直接注入列，无需真实 MySQL）。
func newMySQLDatasource(pool *MySQLPool, db, table string, cols []mysqlColumn) *MySQLDatasource {
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
	return &MySQLDatasource{
		mysqlBase: mysqlBase{pool: pool, schema: schema, valueTypes: valueTypes, filterable: filterable},
		tableRef:  quoteIdent(db) + "." + quoteIdent(table),
	}
}

// deriveValueType 将 MySQL/MariaDB 列类型映射为 sqlitevtab.ValueType。base 取自类型名
// 首 token（去掉括号参数与 unsigned 等修饰）；未知类型映射为 ValueTypeAny（declare
// 省略类型、按值推断，且不参与下推）。
func deriveValueType(colType string) sqlitevtab.ValueType {
	base := strings.ToLower(strings.TrimSpace(colType))
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "serial", "year":
		return sqlitevtab.ValueTypeInt64
	case "float", "double", "real", "decimal", "numeric", "dec", "fixed":
		return sqlitevtab.ValueTypeFloat64
	case "boolean", "bool":
		return sqlitevtab.ValueTypeBool
	case "char", "varchar", "character", "nchar", "nvarchar",
		"tinytext", "text", "mediumtext", "longtext",
		"enum", "set", "json", "date", "time", "datetime", "timestamp":
		return sqlitevtab.ValueTypeString
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return sqlitevtab.ValueTypeBlob
	default:
		return sqlitevtab.ValueTypeAny
	}
}

// Open 实现 Datasource.Open：借出连接，依据下推约束构建参数化查询、同步取回全部行
// 并返回内存游标；连接作用域到 fetch 结束即归还。
func (d *MySQLDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
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

// buildQuery 依据下推约束构建参数化 SELECT 与绑定参数。仅对可下推（非 Any）列合成
// 谓词；OpGLOB/OpMATCH 无 MySQL 直接语义而跳过；无约束时返回全表 SELECT（宽松超集）。
func (d *MySQLDatasource) buildQuery(pushdown []sqlitevtab.Constraint) (string, []any) {
	var sel strings.Builder
	sel.WriteString("SELECT ")
	for i, col := range d.schema.Columns {
		if i > 0 {
			sel.WriteString(", ")
		}
		sel.WriteString(quoteIdent(col.Name))
	}
	sel.WriteString(" FROM ")
	sel.WriteString(d.tableRef)

	var where []string
	var args []any
	for _, cst := range pushdown {
		if !containsInt(d.filterable, cst.Column) {
			continue
		}
		name := quoteIdent(d.schema.Columns[cst.Column].Name)
		switch cst.Op {
		case sqlitevtab.OpEQ:
			where = append(where, name+" = ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpNE:
			where = append(where, name+" != ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpGT:
			where = append(where, name+" > ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpGE:
			where = append(where, name+" >= ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpLT:
			where = append(where, name+" < ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpLE:
			where = append(where, name+" <= ?")
			args = append(args, cst.Value)
		case sqlitevtab.OpLIKE:
			where = append(where, name+" LIKE ?")
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

// containsInt 判断切片是否包含给定整数。
func containsInt(a []int, v int) bool {
	return slices.Contains(a, v)
}

// fetch 执行查询并把全部行扫描进内存（按 valueTypes 归一化，NULL→nil）。返回的行
// 顺序即驱动返回顺序。
func (d *MySQLDatasource) fetch(ctx context.Context, db *sql.DB, query string, args []any) ([][]any, error) {
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
func (d *MySQLDatasource) scanRow(qrows *sql.Rows) ([]any, error) {
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
