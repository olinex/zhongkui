// Package vtab（本文件）提供 ClickHouse 数据源单元测试。
// 与 Postgres 类似，以 go-sqlmock 起 mock db 注入 ClickHousePool（无需真实 ClickHouse），
// 验证类型映射、下推构建与经 SQLitePool RegisterDatasource + Query 的端到端查询。
//
// This file provides ClickHouse datasource unit tests. Like Postgres, a go-sqlmock db is
// injected into a ClickHousePool (no real ClickHouse needed) to verify type mapping,
// pushdown query building and end-to-end queries via SQLitePool.RegisterDatasource + Query.
package vtab

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// newClickHouseMockPool 以注入 key 的 mock db 构建 ClickHousePool，供无需真实 ClickHouse
// 的测试使用。
// newClickHouseMockPool builds a ClickHousePool whose key resolves to an injected mock db,
// for tests that need no real ClickHouse.
func newClickHouseMockPool(t *testing.T, cfg *ClickHouseConfig, db *sql.DB) *ClickHousePool {
	t.Helper()
	pool := NewClickHousePool(cfg)
	clickhouseClients.overrideDB(pool.key, db)
	t.Cleanup(func() { clickhouseClients.overrideDB(pool.key, nil) })
	return pool
}

// chQuery 经 SQLitePool 注册数据源并执行一条只读 SQL，返回列名与行值（表名统一 ch_vtab）。
// chQuery registers the datasource on a SQLitePool and executes one read-only SQL
// statement, returning columns and row values (registered as ch_vtab).
func chQuery(t *testing.T, ds *ClickHouseDatasource, sql string, args ...any) ([]string, [][]any) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() { pool.Close(context.Background()) })
	ctx := context.Background()
	const name = "ch_vtab"
	if err := pool.RegisterDatasource(ctx, name, ds.datasourceSchema(),
		func(resourcepool.Key) (sqlitevtab.Datasource, error) { return ds, nil }); err != nil {
		t.Fatalf("RegisterDatasource: %v", err)
	}
	cols, rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		t.Fatalf("Query %q: %v", sql, err)
	}
	return cols, rows
}

// TestDeriveCHValueType ClickHouse 列类型到 ValueType 的映射（含 Nullable/LowCardinality
// 包装与参数剥离）。
func TestDeriveCHValueType(t *testing.T) {
	Convey("deriveCHValueType 类型映射", t, func() {
		So(deriveCHValueType("Int8"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveCHValueType("UInt64"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveCHValueType("Nullable(Int32)"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveCHValueType("LowCardinality(String)"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveCHValueType("Nullable(LowCardinality(String))"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveCHValueType("Float64"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveCHValueType("Decimal(10, 2)"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveCHValueType("Bool"), ShouldEqual, sqlitevtab.ValueTypeBool)
		So(deriveCHValueType("DateTime64(3)"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveCHValueType("FixedString(16)"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveCHValueType("UUID"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveCHValueType("Array(Int32)"), ShouldEqual, sqlitevtab.ValueTypeAny)
		So(deriveCHValueType("Tuple(a Int64)"), ShouldEqual, sqlitevtab.ValueTypeAny)
	})
}

// TestClickHouseBuildQuery 依据下推约束构建参数化 SELECT（? 占位符 + 双引号标识符）。
func TestClickHouseBuildQuery(t *testing.T) {
	pool := NewClickHousePool(&ClickHouseConfig{Addr: "127.0.0.1", Port: 9000, User: "u", DBName: "app"})
	ds := newClickHouseDatasource(pool, "users", []chColumn{
		{name: "id", vt: sqlitevtab.ValueTypeInt64},
		{name: "name", vt: sqlitevtab.ValueTypeString},
		{name: "meta", vt: sqlitevtab.ValueTypeAny},
	})
	full := `SELECT "id", "name", "meta" FROM "users"`

	Convey("无约束 → 全表 SELECT", t, func() {
		q, args := ds.buildQuery(nil)
		So(q, ShouldEqual, full)
		So(args, ShouldBeNil)
	})

	Convey("比较/等值下推（? 占位符）", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpEQ, Value: int64(42)},
		})
		So(q, ShouldEqual, full+` WHERE "id" = ?`)
		So(args, ShouldResemble, []any{int64(42)})

		q, args = ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpGE, Value: int64(10)},
			{Column: 1, Op: sqlitevtab.OpNE, Value: ""},
		})
		So(q, ShouldEqual, full+` WHERE "id" >= ? AND "name" != ?`)
		So(args, ShouldResemble, []any{int64(10), ""})
	})

	Convey("IS NULL / IS NOT NULL 无参下推", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpISNULL},
		})
		So(q, ShouldEqual, full+` WHERE "name" IS NULL`)
		So(args, ShouldHaveLength, 0)
	})

	Convey("LIKE 因大小写敏感性差异不下推（保持超集）", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpLIKE, Value: "user:%"},
		})
		So(q, ShouldEqual, full)
		So(args, ShouldBeNil)
	})

	Convey("GLOB/MATCH/不可过滤（Any）列不下推", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpGLOB, Value: "*"},
			{Column: 2, Op: sqlitevtab.OpEQ, Value: "x"},
		})
		So(q, ShouldEqual, full)
		So(args, ShouldBeNil)
	})
}

