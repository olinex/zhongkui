package sqlitevtab

import (
	// stdlib
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	// third-party
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// simpleSchema 单列字符串 schema。
// simpleSchema is a single string column schema.
var simpleSchema = Schema{Columns: []Column{{Name: "name", ValueType: ValueTypeString}}}

// registerStatic 注册一个由静态行集支撑的数据源。
// registerStatic registers a datasource backed by a static row set.
func registerStatic(t *testing.T, pool *SQLitePool, name string, schema Schema, data [][]any) {
	t.Helper()
	err := pool.RegisterDatasource(context.Background(), name, schema, func(resourcepool.Key) (Datasource, error) {
		return &testStaticDatasource{data: data}, nil
	})
	So(err, ShouldBeNil)
}

// TestPoolQuery 注册后即可经 SELECT ... FROM <数据源名> 查询。
func TestPoolQuery(t *testing.T) {
	Convey("池查询", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		registerStatic(t, pool, "names", simpleSchema, [][]any{{"a"}, {"b"}})

		cols, rows, err := pool.Query(context.Background(), "SELECT name FROM names ORDER BY name")
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []string{"name"})
		So(rows, ShouldResemble, [][]any{{"a"}, {"b"}})
	})
}

// TestPoolRejectsWrite 只读 authorizer 拒绝写语句且不产生数据变更。
func TestPoolRejectsWrite(t *testing.T) {
	Convey("只读 authorizer 拒绝写语句", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		registerStatic(t, pool, "names", simpleSchema, [][]any{{"a"}, {"b"}})

		for _, sql := range []string{
			"DELETE FROM names",
			"INSERT INTO names VALUES ('x')",
			"UPDATE names SET name = 'y'",
			"DROP TABLE names",
			"CREATE TABLE t(x)",
		} {
			_, _, err := pool.Query(context.Background(), sql)
			So(err, ShouldNotBeNil)
		}
		// 数据未被变更
		_, rows, err := pool.Query(context.Background(), "SELECT name FROM names ORDER BY name")
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{"a"}, {"b"}})
	})
}

// TestPoolMaxRows 单次查询超过行数上限返回明确错误而非截断结果。
func TestPoolMaxRows(t *testing.T) {
	Convey("行数上限", t, func() {
		pool := NewSQLitePool(WithMaxRows(2))
		defer pool.Close(context.Background())
		registerStatic(t, pool, "names", simpleSchema, [][]any{{"a"}, {"b"}, {"c"}})

		_, _, err := pool.Query(context.Background(), "SELECT name FROM names")
		So(err, ShouldNotBeNil)
		So(errors.Is(err, ErrTooManyRows), ShouldBeTrue)
	})
}

// TestPoolDuplicateResultColumn 重复结果列名返回明确错误。
func TestPoolDuplicateResultColumn(t *testing.T) {
	Convey("重复结果列名", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		registerStatic(t, pool, "names", simpleSchema, [][]any{{"a"}})

		_, _, err := pool.Query(context.Background(), "SELECT name AS x, name AS x FROM names")
		So(err, ShouldNotBeNil)
		So(errors.Is(err, ErrDuplicateResultColumn), ShouldBeTrue)
	})
}

