// Package vtab（本文件）提供基于 MySQL/MariaDB 的 vtable 数据源。
// 每个业务表经 MySQLDatasource 在构造期自省表结构后注册为一张独立 vtable，列定义与
// 筛选下推由数据源按 sqlitevtab.Schema 提供。连接模型与 Redis 一致，采用
// "运行时借还，取数即还"：数据源持有一个面向某实例配置的 MySQLPool；
// 每次查询 Open 时 acquire 借出 *sql.DB（必要时懒建连/重连），同步取回全部行后立即
// release 归还。database/sql 的 *sql.DB 内部已池化物理连接，故本层以 resourcepool
// 池化「*sql.DB 实例」：前端配置可能存在多个 mysql/mariadb 实例，每个配置经 Key
// 映射到一个相互独立的 db 池，统一驱动"惰性建连 → 借出 → 归还 → 闲置驱逐 → 优雅关闭"。
//
// 本文件遵循 internal/pkg/sqlitevtab 的 Datasource 超集契约：数据源头过滤是优化，
// sqlite 侧以 Omit=false 复查每个返回行，故任何下推谓词都必须是真实匹配行的超集
// （仅下推与 sqlite 语义一致且类型确定的列），禁止漏行。
//
// This file provides the MySQL/MariaDB vtable datasources. Each business table is
// introspected at construction into an independent vtable via MySQLDatasource, with
// its columns and filter pushdown served by the sqlitevtab.Schema. The connection
// model matches Redis, "borrow on open, return after fetch": a datasource holds a
// MySQLPool bound to one instance config; each query's Open borrows a *sql.DB (lazily
// creating or reconnecting as needed), synchronously fetches all rows and immediately
// releases it. Because database/sql *sql.DB already pools physical connections, this
// layer uses resourcepool to pool *sql.DB instances: front-end config may declare
// multiple mysql/mariadb instances, each mapping to an independent db pool by Key that
// drives "lazy create → borrow → return → idle eviction → graceful close".
//
// This file follows the sqlitevtab Datasource superset contract: upstream filtering
// is only an optimization, and sqlite re-checks every returned row with Omit=false, so
// any pushdown predicate must be a superset of the truly matching rows (only
// semantics-aligned and type-determined columns are pushed), and must never drop one.
package vtab

import (
	// stdlib
	"context"
	"database/sql"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	// third-party
	"github.com/go-sql-driver/mysql"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// defaultMySQLPort 未显式指定端口时使用的 MySQL 默认端口。
// defaultMySQLPort is the default MySQL port when none is specified.
const defaultMySQLPort = 3306

// mysqlIdentPattern 合法标识符（表名/列名）校验模式，与 sqlitevtab 的列名校验一致。
// mysqlIdentPattern validates legal identifiers (table/column names), consistent with
// sqlitevtab's column-name validation.
var mysqlIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// MySQLConfig 描述一个 MySQL/MariaDB 实例的接入配置。组件化形态便于多实例的动态
// 注入与按规范化字段派生彼此独立的连接池 Key。
// MySQLConfig describes how to reach one MySQL/MariaDB instance. The component shape
// eases dynamic injection of multiple instances and deriving an independent pool Key
// from canonical fields.
type MySQLConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1。
	// Addr the host address without port, e.g. 127.0.0.1.
	Addr string
	// Port 端口，默认 3306。
	// Port the port, default 3306.
	Port int
	// User 用户名。
	// User the user name.
	User string
	// Password 密码。
	// Password the password.
	Password string
	// DBName 默认数据库名；本实例下的 vtable 查询均限定于该库。
	// DBName the default database; vtable queries under this instance are scoped to it.
	DBName string
	// Namespace 数据源所属命名空间。仅参与连接池 Key 派生（不参与 DSN），
	// 用于按命名空间隔离连接会话：同一实例在不同命名空间下派生不同 Key、绝不共用连接。
	// Namespace the namespace the datasource belongs to. It only participates in pool-key
	// derivation (not in the DSN) to isolate connection sessions by namespace: the same
	// instance under different namespaces derives different Keys and never shares connections.
	Namespace string
}

// mysqlClients 是进程级 mysql db 池注册表：共享 Manager、Key→驱动配置映射与测试
// 用的 db 覆盖表。前端配置经 NewMySQLPool 注入后，对应 Key 的 *sql.DB 即可在任意
// datasource 中借出。
// mysqlClients is the process-wide mysql db pool registry: a shared Manager, a
// Key→driver-config map, and a test-only db override map. Once a front-end config is
// injected via NewMySQLPool, *sql.DB for that Key become borrowable from any datasource.
var mysqlClients = newMySQLClientRegistry()

