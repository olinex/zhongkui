package vtab

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestPostgresPool PostgresPool（基于 resourcepool.Manager[*sql.DB]）的借/还/清理语义：
// 借出复用闲置 *sql.DB、关闭（Clear）后新借会重建不同句柄；sql.OpenDB 懒建连，故无需
// 真实 PostgreSQL 即可验证池语义。
// TestPostgresPool covers the resourcepool-backed PostgresPool borrow/recycle/clear
// semantics: a newly borrowed *sql.DB reuses the idle one, and a borrow after Close
// (Clear) rebuilds a different handle. As sql.OpenDB connects lazily, no real PostgreSQL
// is needed to verify the pool semantics.
func TestPostgresPool(t *testing.T) {
	ctx := context.Background()

	Convey("Acquire 归还后重新借出复用同一 *sql.DB，Release 幂等", t, func() {
		pool := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"})

		db1, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(db1, ShouldNotBeNil)

		// 归还借出中 db，归还幂等
		pool.Release(ctx, db1)
		pool.Release(ctx, db1)

		db2, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(db2, ShouldEqual, db1)
	})

	Convey("Close（Clear 闲置）后重新借出重建新 *sql.DB", t, func() {
		pool := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app"})

		db1, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		pool.Release(ctx, db1)
		pool.Close(ctx)

		db2, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(db2, ShouldNotEqual, db1)
		So(db2, ShouldNotBeNil)
	})

	Convey("NewPostgresPool 懒建连，不触发网络连接", t, func() {
		pool := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, DBName: "app"})
		db, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(db, ShouldNotBeNil) // sql.OpenDB 在此不建立物理连接
	})

	Convey("命名空间隔离派生不同于默认 Key", t, func() {
		poolA := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app", Namespace: "ns1"})
		poolB := NewPostgresPool(&PostgresConfig{Addr: "127.0.0.1", Port: 5432, User: "u", DBName: "app", Namespace: "ns2"})
		So(poolA.key, ShouldNotEqual, poolB.key)
	})
}
