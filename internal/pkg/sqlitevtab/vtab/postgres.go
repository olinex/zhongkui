// Package vtab（本文件）提供基于 PostgreSQL 的 vtable 数据源。
// 每个业务表经 PostgresDatasource 在构造期自省 information_schema 表结构后注册为一张
// 独立 vtable（详见 postgres_table.go）。连接模型与 MySQL 一致，采用"运行时借还，
// 取数即还"：数据源持有一个面向某实例配置的 PostgresPool；每次查询 Open 时 acquire
// 借出 *sql.DB（必要时懒建连/重连），同步取回全部行后立即 release 归还。
// database/sql 的 *sql.DB 内部已池化物理连接，故本层以 resourcepool 池化「*sql.DB
// 实例」：前端配置可能存在多个 postgres 实例，每个配置经 Key 映射到一个相互独立的
// db 池，统一驱动"惰性建连 → 借出 → 归还 → 闲置驱逐 → 优雅关闭"。
//
// 本文件遵循 internal/pkg/sqlitevtab 的 Datasource 超集契约：数据源头过滤是优化，
// sqlite 侧以 Omit=false 复查每个返回行，故任何下推谓词都必须是真实匹配行的超集
// （见 postgres_table.go 关于 OpLIKE 语义差异的取舍），禁止漏行。
//
// PostgreSQL 标识符用双引号包裹（quotePgIdent），与 MySQL 的反引号不同；驱动经
// pgx/v5 stdlib 注册（sql.OpenDB + stdlib.OpenDB）。
//
// This file provides the PostgreSQL vtable datasource connection layer. Each business
// table is introspected at construction into an independent vtable via
// PostgresDatasource (see postgres_table.go). The connection model matches MySQL,
// "borrow on open, return after fetch": a datasource holds a PostgresPool bound to one
// instance config; each query's Open borrows a *sql.DB (lazily creating or reconnecting
// as needed), synchronously fetches all rows and immediately releases it. Because
// database/sql *sql.DB already pools physical connections, this layer uses resourcepool
// to pool *sql.DB instances: front-end config may declare multiple postgres instances,
// each mapping to an independent db pool by Key that drives "lazy create → borrow →
// return → idle eviction → graceful close".
//
// This file follows the sqlitevtab Datasource superset contract: upstream filtering is
// only an optimization, and sqlite re-checks every returned row with Omit=false, so any
// pushdown predicate must be a superset of the truly matching rows (see
// postgres_table.go for the OpLIKE semantic trade-off), and must never drop one.
//
// PostgreSQL identifiers are wrapped in double quotes (quotePgIdent), unlike MySQL's
// backticks; the driver is registered via pgx/v5 stdlib (sql.OpenDB + stdlib.OpenDB).
package vtab

import (
	// stdlib
	"context"
	"database/sql"
	"fmt"
	"strings"

	// third-party

	"github.com/creasty/defaults"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// PostgresConfig 描述一个 PostgreSQL 实例的接入配置。组件化形态便于多实例的动态注入
// 与按规范化字段派生彼此独立的连接池 Key。
// PostgresConfig describes how to reach one PostgreSQL instance. The component shape
// eases dynamic injection of multiple instances and deriving an independent pool Key
// from canonical fields.
type PostgresConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1。
	// Addr the host address without port, e.g. 127.0.0.1.
	Addr string
	// Port 端口，默认 5432。
	// Port the port, default 5432.
	Port int `default:"5432"`
	// User 用户名。
	// User the user name.
	User string
	// Password 密码。
	// Password the password.
	Password string
	// DBName 默认数据库名；本实例下的 vtable 查询均限定于该库。
	// DBName the default database; vtable queries under this instance are scoped to it.
	DBName string
	// Namespace 数据源所属命名空间。仅参与连接池 Key 派生（不参与 DSN），用于按命名
	// 空间隔离连接会话：同一实例在不同命名空间下派生不同 Key、绝不共用连接。
	// Namespace the namespace the datasource belongs to. It only participates in pool-key
	// derivation (not in the DSN) to isolate connection sessions by namespace: the same
	// instance under different namespaces derives different Keys and never shares connections.
	Namespace string
}

// postgresClients 是进程级 postgres db 池注册表（独立 Key 空间，复用通用 *sql.DB 基座）。
// 前端配置经 NewPostgresPool 注入后，对应 Key 的 *sql.DB 即可在任意 datasource 中借出。
// postgresClients is the postgres db pool registry (its own Key space, reusing the generic
// *sql.DB base). Once a front-end config is injected via NewPostgresPool, *sql.DB for that
// Key become borrowable from any datasource.
var postgresClients = newSQLDBRegistry()

