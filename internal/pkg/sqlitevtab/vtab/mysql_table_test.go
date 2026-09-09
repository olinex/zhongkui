package vtab

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestDeriveValueType MySQL/MariaDB 列类型到 ValueType 的映射。
func TestDeriveValueType(t *testing.T) {
	Convey("deriveValueType 类型映射", t, func() {
		So(deriveValueType("int(11)"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveValueType("BIGINT unsigned"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveValueType("tinyint(1)"), ShouldEqual, sqlitevtab.ValueTypeInt64)
		So(deriveValueType("decimal(10,2)"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveValueType("double"), ShouldEqual, sqlitevtab.ValueTypeFloat64)
		So(deriveValueType("boolean"), ShouldEqual, sqlitevtab.ValueTypeBool)
		So(deriveValueType("varchar(255)"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveValueType("tinytext"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveValueType("json"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveValueType("datetime"), ShouldEqual, sqlitevtab.ValueTypeString)
		So(deriveValueType("blob"), ShouldEqual, sqlitevtab.ValueTypeBlob)
		So(deriveValueType("geometry"), ShouldEqual, sqlitevtab.ValueTypeAny)
	})
}

// TestConvertCell 扫描值按 ValueType 归一化。
func TestConvertCell(t *testing.T) {
	Convey("convertCell 各类型归一化", t, func() {
		So(convertCell([]byte("abc"), sqlitevtab.ValueTypeString), ShouldEqual, "abc")
		So(convertCell(int64(5), sqlitevtab.ValueTypeInt64), ShouldEqual, int64(5))
		So(convertCell([]byte("12"), sqlitevtab.ValueTypeInt64), ShouldEqual, int64(12))
		So(convertCell([]byte("3.14"), sqlitevtab.ValueTypeFloat64), ShouldEqual, 3.14)
		So(convertCell(int64(1), sqlitevtab.ValueTypeBool), ShouldEqual, true)
		So(convertCell([]byte("0"), sqlitevtab.ValueTypeBool), ShouldEqual, false)
		So(convertCell([]byte{1, 2}, sqlitevtab.ValueTypeBlob), ShouldResemble, []byte{1, 2})
		So(convertCell(nil, sqlitevtab.ValueTypeString), ShouldBeNil)
		// 不可转换 → nil
		So(convertCell([]byte("abc"), sqlitevtab.ValueTypeInt64), ShouldBeNil)
	})
}

// TestBuildQuery 依据下推约束构建参数化 SELECT。
func TestBuildQuery(t *testing.T) {
	pool := NewMySQLPool(&MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app"})
	ds := newMySQLDatasource(pool, "app", "users", []mysqlColumn{
		{name: "id", vt: sqlitevtab.ValueTypeInt64},
		{name: "name", vt: sqlitevtab.ValueTypeString},
		{name: "meta", vt: sqlitevtab.ValueTypeAny},
	})
	full := "SELECT `id`, `name`, `meta` FROM `app`.`users`"

	Convey("无约束 → 全表 SELECT", t, func() {
		q, args := ds.buildQuery(nil)
		So(q, ShouldEqual, full)
		So(args, ShouldBeNil)
	})

	Convey("精确等值 / 比较 / LIKE 下推", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpEQ, Value: int64(42)},
		})
		So(q, ShouldEqual, full+" WHERE `id` = ?")
		So(args, ShouldResemble, []any{int64(42)})

		q, args = ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpLIKE, Value: "user:%"},
		})
		So(q, ShouldEqual, full+" WHERE `name` LIKE ?")
		So(args, ShouldResemble, []any{"user:%"})
	})

	Convey("IS NULL / IS NOT NULL 无参下推", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpISNULL},
		})
		So(q, ShouldEqual, full+" WHERE `name` IS NULL")
		So(args, ShouldHaveLength, 0)

		q, _ = ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpISNOTNULL},
		})
		So(q, ShouldEqual, full+" WHERE `name` IS NOT NULL")
	})

	Convey("多条件 AND 组合", t, func() {
		q, args := ds.buildQuery([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpGE, Value: int64(10)},
			{Column: 1, Op: sqlitevtab.OpNE, Value: ""},
		})
		So(q, ShouldEqual, full+" WHERE `id` >= ? AND `name` != ?")
		So(args, ShouldResemble, []any{int64(10), ""})
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

// TestMySQLDatasourceEndToEnd 用 go-sqlmock 起 mock 数据源，经
// MySQLDatasource 自省 + SQLitePool 端到端查询断言。下推为超集提示，sqlite 可能选择
// 全扫本地过滤，故 mock 返回超集行集、由 sqlite 以 Omit=false 复查过滤后断言结果。
func TestMySQLDatasourceEndToEnd(t *testing.T) {
	Convey("mysql vtable 自省与全表查询", t, func() {
		ctx := context.Background()
		const dbname, table = "app", "users"
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		show := sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("id", "int(11)", "NO", "PRI", nil, "").
			AddRow("name", "varchar(255)", "YES", "", nil, "")
		mock.ExpectQuery("SHOW COLUMNS FROM `app`.`users`").WillReturnRows(show)

		pool := newMySQLMockPool(t, &MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: dbname}, db)
		ds, err := NewMySQLDatasource(ctx, pool, table)
		So(err, ShouldBeNil)
		So(ds.datasourceSchema().Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "id", ValueType: sqlitevtab.ValueTypeInt64},
			{Name: "name", ValueType: sqlitevtab.ValueTypeString},
		})

		mock.ExpectQuery("SELECT `id`, `name` FROM `app`.`users`").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "zk").AddRow(2, "wm"))

		_, rows := mysqlQuery(t, ds, `SELECT id, name FROM mysql_vtab`)
		So(rows, ShouldResemble, [][]any{{int64(1), "zk"}, {int64(2), "wm"}})
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("mysql vtable 过滤（LIKE/未命中）由 sqlite 复查兜底", t, func() {
		ctx := context.Background()
		const dbname, table = "app2", "users"
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		show := sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("id", "int(11)", "NO", "PRI", nil, "").
			AddRow("name", "varchar(255)", "YES", "", nil, "")
		mock.ExpectQuery("SHOW COLUMNS FROM `app2`.`users`").WillReturnRows(show)

		pool := newMySQLMockPool(t, &MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: dbname}, db)
		ds, err := NewMySQLDatasource(ctx, pool, table)
		So(err, ShouldBeNil)

		// 返回超集行集（含不匹配行），匹配与否由 sqlite 本地过滤
		mock.ExpectQuery("SELECT `id`, `name` FROM `app2`.`users`").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "user:1").AddRow(2, "other"))
		_, rows := mysqlQuery(t, ds, `SELECT id, name FROM mysql_vtab WHERE name LIKE ?`, "user:%")
		So(rows, ShouldResemble, [][]any{{int64(1), "user:1"}})

		mock.ExpectQuery("SELECT `id`, `name` FROM `app2`.`users`").WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "user:1").AddRow(2, "other"))
		_, rows = mysqlQuery(t, ds, `SELECT id, name FROM mysql_vtab WHERE name LIKE ?`, "nope:%")
		So(rows, ShouldHaveLength, 0)
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("NewMySQLDatasource 非法表名校验", t, func() {
		pool := NewMySQLPool(&MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app"})
		_, err := NewMySQLDatasource(context.Background(), pool, "a-b")
		So(err, ShouldNotBeNil)
	})
}
