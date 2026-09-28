package tagengine_test

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/open-policy-agent/opa/v1/rego"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// ipModel 内置函数端到端测试用的目标结构体，其类型键经 TypeKey 计算作为规则 Model。
type ipModel struct {
	Namespace string `json:"namespace"`
	IP        string `json:"ip"`
}

// registerIPGeo 注册 IP→国家 静态数据源（ip 列可下推过滤）。
func registerIPGeo(t *testing.T, pool *sqlitevtab.SQLitePool) {
	t.Helper()
	schema := sqlitevtab.Schema{
		Columns: []sqlitevtab.Column{
			{Name: "ip", ValueType: sqlitevtab.ValueTypeString},
			{Name: "country", ValueType: sqlitevtab.ValueTypeString},
		},
		Filterable: []int{0},
	}
	err := pool.RegisterDatasource(context.Background(), "ip_geo", schema, func(resourcepool.Key) (sqlitevtab.Datasource, error) {
		return vtab.NewStaticDatasource(schema, [][]any{
			{"8.8.8.8", "US"},
			{"1.2.3.4", "CN"},
		}), nil
	})
	So(err, ShouldBeNil)
}

// TestBuiltin_SQLiteQueryEndToEnd 端到端规则识别：注册数据源 → 规则调用
// sqlite.query → 引擎对目标实例产出标签。规则组仅编译一次，识别时的每次
// 评估实时经内置函数查询数据源，验证"运行期动态注册、即时查询"语义。
func TestBuiltin_SQLiteQueryEndToEnd(t *testing.T) {
	Convey("sqlite.query 端到端规则识别", t, func() {
		engine, db := setupEngine(t)
		pool := sqlitevtab.NewSQLitePool()
		defer pool.Close(context.Background())
		registerIPGeo(t, pool)
		evalCtx := tagengine.WithDatasourcePool(context.Background(), pool)

		targetType, err := tagengine.TypeKey(&ipModel{})
		So(err, ShouldBeNil)
		So(db.Create(&tag.TagRule{
			NamespaceName: "default",
			Name:          "overseas",
			Model:         targetType,
			Rego: `package zki

tags contains "overseas" if {
    some row in sqlite.query("SELECT country FROM ip_geo WHERE ip = ?", [input.ip])
    row.country != "CN"
}`,
			Enabled: true,
		}).Error, ShouldBeNil)

		Convey("境外 IP 命中标签，境内 IP 不命中", func() {
			result, err := engine.Recognize(evalCtx, &ipModel{Namespace: "default", IP: "8.8.8.8"})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []string{"overseas"})

			result, err = engine.Recognize(evalCtx, &ipModel{Namespace: "default", IP: "1.2.3.4"})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})
	})
}

// TestBuiltin_SQLiteQueryParamBinding 查询参数按值绑定到 ? 占位符，注入串
// 仅参与等值比较、不会改变 SQL 语义（防注入）。
func TestBuiltin_SQLiteQueryParamBinding(t *testing.T) {
	Convey("sqlite.query 参数绑定防注入", t, func() {
		pool := sqlitevtab.NewSQLitePool()
		defer pool.Close(context.Background())
		registerIPGeo(t, pool)
		evalCtx := tagengine.WithDatasourcePool(context.Background(), pool)

		group, err := tagengine.CompileGroup(context.Background(), []*tag.TagRule{{
			Name: "leak",
			Rego: `package zki

tags contains "leak" if {
    some row in sqlite.query("SELECT country FROM ip_geo WHERE ip = ?", [input.ip])
    row.country == "CN"
}`,
		}})
		So(err, ShouldBeNil)

		Convey("正常 IP 命中；注入串按值绑定不逃逸", func() {
			tags, err := group.Recognize(evalCtx, rego.EvalInput(map[string]any{"ip": "1.2.3.4"}))
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"leak"})

			// 若 SQL 由字符串拼接生成，该输入会变成 OR '1'='1' 全表匹配
			tags, err = group.Recognize(evalCtx, rego.EvalInput(map[string]any{"ip": "1.2.3.4' OR '1'='1"}))
			So(err, ShouldBeNil)
			So(tags, ShouldBeEmpty)
		})
	})
}

