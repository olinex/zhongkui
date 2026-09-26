// 本文件把数据源类型与 Redis 数据结构注册为枚举类 datasourceType / datasourceStruct。
// datasourceType 的取值空间取自本包的数据源插件注册表（datasourcePlugins，由
// clickhouse/etcd/mongo/mysql/postgres/redis/sqlite_file 等文件的 init() 依次填充），故经
// enum.RegisterSource 声明来源：条目在每次读取时实时派生，完整性与包内 init 的文件顺序无关。
// datasourceStruct 的取值空间取自本包的 Redis 数据结构类型常量（redisKind*），顺序固定，故经
// enum.Register 在注册时快照条目。
//
// This file registers the datasource types and the Redis data structures as the enum classes
// datasourceType / datasourceStruct. datasourceType comes from this package's datasource plugin
// registry (datasourcePlugins, filled by the init() functions of
// clickhouse/etcd/mongo/mysql/postgres/redis/sqlite_file etc.), so it is declared via
// enum.RegisterSource: the entries are derived on every read, independent of the in-package init file
// order. datasourceStruct comes from this package's Redis data-structure kind constants (redisKind*)
// in a fixed order, so enum.Register snapshots its entries at registration time.
package vtab

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// datasourceStructValues 是 datasourceStruct 枚举类的取值空间，取自本包的 Redis 数据结构类型常量
// （redisKind*），声明顺序即枚举类条目顺序与前端下拉的展示顺序。
// datasourceStructValues is the value space of the datasourceStruct enum class, taken from this
// package's Redis data-structure kind constants (redisKind*); the declaration order is the entry order
// of the enum class and the display order of the frontend select.
var datasourceStructValues = []redisTypeKind{
	redisKindString,
	redisKindHash,
	redisKindList,
	redisKindSet,
	redisKindZSet,
}

// init 注册枚举类 datasourceType（来源为数据源插件注册表，读取时派生）与 datasourceStruct
// （取值空间固定，注册时快照）。
// init registers the enum class datasourceType (sourced from the datasource plugin registry, derived on
// read) and datasourceStruct (a fixed value space, snapshotted at registration time).
func init() {
	enum.RegisterSource("datasourceType", func() []enum.Entry {
		return enum.EntriesOf(DatasourceTypeKeys())
	})
	enum.Register(enum.Type{Name: "datasourceStruct", Entries: enum.EntriesOf(datasourceStructValues)})
}
