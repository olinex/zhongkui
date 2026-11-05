// Package vtab（本文件）提供第三方数据源类型的插件化注册机制。
// 进程级注册表以类型键（Key）维护一组 DatasourcePlugin；每一次新增数据源类型只需实现
// DatasourcePlugin 接口并在该类型所在文件的 init() 里 RegisterDatasourceType 自注册，
// 即可被 registerInto / TestConn / IntrospectSchema / ValidateConfigJSON 统一驱动，
// 无需再改动任何按类型分发的 switch 或数据源清单。注册表并发安全，并支持在测试中
// UnregisterDatasourceType 清理。

package vtab

import (
	// 标准库
	"context"
	"io"
	"reflect"
	"sort"
	"sync"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// DatasourcePlugin 描述一种第三方数据源类型的插件化接入契约。
// 实现者提供其唯一类型键（Key）、前端配置的构造/反射、连接探活（TestConn）、表结构自省
// （IntrospectSchema）与带参注册（Register）；数据源相关实现集中在各自 <type>.go 文件，
// 通过 init() 自注册，宿主无需感知具体类型。
type DatasourcePlugin interface {
	// Key 返回该数据源类型的唯一键（如 "redis"），作为注册表索引。
	Key() string
	// NewConfig 返回该类型全新的一份配置指针（如 &RedisConfig{}），供默认值填充与反序列化。
	NewConfig() any
	// ConfigType 返回该类型配置值的反射类型（如 reflect.TypeFor[RedisConfig]()）。
	ConfigType() reflect.Type
	// TestConn 建立连接并校验连通性（不做注册/自省）。
	TestConn(ctx context.Context, cfg any) error
	// IntrospectSchema 解析目标表结构并返回列定义；sqlite_file 场景要求传入前端上传的
	// 明文文件流（io.Reader），其余类型忽略该参数。
	IntrospectSchema(ctx context.Context, cfg any, file io.Reader) ([]DatasourceColumn, error)
	// Register 在指定命名空间池上按配置建立连接池并构造数据源，返回 vtable 数据源。
	Register(ctx context.Context, pool *sqlitevtab.SQLitePool, store *s3.ObjectCache, namespace, name string, cfg any) (sqlitevtab.Datasource, error)
	// ValidateConfig 校验配置 JSON 与数据源类型的结构一致性（不建连、不注册）。
	ValidateConfig(cfg any) error
}

// datasourcePlugins 是进程级数据源插件注册表，按类型键索引、并发安全。
var (
	datasourcePluginsMu sync.RWMutex
	datasourcePlugins   = map[string]DatasourcePlugin{}
)

// RegisterDatasourceType 注册一种数据源类型；重复 Key 时 panic。
func RegisterDatasourceType(p DatasourcePlugin) {
	datasourcePluginsMu.Lock()
	defer datasourcePluginsMu.Unlock()
	if _, ok := datasourcePlugins[p.Key()]; ok {
		panic("duplicate datasource plugin type " + p.Key())
	}
	datasourcePlugins[p.Key()] = p
}

// UnregisterDatasourceType 注销一种数据源类型，供测试 Cleanup 使用。
func UnregisterDatasourceType(typ string) {
	datasourcePluginsMu.Lock()
	defer datasourcePluginsMu.Unlock()
	delete(datasourcePlugins, typ)
}

// DatasourceTypes 返回全部已注册插件，按 Key 字典序排序。
func DatasourceTypes() []DatasourcePlugin {
	datasourcePluginsMu.RLock()
	defer datasourcePluginsMu.RUnlock()
	out := make([]DatasourcePlugin, 0, len(datasourcePlugins))
	for _, p := range datasourcePlugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// DatasourceTypeKeys 返回全部已注册类型键，按字典序排序。
func DatasourceTypeKeys() []string {
	datasourcePluginsMu.RLock()
	defer datasourcePluginsMu.RUnlock()
	out := make([]string, 0, len(datasourcePlugins))
	for k := range datasourcePlugins {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// lookupDatasourcePlugin 按类型键查表返回对应插件（未注册时 ok 为 false）。
func lookupDatasourcePlugin(typ string) (DatasourcePlugin, bool) {
	datasourcePluginsMu.RLock()
	defer datasourcePluginsMu.RUnlock()
	p, ok := datasourcePlugins[typ]
	return p, ok
}

// IsDatasourceTypeRegistered 判断类型键是否已有插件注册。
func IsDatasourceTypeRegistered(typ string) bool {
	_, ok := lookupDatasourcePlugin(typ)
	return ok
}