// mysqlBase 是各 MySQL vtable 数据源的共享基座：持有连接池、schema、列值类型与
// 可下推（非 Any）列索引。
// mysqlBase is the shared base of the MySQL vtable datasources, holding the pool,
// schema, per-column value types and the filterable (non-Any) column indexes.
type mysqlBase struct {
	pool       *MySQLPool
	schema     sqlitevtab.Schema
	valueTypes []sqlitevtab.ValueType
	filterable []int
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
// datasourceSchema returns the datasource schema for registration and testing.
func (b *mysqlBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// MySQLPool 是基于 resourcepool 的 mysql db 池句柄，绑定一个实例配置派生的 Key。
// 底层为进程级共享的 resourcepool.Manager[*sql.DB]：按 Key 维护相互独立的
// database/sql 句柄池。对 database/sql 而言一个 *sql.DB 内部已池化物理连接，这里
// 池化的是「*sql.DB 实例」：前端配置可能存在多个 mysql/mariadb 实例，每个配置对应
// 一个 Key 一个 db 池，做到跨实例复用、失效自动重建与闲置回收。*sql.DB 并发安全，
// 池内可驻留多个实例以服务并发查询。
// MySQLPool is a resourcepool-backed mysql db pool handle bound to a Key derived from
// an instance configuration. Under the hood is a process-wide
// resourcepool.Manager[*sql.DB]: each Key maintains an independent database/sql handle
// pool. For database/sql one *sql.DB already pools physical connections, and this
// layer pools *sql.DB instances: front-end config may declare multiple mysql/mariadb
// instances, each mapping to a Key and a dedicated db pool that is reused, auto-rebuilt
// on staleness and reclaimed when idle. *sql.DB values are concurrency-safe, so a pool
// can hold several instances to serve concurrent queries.
type MySQLPool struct {
	key resourcepool.Key
	db  string
}

// mysqlClientRegistry 按 Key 维护 mysql db 池与配置。
// mysqlClientRegistry maintains mysql db pools and their config by Key.
type mysqlClientRegistry struct {
	mu       sync.RWMutex
	manager  *resourcepool.Manager[*sql.DB]
	cfgs     map[resourcepool.Key]*mysql.Config
	override map[resourcepool.Key]*sql.DB // 仅供测试注入 mock db
}

// newMySQLClientRegistry 创建注册表并装配进程级 Manager。Manager 采用默认资源池
// 配置（testOnBorrow 探活、失效自动重建、闲置驱逐），与其他池一致。
// newMySQLClientRegistry builds the registry and its process-wide Manager with the
// default resourcepool config, consistent with other pools.
func newMySQLClientRegistry() *mysqlClientRegistry {
	r := &mysqlClientRegistry{cfgs: make(map[resourcepool.Key]*mysql.Config), override: make(map[resourcepool.Key]*sql.DB)}
	r.manager = resourcepool.NewManager(r.makeDBFor, resourcepool.NewDefaultConfig())
	return r
}

// makeDBFor 依据 Key 从注册表取配置并懒创建 *sql.DB（resourcepool.Make）；测试通过
// override 注入 mock db 时直接返回之。返回的 *sql.DB 不实现 resourcepool.Lifespan
// （内部自愈连接），故仅按纯对象复用，销毁时由 resourcepool 调用其 io.Closer。
// makeDBFor looks up the config for a Key and lazily creates a *sql.DB
// (resourcepool.Make); tests bypass it by overriding with a mock db. The returned
// *sql.DB does not implement resourcepool.Lifespan (it self-heals connections), so it
// is reused as a plain object and closed by resourcepool via io.Closer.
func (r *mysqlClientRegistry) makeDBFor(key resourcepool.Key) (*sql.DB, error) {
	r.mu.RLock()
	if db, ok := r.override[key]; ok {
		r.mu.RUnlock()
		return db, nil
	}
	cfg := r.cfgs[key]
	r.mu.RUnlock()
	if cfg == nil {
		return nil, errors.Newf("mysql: config not registered for key %q", key)
	}
	return sql.Open("mysql", cfg.FormatDSN())
}

// overrideDB 设置/清除指定 Key 的 mock db 注入（仅供测试）。
// overrideDB sets/clears a mocked db override for a Key (test only).
func (r *mysqlClientRegistry) overrideDB(key resourcepool.Key, db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if db == nil {
		delete(r.override, key)
	} else {
		r.override[key] = db
	}
}

// toDriverConfig 将组件化配置编为 go-sql-driver 的 *mysql.Config，用于 FormatDSN。
// toDriverConfig compiles the component config into go-sql-driver's *mysql.Config for
// FormatDSN.
func (c *MySQLConfig) toDriverConfig() *mysql.Config {
	port := c.Port
	if port == 0 {
		port = defaultMySQLPort
	}
	return &mysql.Config{
		User:   c.User,
		Passwd: c.Password,
		Net:    "tcp",
		Addr:   fmt.Sprintf("%s:%d", c.Addr, port),
		DBName: c.DBName,
	}
}

// mysqlKey 由配置规范化出实例 Key（Namespace + Addr + User + DBName 编码，不含密码）：
// 语义配置差异映射不同 Key，从而在 Manager 中维护相互独立的 db 池；命名空间维度保证
// 同一实例在不同命名空间下维护相互独立的 db 池（隔离连接与凭据）。
// mysqlKey derives the instance Key from config (Namespace + Addr + User + DBName,
// excluding the password): different canonical configs map to different Keys so the
// Manager keeps independent db pools; the namespace dimension guarantees the same
// instance keeps independent db pools across namespaces (isolating connections and
// credentials).
func mysqlKey(c *mysql.Config, namespace string) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s:%s/%s", namespace, c.Addr, c.User, c.DBName))
}

// NewMySQLPool 注入一份 mysql/mariadb 实例配置并返回其池句柄。db 惰性创建（借出时
// 才建立），故构造本身不触发网络连接。同一实例配置（Key）重复注入会覆盖。
// NewMySQLPool injects one mysql/mariadb instance configuration and returns its pool
// handle. Databases are created lazily on borrow, so construction initiates no network
// connection. Re-injecting the same instance config (Key) overwrites it.
func NewMySQLPool(cfg *MySQLConfig) *MySQLPool {
	dc := cfg.toDriverConfig()
	key := mysqlKey(dc, cfg.Namespace)
	mysqlClients.mu.Lock()
	mysqlClients.cfgs[key] = dc
	mysqlClients.mu.Unlock()
	return &MySQLPool{key: key, db: dc.DBName}
}

// Acquire 从该实例的池中借出并返回一个可用的 *sql.DB（懒建连）。*sql.DB 并发安全，
// 可被局部借出使用。返回非 nil 错误表示借出失败。
// Acquire borrows a usable *sql.DB from this instance's pool (lazily connecting).
// *sql.DB values are concurrency-safe and may be borrowed and used locally. A non-nil
// error means the borrow failed.
func (p *MySQLPool) Acquire(ctx context.Context) (*sql.DB, error) {
	return mysqlClients.manager.Acquire(ctx, p.key)
}

// Release 将 *sql.DB 归还该实例的池以便复用。
// Release returns the *sql.DB to this instance's pool for reuse.
func (p *MySQLPool) Release(ctx context.Context, db *sql.DB) {
	mysqlClients.manager.Release(ctx, p.key, db)
}

// Close 销毁该实例池内的闲置 db（借出中的不受影响），用于配置删除/连接失效场景。
// 透传 ctx 以维护取消/超时语义。进程级全部 db 池的优雅关闭由接入方统一触发
// CloseAllMySQLClients。
// Close destroys idle databases in this instance's pool (borrowed ones are unaffected),
// for config-removal or connection-staleness scenarios. ctx is propagated to preserve
// cancel/timeout semantics. Graceful close of every process-wide db pool is triggered
// by the integrator via CloseAllMySQLClients.
func (p *MySQLPool) Close(ctx context.Context) {
	mysqlClients.manager.Clear(ctx, p.key)
}

// CloseAllMySQLClients 关闭全部 mysql db 池并销毁存活对象，释放底层连接。供接入方
// 在进程优雅关闭阶段调用。
// CloseAllMySQLClients closes every mysql db pool and destroys live objects, releasing
// underlying connections. It is meant for graceful shutdown.
func CloseAllMySQLClients(ctx context.Context) {
	mysqlClients.manager.CloseAll(ctx)
}

// quoteIdent 用反引号包裹标识符（表名/列名），并转义其中的反引号，用于把数据源与列名
// 安全地嵌入 SQL，防止注入。
// quoteIdent wraps an identifier (table/column name) in backticks, escaping inner
// backticks, to embed datasource and column names into SQL safely against injection.
func quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// convertCell 将 driver 扫描值按列的 ValueType 归一化为 sqlite 值；nil 保持 nil，
// 转换失败的异常值按 nil 处理（sqlite 复查仍以 Omit=false 兜底，真实匹配行不会被丢弃）。
// convertCell normalizes a scanned driver value to the sqlite value per the column's
// ValueType; nil stays nil, and an unconvertible value falls back to nil (the sqlite
// Omit=false re-check still backstops, so a truly matching row is never dropped).
func convertCell(v any, vt sqlitevtab.ValueType) any {
	if v == nil {
		return nil
	}
	switch vt {
	case sqlitevtab.ValueTypeInt64:
		if n, ok := asInt64(v); ok {
			return n
		}
		return nil
	case sqlitevtab.ValueTypeFloat64:
		if f, ok := asFloat64(v); ok {
			return f
		}
		return nil
	case sqlitevtab.ValueTypeBool:
		if b, ok := asBool(v); ok {
			return b
		}
		return nil
	case sqlitevtab.ValueTypeBlob:
		if b, ok := asBlob(v); ok {
			return b
		}
		return nil
	default: // ValueTypeString / ValueTypeAny
		return stringify(v)
	}
}

// stringify 归一化为字符串。
// stringify normalizes to a string.
func stringify(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprint(v)
	}
}

