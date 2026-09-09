// Package vtab（本文件）提供前端配置驱动的统一注册封装（register.go / pools.go）的测试：
// 本地名校验、redis 各结构/mysql 注册成功、非法类型/结构/本地名报错、命名空间隔离
// （同名列在不同命名空间各自独立注册为裸名 vtable、连接池 Key 含命名空间维度）、
// 以及注册/注销生命周期。
//
// This file tests the unified registration wrapper (register.go / pools.go): local-name
// validation, successful redis/mysql registration, error cases for invalid types/structs/
// names, namespace isolation (the same name across namespaces yields independent bare-name
// vtables and namespace-scoped pool keys), and the register/unregister lifecycle.
package vtab

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"
)

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

func TestRegisterRedis(t *testing.T) {
	for _, structType := range []string{"string", "hash", "list", "set", "zset"} {
		Convey("redis 结构 "+structType+" 以裸名注册成功", t, func() {
			ctx := context.Background()
			pools := NewPools()
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
		pools := NewPools()
		t.Cleanup(func() { pools.Close(ctx) })
		cfg := []byte(`{"addr":"127.0.0.1","struct":"stream"}`)
		So(pools.Register(ctx, "prod", "x", DatasourceTypeRedis, cfg), ShouldNotBeNil)
	})

	Convey("redis 配置非 JSON 报错", t, func() {
		ctx := context.Background()
		pools := NewPools()
		t.Cleanup(func() { pools.Close(ctx) })
		So(pools.Register(ctx, "prod", "x", DatasourceTypeRedis, []byte("{bad")), ShouldNotBeNil)
	})
}

func TestRegisterMySQL(t *testing.T) {
	Convey("mysql 表以裸名注册成功", t, func() {
		ctx := context.Background()
		pools := NewPools()
		t.Cleanup(func() { pools.Close(ctx) })
		db, mock, err := sqlmock.New()
		So(err, ShouldBeNil)
		t.Cleanup(func() { db.Close() })

		show := sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
			AddRow("id", "int(11)", "NO", "PRI", nil, "").
			AddRow("name", "varchar(255)", "YES", "", nil, "")
		mock.ExpectQuery("SHOW COLUMNS FROM `app`.`users`").WillReturnRows(show)

		// 先注入 mock db 覆盖到 Register 内部 NewMySQLPool 将派生的连接池 Key
		hp := NewMySQLPool(&MySQLConfig{Addr: "127.0.0.1", Port: 3306, User: "u", DBName: "app", Namespace: "prod"})
		mysqlClients.overrideDB(hp.key, db)
		t.Cleanup(func() { mysqlClients.overrideDB(hp.key, nil) })

		cfg := []byte(`{"addr":"127.0.0.1","port":3306,"user":"u","dbName":"app","table":"users"}`)
		So(pools.Register(ctx, "prod", "users_tbl", DatasourceTypeMySQL, cfg), ShouldBeNil)
		pool := pools.Pool("prod")
		So(pool.Datasources(), ShouldContain, "users_tbl")
		So(mock.ExpectationsWereMet(), ShouldBeNil)
	})

	Convey("非法数据源类型报错", t, func() {
		ctx := context.Background()
		pools := NewPools()
		t.Cleanup(func() { pools.Close(ctx) })
		So(pools.Register(ctx, "prod", "x", "oracle", []byte(`{}`)), ShouldNotBeNil)
	})
}

func TestRegisterNamespaceIsolation(t *testing.T) {
	Convey("同名列跨命名空间各自注册成独立裸名 vtable", t, func() {
		ctx := context.Background()
		pools := NewPools()
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

func TestUnregister(t *testing.T) {
	Convey("Unregister 注销已注册数据源（裸名）", t, func() {
		ctx := context.Background()
		pools := NewPools()
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
