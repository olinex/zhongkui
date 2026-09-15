// Package vtab（本文件）提供第三方数据源的前端配置驱动的统一注册逻辑。
// 每个数据源行对应一张 sqlite vtable，归属到命名空间池（vtab.Pools）并以裸名注册；
// registerInto 解析配置（redis 结构 / mysql、postgres、clickhouse 表 / sqlite 文件表 /
// etcd 键值 / mongo 集合）、按命名空间隔离地建立连接池并构造数据源。CRUD 处理器与启动
// 重载统一经 Pools.Register/Unregister 收敛到本文件，保证建立/注销逻辑一致且可测。
//
// 命名空间隔离：连接池 Key 派生含命名空间维度（MySQLConfig.Namespace 与
// NewRedisPoolScoped），同一外部实例在不同命名空间下派生不同连接池；vtable 以裸名
// （{name}）注册到各自命名空间的 SQLitePool，rego 规则直接引用裸名。
//
// This file provides the unified front-end-config-driven registration logic for
// third-party datasources. Each datasource row maps to one sqlite vtable, owned by
// a namespace pool (vtab.Pools) and registered under its bare name; registerInto
// parses the config (redis structure / mysql table), builds a namespace-isolated
// connection pool and constructs the datasource. CRUD handlers and startup reload
// converge through Pools.Register/Unregister so the establish/remove logic stays
// consistent and testable.
//
// Namespace isolation: connection-pool keys include the namespace dimension
// (MySQLConfig.Namespace and NewRedisPoolScoped), so the same external instance
// derives different pools across namespaces; vtables are registered under their
// bare name ({name}) on each namespace's own SQLitePool, which rego rules reference
// directly without any prefix.
package vtab

import (
	// stdlib
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	// third-party
	"github.com/creasty/defaults"
	"github.com/redis/go-redis/v9"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// 第三方数据源类型键，作为 Datasource.Type 的唯一来源（单一事实来源，模型层不复定义）。
// Third-party datasource type keys, the single source of truth for Datasource.Type
// (not redefined in the model layer).
const (
	// DatasourceTypeRedis Redis 数据结构数据源
	// DatasourceTypeRedis the Redis data-structure datasource
	DatasourceTypeRedis = "redis"
	// DatasourceTypeMySQL MySQL/MariaDB 业务表数据源
	// DatasourceTypeMySQL the MySQL/MariaDB table datasource
	DatasourceTypeMySQL = "mysql"
	// DatasourceTypePostgres PostgreSQL 业务表数据源
	// DatasourceTypePostgres the PostgreSQL table datasource
	DatasourceTypePostgres = "postgres"
	// DatasourceTypeClickHouse ClickHouse 业务表数据源
	// DatasourceTypeClickHouse the ClickHouse table datasource
	DatasourceTypeClickHouse = "clickhouse"
	// DatasourceTypeSQLiteFile SQLite 文件业务表数据源
	// DatasourceTypeSQLiteFile the SQLite-file table datasource
	DatasourceTypeSQLiteFile = "sqlite_file"
	// DatasourceTypeEtcd etcd 键值数据源
	// DatasourceTypeEtcd the etcd key-value datasource
	DatasourceTypeEtcd = "etcd"
	// DatasourceTypeMongo MongoDB 集合数据源
	// DatasourceTypeMongo the MongoDB collection datasource
	DatasourceTypeMongo = "mongo"
)

// localNamePattern 合法本地数据源名模式，与 sqlitevtab 的列名/数据源名校验一致。
// localNamePattern is the pattern of a valid local datasource name, consistent with
// sqlitevtab's column/datasource-name validation.
var localNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RedisConfig 描述一个 redis 数据源的前端配置。Struct 为要暴露的 Redis 数据结构，
// 取值 string/hash/list/set/zset，各对应一张 vtable。
// RedisConfig describes the front-end config of a redis datasource. Struct is the Redis
// data structure to expose, one of string/hash/list/set/zset, each mapping to a vtable.
type RedisConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	// Addr the host address without port, e.g. 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 6379
	// Port the port, default 6379
	Port int `json:"port" default:"6379"`
	// Password 密码（可选）
	// Password the password (optional)
	Password string `json:"password"`
	// DB 逻辑库号，默认 0
	// DB the logical database number, default 0
	DB int `json:"db" default:"0"`
	// Struct 要暴露的 Redis 数据结构：string/hash/list/set/zset
	// Struct the Redis data structure to expose: string/hash/list/set/zset
	Struct string `json:"struct" binding:"required"`
}