// asInt64 解析为 int64（含整形、float、字符串/字节表示的整数字面量）。
// asInt64 parses an int64 (integers, floats and string/[]byte integer literals).
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		return int64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case float64:
		return int64(n), true
	case []byte:
		i, err := strconv.ParseInt(string(n), 10, 64)
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}

// asFloat64 解析为 float64（含浮点、整形、decimal 的字符串/字节表示）。
// asFloat64 parses a float64 (floats, integers and string/[]byte decimal literals).
func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case []byte:
		f, err := strconv.ParseFloat(string(n), 64)
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// asBool 解析为 bool（含 bool、tinyint 整数、字符串/字节表示的 0/1 或 bool 字面量）。
// asBool parses a bool (bool, tinyint integers and string/[]byte 0/1 or bool literals).
func asBool(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case int64:
		return b != 0, true
	case int:
		return b != 0, true
	case uint64:
		return b != 0, true
	case []byte:
		return parseBoolString(string(b))
	case string:
		return parseBoolString(b)
	default:
		return false, false
	}
}

// parseBoolString 解析 0/1 或 bool 字面字符串。
// parseBoolString parses 0/1 or bool literal strings.
func parseBoolString(s string) (bool, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, false
	}
	if s == "0" {
		return false, true
	}
	if s == "1" {
		return true, true
	}
	p, err := strconv.ParseBool(s)
	return p, err == nil
}

