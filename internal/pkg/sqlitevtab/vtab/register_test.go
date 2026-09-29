// Package vtab（本文件）提供前端配置驱动的统一注册封装（register.go / pools.go）的测试：
// 本地名校验、redis 各结构/mysql 注册成功、非法类型/结构/本地名报错、命名空间隔离
// （同名列在不同命名空间各自独立注册为裸名 vtable、连接池 Key 含命名空间维度）、
// 以及注册/注销生命周期。

package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestParseDatasourceColumns 验证数据源列映射的解析与非法列拒绝。
func TestParseDatasourceColumns(t *testing.T) {
	Convey("parseDatasourceColumns 合法列映射", t, func() {
		cols, err := parseDatasourceColumns([]DatasourceColumn{
			{Name: "id", Type: "integer"},
			{Name: "score", Type: "float"},
			{Name: "name", Type: "string"},
			{Name: "ok", Type: "bool"},
			{Name: "data", Type: "blob"},
			{Name: "raw", Type: "any"},
		})
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []sqlitevtab.Column{
			{Name: "id", ValueType: sqlitevtab.ValueTypeInt64},
			{Name: "score", ValueType: sqlitevtab.ValueTypeFloat64},
			{Name: "name", ValueType: sqlitevtab.ValueTypeString},
			{Name: "ok", ValueType: sqlitevtab.ValueTypeBool},
			{Name: "data", ValueType: sqlitevtab.ValueTypeBlob},
			{Name: "raw", ValueType: sqlitevtab.ValueTypeAny},
		})
	})

	Convey("parseDatasourceColumns 拒绝空/非法列", t, func() {
		_, err := parseDatasourceColumns(nil)
		So(err, ShouldNotBeNil)
		_, err = parseDatasourceColumns([]DatasourceColumn{{Name: "a-b", Type: "string"}})
		So(err, ShouldNotBeNil)
		_, err = parseDatasourceColumns([]DatasourceColumn{{Name: "1a", Type: "string"}})
		So(err, ShouldNotBeNil)
		_, err = parseDatasourceColumns([]DatasourceColumn{{Name: "a", Type: "nope"}})
		So(err, ShouldNotBeNil)
		_, err = parseDatasourceColumns([]DatasourceColumn{
			{Name: "a", Type: "string"}, {Name: "a", Type: "string"},
		})
		So(err, ShouldNotBeNil)
	})
}

// TestRegisterMySQLWithManualSchema 验证 MySQL 手动 schema 注册跳过建连自省、非法列报错。
func TestRegisterMySQLWithManualSchema(t *testing.T) {
	Convey("mysql 手动 schema 注册跳过 SHOW COLUMNS", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		// 无需注入 mock db：手动 schema 路径不做任何建连自省
		cfg := []byte(`{"addr":"127.0.0.1","port":3306,"user":"u","dbName":"app","table":"users",
			"schema":[{"name":"id","type":"integer"},{"name":"name","type":"string"}]}`)
		So(pools.Register(ctx, "prod", "users_manual", DatasourceTypeMySQL, cfg), ShouldBeNil)
		So(pools.Pool("prod").Datasources(), ShouldContain, "users_manual")
	})

	Convey("mysql 手动 schema 非法列报错", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		cfg := []byte(`{"addr":"127.0.0.1","table":"users","schema":[{"name":"a-b","type":"string"}]}`)
		So(pools.Register(ctx, "prod", "x", DatasourceTypeMySQL, cfg), ShouldNotBeNil)
	})
}

// TestValidateLocalName 验证本地名的合法标识符校验。
func TestValidateLocalName(t *testing.T) {
	Convey("ValidateLocalName 通过合法标识符", t, func() {
		So(ValidateLocalName("user_list"), ShouldBeNil)
		So(ValidateLocalName("abc0"), ShouldBeNil)
	})

	for _, bad := range []string{"", "1abc", "a-b", "a b"} {
		Convey("ValidateLocalName 拒绝 "+bad, t, func() {
			So(ValidateLocalName(bad), ShouldNotBeNil)
		})
	}
}

// TestRegisterRedis 验证 Redis 各结构以裸名注册、非法 struct 与非法 JSON 报错。
func TestRegisterRedis(t *testing.T) {
	for _, structType := range []string{"string", "hash", "list", "set", "zset"} {
		Convey("redis 结构 "+structType+" 以裸名注册成功", t, func() {
			ctx := context.Background()
			pools := NewPools(nil)
			t.Cleanup(func() { pools.Close(ctx) })

			cfg := []byte(`{"addr":"127.0.0.1","port":6379,"db":0,"struct":"` + structType + `"}`)
			So(pools.Register(ctx, "prod", "ds"+structType, DatasourceTypeRedis, cfg), ShouldBeNil)
			pool := pools.Pool("prod")
			// 裸名注册（不带命名空间前缀），rego 规则直接引用
			So(pool.Datasources(), ShouldContain, "ds"+structType)
			So(pool.Datasources(), ShouldNotContain, "prod__ds"+structType)
		})
	}

	Convey("非法 redis struct 报错", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		cfg := []byte(`{"addr":"127.0.0.1","struct":"stream"}`)
		So(pools.Register(ctx, "prod", "x", DatasourceTypeRedis, cfg), ShouldNotBeNil)
	})

	Convey("redis 配置非 JSON 报错", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		So(pools.Register(ctx, "prod", "x", DatasourceTypeRedis, []byte("{bad")), ShouldNotBeNil)
	})
}

