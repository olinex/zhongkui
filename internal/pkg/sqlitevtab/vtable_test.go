package sqlitevtab

import (
	// 标准库
	"context"
	"sync"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/resourcepool"
)

// recordingDatasource 记录每次 Open 收到的下推约束与查询上下文，内部委托
// StaticDatasource 产出数据，用于断言 WHERE 约束下推语义。
type recordingDatasource struct {
	schema Schema
	data   [][]any

	mu       sync.Mutex
	opens    int
	lastCtx  context.Context
	lastPush []Constraint
}

// Open 记录约束与上下文并委托静态数据源。
func (d *recordingDatasource) Open(ctx context.Context, pushdown []Constraint) (Rows, error) {
	d.mu.Lock()
	d.opens++
	d.lastCtx = ctx
	d.lastPush = append([]Constraint(nil), pushdown...)
	d.mu.Unlock()
	return (&testStaticDatasource{data: d.data}).Open(ctx, pushdown)
}

// lastPushdown 返回最近一次 Open 的下推约束副本。
func (d *recordingDatasource) lastPushdown() []Constraint {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Constraint(nil), d.lastPush...)
}

// registerRecording 注册一个带 recordingDatasource 的数据源并返回该实例。
func registerRecording(t *testing.T, pool *SQLitePool, name string, schema Schema, data [][]any) *recordingDatasource {
	t.Helper()
	rec := &recordingDatasource{schema: schema, data: data}
	err := pool.RegisterDatasource(context.Background(), name, schema, func(resourcepool.Key) (Datasource, error) {
		return rec, nil
	})
	So(err, ShouldBeNil)
	return rec
}

// ipGeoSchema 测试用 schema：ip 与 score 可过滤，name 不可过滤。
var ipGeoSchema = Schema{
	Columns: []Column{
		{Name: "ip", ValueType: ValueTypeString},
		{Name: "score", ValueType: ValueTypeInt64},
		{Name: "name", ValueType: ValueTypeString},
	},
	Filterable: []int{0, 1},
}

// ipGeoData 测试用行集。
var ipGeoData = [][]any{
	{"8.8.8.8", int64(90), "google"},
	{"1.2.3.4", int64(120), "cn"},
	{"5.6.7.8", int64(60), "us"},
}

// TestPushdown_EQ 可过滤列上的等值约束应下推为 OpEQ。
func TestPushdown_EQ(t *testing.T) {
	Convey("等值约束下推", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "geo", ipGeoSchema, ipGeoData)

		cols, rows, err := pool.Query(context.Background(), "SELECT ip FROM geo WHERE ip = ?", "1.2.3.4")
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []string{"ip"})
		So(rows, ShouldResemble, [][]any{{"1.2.3.4"}})
		So(rec.lastPushdown(), ShouldResemble, []Constraint{{Column: 0, Op: OpEQ, Value: "1.2.3.4"}})
	})
}

// TestPushdown_Range 可过滤列上的范围约束应下推（GT 组合 GE/LT/LE）。
func TestPushdown_Range(t *testing.T) {
	Convey("范围约束下推", t, func() {
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "geo", ipGeoSchema, ipGeoData)

		_, rows, err := pool.Query(context.Background(), "SELECT ip FROM geo WHERE score > ? AND score <= ?", int64(60), int64(120))
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{"8.8.8.8"}, {"1.2.3.4"}})
		So(rec.lastPushdown(), ShouldResemble, []Constraint{
			{Column: 1, Op: OpGT, Value: int64(60)},
			{Column: 1, Op: OpLE, Value: int64(120)},
		})
	})
}

// TestPushdown_Like LIKE 约束应下推为 OpLIKE（值原样透传）。
func TestPushdown_Like(t *testing.T) {
	Convey("LIKE 约束下推", t, func() {
		schema := Schema{Columns: []Column{{Name: "name", ValueType: ValueTypeString}}, Filterable: []int{0}}
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "names", schema, [][]any{{"google"}, {"good"}, {"bad"}})

		_, rows, err := pool.Query(context.Background(), "SELECT name FROM names WHERE name LIKE ?", "go%")
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{"google"}, {"good"}})
		So(rec.lastPushdown(), ShouldResemble, []Constraint{{Column: 0, Op: OpLIKE, Value: "go%"}})
	})
}

// TestPushdown_IsNull IS NULL 约束应下推为 OpISNULL（无右操作数）。
func TestPushdown_IsNull(t *testing.T) {
	Convey("IS NULL 约束下推", t, func() {
		schema := Schema{Columns: []Column{{Name: "name", ValueType: ValueTypeString}}, Filterable: []int{0}}
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "names", schema, [][]any{{"google"}, {nil}})

		_, rows, err := pool.Query(context.Background(), "SELECT name FROM names WHERE name IS NULL")
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{nil}})
		So(rec.lastPushdown(), ShouldResemble, []Constraint{{Column: 0, Op: OpISNULL}})
	})
}

// TestNoFilterable_FullScan 无 Filterable 列时退化为全表扫描：Open 不收到
// 下推约束，但 sqlite 侧复查仍保证结果正确。
func TestNoFilterable_FullScan(t *testing.T) {
	Convey("无 Filterable 退化为全表扫描", t, func() {
		schema := Schema{Columns: []Column{{Name: "name", ValueType: ValueTypeString}}}
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		rec := registerRecording(t, pool, "names", schema, [][]any{{"google"}, {"good"}, {"bad"}})

		_, rows, err := pool.Query(context.Background(), "SELECT name FROM names WHERE name = ?", "good")
		So(err, ShouldBeNil)
		So(rows, ShouldResemble, [][]any{{"good"}})
		So(rec.lastPushdown(), ShouldBeEmpty)
	})
}

// TestResultTypeConversion 行值按列 ValueType 转换为 sqlite 结果类型。
func TestResultTypeConversion(t *testing.T) {
	Convey("行值类型转换", t, func() {
		schema := Schema{Columns: []Column{
			{Name: "s", ValueType: ValueTypeString},
			{Name: "i", ValueType: ValueTypeInt64},
			{Name: "f", ValueType: ValueTypeFloat64},
			{Name: "b", ValueType: ValueTypeBool},
			{Name: "n", ValueType: ValueTypeAny},
		}}
		pool := NewSQLitePool()
		defer pool.Close(context.Background())
		err := pool.RegisterDatasource(context.Background(), "types", schema, func(resourcepool.Key) (Datasource, error) {
			return &testStaticDatasource{data: [][]any{{
				"text", int64(42), 3.5, true, nil,
			}}}, nil
		})
		So(err, ShouldBeNil)

		cols, rows, err := pool.Query(context.Background(), "SELECT * FROM types")
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []string{"s", "i", "f", "b", "n"})
		So(rows, ShouldResemble, [][]any{{"text", int64(42), 3.5, int64(1), nil}})
	})
}
