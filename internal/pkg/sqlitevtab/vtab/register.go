// Package vtab 是「数据源插件体系 + sqlite vtable」的落地层：在通用设施
// internal/pkg/sqlitevtab（负责 vtable 适配、schema 声明、下推与只读执行）之上，提供可直接
// 复用的各类型数据源实现、前端配置驱动的统一注册，以及按命名空间隔离的 vtable 池持有者 Pools。
// 每个第三方数据源行对应一张 sqlite vtable，注册后即可被 rego 规则经 sqlite.query 以裸名查询。
//
// ## 插件化注册机制
//
// 数据源类型以 DatasourcePlugin 接口描述：类型键（Key）、前端配置的构造与反射（NewConfig /
// ConfigType）、连通性探活（TestConn）、表结构自省（IntrospectSchema）与带参注册（Register），
// 另有不做连接的配置校验（ValidateConfig）。实现集中在各自的 <type>.go 文件内，并经 init 期
// RegisterDatasourceType 自注册进进程级、并发安全的插件注册表。由此，新增一种数据源类型只需
// 实现该接口并自注册，即可被 registerInto / TestConn / IntrospectSchema / ValidateConfigJSON
// 统一驱动，无需再改动任何按类型分发的 switch 或维护一份平行的类型清单；DatasourceTypes /
// DatasourceTypeKeys 供枚举与诊断，UnregisterDatasourceType 供测试清理。类型键常量
// （DatasourceTypeRedis 等）是 Datasource.Type 的唯一来源，模型层不再复定义。
//
// 当前支持的类型：redis（按 Config.Struct 暴露 string / hash / list / set / zset 之一）、mysql、
// postgres、clickhouse（各以一张业务表为一张 vtable）、sqlite_file（前端上传 .db 文件）、etcd
// （key/value）、mongo（一个集合）。表型数据源（mysql / postgres / clickhouse / sqlite_file）与
// mongo 的 schema 必须在配置中完整提供（前端经 /datasource/introspect 自省或手动填写，见各自的
// `*Config.Schema`），注册时不再自动自省；redis 与 etcd 的结构固定，由实现直接给出。
//
// ## 注册与命名空间隔离
//
// 注册统一入口是 Pools.Register(namespace, name, typ, cfgJSON)，内部经 registerInto 收敛为一条
// 固定管线：校验本地名（合法 SQL 标识符）→ 按类型解码配置（先补默认值再反序列化，单一解析点
// decodeDatasourceConfig）→ 委托插件的 Register 建立按命名空间隔离的连接池并构造数据源 → 取回
// schema → 以裸名注册到该命名空间的 SQLitePool。因此建立与注销逻辑一致且可测。
//
// 隔离维度是命名空间：Pools 为每个 namespace 独立持有一个 SQLitePool，同一外部实例在不同命名
// 空间下派生不同连接池，凭据与连接互不串扰；vtable 以裸名（{name}）注册到各自命名空间的池，
// 消除了 rego 规则里冗余的 {namespace}__ 前缀，并规避了 SQL 保留关键字与命名冲突的边界问题
// （由 sqlitevtab 的 validateDatasourceName 兜底校验保留字）。Pools 自身不持有数据库引用：扫描
// 已启用数据源并由 Pools.Register 恢复注册，由外部的 datasource.Reload 驱动，避免循环依赖。
//
// ## 预检
//
// 落库注册之前，视图层可先调用两个纯校验函数（precheck.go）：TestConn 按类型建立连接并校验
// 连通性（redis 经 Ping、mysql/postgres/clickhouse 经 PingContext、etcd 经 MemberList、mongo 经
// Ping(Primary)；sqlite_file 无网络连接，返回不支持），IntrospectSchema 建立连接后解析目标表结构
// 并映射为列定义（列名 + 类型键），供前端在创建数据源时展示与补全。二者复用与注册相同的配置
// 解码与数据源逻辑，但既不落库也不注册 vtable，命名空间固定为空串，故其临时连接池不与真实注册
// 的命名空间池串扰。另有 ValidateConfigJSON 只做解码级（含类型一致性）校验：停用数据源时仍需
// 拒绝非法配置落库，但不希望触发建连自省，故与 Register 分离。
//
// ## 与 sqlitevtab 的配合
//
// 本包只负责「有哪些数据源、如何建连、schema 是什么」；vtable 的 schema 声明（CREATE TABLE 的列
// 与亲和性）、下推约束（BestIndex / xFilter）与结果列输出（xColumn）全部由 internal/pkg/sqlitevtab
// 代理，数据源实现只需实现其 Datasource 接口（Open 返回行游标 Rows），无需接触 sqlite 细节。
// 该设施把查询限制为只读（连接级 authorizer 拒绝一切写操作）并设单次查询行数上限（默认 1000），
// 故第三方数据源不会经此通道产生副作用或超大结果集。数据源为表型时，下推过滤只是优化，实现
// 返回的必须是真实匹配行的超集（sqlite 侧会逐行复查，漏行无法找回）。
//
// ## 生命周期与枚举
//
// Pools 实现 do.ShutdownerWithContextAndError，供 samber/do 在优雅关闭阶段驱动其名下全部
// SQLitePool 关闭；sqlite_file 数据源的原件经通用 S3 对象缓存加密留存（StoreSQLiteObject /
// RemoveSQLiteObject，对象键由命名空间 + 数据源名派生），S3 未启用时注册报「需要启用 S3」。
// enum.go 把数据源类型（datasourceType，取值空间实时取自插件注册表）与 redis 结构类型
// （datasourceStruct）注册为枚举类，供前端取用而无需自行声明字面量。
package vtab