// TestPoolDynamicLifecycle 运行期动态注册/覆盖/注销：注册后立即可查询，
// 覆盖后新工厂立即生效，注销后查询返回明确错误，重注册后恢复。
// 连接惰性同步（版本号 diff）使既有连接无需重启即可感知全部变更。
func TestPoolDynamicLifecycle(t *testing.T) {
	Convey("运行期动态注册/覆盖/注销", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())

		// 预热一条既有连接
		registerStatic(t, pool, "alpha", simpleSchema, [][]any{{"a1"}})
		_, _, err := pool.Query(context.Background(), "SELECT name FROM alpha")
		So(err, ShouldBeNil)

		Convey("注册新数据源后既有连接立即可查询", func() {
			registerStatic(t, pool, "beta", simpleSchema, [][]any{{"b1"}, {"b2"}})
			cols, rows, err := pool.Query(context.Background(), "SELECT name FROM beta ORDER BY name")
			So(err, ShouldBeNil)
			So(cols, ShouldResemble, []string{"name"})
			So(rows, ShouldResemble, [][]any{{"b1"}, {"b2"}})
		})

		Convey("覆盖数据源后新工厂立即生效（实例池重建）", func() {
			err := pool.RegisterDatasource(context.Background(), "alpha", simpleSchema, func(resourcepool.Key) (Datasource, error) {
				return &testStaticDatasource{data: [][]any{{"a2"}}}, nil
			})
			So(err, ShouldBeNil)
			_, rows, err := pool.Query(context.Background(), "SELECT name FROM alpha")
			So(err, ShouldBeNil)
			So(rows, ShouldResemble, [][]any{{"a2"}}) // 不再返回旧数据 a1
		})

		Convey("注销后查询返回明确错误", func() {
			err := pool.UnregisterDatasource(context.Background(), "alpha")
			So(err, ShouldBeNil)
			_, _, err = pool.Query(context.Background(), "SELECT name FROM alpha")
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrDatasourceNotRegistered), ShouldBeTrue)
			So(pool.Datasources(), ShouldNotContain, "alpha")
		})

		Convey("注销后可重新注册同名数据源", func() {
			So(pool.UnregisterDatasource(context.Background(), "alpha"), ShouldBeNil)
			registerStatic(t, pool, "alpha", simpleSchema, [][]any{{"a3"}})
			_, rows, err := pool.Query(context.Background(), "SELECT name FROM alpha")
			So(err, ShouldBeNil)
			So(rows, ShouldResemble, [][]any{{"a3"}})
		})

		Convey("注销未注册的数据源返回错误", func() {
			err := pool.UnregisterDatasource(context.Background(), "missing")
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrDatasourceNotRegistered), ShouldBeTrue)
		})
	})
}

// closableDatasource 实现 Datasource 与 io.Closer 的数据源，记录 Close 调用次数，
// 验证实例销毁由资源池驱动（CloseAll 时经 io.Closer 关闭）。
// 注意：数据源标准接口要求 Open(ctx, pushdown)（返回行游标），与
// resourcepool.Lifespan 的 Open(ctx) error（建立底层资源）方法名冲突，
// 因此数据源类型无法同时实现 Lifespan；资源池仍负责其惰性创建（Make）、
// 复用与销毁（io.Closer）。
// closableDatasource implements Datasource and io.Closer, recording Close calls
// to verify instance destruction is driven by the pool (via io.Closer on
// CloseAll). Note: the Datasource contract requires Open(ctx, pushdown) (returns
// a row cursor), which conflicts by name with Lifespan.Open(ctx) error
// (establishing the underlying resource), so a datasource type cannot also
// implement Lifespan; the pool still manages lazy creation (Make), reuse and
// destruction (io.Closer).
type closableDatasource struct {
	static *testStaticDatasource
	closes int32
}

// Open 实现 Datasource.Open。
// Open implements Datasource.Open.
func (d *closableDatasource) Open(ctx context.Context, pushdown []Constraint) (Rows, error) {
	return d.static.Open(ctx, pushdown)
}

// Close 实现 io.Closer（资源池 CloseAll 时销毁，计数）。
// Close implements io.Closer (destroyed by the pool on CloseAll, counted).
func (d *closableDatasource) Close() error {
	atomic.AddInt32(&d.closes, 1)
	return nil
}

// TestPoolDatasourceLifecycle 数据源实例生命周期由 resourcepool 统一驱动：
// 首次查询惰性创建（Make）、复用不重建、CloseAll 时经 io.Closer 销毁。
func TestPoolDatasourceLifecycle(t *testing.T) {
	Convey("数据源实例生命周期由资源池驱动", t, func() {
		pool := NewSQLitePool()
		var created []*closableDatasource
		err := pool.RegisterDatasource(context.Background(), "lived", simpleSchema, func(resourcepool.Key) (Datasource, error) {
			ds := &closableDatasource{static: &testStaticDatasource{data: [][]any{{"a"}}}}
			created = append(created, ds)
			return ds, nil
		})
		So(err, ShouldBeNil)

		// 首次查询：实例惰性创建（Make 调用一次）
		_, rows, err := pool.Query(context.Background(), "SELECT name FROM lived")
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{"a"}})
		So(created, ShouldHaveLength, 1)
		So(atomic.LoadInt32(&created[0].closes), ShouldEqual, 0)

		// 再次查询：复用同一实例，不重建
		_, _, err = pool.Query(context.Background(), "SELECT name FROM lived")
		So(err, ShouldBeNil)
		So(created, ShouldHaveLength, 1)

		// CloseAll：实例被 Close 销毁
		pool.Close(context.Background())
		So(atomic.LoadInt32(&created[0].closes), ShouldEqual, 1)
	})
}

