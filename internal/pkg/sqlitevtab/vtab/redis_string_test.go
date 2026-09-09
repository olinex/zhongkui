package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestRedisStringDatasource redis_string 的精确/未命中/类型不匹配/模式扫描行为。
func TestRedisStringDatasource(t *testing.T) {
	Convey("redis_string 精确等值查询", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(context.Background(), "name", "zk", 0)
		ds := NewRedisStringDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, value FROM redis_vtab WHERE key = ?`, "name")
		So(rows, ShouldResemble, [][]any{{"name", "zk"}})
	})

	Convey("redis_string 未命中与类型不匹配", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.HSet(context.Background(), "h", "f", "v")
		ds := NewRedisStringDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, value FROM redis_vtab WHERE key = ?`, "missing")
		So(rows, ShouldHaveLength, 0)

		// 类型非 string（hash）精确命中→无行
		_, rows = redisQuery(t, ds, `SELECT key, value FROM redis_vtab WHERE key = ?`, "h")
		So(rows, ShouldHaveLength, 0)
	})

	Convey("redis_string 模式扫描", t, func() {
		ctx := context.Background()
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(ctx, "user:1", "a", 0)
		c.Set(ctx, "user:2", "b", 0)
		c.Set(ctx, "other:9", "c", 0)
		c.HSet(ctx, "user:3", "f", "v") // hash，不应出现在 string 结果
		ds := NewRedisStringDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, value FROM redis_vtab WHERE key LIKE ?`, "user:%")
		So(rows, ShouldResemble, [][]any{{"user:1", "a"}, {"user:2", "b"}})
	})

	Convey("redis_string schema 与列可下推", t, func() {
		ds := NewRedisStringDatasource(NewRedisPool(&redis.Options{}, "default"))
		schema := ds.datasourceSchema()
		So(schema.Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "key", ValueType: sqlitevtab.ValueTypeString},
			{Name: "value", ValueType: sqlitevtab.ValueTypeString},
		})
		So(schema.Filterable, ShouldResemble, []int{0})
	})
}