import (
	// 标准库
	"context"
	"encoding/json"
	"regexp"
	"strings"

	// 第三方

	"github.com/creasty/defaults"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// 第三方数据源类型键，作为 Datasource.Type 的唯一来源（单一事实来源，模型层不复定义）。
const (
	// DatasourceTypeRedis Redis 数据结构数据源
	DatasourceTypeRedis = "redis"
	// DatasourceTypeMySQL MySQL/MariaDB 业务表数据源
	DatasourceTypeMySQL = "mysql"
	// DatasourceTypePostgres PostgreSQL 业务表数据源
	DatasourceTypePostgres = "postgres"
	// DatasourceTypeClickHouse ClickHouse 业务表数据源
	DatasourceTypeClickHouse = "clickhouse"
	// DatasourceTypeSQLiteFile SQLite 文件业务表数据源
	DatasourceTypeSQLiteFile = "sqlite_file"
	// DatasourceTypeEtcd etcd 键值数据源
	DatasourceTypeEtcd = "etcd"
	// DatasourceTypeMongo MongoDB 集合数据源
	DatasourceTypeMongo = "mongo"
)

// localNamePattern 合法本地数据源名模式，与 sqlitevtab 的列名/数据源名校验一致。
var localNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RedisConfig 描述一个 redis 数据源的前端配置。Struct 为要暴露的 Redis 数据结构，
// 取值 string/hash/list/set/zset，各对应一张 vtable。
type RedisConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 6379
	Port int `json:"port" default:"6379"`
	// Password 密码（可选）
	Password string `json:"password" form:"kind=password"`
	// DB 逻辑库号，默认 0
	DB int `json:"db" default:"0"`
	// Struct 要暴露的 Redis 数据结构：string/hash/list/set/zset
	Struct string `json:"struct" binding:"required"`
}