// TestRegisterMySQL 验证 MySQL 无 schema 注册报错与非法数据源类型报错。
func TestRegisterMySQL(t *testing.T) {
	Convey("mysql 无 schema 注册报错（后端不再自动自省）", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })

		cfg := []byte(`{"addr":"127.0.0.1","port":3306,"user":"u","dbName":"app","table":"users"}`)
		So(pools.Register(ctx, "prod", "users_tbl", DatasourceTypeMySQL, cfg), ShouldNotBeNil)
		pool := pools.Pool("prod")
		So(pool.Datasources(), ShouldNotContain, "users_tbl")
	})

	Convey("非法数据源类型报错", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		So(pools.Register(ctx, "prod", "x", "oracle", []byte(`{}`)), ShouldNotBeNil)
	})
}

// TestRegisterTableDatasourceRequiresSchema 验证表类数据源缺 schema 时注册报错。
func TestRegisterTableDatasourceRequiresSchema(t *testing.T) {
	for _, tc := range []struct {
		typ string
		cfg string
	}{
		{DatasourceTypeMySQL, `{"addr":"127.0.0.1","user":"u","dbName":"app","table":"t"}`},
		{DatasourceTypePostgres, `{"addr":"127.0.0.1","user":"u","dbName":"app","table":"t"}`},
		{DatasourceTypeClickHouse, `{"addr":"127.0.0.1","user":"u","dbName":"app","table":"t"}`},
		{DatasourceTypeSQLiteFile, `{"table":"t"}`},
	} {
		Convey("类型 "+tc.typ+" 无 schema 注册报错（后端不再自动自省）", t, func() {
			ctx := context.Background()
			pools := NewPools(nil)
			t.Cleanup(func() { pools.Close(ctx) })
			So(pools.Register(ctx, "prod", "x", tc.typ, []byte(tc.cfg)), ShouldNotBeNil)
		})
	}
}

// TestRegisterNamespaceIsolation 验证同名列跨命名空间独立注册，连接池 Key 含命名空间维度。
func TestRegisterNamespaceIsolation(t *testing.T) {
	Convey("同名列跨命名空间各自注册成独立裸名 vtable", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })

		cfg := []byte(`{"addr":"127.0.0.1","port":6379,"db":0,"struct":"string"}`)
		So(pools.Register(ctx, "ns1", "x", DatasourceTypeRedis, cfg), ShouldBeNil)
		So(pools.Register(ctx, "ns2", "x", DatasourceTypeRedis, cfg), ShouldBeNil)
		pool1 := pools.Pool("ns1")
		pool2 := pools.Pool("ns2")
		// 各自独立池，同名列以裸名各驻其池，互不串扰
		So(pool1.Datasources(), ShouldContain, "x")
		So(pool2.Datasources(), ShouldContain, "x")
	})

	Convey("连接池 Key 含命名空间维度，跨命名空间不共用", t, func() {
		opts := &redis.Options{Addr: "127.0.0.1:6379", DB: 1}
		So(NewRedisPool(opts, "ns1").key, ShouldNotEqual, NewRedisPool(opts, "ns2").key)
		So(NewRedisPool(opts, "ns1").key, ShouldEqual, NewRedisPool(opts, "ns1").key)

		cfg1 := &MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app", Namespace: "ns1"}
		cfg2 := &MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app", Namespace: "ns2"}
		So(NewMySQLPool(cfg1).key, ShouldNotEqual, NewMySQLPool(cfg2).key)
		So(NewMySQLPool(cfg1).key, ShouldEqual, NewMySQLPool(cfg1).key)
	})
}

// TestUnregister 验证注销已注册数据源及重复注销的幂等行为。
func TestUnregister(t *testing.T) {
	Convey("Unregister 注销已注册数据源（裸名）", t, func() {
		ctx := context.Background()
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		cfg := []byte(`{"addr":"127.0.0.1","struct":"string"}`)
		So(pools.Register(ctx, "prod", "x", DatasourceTypeRedis, cfg), ShouldBeNil)
		So(pools.Unregister(ctx, "prod", "x"), ShouldBeNil)
		pool := pools.Pool("prod")
		So(pool.Datasources(), ShouldNotContain, "x")
		// 重复注销（已注销/未注册）不报错
		So(pools.Unregister(ctx, "prod", "x"), ShouldBeNil)
		// 命名空间池不存在也幂等不报错
		So(pools.Unregister(ctx, "nope", "x"), ShouldBeNil)
	})
}