// TestBuiltin_SQLiteQueryNotActivated 池未绑定到求值上下文时，StrictBuiltinErrors 场景下
// sqlite.query 错误浮出为 ErrSQLitePoolNotActivated。
func TestBuiltin_SQLiteQueryNotActivated(t *testing.T) {
	Convey("sqlite.query 未绑池时错误浮出", t, func() {
		// 不绑定任何池，直接以裸 ctx 求值。
		group, err := tagengine.CompileGroup(context.Background(), []*tag.TagRule{{
			Name: "query",
			Rego: `package zki

tags contains "x" if {
    some row in sqlite.query("SELECT country FROM ip_geo WHERE ip = ?", [input.ip])
}`,
		}}, rego.StrictBuiltinErrors(true))
		So(err, ShouldBeNil)

		_, err = group.Recognize(context.Background(), rego.EvalInput(map[string]any{"ip": "8.8.8.8"}))
		So(err, ShouldNotBeNil)
		So(errors.Is(err, tagengine.ErrSQLitePoolNotActivated), ShouldBeTrue)
	})
}

// TestValidateRegoRules_SQLiteQueryBuiltin 引用 sqlite.query 的规则在
// InspectRego 与 ValidateRegoRules（含组级语义编译与类型检查）中均合法，
// 不误报 "undefined function"。
func TestValidateRegoRules_SQLiteQueryBuiltin(t *testing.T) {
	Convey("引用 sqlite.query 的规则校验不误报 undefined function", t, func() {
		source := `package zki

tags contains "overseas" if {
    some row in sqlite.query("SELECT country FROM ip_geo WHERE ip = ?", [input.ip])
    row.country != "CN"
}`

		Convey("InspectRego 合法", func() {
			errs, ok := tagengine.InspectRego(source)
			So(ok, ShouldBeTrue)
			So(errs, ShouldBeEmpty)
		})

		Convey("ValidateRegoRules 合法（组级语义编译含类型检查）", func() {
			errs, ok := tagengine.ValidateRegoRules(context.Background(), []*tag.TagRule{{Name: "overseas", Rego: source}})
			So(ok, ShouldBeTrue)
			So(errs, ShouldBeEmpty)
		})
	})
}

// TestBuiltin_SQLiteQueryDryRun dry-run / 样例校验路径无回归：规则校验 →
// 组编译 → 样例输入评估全链路可用。
func TestBuiltin_SQLiteQueryDryRun(t *testing.T) {
	Convey("dry-run 校验路径无回归", t, func() {
		pool := sqlitevtab.NewSQLitePool()
		defer pool.Close(context.Background())
		registerIPGeo(t, pool)
		evalCtx := tagengine.WithDatasourcePool(context.Background(), pool)

		source := `package zki

tags contains "overseas" if {
    some row in sqlite.query("SELECT country FROM ip_geo WHERE ip = ?", [input.ip])
    row.country != "CN"
}`
		rule := &tag.TagRule{Name: "overseas", Rego: source}

		// dry-run 流程：规则校验 → 组编译 → 样例输入评估
		errs, ok := tagengine.ValidateRegoRules(context.Background(), []*tag.TagRule{rule})
		So(ok, ShouldBeTrue)
		So(errs, ShouldBeEmpty)

		group, err := tagengine.CompileGroup(context.Background(), []*tag.TagRule{rule})
		So(err, ShouldBeNil)

		Convey("样例命中与未命中结果正确", func() {
			tags, err := group.Recognize(evalCtx, rego.EvalInput(map[string]any{"ip": "8.8.8.8"}))
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"overseas"})

			tags, err = group.Recognize(evalCtx, rego.EvalInput(map[string]any{"ip": "1.2.3.4"}))
			So(err, ShouldBeNil)
			So(tags, ShouldBeEmpty)
		})
	})
}