// MySQLTableConfig 描述一个 MySQL/MariaDB 单表数据源的前端配置。
// Table 为该实例 DBName 库下要暴露的业务表，注册时经 SHOW COLUMNS 自省建 schema。
// MySQLTableConfig describes the front-end config of a MySQL/MariaDB single-table
// datasource. Table is the business table under the instance's DBName to expose; the
// schema is built by SHOW COLUMNS introspection on registration.
type MySQLTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	// Addr the host address without port, e.g. 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 3306
	// Port the port, default 3306
	Port int `json:"port" default:"3306"`
	// User 用户名
	// User the user name
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	// Password the password (optional)
	Password string `json:"password"`
	// DBName 默认数据库名，查询限定于该库
	// DBName the default database, queries scoped to it
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	// Table the business table name to expose
	Table string `json:"table" binding:"required"`
	// Schema 手动指定的列定义（可选）。提供时跳过 SHOW COLUMNS 自省，按此 schema 注册；
	// 缺省时回退自省。适用于账号无表结构查看权限、需客户自行提供数据结构的场景。
	// Schema the manually-specified column definitions (optional). When provided, SHOW
	// COLUMNS introspection is skipped and the vtable registers with this schema; when
	// absent, introspection is used. Intended for accounts lacking table-structure
	// privileges, where the customer supplies the schema themselves.
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// PostgresTableConfig 描述一个 PostgreSQL 单表数据源的前端配置。
// Table 为该实例 DBName 库下（默认 schema）要暴露的业务表，注册时经
// information_schema.columns 自省建 schema。
// PostgresTableConfig describes the front-end config of a PostgreSQL single-table
// datasource. Table is the business table under the instance's DBName (default schema)
// to expose; the schema is built by information_schema.columns introspection on
// registration.
type PostgresTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	// Addr the host address without port, e.g. 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 5432
	// Port the port, default 5432
	Port int `json:"port" default:"5432"`
	// User 用户名
	// User the user name
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	// Password the password (optional)
	Password string `json:"password"`
	// DBName 默认数据库名，查询限定于该库
	// DBName the default database, queries scoped to it
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	// Table the business table name to expose
	Table string `json:"table" binding:"required"`
	// Schema 同 MySQLTableConfig.Schema 语义：提供时跳过 information_schema 自省。
	// Schema has the same semantics as MySQLTableConfig.Schema: introspection is skipped
	// when provided.
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// ClickHouseTableConfig 描述一个 ClickHouse 单表数据源的前端配置。
// Table 为该实例 DBName 库下要暴露的业务表，注册时经 DESCRIBE TABLE 自省建 schema。
// ClickHouseTableConfig describes the front-end config of a ClickHouse single-table
// datasource. Table is the business table under the instance's DBName to expose; the
// schema is built by DESCRIBE TABLE introspection on registration.
type ClickHouseTableConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1
	// Addr the host address without port, e.g. 127.0.0.1
	Addr string `json:"addr" binding:"required"`
	// Port 端口，默认 9000（原生协议）
	// Port the port, default 9000 (native protocol)
	Port int `json:"port" default:"9000"`
	// User 用户名
	// User the user name
	User string `json:"user" binding:"required"`
	// Password 密码（可选）
	// Password the password (optional)
	Password string `json:"password"`
	// DBName 默认数据库名，查询限定于该库
	// DBName the default database, queries scoped to it
	DBName string `json:"dbName" binding:"required"`
	// Table 要暴露的业务表名
	// Table the business table name to expose
	Table string `json:"table" binding:"required"`
	// Schema 同 MySQLTableConfig.Schema 语义：提供时跳过 DESCRIBE TABLE 自省。
	// Schema has the same semantics as MySQLTableConfig.Schema: introspection is skipped
	// when provided.
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// SQLiteFileTableConfig 描述一个 SQLite 文件业务表数据源的前端配置。
// .db 文件由前端上传、经 S3 对象存储加密持久化（对象键由命名空间+数据源名派生，见
// SQLiteObjectKey）；Table 为该文件内要暴露的表，注册时经 PRAGMA table_info 自省。
// SQLiteFileTableConfig describes the front-end config of a SQLite-file table
// datasource. The .db file is uploaded by the frontend and stored encrypted in S3 (object
// key derived from namespace + datasource name, see SQLiteObjectKey); Table is the table
// to expose, and the schema is built by PRAGMA table_info introspection on registration.
type SQLiteFileTableConfig struct {
	// Table 要暴露的业务表名
	// Table the business table name to expose
	Table string `json:"table" binding:"required"`
	// Schema 同 MySQLTableConfig.Schema 语义：提供时跳过 PRAGMA table_info 自省。
	// Schema has the same semantics as MySQLTableConfig.Schema: introspection is skipped
	// when provided.
	Schema []DatasourceColumn `json:"schema" form:"kind=columns"`
}

