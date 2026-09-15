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

// newPostgresMockPool 以注入 key 的 mock db 构建 PostgresPool，供无需真实 PostgreSQL
// 的测试使用（参照 newMySQLMockPool）。
// newPostgresMockPool builds a PostgresPool whose key resolves to an injected mock db,
// for tests that need no real PostgreSQL (mirrors newMySQLMockPool).
func newPostgresMockPool(t *testing.T, cfg *PostgresConfig, db *sql.DB) *PostgresPool {
	t.Helper()
	pool := NewPostgresPool(cfg)
	postgresClients.overrideDB(pool.key, db)
	t.Cleanup(func() { postgresClients.overrideDB(pool.key, nil) })
	return pool
}

// pgQuery 经 SQLitePool 注册数据源并执行一条只读 SQL，返回列名与行值（表名统一 pg_vtab）。
// pgQuery registers the datasource on a SQLitePool and executes one read-only SQL
// statement, returning columns and row values (registered as pg_vtab).
func pgQuery(t *testing.T, ds *PostgresDatasource, sql string, args ...any) ([]string, [][]any) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() { pool.Close(context.Background()) })
	ctx := context.Background()
	const name = "pg_vtab"
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

// TestDerivePgValueType PostgreSQL udt_name 到 ValueType 的映射。
func TestDerivePgValueType(t *testing.T) {
	Convey("derivePgValueType 类型映射", t, func() {
		So(derivePgValueType("int2"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(derivePgValueType("int4"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(derivePgValueType("int8"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(derivePgValueType("numeric"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(derivePgValueType("float8"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(derivePgValueType("bool"), ShouldEqual, sqlitevtab.ValueTypeBool)
		So(derivePgValueType("varchar"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(derivePgValueType("text"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(derivePgValueType("timestamptz"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(derivePgValueType("jsonb"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(derivePgValueType("bytea"), ShouldEqual, sqlitevtab.ValueTypeBlob)
		So(derivePgValueType("geometry"), ShouldEqual, sqlitevtab.ValueTypeAny)
	})
}

// TestPgBuildQuery 依据下推约束构建参数化 SELECT（$n 占位符 + 双引号标识符）。
func TestPgBuildQuery(t *testing.T) {
	pool := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"})
	ds := newPostgresDatasource(pool, "app", "users", []pgColumn{
		{name: "id", vt: sqlitevtab.ValueTypeInt64},
		{name: "name", vt: sqlitevtab.ValueTypeString},
		{name: "meta", vt: sqlitevtab.ValueTypeAny},
	})
	full := `SELECT "id", "name", "meta" FROM "app"."users"`

	Convey("无约束 → 全表 SELECT", t, func() {
		q, args := ds.buildQuery(nil)
		So(q, ShouldEqual, full)
		So(args, ShouldBeNil)
	})

	Convey("精确等值 / 比较下推（$n 占位符递增）", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpEQ, Value: int64(42)},
		})
		So(q, ShouldEqual, full+` WHERE "id" = $1`)
		So(args, ShouldResemble, []any{int64(42)})

		q, args = ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpGE, Value: int64(10)},
			{Column: 1, Op: sqlitevtab.OpNE, Value: ""},
		})
		So(q, ShouldEqual, full+` WHERE "id" >= $1 AND "name" != $2`)
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

// TestPostgresDatasourceEndToEnd 用 go-sqlmock 起 mock 数据源，经 PostgresDatasource
// 自省 + SQLitePool 端到端查询断言。下推为超集提示，sqlite 可能选择全扫本地过滤，故
// mock 返回超集行集、由 sqlite 以 Omit=false 复查过滤后断言结果。
func TestPostgresDatasourceEndToEnd(t *testing.T) {
	ctx := context.Background()
	const dbname, table = "app", "users"

	Convey("postgres vtable 自省与全表查询", t, func() {
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		intro := sqlmock.NewRows([]string{"column_name", "udt_name"}).
			AddRow("id", "int4").AddRow("name", "text")
		mock.ExpectQuery("SELECT column_name, udt_name FROM information_schema.columns").
			WillReturnRows(intro)

		pool := newPostgresMockPool(t, &PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: dbname}, db)
		ds, err := NewPostgresDatasource(ctx, pool, table)
		So(err, ShouldBeNil)
		So(ds.datasourceSchema().Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "id", ValueType: sqlitevtab.ValueTypeInt64},
			{Name: "name", ValueType: sqlitevtab.ValueTypeString},
		})

		mock.ExpectQuery(`SELECT "id", "name" FROM "app"."users"`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "zk").AddRow(2, "wm"))

		_, rows := pgQuery(t, ds, `SELECT id, name FROM pg_vtab`)
		So(rows, ShouldResemble, [][]any{{int64(1), "zk"}, {int64(2), "wm"}})
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("postgres vtable 过滤（LIKE 不下推，由 sqlite 复查兜底）", t, func() {
		const dbname, table = "app2", "users"
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		intro := sqlmock.NewRows([]string{"column_name", "udt_name"}).
			AddRow("id", "int4").AddRow("name", "text")
		mock.ExpectQuery("SELECT column_name, udt_name FROM information_schema.columns").
			WillReturnRows(intro)

		pool := newPostgresMockPool(t, &PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: dbname}, db)
		ds, err := NewPostgresDatasource(ctx, pool, table)
		So(err, ShouldBeNil)

		// LIKE 未下推 → mock 只被请求全表 SELECT；匹配与否由 sqlite 本地过滤
		mock.ExpectQuery(`SELECT "id", "name" FROM "app2"."users"`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "user:1").AddRow(2, "other"))
		_, rows := pgQuery(t, ds, `SELECT id, name FROM pg_vtab WHERE name LIKE ?`, "user:%")
		So(rows, ShouldResemble, [][]any{{int64(1), "user:1"}})

		mock.ExpectQuery(`SELECT "id", "name" FROM "app2"."users"`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "user:1").AddRow(2, "other"))
		_, rows = pgQuery(t, ds, `SELECT id, name FROM pg_vtab WHERE name LIKE ?`, "nope:%")
		So(rows, ShouldHaveLength, 0)
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("NewPostgresDatasource 非法表名校验", t, func() {
		pool := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"})
		_, err := NewPostgresDatasource(context.Background(), pool, "a-b")
		So(err, ShouldNotBeNil)
	})
}
