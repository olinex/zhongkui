package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"
)

// TestRedisHashDatasource redis_hash 的 field 精确/全 field/类型不匹配行为。
func TestRedisHashDatasource(t *testing.T) {
	ctx := context.Background()

	Convey("redis_hash field 精确等值", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.HSet(ctx, "h", "f1", "v1")
		ds := NewRedisHashDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, field, value FROM redis_vtab WHERE key = ? AND field = ?`, "h", "f1")
		So(rows, ShouldResemble, [][]any{{"h", "f1", "v1"}})
	})

	Convey("redis_hash 全 field 且 field 升序", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.HSet(ctx, "h", "b", "bv")
		c.HSet(ctx, "h", "a", "av")
		ds := NewRedisHashDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, field, value FROM redis_vtab WHERE key = ?`, "h")
		So(rows, ShouldResemble, [][]any{{"h", "a", "av"}, {"h", "b", "bv"}})
	})

	Convey("redis_hash 类型不匹配与未命中", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(ctx, "s", "sv", 0)
		ds := NewRedisHashDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, field, value FROM redis_vtab WHERE key = ?`, "s")
		So(rows, ShouldHaveLength, 0)
		_, rows = redisQuery(t, ds, `SELECT key, field, value FROM redis_vtab WHERE key = ?`, "missing")
		So(rows, ShouldHaveLength, 0)
	})

	Convey("redis_hash schema 与列可下推", t, func() {
		ds := NewRedisHashDatasource(NewRedisPool(&redis.Options{}, "default"))
		schema := ds.datasourceSchema()
		So(len(schema.Columns), ShouldEqual, 3)
		So(schema.Filterable, ShouldResemble, []int{0, 1})
	})
}