// MongoCollectionConfig 描述一个 MongoDB 集合数据源的前端配置。
// MongoDB 在数据库层面无固定 schema，故数据源以「一个集合」为一张 vtable 时，
// schema 必须由前端在注册时完整提供（必填）；查询只能引用 schema 中声明的列。
// MongoCollectionConfig describes the front-end config of a MongoDB collection
// datasource. As MongoDB has no fixed schema, the schema must be fully provided by
// the frontend at registration (required) when one collection is exposed as a vtable;
// queries may only reference the schema-declared columns.
type MongoCollectionConfig struct {
	// URI 连接串（含认证），如 mongodb://user:pass@127.0.0.1:27017
	// URI the connection string (with auth), e.g. mongodb://user:pass@127.0.0.1:27017
	URI string `json:"uri" binding:"required"`
	// DB 数据库名
	// DB the database name
	DB string `json:"db" binding:"required"`
	// Collection 集合名
	// Collection the collection name
	Collection string `json:"collection" binding:"required"`
	// Schema 列定义（必填）：声明列名与 ValueType，作为该集合 vtable 的唯一 schema 来源。
	// Schema the column definitions (required): declares names and ValueTypes, serving as the
	// only schema source of the collection vtable.
	Schema []DatasourceColumn `json:"schema" form:"kind=columns" binding:"required"`
}

// EtcdTableConfig 描述一个 etcd 键值数据源的前端配置。etcd 无业务表概念，一个数据源
// 即一张 key/value vtable；Endpoints 为逗号分隔的 host:port 列表。
// EtcdTableConfig describes the front-end config of an etcd key-value datasource. etcd
// has no business-table concept; one datasource is a single key/value vtable. Endpoints
// is a comma-separated list of host:port.
type EtcdTableConfig struct {
	// Endpoints 逗号分隔的端点列表，如 127.0.0.1:2379
	// Endpoints the comma-separated endpoint list, e.g. 127.0.0.1:2379
	Endpoints string `json:"endpoints" binding:"required"`
	// Username 用户名（可选）
	// Username the user name (optional)
	Username string `json:"username"`
	// Password 密码（可选）
	// Password the password (optional)
	Password string `json:"password"`
}

// DatasourceColumn 手动指定的一列：Name 为暴露到 vtable 的列名（合法 SQL 标识符），
// Type 为该列值的类型键（any/integer/float/string/blob/bool 之一）。
// DatasourceColumn is one manually-specified column: Name is the column name exposed to
// the vtable (a valid SQL identifier), Type is the value type key
// (one of any/integer/float/string/blob/bool).
type DatasourceColumn struct {
	// Name 列名，须为合法 SQL 标识符
	// Name the column name, must be a valid SQL identifier
	Name string `json:"name" binding:"required"`
	// Type 列值类型键：any/intger/float/string/blob/bool
	// Type the column value type key: any/integer/float/string/blob/bool
	Type string `json:"type" binding:"required"`
}

// parseDatasourceColumns 把前端手动下发的列定义转换为 vtable 注册用 schema。校验列名
// 为合法 SQL 标识符且不重复，类型键映射到 sqlitevtab.ValueType；失败上抛带上下文的错误。
// parseDatasourceColumns converts front-end-submitted column definitions into the schema
// used for vtable registration. It validates that names are unique valid SQL identifiers
// and maps type keys to sqlitevtab.ValueType, returning contextual errors on failure.
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
// columnValueType maps a manual-schema type key to sqlitevtab.ValueType; unknown keys error.
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
// ValidateLocalName validates the datasource local name: a valid SQL identifier. As
// vtables are registered under their bare name on the per-namespace pool, the "__"/
// trailing-underscore constraints needed for scoped names no longer apply; SQLite
// reserved keywords are additionally checked by RegisterDatasource's
// validateDatasourceName.
func ValidateLocalName(name string) error {
	if !localNamePattern.MatchString(name) {
		return errors.Newf("tag datasource: invalid local name %q", name)
	}
	return nil
}

