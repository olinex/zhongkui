// Package vtab（本文件）提供 etcd 数据源单元测试。
// etcd 键值对数据源无法像 Redis/MySQL 那样本地起 mock（clientv3.Client 为具体类型），
// 故聚焦纯逻辑：LIKE 模式前缀提取（likePrefix）与 key 下推计划解析（resolveKeyPlan）。

package vtab

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestLikePrefix 验证 LIKE 模式到字面量前缀的提取。
func TestLikePrefix(t *testing.T) {
	Convey("likePrefix 首通配符前字面量前缀", t, func() {
		So(likePrefix("user:%"), ShouldEqual, "user:")
		So(likePrefix("user%"), ShouldEqual, "user")
		So(likePrefix("a_b"), ShouldEqual, "a")
		So(likePrefix("%suffix"), ShouldEqual, "")
		So(likePrefix("plain"), ShouldEqual, "")
		So(likePrefix(""), ShouldEqual, "")
	})
}

// TestEtcdResolveKeyPlan 验证 key 列下推计划：精确等值/前缀范围/全量范围。
func TestEtcdResolveKeyPlan(t *testing.T) {
	ds := NewEtcdDatasource(nil)

	Convey("key 精确等值 → exact", t, func() {
		plan := ds.resolveKeyPlan([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpEQ, Value: "/users/1"},
		})
		So(plan, ShouldResemble, etcdKeyPlan{exact: true, prefix: "/users/1"})
	})

	Convey("key 非 string 等值 → 忽略", t, func() {
		plan := ds.resolveKeyPlan([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpEQ, Value: int64(1)},
		})
		So(plan, ShouldResemble, etcdKeyPlan{})
	})

	Convey("key LIKE 有前缀 → prefix 范围", t, func() {
		plan := ds.resolveKeyPlan([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpLIKE, Value: "/users/%"},
		})
		So(plan, ShouldResemble, etcdKeyPlan{prefix: "/users/"})
	})

	Convey("key LIKE 无字面前缀 → full 范围", t, func() {
		plan := ds.resolveKeyPlan([]sqlitevtab.Constraint{
			{Column: 0, Op: sqlitevtab.OpLIKE, Value: "%a%"},
		})
		So(plan, ShouldResemble, etcdKeyPlan{})
	})

	Convey("非 key 列 / 缺省 → full 范围", t, func() {
		plan := ds.resolveKeyPlan([]sqlitevtab.Constraint{
			{Column: 1, Op: sqlitevtab.OpEQ, Value: "v"},
		})
		So(plan, ShouldResemble, etcdKeyPlan{})
		So(ds.resolveKeyPlan(nil), ShouldResemble, etcdKeyPlan{})
	})

	Convey("schema 固定 key/value 两列且 key 可下推", t, func() {
		So(ds.datasourceSchema().Filterable, ShouldResemble, []int{0})
	})
}

// TestEtcdDatasourceSchema 验证 etcd vtable 固定两列结构。
func TestEtcdDatasourceSchema(t *testing.T) {
	Convey("etcd 固定两列 schema", t, func() {
		ds := NewEtcdDatasource(nil)
		So(ds.datasourceSchema().Columns, ShouldResemble, []sqlitevtab.Column{
			{Name: "key", ValueType: sqlitevtab.ValueTypeString},
			{Name: "value", ValueType: sqlitevtab.ValueTypeString},
		})
	})
}
