// Package vtab（本文件）提供数据源预检能力（precheck.go）的单元测试：配置解码复用
// （decodeDatasourceConfig）、列类型键反向映射（valueTypeKey）、TestConn 连通性校验与
// IntrospectSchema 表结构自省（含 sqlite_file 分支）。MySQL/Postgres/ClickHouse 经
// go-sqlmock 注入、Redis 经 miniredis 起进程内实例验证成功路径；etcd 与 mongo 因
// clientv3.Client / mongo.Client 为具体类型、无本地 mock 而仅覆盖解码级路径。

package vtab

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestDecodeDatasourceConfig 验证 decodeDatasourceConfig 复用解析入口：各类型返回类型化
// 指针与默认值填充，未知类型/非法 JSON 报错。
func TestDecodeDatasourceConfig(t *testing.T) {
	Convey("decodeDatasourceConfig 返回类型化指针并应用 default tag", t, func() {
		rc, err := decodeDatasourceConfig(DatasourceTypeRedis, []byte(`{"addr":"127.0.0.1"}`))
		So(err, ShouldBeNil)
		redisCfg, ok := rc.(*RedisConfig)
		So(ok, ShouldBeTrue)
		So(redisCfg.Port, ShouldEqual, 6379) // default tag 生效

		mc, err := decodeDatasourceConfig(DatasourceTypeMySQL, []byte(`{"addr":"h","user":"u","dbName":"d","table":"t"}`))
		So(err, ShouldBeNil)
		mysqlCfg, ok := mc.(*MySQLTableConfig)
		So(ok, ShouldBeTrue)
		So(mysqlCfg.Port, ShouldEqual, 3306)

		pc, err := decodeDatasourceConfig(DatasourceTypePostgres, []byte(`{"addr":"h","user":"u","dbName":"d","table":"t"}`))
		So(err, ShouldBeNil)
		pgCfg, ok := pc.(*PostgresTableConfig)
		So(ok, ShouldBeTrue)
		So(pgCfg.Port, ShouldEqual, 5432)

		cc, err := decodeDatasourceConfig(DatasourceTypeClickHouse, []byte(`{"addr":"h","user":"u","dbName":"d","table":"t"}`))
		So(err, ShouldBeNil)
		chCfg, ok := cc.(*ClickHouseTableConfig)
		So(ok, ShouldBeTrue)
		So(chCfg.Port, ShouldEqual, 9000)

		sc, err := decodeDatasourceConfig(DatasourceTypeSQLiteFile, []byte(`{"table":"t"}`))
		So(err, ShouldBeNil)
		_, ok = sc.(*SQLiteFileTableConfig)
		So(ok, ShouldBeTrue)

		ec, err := decodeDatasourceConfig(DatasourceTypeEtcd, []byte(`{"endpoints":"h:1"}`))
		So(err, ShouldBeNil)
		_, ok = ec.(*EtcdTableConfig)
		So(ok, ShouldBeTrue)

		oc, err := decodeDatasourceConfig(DatasourceTypeMongo, []byte(`{"uri":"u","db":"d","collection":"c"}`))
		So(err, ShouldBeNil)
		_, ok = oc.(*MongoCollectionConfig)
		So(ok, ShouldBeTrue)
	})

	Convey("decodeDatasourceConfig 未知类型 / 非法 JSON 报错", t, func() {
		_, err := decodeDatasourceConfig("oracle", []byte(`{}`))
		So(err, ShouldNotBeNil)

		_, err = decodeDatasourceConfig(DatasourceTypeRedis, []byte(`{bad`))
		So(err, ShouldNotBeNil)
	})
}

