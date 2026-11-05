package vtab

import (
	// 标准库
	"context"
	"io"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestStaticDatasource 参考数据源的行迭代与 EOF 语义。
func TestStaticDatasource(t *testing.T) {
	Convey("StaticDatasource 行迭代与 EOF", t, func() {
		schema := sqlitevtab.Schema{Columns: []sqlitevtab.Column{{Name: "k", ValueType: sqlitevtab.ValueTypeString}}}
		ds := NewStaticDatasource(schema, [][]any{{"a"}, {"b"}})
		rows, err := ds.Open(context.Background(), nil)
		So(err, ShouldBeNil)
		defer rows.Close()

		first, err := rows.Next()
		So(err, ShouldBeNil)
		So(first, ShouldResemble, []any{"a"})
		second, err := rows.Next()
		So(err, ShouldBeNil)
		So(second, ShouldResemble, []any{"b"})
		_, err = rows.Next()
		So(errors.Is(err, io.EOF), ShouldBeTrue)
		// Close 幂等
		So(rows.Close(), ShouldBeNil)
	})
}
