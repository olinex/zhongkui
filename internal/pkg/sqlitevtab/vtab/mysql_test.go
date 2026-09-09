// Package vtab（本文件）提供 MySQL/MariaDB vtable 数据源的共享测试工具：
// 以 go-sqlmock 起一个 mock 数据源注入 MySQLPool，避免依赖真实 MySQL；
// 并提供经 SQLitePool RegisterDatasource + Query 的端到端查询辅助。
//
// This file provides shared test helpers for the MySQL/MariaDB vtable datasources: a
// go-sqlmock-based datasource injected into a MySQLPool (no real MySQL needed), plus an
// end-to-end query helper through SQLitePool.RegisterDatasource + Query.
package vtab

import (
	"context"
	"database/sql"
	"testing"

	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// newMySQLMockPool 以注入 key 的 mock db 构建 MySQLPool，供无需真实 MySQL 的测试使用。
// newMySQLMockPool builds a MySQLPool whose key resolves to an injected mock db, for
// tests that need no real MySQL.
func newMySQLMockPool(t *testing.T, cfg *MySQLConfig, db *sql.DB) *MySQLPool {
	t.Helper()
	pool := NewMySQLPool(cfg)
	mysqlClients.overrideDB(pool.key, db)
	t.Cleanup(func() { mysqlClients.overrideDB(pool.key, nil) })
	return pool
}

// mysqlQuery 经 SQLitePool 注册数据源并执行一条只读 SQL，返回列名与行值。
// mysqlQuery registers the datasource on a SQLitePool and executes one read-only SQL
// statement, returning columns and row values.
func mysqlQuery(t *testing.T, ds schemaProvider, sql string, args ...any) ([]string, [][]any) {
	t.Helper()
	pool := sqlitevtab.NewSQLitePool()
	t.Cleanup(func() {
		pool.Close(context.Background())
	})
	ctx := context.Background()
	const name = "mysql_vtab"
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