// TestValueTypeKey 验证 sqlitevtab.ValueType 到手动 schema 类型键的反向映射。
func TestValueTypeKey(t *testing.T) {
	Convey("valueTypeKey 各类型键映射", t, func() {
		So(valueTypeKey(sqlitevtab.ValueTypeAny), ShouldEqual, "any")
		So(valueTypeKey(sqlitevtab.ValueTypeInt64), ShouldEqual, "integer")
		So(valueTypeKey(sqlitevtab.ValueTypeFloat64), ShouldEqual, "float")
		So(valueTypeKey(sqlitevtab.ValueTypeString), ShouldEqual, "string")
		So(valueTypeKey(sqlitevtab.ValueTypeBlob), ShouldEqual, "blob")
		So(valueTypeKey(sqlitevtab.ValueTypeBool), ShouldEqual, "bool")
	})
}

// TestTestConn 验证连通性探活：sqlite_file 不支持；mysql/postgres/clickhouse 经
// go-sqlmock 注入验证成功与失败；redis 经 miniredis 验证成功。
func TestTestConn(t *testing.T) {
	ctx := context.Background()

	Convey("sqlite_file 返回不支持连接测试", t, func() {
		err := TestConn(ctx, DatasourceTypeSQLiteFile, []byte(`{"table":"t"}`))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "connection test is not supported")
	})

	Convey("redis 经 miniredis 探活成功", t, func() {
		host, port := mockRedisAddr(t)
		cfg := []byte(`{"addr":"` + host + `","port":` + port + `,"struct":"string"}`)
		So(TestConn(ctx, DatasourceTypeRedis, cfg), ShouldBeNil)
	})

	Convey("mysql 经 mock db 探活成功与失败", t, func() {
		cfg := []byte(`{"addr":"127.0.0.1","port":3306,"user":"u","dbName":"app","table":"users"}`)
		hp := NewMySQLPool(&MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app"})
		// 清空该 Key 池内的闲置对象，使下次借出经 Make（override）重建 mock db，
		// 规避进程级池跨测试缓存对注入的干扰。
		hp.Close(ctx)

		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })
		mysqlClients.overrideDB(hp.key, db)
		t.Cleanup(func() { mysqlClients.overrideDB(hp.key, nil) })
		mock.ExpectPing()
		So(TestConn(ctx, DatasourceTypeMySQL, cfg), ShouldBeNil)
		So(mock.ExpectationsWereMet(), ShouldBeNil)

		// 失败路径：PingContext 返回错误
		dbF, mockF, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		So(err, ShouldBeNil)
		t.Cleanup(func() { dbF.Close() })
		mysqlClients.overrideDB(hp.key, dbF)
		hp.Close(ctx)
		mockF.ExpectPing().WillReturnError(errors.New("boom"))
		So(TestConn(ctx, DatasourceTypeMySQL, cfg), ShouldNotBeNil)
		So(mockF.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("postgres / clickhouse 经 mock db 探活成功", t, func() {
		pgCfg := []byte(`{"addr":"127.0.0.1","port":5432,"user":"u","dbName":"app","table":"users"}`)
		chCfg := []byte(`{"addr":"127.0.0.1","port":9000,"user":"u","dbName":"app","table":"users"}`)

		// postgres
		pgk := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"}).key
		NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"}).Close(ctx)
		pgDb, pgMock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		So(err, ShouldBeNil)
		t.Cleanup(func() { pgDb.Close() })
		postgresClients.overrideDB(pgk, pgDb)
		t.Cleanup(func() { postgresClients.overrideDB(pgk, nil) })
		pgMock.ExpectPing()
		So(TestConn(ctx, DatasourceTypePostgres, pgCfg), ShouldBeNil)
		So(pgMock.ExpectationsWereMet(), ShouldBeNil)

		// clickhouse
		chk := NewClickHousePool(&ClickHouseConfig{Addr: "127.0.0.1", Port: 9000, User: "u", DBName: "app"}).key
		NewClickHousePool(&ClickHouseConfig{Addr: "127.0.0.1", Port: 9000, User: "u", DBName: "app"}).Close(ctx)
		chDb, chMock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		So(err, ShouldBeNil)
		t.Cleanup(func() { chDb.Close() })
		clickhouseClients.overrideDB(chk, chDb)
		t.Cleanup(func() { clickhouseClients.overrideDB(chk, nil) })
		chMock.ExpectPing()
		So(TestConn(ctx, DatasourceTypeClickHouse, chCfg), ShouldBeNil)
		So(chMock.ExpectationsWereMet(), ShouldBeNil)
	})
}

// TestIntrospectSchema 验证表结构自省：mongo/sqlite_file 报错；mysql 经 mock db 自省
// 列映射；redis 经 miniredis 返回固定列。
func TestIntrospectSchema(t *testing.T) {
	ctx := context.Background()

	Convey("mongo 无固定表结构报错", t, func() {
		_, err := IntrospectSchema(ctx, DatasourceTypeMongo, []byte(`{"uri":"u","db":"d","collection":"c"}`), nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "no fixed schema")
	})

	Convey("sqlite_file 未上传 .db 文件报错", t, func() {
		_, err := IntrospectSchema(ctx, DatasourceTypeSQLiteFile, []byte(`{"table":"t"}`), nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "requires an uploaded .db file")
	})

	Convey("mysql 经 mock db 自省列映射", t, func() {
		hp := NewMySQLPool(&MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app"})
		hp.Close(ctx)
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })
		mysqlClients.overrideDB(hp.key, db)
		t.Cleanup(func() { mysqlClients.overrideDB(hp.key, nil) })

		mock.ExpectQuery("SHOW COLUMNS FROM `app`.`users`").WillReturnRows(
			sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
				AddRow("id", "int(11)", "NO", "PRI", nil, "").
				AddRow("name", "varchar(255)", "YES", "", nil, "").
				AddRow("score", "double", "YES", "", nil, ""))
		cols, err := IntrospectSchema(ctx, DatasourceTypeMySQL,
			[]byte(`{"addr":"127.0.0.1","port":3306,"user":"u","dbName":"app","table":"users"}`), nil)
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []DatasourceColumn{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "string"},
			{Name: "score", Type: "float"},
		})
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("redis 经 miniredis 返回 string 结构固定列", t, func() {
		host, port := mockRedisAddr(t)
		cfg := []byte(`{"addr":"` + host + `","port":` + port + `,"struct":"string"}`)
		cols, err := IntrospectSchema(ctx, DatasourceTypeRedis, cfg, nil)
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []DatasourceColumn{
			{Name: "key", Type: "string"},
			{Name: "value", Type: "string"},
		})
	})

	Convey("sqlite_file 经上传 .db 文件自省列定义", t, func() {
		f, err := os.CreateTemp("", "zk-test-*.db")
		So(err, ShouldBeNil)
		path := f.Name()
		So(f.Close(), ShouldBeNil)
		defer func() { _ = os.Remove(path) }()

		db, err := sql.Open("sqlite", path)
		So(err, ShouldBeNil)
		_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, score REAL, active BOOLEAN, data BLOB)`)
		So(err, ShouldBeNil)
		So(db.Close(), ShouldBeNil)

		raw, err := os.ReadFile(path)
		So(err, ShouldBeNil)
		cols, err := IntrospectSchema(ctx, DatasourceTypeSQLiteFile, []byte(`{"table":"users"}`), bytes.NewReader(raw))
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []DatasourceColumn{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "string"},
			{Name: "score", Type: "float"},
			{Name: "active", Type: "bool"},
			{Name: "data", Type: "blob"},
		})
	})
}

// mockRedisAddr 起一个 miniredis 并返回 host 与 port 两个字符串片段。
func mockRedisAddr(t *testing.T) (host, port string) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(s.Close)
	addr := s.Addr() // "127.0.0.1:PORT"
	idx := strings.LastIndex(addr, ":")
	return addr[:idx], addr[idx+1:]
}
