// Package vtab（本文件）提供 SQLite 文件数据源单元测试。
// 用真实临时 .db 文件（github.com/glebarez/go-sqlite 驱动）做端到端验证：自省 PRAGMA
// table_info、值归一化，以及源与 sqlitevtab 同语义下的 LIKE 下推。
//
// This file provides SQLite-file datasource unit tests. A real temp .db file
// (github.com/glebarez/go-sqlite driver) is used for end-to-end verification: PRAGMA
// table_info introspection, value normalization, and LIKE pushdown under identical
// SQLite semantics.
package vtab

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// newSQLiteFileMockPool 以注入 key 的 mock db 构建 SQLiteFilePool。实际上 sqlite 文件走
// 真实驱动，helper 仅为保持与其它池一致的注入点。
// newSQLiteFileMockPool builds a SQLiteFilePool with an injected mock db for its key.
// Since sqlite files use a real driver, the helper only keeps the injection point
// consistent with the other pools.
func newSQLiteFileMockPool(t *testing.T, cfg *SQLiteFileConfig, db *sql.DB) *SQLiteFilePool {
	t.Helper()
	pool := NewSQLiteFilePool(cfg)
	sqliteFileClients.overrideDB(pool.key, db)
	t.Cleanup(func() { sqliteFileClients.overrideDB(pool.key, nil) })
	return pool
}

// sqQuery 经 SQLitePool 注册数据源并执行一条只读 SQL，返回列名与行值（表名统一 sq_vtab）。
// sqQuery registers the datasource on a SQLitePool and executes one read-only SQL
// statement, returning columns and row values (registered as sq_vtab).
func sqQuery(t *testing.T, ds *SQLiteFileDatasource, sql string, args ...any) ([]string, [][]any) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() { pool.Close(context.Background()) })
	ctx := context.Background()
	const name = "sq_vtab"
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

// TestDeriveSQLiteValueType SQLite 列类型声明（按类型亲和）到 ValueType 的映射。
func TestDeriveSQLiteValueType(t *testing.T) {
	Convey("deriveSQLiteValueType 类型亲和映射", t, func() {
		So(deriveSQLiteValueType("INTEGER"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveSQLiteValueType("INT"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveSQLiteValueType("BIGINT"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveSQLiteValueType("UINT32"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveSQLiteValueType("REAL"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveSQLiteValueType("FLOAT"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveSQLiteValueType("DOUBLE"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveSQLiteValueType("NUMERIC"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveSQLiteValueType("BOOLEAN"), ShouldEqual, sqlitevtab.ValueTypeBool)
		So(deriveSQLiteValueType("BLOB"), ShouldEqual, sqlitevtab.ValueTypeBlob)
		So(deriveSQLiteValueType("TEXT"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveSQLiteValueType("VARCHAR(32)"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveSQLiteValueType(""), ShouldEqual, sqlitevtab.ValueTypeString)
	})
}

// TestSQLiteFileBuildQuery 依据下推约束构建参数化 SELECT。与源同为 SQLite，LIKE 可安全
// 下推。
func TestSQLiteFileBuildQuery(t *testing.T) {
	pool := NewSQLiteFilePool(&SQLiteFileConfig{Path: "/tmp/app.db"})
	ds := newSQLiteFileDatasource(pool, "users", []sqColumn{
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
			{Column: 0, Op: sqlitevtab.OpEQ, Value: int64(1)},
			{Column: 1, Op: sqlitevtab.OpNE, Value: ""},
		})
		So(q, ShouldEqual, full+` WHERE "id" = ? AND "name" != ?`)
		So(args, ShouldResemble, []any{int64(1), ""})
	})

	Convey("LIKE 下推（同 SQLite 语义）", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpLIKE, Value: "user:%"},
		})
		So(q, ShouldEqual, full+` WHERE "name" LIKE ? ESCAPE '\'`)
		So(args, ShouldResemble, []any{"user:%"})
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

// TestSQLiteFileDatasourceEndToEnd 用真实临时 .db 文件做自省 + 端到端查询，含 LIKE 下推。
func TestSQLiteFileDatasourceEndToEnd(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")

	// 预置一个 SQLite 文件与业务表数据（普通断言，不置于 Convey 上下文之外）
	setup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err = setup.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err = setup.Exec(`INSERT INTO users (id, name) VALUES (1, 'user:1'), (2, 'other')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	setup.Close()

	Convey("sqlite_file vtable 自省与全表查询", t, func() {
		pool := NewSQLiteFilePool(&SQLiteFileConfig{Path: path})
		ds, err := NewSQLiteFileDatasource(ctx, pool, "users")
		So(err, ShouldBeNil)
		So(ds.datasourceSchema().Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "id", ValueType: sqlitevtab.ValueTypeInt64},
			{Name: "name", ValueType: sqlitevtab.ValueTypeString},
		})

		_, rows := sqQuery(t, ds, `SELECT id, name FROM sq_vtab ORDER BY id`)
		So(rows, ShouldResemble, [][]any{{int64(1), "user:1"}, {int64(2), "other"}})
	})

	Convey("sqlite_file vtable 过滤（LIKE 下推，语义一致）", t, func() {
		pool := NewSQLiteFilePool(&SQLiteFileConfig{Path: path})
		ds, err := NewSQLiteFileDatasource(ctx, pool, "users")
		So(err, ShouldBeNil)

		_, rows := sqQuery(t, ds, `SELECT id, name FROM sq_vtab WHERE name LIKE ?`, "USER:%")
		So(rows, ShouldEqual, [][]any{{int64(1), "user:1"}})

		_, rows = sqQuery(t, ds, `SELECT id, name FROM sq_vtab WHERE name LIKE ?`, "nope:%")
		So(rows, ShouldHaveLength, 0)
	})
}