// pgBase 是各 PostgreSQL vtable 数据源的共享基座：持有连接池、schema、列值类型与
// 可下推（非 Any）列索引。
// pgBase is the shared base of the PostgreSQL vtable datasources, holding the pool,
// schema, per-column value types and the filterable (non-Any) column indexes.
type pgBase struct {
	pool       *PostgresPool
	schema     sqlitevtab.Schema
	valueTypes []sqlitevtab.ValueType
	filterable []int
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
// datasourceSchema returns the datasource schema for registration and testing.
func (b *pgBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// PostgresPool 是 postgres 的 sqlDBPool 句柄，绑定一个实例配置派生的 Key（复用通用
// *sql.DB 基座）。*sql.DB 并发安全，可被局部借出使用；数据源自省表结构时以 pool.db
// 限定到库。
// PostgresPool is the postgres sqlDBPool handle bound to a Key derived from an instance
// config (reusing the generic *sql.DB base). *sql.DB values are concurrency-safe and may
// be borrowed and used locally; datasources scope introspection to the database via pool.db.
type PostgresPool struct {
	sqlDBPool
	db string
}

// toConnConfig 将组件化配置编为 pgx/v5 的 *pgx.ConnConfig（含 DSN 与密码）。
// 内网访问默认关闭 sslmode，避免连本机/内网实例时的 TLS 握手；密码单独赋值以规避
// DSN 转义问题。ParseConfig 仅对格式非法返回错误，正常难以触发，防御性兜底构造。
// toConnConfig compiles the component config into a *pgx.ConnConfig (DSN plus password).
// sslmode is disabled by default for intranet access to avoid TLS handshakes; the
// password is assigned separately to dodge DSN escaping issues. ParseConfig only errors
// on malformed input (hard to trigger), so a defensive fallback is provided.
func (c *PostgresConfig) toConnConfig() *pgx.ConnConfig {
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable",
		c.Addr, c.Port, c.User, c.DBName))
	if err != nil {
		// ParseConfig 仅对格式非法返回错误，正常难以触发；toConnConfig 不返回 error，
		// 防御性以字段赋值兜底（避免 promoted-field 复合字面量在 go1.26 下的限制）。
		cfg = &pgx.ConnConfig{}
		cfg.Host = c.Addr
		cfg.Port = uint16(c.Port)
		cfg.User = c.User
		cfg.Database = c.DBName
	}
	cfg.Password = c.Password
	return cfg
}

// pgKey 由配置规范化出实例 Key（Namespace + Addr + User + DBName，不含密码）：语义
// 配置差异映射不同 Key，从而在 Manager 中维护相互独立的 db 池；命名空间维度保证同一
// 实例在不同命名空间下维护相互独立的 db 池（隔离连接与凭据）。
// pgKey derives the instance Key from config (Namespace + Addr + User + DBName, excluding
// the password): different canonical configs map to different Keys so the Manager keeps
// independent db pools; the namespace dimension guarantees the same instance keeps
// independent db pools across namespaces (isolating connections and credentials).
func pgKey(c *pgx.ConnConfig, namespace string) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s:%s/%s", namespace, c.Host, c.User, c.Database))
}

// NewPostgresPool 注入一份 postgres 实例配置并返回其池句柄。db 惰性创建（借出时才
// 建立），故构造本身不触发网络连接。同一实例配置（Key）重复注入会覆盖。
// NewPostgresPool injects one postgres instance configuration and returns its pool
// handle. Databases are created lazily on borrow, so construction initiates no network
// connection. Re-injecting the same instance config (Key) overwrites it.
func NewPostgresPool(cfg *PostgresConfig) *PostgresPool {
	// 先应用 default tag 填充默认端口 5432，再由调用方显式设置覆盖；
	// defaults.Set 仅在缺省（零值）字段上生效，失败即为 default tag 格式错误（致命不变量）。
	if err := defaults.Set(cfg); err != nil {
		panic(fmt.Sprintf("postgres: apply defaults: %v", err))
	}
	cc := cfg.toConnConfig()
	key := pgKey(cc, cfg.Namespace)
	postgresClients.register(key, func() (*sql.DB, error) {
		return stdlib.OpenDB(*cc), nil
	})
	return &PostgresPool{sqlDBPool: sqlDBPool{reg: postgresClients, key: key}, db: cc.Database}
}

// CloseAllPostgresClients 关闭全部 postgres db 池并释放底层连接。供接入方在进程优雅
// 关闭阶段调用。
// CloseAllPostgresClients closes every postgres db pool and destroys live objects,
// releasing underlying connections. It is meant for graceful shutdown.
func CloseAllPostgresClients(ctx context.Context) {
	postgresClients.closeAll(ctx)
}

// quotePgIdent 用双引号包裹标识符（表名/列名），并转义其中的双引号，用于把数据源与
// 列名安全地嵌入 SQL，防止注入。
// quotePgIdent wraps an identifier (table/column name) in double quotes, escaping inner
// double quotes, to embed datasource and column names into SQL safely against injection.
func quotePgIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
