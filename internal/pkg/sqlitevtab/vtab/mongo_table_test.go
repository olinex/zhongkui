// Package vtab（本文件）提供 MongoDB 数据源单元测试。
// Mongo 客户端不易本地 mock，聚焦纯逻辑：单元格归一化（cell/mongoInt64/mongoFloat/
// mongoText）与 Filterable 恒空契约。
package vtab

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/v2/bson"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestMongoNumericNormalization 数值与布尔单元格归一化。
func TestMongoNumericNormalization(t *testing.T) {
	Convey("mongoInt64 归一化", t, func() {
		So(mongoInt64(int(3)), ShouldEqual, int64(3))
		So(mongoInt64(int32(3)), ShouldEqual, int64(3))
		So(mongoInt64(int64(3)), ShouldEqual, int64(3))
		So(mongoInt64(float64(3.9)), ShouldEqual, int64(3))
		So(mongoInt64("x"), ShouldEqual, int64(0))
	})

	Convey("mongoFloat 归一化", t, func() {
		So(mongoFloat(int(3)), ShouldEqual, float64(3))
		So(mongoFloat(int64(3)), ShouldEqual, float64(3))
		So(mongoFloat(float32(1.5)), ShouldEqual, float64(1.5))
		So(mongoFloat(float64(1.5)), ShouldEqual, float64(1.5))
		So(mongoFloat("x"), ShouldEqual, float64(0))
	})

	Convey("mongoText 格式化", t, func() {
		So(mongoText("abc"), ShouldEqual, "abc")
		So(mongoText([]byte("abc")), ShouldEqual, "abc")
		So(mongoText(bson.ObjectID{0x01, 0x02}), ShouldEqual, "010200000000000000000000")
		So(mongoText(42), ShouldEqual, "42")
	})
}

// TestMongoCell 按注入列对文档单元格归一化（缺失字段→nil，复杂值→文本）。
func TestMongoCell(t *testing.T) {
	ds := newMongoDatasource(nil, []mongoColumn{
		{name: "id", vt: sqlitevtab.ValueTypeInt64},
		{name: "name", vt: sqlitevtab.ValueTypeString},
		{name: "active", vt: sqlitevtab.ValueTypeBool},
		{name: "score", vt: sqlitevtab.ValueTypeFloat64},
		{name: "tags", vt: sqlitevtab.ValueTypeAny},
	})

	Convey("cell 归一化与缺省", t, func() {
		doc := map[string]any{
			"id":     int64(1),
			"name":   "zk",
			"active": true,
			"score":  float64(9.5),
			"tags":   bson.A{"a", "b"},
		}
		So(ds.cell(doc, "id", sqlitevtab.ValueTypeInt64), ShouldEqual, int64(1))
		So(ds.cell(doc, "name", sqlitevtab.ValueTypeString), ShouldEqual, "zk")
		So(ds.cell(doc, "active", sqlitevtab.ValueTypeBool), ShouldEqual, true)
		So(ds.cell(doc, "score", sqlitevtab.ValueTypeFloat64), ShouldEqual, float64(9.5))
		So(ds.cell(doc, "none", sqlitevtab.ValueTypeString), ShouldBeNil)
	})

	Convey("Filterable 恒空（不下推）", t, func() {
		So(ds.datasourceSchema().Filterable, ShouldHaveLength, 0)
	})
}
