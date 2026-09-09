package vtab

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestRedisZSetDatasource redis_zset 的成员枚举、score 数值类型与升序行为。
func TestRedisZSetDatasource(t *testing.T) {
	ctx := context.Background()

	Convey("redis_zset 成员枚举与 score", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.ZAdd(ctx, "z", redis.Z{Score: 3, Member: "b"}, redis.Z{Score: 1, Member: "a"})
		ds := NewRedisZSetDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, member, score FROM redis_vtab WHERE key = ?`, "z")
		So(rows, ShouldResemble, [][]any{{"z", "a", 1.0}, {"z", "b", 3.0}})
	})

	Convey("redis_zset 未命中与类型不匹配", t, func() {
		_, pool := newRedisTestPool(t)
		c := seedClient(t, pool)
		c.Set(ctx, "str", "sv", 0)
		ds := NewRedisZSetDatasource(pool)

		_, rows := redisQuery(t, ds, `SELECT key, member, score FROM redis_vtab WHERE key = ?`, "str")
		So(rows, ShouldHaveLength, 0)
		_, rows = redisQuery(t, ds, `SELECT key, member, score FROM redis_vtab WHERE key = ?`, "missing")
		So(rows, ShouldHaveLength, 0)
	})

	Convey("redis_zset schema 与列可下推", t, func() {
		ds := NewRedisZSetDatasource(NewRedisPool(&redis.Options{}, "default"))
		schema := ds.datasourceSchema()
		So(schema.Columns[2], ShouldResemble, sqlitevtab.Column{Name: "score", ValueType: sqlitevtab.ValueTypeFloat64})
		So(schema.Filterable, ShouldResemble, []int{0})
	})
}