// TestClickHouseDatasourceEndToEnd 用 go-sqlmock 起 mock 数据源，经 ClickHouseDatasource
// 自省 + SQLitePool 端到端查询断言。
func TestClickHouseDatasourceEndToEnd(t *testing.T) {
	ctx := context.Background()
	const dbname, table = "app", "users"

	Convey("clickhouse vtable 自省与全表查询", t, func() {
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		intro := sqlmock.NewRows([]string{"name", "type", "default_type", "default_expression", "comment"}).
			AddRow("id", "UInt64", "", "", "").AddRow("name", "String", "", "", "")
		mock.ExpectQuery("DESCRIBE TABLE").WillReturnRows(intro)

		pool := newClickHouseMockPool(t, &ClickHouseConfig{Addr: "127.0.0.1", DBName: dbname}, db)
		ds, err := NewClickHouseDatasource(ctx, pool, table)
		So(err, ShouldBeNil)
		So(ds.datasourceSchema().Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "id", ValueType: sqlitevtab.ValueTypeInt64},
			{Name: "name", ValueType: sqlitevtab.ValueTypeString},
		})

		mock.ExpectQuery(`SELECT "id", "name" FROM "users"`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(uint64(1), "zk").AddRow(uint64(2), "wm"))

		_, rows := chQuery(t, ds, `SELECT id, name FROM ch_vtab`)
		So(rows, ShouldResemble, [][]any{{int64(1), "zk"}, {int64(2), "wm"}})
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("clickhouse vtable 等值下推", t, func() {
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		intro := sqlmock.NewRows([]string{"name", "type", "default_type", "default_expression", "comment"}).
			AddRow("id", "UInt64", "", "", "").AddRow("name", "String", "", "", "")
		mock.ExpectQuery("DESCRIBE TABLE").WillReturnRows(intro)

		// 使用与上一 Convey 不同的 DBName，避免同一池 Key 复用上一 mock db
		pool := newClickHouseMockPool(t, &ClickHouseConfig{Addr: "127.0.0.1", DBName: "app2"}, db)
		ds, err := NewClickHouseDatasource(ctx, pool, table)
		So(err, ShouldBeNil)

		// EQ 下推为参数化谓词；字面量经 sqlitevtab 绑定为 int64，sqlite 仍按超集复查
		mock.ExpectQuery(`SELECT "id", "name" FROM "users" WHERE "id" = \?`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(uint64(1), "zk"))
		_, rows := chQuery(t, ds, `SELECT id, name FROM ch_vtab WHERE id = 1`)
		So(rows, ShouldResemble, [][]any{{int64(1), "zk"}})
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("NewClickHouseDatasource 非法表名校验", t, func() {
		pool := NewClickHousePool(&ClickHouseConfig{Addr: "127.0.0.1", DBName: "app"})
		_, err := NewClickHouseDatasource(context.Background(), pool, "a-b")
		So(err, ShouldNotBeNil)
	})
}
