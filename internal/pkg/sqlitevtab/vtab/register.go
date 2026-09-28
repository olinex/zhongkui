// Package vtab（本文件）提供第三方数据源的前端配置驱动的统一注册逻辑。
// 每个数据源行对应一张 sqlite vtable，归属到命名空间池（vtab.Pools）并以裸名注册；
// registerInto 解析配置（redis 结构 / mysql、postgres、clickhouse 表 / sqlite 文件表 /
// etcd 键值 / mongo 集合）、按命名空间隔离地建立连接池并构造数据源。CRUD 处理器与启动
// 重载统一经 Pools.Register/Unregister 收敛到本文件，保证建立/注销逻辑一致且可测。
//
// 命名空间隔离：连接池 Key 派生含命名空间维度（MySQLConfig.Namespace 与
// NewRedisPoolScoped），同一外部实例在不同命名空间下派生不同连接池；vtable 以裸名
// （{name}）注册到各自命名空间的 SQLitePool，rego 规则直接引用裸名。
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
