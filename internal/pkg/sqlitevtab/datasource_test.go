package sqlitevtab

import (
	// 标准库
	"context"
	"io"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// TestValidateDatasourceName 数据源名必须是合法 SQL 标识符。
func TestValidateDatasourceName(t *testing.T) {
	Convey("数据源名校验", t, func() {
		for _, name := range []string{"ip_geo", "geo", "A1", "_x"} {
			So(validateDatasourceName(name), ShouldBeNil)
		}
		for _, name := range []string{"", "1abc", "a-b", "a b", "a.b", "select"} {
			err := validateDatasourceName(name)
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrInvalidDatasourceName), ShouldBeTrue)
		}
	})
}

// TestValidateSchema schema 校验：列名非法、空列、重名列、Filterable 越界。
func TestValidateSchema(t *testing.T) {
	Convey("schema 校验", t, func() {
		Convey("合法 schema 应通过", func() {
			err := validateSchema(Schema{Columns: []Column{{Name: "a"}, {Name: "b"}}, Filterable: []int{0, 1}})
			So(err, ShouldBeNil)
		})
		Convey("空列应报错", func() {
			So(validateSchema(Schema{}), ShouldNotBeNil)
		})
		Convey("非法列名应报错", func() {
			err := validateSchema(Schema{Columns: []Column{{Name: "a-b"}}})
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrInvalidColumnName), ShouldBeTrue)
		})
		Convey("重名列应报错", func() {
			err := validateSchema(Schema{Columns: []Column{{Name: "a"}, {Name: "a"}}})
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrDuplicateColumn), ShouldBeTrue)
		})
		Convey("Filterable 越界应报错", func() {
			err := validateSchema(Schema{Columns: []Column{{Name: "a"}}, Filterable: []int{1}})
			So(err, ShouldNotBeNil)
			So(errors.Is(err, ErrColumnIndexOutOfRange), ShouldBeTrue)
		})
	})
}

// TestStaticDatasource 参考数据源的行迭代与 EOF 语义已随实现迁移至
// vtab 子包（vtab/static_test.go）。

// testStaticDatasource 测试用静态数据源：忽略下推约束、返回全部行。
// 符合 Datasource 契约（过滤结果只需是真实匹配行的超集，正确性由 sqlite 侧
// Omit=false 复查兜底），供池/vtable 行为测试复用，避免内部测试包因引用
// vtab 子包（其依赖本包）造成循环导入。
type testStaticDatasource struct {
	data [][]any
}

// Open 实现 Datasource.Open：忽略下推约束，直接返回全部行。
func (d *testStaticDatasource) Open(ctx context.Context, pushdown []Constraint) (Rows, error) {
	return &testStaticRows{data: d.data}, nil
}

// testStaticRows 是 testStaticDatasource 的行游标。
type testStaticRows struct {
	data [][]any
	idx  int
}

// Next 返回下一行（行值的副本），迭代结束返回 io.EOF。
func (r *testStaticRows) Next() ([]any, error) {
	if r.idx >= len(r.data) {
		return nil, io.EOF
	}
	row := r.data[r.idx]
	r.idx++
	return append([]any(nil), row...), nil
}

// Close 释放游标引用。
func (r *testStaticRows) Close() error {
	r.data = nil
	return nil
}
