// Package vtab（本文件）证明第三方数据源插件化接入的可扩展性：新增一种数据源类型只需
// 实现 DatasourcePlugin 接口并一次性 RegisterDatasourceType 自注册，即可被
// decodeDatasourceConfig / TestConn / IntrospectSchema / registerInto（经 Pools.Register）
// 与类型清单统一驱动，无需改动任何按类型分发的既有代码。测试注册一个合成插件
// extensibility_probe，覆盖：默认值填充、连通测试委托、表结构自省委托、端到端注册后
// sqlite 查询、类型列举，以及恶意类型键解码报错。

package vtab

import (
	// 标准库
	"context"
	"io"
	"reflect"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	pkgerrors "zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// extensibilityProbeKey 是本测试合成数据源类型的唯一键。
const extensibilityProbeKey = "extensibility_probe"

// probeConfig 是合成数据源的前端配置。Label 带 default tag，用于验证 NewConfig 后统一
// 经 defaults.Set 填充默认值；Required 用于 ValidateConfig 校验。
type probeConfig struct {
	Label    string `json:"label" default:"xyz"`
	Required string `json:"required"`
}

// probePlugin 实现 DatasourcePlugin 接口的合成插件。
type probePlugin struct{}

// Key 返回数据源类型键。
func (p *probePlugin) Key() string { return extensibilityProbeKey }

// NewConfig 返回该类全新配置指针。
func (p *probePlugin) NewConfig() any { return &probeConfig{} }

// ConfigType 返回配置值的反射类型。
func (p *probePlugin) ConfigType() reflect.Type { return reflect.TypeFor[probeConfig]() }

// TestConn 委托式连通测试：返回 nil 表示探活成功。
func (p *probePlugin) TestConn(_ context.Context, cfg any) error {
	return nil
}

// IntrospectSchema 委托式自省：固定返回 k/v 两列，忽略 file 参数。
func (p *probePlugin) IntrospectSchema(_ context.Context, _ any, _ io.Reader) ([]DatasourceColumn, error) {
	return []DatasourceColumn{
		{Name: "k", Type: "string"},
		{Name: "v", Type: "string"},
	}, nil
}

// Register 从配置构造行集数据源并返回。静态数据源不实现 dsSchemaProvider，故包装为
// probeDatasource 以对外暴露 datasourceSchema()，供 registerInto 的 schemaOf 复用其内部 schema。
func (p *probePlugin) Register(_ context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, _, _ string, cfg any) (sqlitevtab.Datasource, error) {
	if _, ok := cfg.(*probeConfig); !ok {
		return nil, pkgerrors.Newf("extensibility_probe: unexpected config type %T", cfg)
	}
	schema := sqlitevtab.Schema{Columns: []sqlitevtab.Column{
		{Name: "k", ValueType: sqlitevtab.ValueTypeString},
		{Name: "v", ValueType: sqlitevtab.ValueTypeString},
	}}
	data := [][]any{
		{"a", "1"},
		{"b", "2"},
	}
	return &probeDatasource{StaticDatasource: NewStaticDatasource(schema, data), schema: schema}, nil
}

// ValidateConfig 校验配置结构：Required 为空时报错。
func (p *probePlugin) ValidateConfig(cfg any) error {
	pc, ok := cfg.(*probeConfig)
	if !ok {
		return pkgerrors.Newf("extensibility_probe: unexpected config type %T", cfg)
	}
	if pc.Required == "" {
		return pkgerrors.New("extensibility_probe: required field is empty")
	}
	return nil
}

// probeDatasource 包装静态数据源以提供 datasourceSchema()，补齐 dsSchemaProvider 契约。
// probeDatasource wraps the static datasource to provide datasourceSchema(), fulfilling the
// dsSchemaProvider contract.
type probeDatasource struct {
	*StaticDatasource
	schema sqlitevtab.Schema
}

// datasourceSchema 返回注册用 schema。
func (d *probeDatasource) datasourceSchema() sqlitevtab.Schema { return d.schema }

// TestExtensibility 验证新增一类插件化数据源类型即可被统一分发驱动，无需改动既有代码。
func TestExtensibility(t *testing.T) {
	const (
		bareName = "probe_tbl"
	)
	ctx := context.Background()
	// 仅传部分字段，verify Label 默认值经 defaults.Set 填充；Required 供 ValidateConfig 校验。
	partialJSON := []byte(`{"required":"hello"}`)

	RegisterDatasourceType(&probePlugin{})
	t.Cleanup(func() { UnregisterDatasourceType(extensibilityProbeKey) })

	Convey("扩展类型出现在类型清单中", t, func() {
		So(DatasourceTypeKeys(), ShouldContain, extensibilityProbeKey)
		types := make([]string, 0, len(DatasourceTypes()))
		for _, plugin := range DatasourceTypes() {
			types = append(types, plugin.Key())
		}
		So(types, ShouldContain, extensibilityProbeKey)
	})

	Convey("decodeDatasourceConfig 解码为类型化指针并应用 default tag", t, func() {
		cfg, err := decodeDatasourceConfig(extensibilityProbeKey, partialJSON)
		So(err, ShouldBeNil)
		pc, ok := cfg.(*probeConfig)
		So(ok, ShouldBeTrue)
		So(pc.Label, ShouldEqual, "xyz") // default:"xyz" 生效
		So(pc.Required, ShouldEqual, "hello")
	})

	Convey("ValidateConfigJSON 调用插件注册表（空 Required 报错）", t, func() {
		So(ValidateConfigJSON(extensibilityProbeKey, partialJSON), ShouldBeNil)
		So(ValidateConfigJSON(extensibilityProbeKey, []byte(`{"label":"ok"}`)), ShouldNotBeNil)
	})

	Convey("TestConn 委托插件探活成功", t, func() {
		So(TestConn(ctx, extensibilityProbeKey, partialJSON), ShouldBeNil)
	})

	Convey("IntrospectSchema 委托插件返回固定两列", t, func() {
		cols, err := IntrospectSchema(ctx, extensibilityProbeKey, partialJSON, nil)
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []DatasourceColumn{
			{Name: "k", Type: "string"},
			{Name: "v", Type: "string"},
		})
	})

	Convey("registerInto 经 Pools.Register 端到端注册并可 sqlite 查询", t, func() {
		pools := NewPools(nil)
		t.Cleanup(func() { pools.Close(ctx) })
		So(pools.Register(ctx, "prod", bareName, extensibilityProbeKey, partialJSON), ShouldBeNil)
		pool := pools.Pool("prod")
		So(pool.Datasources(), ShouldContain, bareName)

		cols, rows, err := pool.Query(ctx, "SELECT k FROM "+bareName)
		So(err, ShouldBeNil)
		So(cols, ShouldResemble, []string{"k"})
		So(rows, ShouldResemble, [][]any{{"a"}, {"b"}})
	})

	Convey("恶意类型键解码报 unsupported type", t, func() {
		_, err := decodeDatasourceConfig("no_such_type", []byte(`{}`))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unsupported type")
	})
}
