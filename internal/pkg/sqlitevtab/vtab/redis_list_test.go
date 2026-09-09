package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestRedisListDatasource redis_list 的元素枚举与 position 序号行为。
func TestRedisListDatasource(t *testing.T) {
	ctx := context.Background()

	Convey("redis_list 元素枚举与 position", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.RPush(ctx, "l", "a", "b", "c")
		ds := NewRedisListDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, position, value FROM redis_vtab WHERE key = ?`, "l")
		So(rows, ShouldResemble, [][]any{{"l", int64(0), "a"}, {"l", int64(1), "b"}, {"l", int64(2), "c"}})
	})

	Convey("redis_list 未命中与类型不匹配", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(ctx, "s", "sv", 0)
		ds := NewRedisListDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, position, value FROM redis_vtab WHERE key = ?`, "s")
		So(rows, ShouldHaveLength, 0)
		_, rows = redisQuery(t, ds, `SELECT key, position, value FROM redis_vtab WHERE key = ?`, "missing")
		So(rows, ShouldHaveLength, 0)
	})

	Convey("redis_list schema 与列可下推", t, func() {
		ds := NewRedisListDatasource(NewRedisPool(&redis.Options{}, "default"))
		schema := ds.datasourceSchema()
		So(schema.Columns[1], ShouldResemble, sqlitevtab.Column{Name: "position", ValueType: sqlitevtab.ValueTypeInt64})
		So(schema.Filterable, ShouldResemble, []int{0})
	})
}