// MySQLTableConfig 描述一个 MySQL/MariaDB 单表数据源的前端配置。
// Table 为该实例 DBName 库下要暴露的业务表，注册时经 SHOW COLUMNS 自省建 schema。
type MySQLTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 3306
	Port int `json:"port" default:"3306"`
	// User 用户名
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	Password string `json:"password" form:"kind=password"`
	// DBName 默认数据库名，查询限定于该库
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	Table string `json:"table" binding:"required"`
	// Schema 列定义（**必填**）。注册时要求完整列定义，由前端经 /datasource/introspect
	// 自省（SHOW COLUMNS）或手动提供；后端注册不再自动自省。适用于需主动控制列名的场景。
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// PostgresTableConfig 描述一个 PostgreSQL 单表数据源的前端配置。
// Table 为该实例 DBName 库下（默认 schema）要暴露的业务表，注册时经
// information_schema.columns 自省建 schema。
type PostgresTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 5432
	Port int `json:"port" default:"5432"`
	// User 用户名
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	Password string `json:"password" form:"kind=password"`
	// DBName 默认数据库名，查询限定于该库
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	Table string `json:"table" binding:"required"`
	// Schema 列定义（**必填**），同 MySQLTableConfig.Schema 语义：注册时要求完整列定义，
	// 由前端自省或手动提供，后端不再自动自省。
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// ClickHouseTableConfig 描述一个 ClickHouse 单表数据源的前端配置。
// Table 为该实例 DBName 库下要暴露的业务表，注册时经 DESCRIBE TABLE 自省建 schema。
type ClickHouseTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 9000（原生协议）
	Port int `json:"port" default:"9000"`
	// User 用户名
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	Password string `json:"password" form:"kind=password"`
	// DBName 默认数据库名，查询限定于该库
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	Table string `json:"table" binding:"required"`
	// Schema 列定义（**必填**），同 MySQLTableConfig.Schema 语义：注册时要求完整列定义，
	// 由前端自省或手动提供，后端不再自动自省。
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// SQLiteFileTableConfig 描述一个 SQLite 文件业务表数据源的前端配置。
// .db 文件由前端上传、经 S3 对象存储加密持久化（对象键由命名空间+数据源名派生，见
// SQLiteObjectKey）；Table 为该文件内要暴露的表，注册时经 PRAGMA table_info 自省。
type SQLiteFileTableConfig struct {
	// Table 要暴露的业务表名
	Table string `json:"table" binding:"required"`
	// Schema 列定义（**必填**），同 MySQLTableConfig.Schema 语义：注册时要求完整列定义，
	// 由前端自省或手动提供，后端不再自动自省。
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// MongoCollectionConfig 描述一个 MongoDB 集合数据源的前端配置。
// MongoDB 在数据库层面无固定 schema，故数据源以「一个集合」为一张 vtable 时，
// schema 必须由前端在注册时完整提供（必填）；查询只能引用 schema 中声明的列。
type MongoCollectionConfig struct {
	// URI 连接串（含认证），如 mongodb://user:pass@127.0.0.1:27017
	URI string `json:"uri" binding:"required"`
	// DB 数据库名
	DB string `json:"db" binding:"required"`
	// Collection 集合名
	Collection string `json:"collection" binding:"required"`
	// Schema 列定义（必填）：声明列名与 ValueType，作为该集合 vtable 的唯一 schema 来源。
	Schema []DatasourceColumn `json:"schema" form:"kind=columns" binding:"required"`
}

// EtcdTableConfig 描述一个 etcd 键值数据源的前端配置。etcd 无业务表概念，一个数据源
// 即一张 key/value vtable；Endpoints 为逗号分隔的 host:port 列表。
type EtcdTableConfig struct {
	// Endpoints 逗号分隔的端点列表，如 127.0.0.1:2379
	Endpoints string `json:"endpoints" binding:"required"`
	// Username 用户名（可选）
	Username string `json:"username"`
	// Password 密码（可选）
	Password string `json:"password" form:"kind=password"`
}

// DatasourceColumn 手动指定的一列：Name 为暴露到 vtable 的列名（合法 SQL 标识符），
// Type 为该列值的类型键（any/integer/float/string/blob/bool 之一）。
type DatasourceColumn struct {
	// Name 列名，须为合法 SQL 标识符
	Name string `json:"name" binding:"required"`
	// Type 列值类型键：any/intger/float/string/blob/bool
	Type string `json:"type" binding:"required"`
}

// parseDatasourceColumns 把前端手动下发的列定义转换为 vtable 注册用 schema。校验列名
// 为合法 SQL 标识符且不重复，类型键映射到 sqlitevtab.ValueType；失败上抛带上下文的错误。
func parseDatasourceColumns(cols []DatasourceColumn) ([]sqlitevtab.Column, error) {
	if len(cols) == 0 {
		return nil, errors.New("tag datasource: manual schema requires at least one column")
	}
	out := make([]sqlitevtab.Column, 0, len(cols))
	seen := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		if !localNamePattern.MatchString(c.Name) {
			return nil, errors.Newf("tag datasource: invalid column identifier %q", c.Name)
		}
		if _, ok := seen[c.Name]; ok {
			return nil, errors.Newf("tag datasource: duplicate column %q", c.Name)
		}
		seen[c.Name] = struct{}{}
		vt, err := columnValueType(c.Type)
		if err != nil {
			return nil, err
		}
		out = append(out, sqlitevtab.Column{Name: c.Name, ValueType: vt})
	}
	return out, nil
}

// columnValueType 把手动 schema 的类型键映射到 sqlitevtab.ValueType；未知键报错。
func columnValueType(typ string) (sqlitevtab.ValueType, error) {
	switch typ {
	case "any":
		return sqlitevtab.ValueTypeAny, nil
	case "integer":
		return sqlitevtab.ValueTypeInt64, nil
	case "float":
		return sqlitevtab.ValueTypeFloat64, nil
	case "string":
		return sqlitevtab.ValueTypeString, nil
	case "blob":
		return sqlitevtab.ValueTypeBlob, nil
	case "bool":
		return sqlitevtab.ValueTypeBool, nil
	default:
		return 0, errors.Newf("tag datasource: unsupported column type %q", typ)
	}
}

// ValidateLocalName 校验数据源的本地 name：合法 SQL 标识符。vtable 以裸名注册到各自
// 命名空间的池，故不再需要 scoped 名场景下的 "__"/尾下划线约束；SQLite 保留关键字由
// RegisterDatasource 的 validateDatasourceName 兜底校验。
func ValidateLocalName(name string) error {
	if !localNamePattern.MatchString(name) {
		return errors.Newf("tag datasource: invalid local name %q", name)
	}
	return nil
}

