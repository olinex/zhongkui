// Package vtab（本文件）提供第三方数据源的前端配置驱动的统一注册逻辑。
// 每个数据源行对应一张 sqlite vtable，归属到命名空间池（vtab.Pools）并以裸名注册；
// registerInto 解析配置（redis 结构 / mysql 表）、按命名空间隔离地建立连接池并构造
// 数据源。CRUD 处理器与启动重载统一经 Pools.Register/Unregister 收敛到本文件，保证
// 建立/注销逻辑一致且可测。
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

	// third-party
	"github.com/creasty/defaults"
	"github.com/redis/go-redis/v9"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
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
func registerInto(ctx context.Context, pool *sqlitevtab.SQLitePool, namespace, name, typ string, cfgData []byte) error {
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
		if ds, err = NewMySQLDatasource(ctx, poolHandle, cfg.Table); err != nil {
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
	default:
		return errors.Newf("tag datasource: unsupported type %q", typ)
	}
	return nil
}