// redisDatasourceFactory 依据 Redis 数据结构类型分派到对应数据源构造器。
// redisDatasourceFactory dispatches to the data-structure-specific datasource
// constructor by the Redis structure type.
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
// （裸名/schema）+ 各构造器承担；MySQL 会在注册期自省一次表结构（连库校验）。
// registerInto parses the config and registers one sqlite vtable under its bare name
// on the given namespace pool. It parses cfgData by typ, builds a namespace-isolated
// connection pool, constructs the datasource and registers it under the bare name on
// pool (already resolved by the caller Pools.Register for the current namespace). Once
// registered the vtable is queryable by its bare name via sqlite.query. Validation is
// done by ValidateLocalName (local name) + RegisterDatasource (bare name/schema) + the
// constructors; MySQL introspects the table once during registration (a connection-level
// check).
func registerInto(ctx context.Context, pool *sqlitevtab.SQLitePool, store *s3.ObjectCache, namespace, name, typ string, cfgData []byte) error {
	if err := ValidateLocalName(name); err != nil {
		return err
	}

	var ds sqlitevtab.Datasource
	switch typ {
	case DatasourceTypeRedis:
		var cfg RedisConfig
		// 先应用 default tag 填充默认值，再由实际传入的 JSON 覆盖；缺省字段保持默认值。
		if err := defaults.Set(&cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: apply defaults: %v", err))
		}
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid redis config: %v", err))
		}
		opts := &redis.Options{Addr: fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port), Password: cfg.Password, DB: cfg.DB}
		poolHandle := NewRedisPool(opts, namespace)
		var err error
		if ds, err = redisDatasourceFactory(poolHandle, cfg.Struct); err != nil {
			return err
		}
	case DatasourceTypeMySQL:
		var cfg MySQLTableConfig
		// 先应用 default tag 填充默认值（含默认端口 3306），再由实际传入的 JSON 覆盖。
		if err := defaults.Set(&cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: apply defaults: %v", err))
		}
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid mysql config: %v", err))
		}
		poolHandle := NewMySQLPool(&MySQLConfig{
			Addr:      cfg.Addr,
			Port:      cfg.Port,
			User:      cfg.User,
			Password:  cfg.Password,
			DBName:    cfg.DBName,
			Namespace: namespace,
		})
		var err error
		if len(cfg.Schema) > 0 {
			var cols []sqlitevtab.Column
			if cols, err = parseDatasourceColumns(cfg.Schema); err != nil {
				return err
			}
			if ds, err = NewMySQLDatasourceWithSchema(poolHandle, cfg.Table, cols); err != nil {
				return err
			}
		} else if ds, err = NewMySQLDatasource(ctx, poolHandle, cfg.Table); err != nil {
			return err
		}
	case DatasourceTypePostgres:
		var cfg PostgresTableConfig
		// 先应用 default tag 填充默认值（含默认端口 5432），再由实际传入的 JSON 覆盖。
		if err := defaults.Set(&cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: apply defaults: %v", err))
		}
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid postgres config: %v", err))
		}
		poolHandle := NewPostgresPool(&PostgresConfig{
			Addr:      cfg.Addr,
			Port:      cfg.Port,
			User:      cfg.User,
			Password:  cfg.Password,
			DBName:    cfg.DBName,
			Namespace: namespace,
		})
		var err error
		if len(cfg.Schema) > 0 {
			var cols []sqlitevtab.Column
			if cols, err = parseDatasourceColumns(cfg.Schema); err != nil {
				return err
			}
			if ds, err = NewPostgresDatasourceWithSchema(poolHandle, cfg.Table, cols); err != nil {
				return err
			}
		} else if ds, err = NewPostgresDatasource(ctx, poolHandle, cfg.Table); err != nil {
			return err
		}
	case DatasourceTypeClickHouse:
		var cfg ClickHouseTableConfig
		if err := defaults.Set(&cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: apply defaults: %v", err))
		}
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid clickhouse config: %v", err))
		}
		poolHandle := NewClickHousePool(&ClickHouseConfig{
			Addr: cfg.Addr, Port: cfg.Port, User: cfg.User, Password: cfg.Password,
			DBName: cfg.DBName, Namespace: namespace,
		})
		var err error
		if len(cfg.Schema) > 0 {
			var cols []sqlitevtab.Column
			if cols, err = parseDatasourceColumns(cfg.Schema); err != nil {
				return err
			}
			if ds, err = NewClickHouseDatasourceWithSchema(poolHandle, cfg.Table, cols); err != nil {
				return err
			}
		} else if ds, err = NewClickHouseDatasource(ctx, poolHandle, cfg.Table); err != nil {
			return err
		}
	case DatasourceTypeSQLiteFile:
		var cfg SQLiteFileTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid sqlite_file config: %v", err))
		}
		if store == nil {
			return errors.New("tag datasource: sqlite_file requires S3 object storage (ObjectCache) to be enabled")
		}
		// 先从 S3 拉取并解密为本地明文文件（内存优先、超阈落盘，见 internal/pkg/s3/objectcache.go），
		// 再把该明文路径交给连接池打开；对象键由命名空间+数据源名派生，随生命周期稳定。
		objectKey := SQLiteObjectKey(namespace, name)
		var path string
		if p, err := store.Materialize(ctx, objectKey); err != nil {
			return err
		} else {
			path = p
		}
		poolHandle := NewSQLiteFilePool(&SQLiteFileConfig{Path: path, Namespace: namespace})
		var err error
		if len(cfg.Schema) > 0 {
			var cols []sqlitevtab.Column
			if cols, err = parseDatasourceColumns(cfg.Schema); err != nil {
				return err
			}
			if ds, err = NewSQLiteFileDatasourceWithSchema(poolHandle, cfg.Table, cols); err != nil {
				return err
			}
		} else if ds, err = NewSQLiteFileDatasource(ctx, poolHandle, cfg.Table); err != nil {
			return err
		}
	case DatasourceTypeEtcd:
		var cfg EtcdTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid etcd config: %v", err))
		}
		ds = NewEtcdDatasource(NewEtcdPool(&EtcdConfig{
			Endpoints: splitEndpoints(cfg.Endpoints), Username: cfg.Username,
			Password: cfg.Password, Namespace: namespace,
		}))
	case DatasourceTypeMongo:
		var cfg MongoCollectionConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid mongo config: %v", err))
		}
		if len(cfg.Schema) == 0 {
			return errors.New("mongo: schema must be provided during registration")
		}
		var err error
		var cols []sqlitevtab.Column
		if cols, err = parseDatasourceColumns(cfg.Schema); err != nil {
			return err
		}
		if ds, err = NewMongoDatasourceWithSchema(NewMongoPool(&MongoConfig{
			URI: cfg.URI, DB: cfg.DB, Collection: cfg.Collection, Namespace: namespace,
		}), cols); err != nil {
			return err
		}
	default:
		return errors.Newf("tag datasource: unsupported type %q", typ)
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
// schemaOf fetches the registration schema from a datasource. Datasources implement the
// dsSchemaProvider interface to reuse their internal schema (avoiding duplication).
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
// ValidateConfigJSON validates only that the config JSON is structurally consistent with
// the datasource type (no connection, no registration). Disabling flows still validate the
// config to avoid persisting an invalid one, but must not trigger connection-level
// introspection, so this stays a decode-level check separate from Register.
func ValidateConfigJSON(typ string, cfgData []byte) error {
	switch typ {
	case DatasourceTypeRedis:
		var cfg RedisConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid redis config: %v", err))
		}
		if _, err := redisDatasourceFactory(nil, cfg.Struct); err != nil {
			return err
		}
	case DatasourceTypeMySQL:
		var cfg MySQLTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid mysql config: %v", err))
		}
	case DatasourceTypePostgres:
		var cfg PostgresTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid postgres config: %v", err))
		}
	case DatasourceTypeClickHouse:
		var cfg ClickHouseTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid clickhouse config: %v", err))
		}
	case DatasourceTypeSQLiteFile:
		var cfg SQLiteFileTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid sqlite_file config: %v", err))
		}
	case DatasourceTypeEtcd:
		var cfg EtcdTableConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid etcd config: %v", err))
		}
	case DatasourceTypeMongo:
		var cfg MongoCollectionConfig
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			return errors.WithStack(errors.Newf("tag datasource: invalid mongo config: %v", err))
		}
	default:
		return errors.Newf("tag datasource: unsupported type %q", typ)
	}
	return nil
}

// splitEndpoints 把逗号分隔的端点串拆成列表并去掉空项/空白。
// splitEndpoints splits a comma-separated endpoint string into a list, dropping empty/
// whitespace entries.
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
