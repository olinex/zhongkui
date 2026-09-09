package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestRedisSetDatasource redis_set 的成员枚举与升序行为。
func TestRedisSetDatasource(t *testing.T) {
	ctx := context.Background()

	Convey("redis_set 成员枚举与升序", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.SAdd(ctx, "s", "b", "a")
		ds := NewRedisSetDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, member FROM redis_vtab WHERE key = ?`, "s")
		So(rows, ShouldResemble, [][]any{{"s", "a"}, {"s", "b"}})
	})

	Convey("redis_set 未命中与类型不匹配", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(ctx, "str", "sv", 0)
		ds := NewRedisSetDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, member FROM redis_vtab WHERE key = ?`, "str")
		So(rows, ShouldHaveLength, 0)
		_, rows = redisQuery(t, ds, `SELECT key, member FROM redis_vtab WHERE key = ?`, "missing")
		So(rows, ShouldHaveLength, 0)
	})

	Convey("redis_set 模式扫描", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.SAdd(ctx, "grp:1", "x", "y")
		c.SAdd(ctx, "grp:2", "z")
		c.SAdd(ctx, "no", "n")
		ds := NewRedisSetDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, member FROM redis_vtab WHERE key LIKE ?`, "grp:%")
		So(rows, ShouldResemble, [][]any{{"grp:1", "x"}, {"grp:1", "y"}, {"grp:2", "z"}})
	})

	Convey("redis_set schema 与列可下推", t, func() {
		ds := NewRedisSetDatasource(NewRedisPool(&redis.Options{}, "default"))
		schema := ds.datasourceSchema()
		So(schema.Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "key", ValueType: sqlitevtab.ValueTypeString},
			{Name: "member", ValueType: sqlitevtab.ValueTypeString},
		})
		So(schema.Filterable, ShouldResemble, []int{0})
	})
}
