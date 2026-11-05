package vtab

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestRedisPool RedisPool（基于 resourcepool.Manager[*redis.Client]）的借/还/清理语义：
// 借出复用闲置 client、关闭（Clear）后新借会重建不同 client。
func TestRedisPool(t *testing.T) {
	ctx := context.Background()

	Convey("Acquire 归还后重新借出复用同一 client，Release 幂等", t, func() {
		_, pool := newRedisTestPool(t)

		c1, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)

		// 归还借出中 client，归还幂等
		pool.Release(ctx, c1)
		pool.Release(ctx, c1)

		c2, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(c2, ShouldEqual, c1)
	})

	Convey("Close（Clear 闲置）后重新借出重建新 client", t, func() {
		_, pool := newRedisTestPool(t)

		c1, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		pool.Release(ctx, c1)
		pool.Close(ctx)

		c2, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)
		So(c2, ShouldNotEqual, c1)
		So(c2, ShouldNotBeNil)
	})

	Convey("newRedisTestPool 返回可实际查询的 client", t, func() {
		s, pool := newRedisTestPool(t)
		c, err := pool.Acquire(ctx)
		So(err, ShouldBeNil)

		So(c.Set(ctx, "k", "v", 0).Err(), ShouldBeNil)
		So(c.Get(ctx, "k").Val(), ShouldEqual, "v")
		So(s.Exists("k"), ShouldBeTrue)
	})
}