// asBlob 保持二进制字节。
// asBlob keeps binary bytes.
func asBlob(v any) ([]byte, bool) {
	switch b := v.(type) {
	case []byte:
		return b, true
	case string:
		return []byte(b), true
	default:
		return nil, false
	}
}

// mysqlRows 是预先行集的行游标：行在 Open 阶段已同步取回内存，连接已即时归还，
// 游标自身不持有任何连接或外部资源，Close 仅释放引用（幂等）。
// mysqlRows is a precomputed row-set cursor: rows are synchronously fetched into memory
// during Open and the connection is released immediately, so the cursor holds no
// connection or external resource; Close merely drops references (idempotent).
type mysqlRows struct {
	rows [][]any
	idx  int
}

// newMySQLRows 构造内存行游标。
// newMySQLRows builds an in-memory row cursor.
func newMySQLRows(rows [][]any) *mysqlRows {
	return &mysqlRows{rows: rows}
}

// Next 返回下一行（直接引用，不拷贝；调用方按需保留时自行拷贝）。
// Next returns the next row (by reference, not a copy; callers copy if needed).
func (r *mysqlRows) Next() ([]any, error) {
	if r.idx >= len(r.rows) {
		return nil, io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	return row, nil
}

// Close 释放行集引用（幂等）。
// Close drops the row-set reference (idempotent).
func (r *mysqlRows) Close() error {
	r.rows = nil
	return nil
}