// ctxPropKey 上下文透传测试键。
// ctxPropKey is the context propagation test key.
type ctxPropKey string

// ctxPropDatasource 记录 Open 收到的上下文。
// ctxPropDatasource records the context received by Open.
type ctxPropDatasource struct {
	recordingDatasource
}

// TestPoolQueryContextPropagation 查询上下文经连接对象透传给数据源 Open。
func TestPoolQueryContextPropagation(t *testing.T) {
	Convey("查询上下文透传给数据源", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "geo", ipGeoSchema, ipGeoData)

		key := ctxPropKey("trace-id")
		ctx := context.WithValue(context.Background(), key, "abc")
		_, _, err := pool.Query(ctx, "SELECT ip FROM geo WHERE ip = ?", "8.8.8.8")
		So(err, ShouldBeNil)
		So(rec.lastCtx, ShouldNotBeNil)
		So(rec.lastCtx.Value(key), ShouldEqual, "abc")
	})
}

// TestPoolConcurrentQueries 并发查询多个数据源结果正确且互不串扰。
func TestPoolConcurrentQueries(t *testing.T) {
	Convey("并发查询正确性", t, func() {
		pool := NewSQLitePool(WithConnections(4))
		defer pool.Close(context.Background())

		const n = 4
		for i := 0; i < n; i++ {
			name := "ds" + string(rune('a'+i))
			data := [][]any{{name + "-1"}, {name + "-2"}}
			registerStatic(t, pool, name, simpleSchema, data)
		}

		var wg sync.WaitGroup
		errCh := make(chan error, n*10)
		for g := 0; g < n; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					name := "ds" + string(rune('a'+j%n))
					_, rows, err := pool.Query(context.Background(), "SELECT name FROM "+name+" ORDER BY name")
					if err != nil {
						errCh <- err
						return
					}
					if len(rows) != 2 || rows[0][0] != name+"-1" || rows[1][0] != name+"-2" {
						errCh <- errors.Newf("unexpected rows for %s: %v", name, rows)
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			So(err, ShouldBeNil)
		}
	})
}

// TestPoolQueryTimeout 查询上下文无截止时间时套用连接级默认超时；
// 数据源阻塞超过超时后查询应返回错误。
func TestPoolQueryTimeout(t *testing.T) {
	Convey("查询默认超时", t, func() {
		pool := NewSQLitePool(WithQueryTimeout(50 * time.Millisecond))
		defer pool.Close(context.Background())

		// 阻塞型数据源：Open 返回永不结束的行集，等待 ctx 取消
		blocking := Schema{Columns: []Column{{Name: "x", ValueType: ValueTypeInt64}}}
		err := pool.RegisterDatasource(context.Background(), "blocking", blocking, func(resourcepool.Key) (Datasource, error) {
			return &blockingDatasource{}, nil
		})
		So(err, ShouldBeNil)

		start := time.Now()
		_, _, err = pool.Query(context.Background(), "SELECT x FROM blocking")
		elapsed := time.Since(start)
		So(err, ShouldNotBeNil)
		So(elapsed, ShouldBeLessThan, 2*time.Second)
	})
}

// blockingDatasource 返回一个随 ctx 取消而结束的阻塞行集。
// blockingDatasource returns a row set that blocks until the ctx is done.
type blockingDatasource struct{}

// Open 返回等待 ctx 取消的阻塞行集。
// Open returns a blocking row set that waits for ctx cancellation.
func (d *blockingDatasource) Open(ctx context.Context, pushdown []Constraint) (Rows, error) {
	return &blockingRows{ctx: ctx}, nil
}

// blockingRows 阻塞到 ctx 取消后以错误结束。
// blockingRows blocks until ctx cancellation, then fails with an error.
type blockingRows struct {
	ctx  context.Context
	done bool
}

// Next 阻塞等待 ctx 取消后返回错误。
// Next blocks until ctx cancellation and then returns an error.
func (r *blockingRows) Next() ([]any, error) {
	if r.done {
		return nil, io.EOF
	}
	r.done = true
	<-r.ctx.Done()
	return nil, r.ctx.Err()
}

// Close 无资源可释放。
// Close releases nothing.
func (r *blockingRows) Close() error { return nil }