// redisDatasourceFactory 依据 Redis 数据结构类型分派到对应数据源构造器。
func redisDatasourceFactory(pool *RedisPool, structType string) (sqlitevtab.Datasource, error) {
	switch structType {
	case "string":
		return NewRedisStringDatasource(pool), nil
	case "hash":
		return NewRedisHashDatasource(pool), nil
	case "list":
		return NewRedisListDatasource(pool), nil
	case "set":
		return NewRedisSetDatasource(pool), nil
	case "zset":
		return NewRedisZSetDatasource(pool), nil
	default:
		return nil, errors.Newf("tag datasource: unsupported redis struct %q", structType)
	}
}

// registerInto 在指定命名空间池上解析配置并以裸名注册一张 sqlite vtable。
// 按 typ 解析 cfgData，按命名空间隔离地建立连接池、构造数据源，并以裸 name 注册到
// pool（该 pool 已由调用方 Pools.Register 解析为当前命名空间的池）。注册成功后 vtable
// 即可经 sqlite.query 以裸名查询。校验由 ValidateLocalName（本地名）+ RegisterDatasource
// （裸名/schema）+ 各构造器承担；表型数据源（mysql/postgres/clickhouse/sqlite_file）要求
// config 必须带完整 schema（由前端经 /datasource/introspect 自省或手动提供），注册时不再
// 自动自省。
func registerInto(ctx context.Context, pool *sqlitevtab.SQLitePool, store *s3.ObjectCache, namespace, name, typ string, cfgData []byte) error {
	if err := ValidateLocalName(name); err != nil {
		return err
	}

	config, err := decodeDatasourceConfig(typ, cfgData)
	if err != nil {
		return err
	}

	p, ok := lookupDatasourcePlugin(typ)
	if !ok {
		return errors.Newf("tag datasource: unsupported type %q", typ)
	}

	ds, err := p.Register(ctx, pool, store, namespace, name, config)
	if err != nil {
		return err
	}

	schema, err := schemaOf(ds)
	if err != nil {
		return err
	}
	makeDS := func(resourcepool.Key) (sqlitevtab.Datasource, error) { return ds, nil }
	return pool.RegisterDatasource(ctx, name, schema, makeDS)
}

// schemaOf 从数据源取出注册用 schema。数据源实现 dsSchemaProvider 接口以复用其内部
// schema（避免重复构建）。
type dsSchemaProvider interface {
	datasourceSchema() sqlitevtab.Schema
}

func schemaOf(ds sqlitevtab.Datasource) (sqlitevtab.Schema, error) {
	if p, ok := ds.(dsSchemaProvider); ok {
		return p.datasourceSchema(), nil
	}
	return sqlitevtab.Schema{}, errors.Newf("tag datasource: datasource %T does not expose a schema", ds)
}

// ValidateConfigJSON 仅校验配置 JSON 与数据源类型的结构一致性（不建立连接、不注册
// vtable）。停用场景下处理器仍需校验配置合法性避免非法配置落库，但不希望触发建连
// 自省，故与 Register 分离为只做解码级校验。
func ValidateConfigJSON(typ string, cfgData []byte) error {
	cfg, err := decodeDatasourceConfig(typ, cfgData)
	if err != nil {
		return err
	}
	// 委托各类型插件校验配置与类型的一致性（Redis 额外校验客户端要暴露的结构类型，其余
	// 类型仅做解码级结构校验）。
	p, ok := lookupDatasourcePlugin(typ)
	if !ok {
		return errors.Newf("tag datasource: unsupported type %q", typ)
	}
	return p.ValidateConfig(cfg)
}

// decodeDatasourceConfig 按数据源类型解码配置 JSON（先应用 default tag 再反序列化，
// 未知类型返回错误），返回该类型的配置指针。供 Register / TestConn / IntrospectSchema /
// ValidateConfigJSON 复用同一解析逻辑（单一事实来源）。
func decodeDatasourceConfig(typ string, cfgData []byte) (any, error) {
	p, ok := lookupDatasourcePlugin(typ)
	if !ok {
		return nil, errors.Newf("tag datasource: unsupported type %q", typ)
	}
	cfg := p.NewConfig()
	if err := defaults.Set(cfg); err != nil {
		return nil, errors.WithStack(errors.Newf("tag datasource: apply defaults: %v", err))
	}
	if err := json.Unmarshal(cfgData, cfg); err != nil {
		return nil, errors.WithStack(errors.Newf("tag datasource: invalid %s config: %v", typ, err))
	}
	return cfg, nil
}

// splitEndpoints 把逗号分隔的端点串拆成列表并去掉空项/空白。
func splitEndpoints(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
