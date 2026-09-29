// Package vtab（本文件）提供 Redis vtable 数据源的共享测试工具：
// 以 miniredis 起一个进程内 Redis，返回 *redis.Client；并提供经 SQLitePool
// RegisterDatasource + Query 的端到端查询辅助。

package vtab

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// schemaProvider 抽象一个既是 Datasource 又暴露 schema 的数据源，供测试注册。
type schemaProvider interface {
	sqlitevtab.Datasource
	datasourceSchema() sqlitevtab.Schema
}

// newRedisTestPool 启动一个 miniredis 实例并返回面向它的 RedisPool（注入 client 子句）。
func newRedisTestPool(t *testing.T) (*miniredis.Miniredis, *RedisPool) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(s.Close)
	pool := NewRedisPool(&redis.Options{Addr: s.Addr()}, "default")
	t.Cleanup(func() { pool.Close(context.Background()) })
	return s, pool
}

// seedClient 从模拟池中借出最小 client 供测试播种/断言使用，不显式归还（单 client 复用）。
func seedClient(t *testing.T, pool *RedisPool) *redis.Client {
	t.Helper()
	c, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	return c
}

// redisQuery 经 SQLitePool 注册数据源并执行一条只读 SQL，返回列名与行值。
func redisQuery(t *testing.T, ds schemaProvider, sql string, args ...any) ([]string, [][]any) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() {
		pool.Close(context.Background())
	})
	ctx := context.Background()
	const name = "redis_vtab"
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

// redisQueryErr 是 redisQuery 的可看错版本，供调试与断言。
func redisQueryErr(t *testing.T, ds schemaProvider, sql string, args ...any) ([]string, [][]any, error) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() { pool.Close(context.Background()) })
	ctx := context.Background()
	if err := pool.RegisterDatasource(ctx, "redis_vtab", ds.datasourceSchema(), func(resourcepool.Key) (sqlitevtab.Datasource, error) { return ds, nil }); err != nil {
		return nil, nil, err
	}
	return pool.Query(ctx, sql, args...)
}
