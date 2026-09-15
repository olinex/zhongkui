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

	// third-party

	"github.com/creasty/defaults"
	"github.com/go-sql-driver/mysql"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

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
	Port int `default:"3306"`
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

// mysqlClients 是进程级 mysql db 池注册表（独立 Key 空间，复用通用 *sql.DB 基座）。
// 前端配置经 NewMySQLPool 注入后，对应 Key 的 *sql.DB 即可在任意 datasource 中借出。
// mysqlClients is the mysql db pool registry (its own Key space, reusing the generic
// *sql.DB base). Once a front-end config is injected via NewMySQLPool, *sql.DB for that
// Key become borrowable from any datasource.
var mysqlClients = newSQLDBRegistry()

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

// MySQLPool 是 mysql 的 sqlDBPool 句柄，绑定一个实例配置派生的 Key（复用通用 *sql.DB
// 基座）。*sql.DB 并发安全，可被局部借出使用；数据源自省表结构时以 pool.db 限定到库。
// MySQLPool is the mysql sqlDBPool handle bound to a Key derived from an instance config
// (reusing the generic *sql.DB base). *sql.DB values are concurrency-safe and may be
// borrowed and used locally; datasources scope introspection to the database via pool.db.
type MySQLPool struct {
	sqlDBPool
	db string
}

// toDriverConfig 将组件化配置编为 go-sql-driver 的 *mysql.Config，用于 FormatDSN。
// toDriverConfig compiles the component config into go-sql-driver's *mysql.Config for
// FormatDSN.
func (c *MySQLConfig) toDriverConfig() *mysql.Config {
	return &mysql.Config{
		User:   c.User,
		Passwd: c.Password,
		Net:    "tcp",
		Addr:   fmt.Sprintf("%s:%d", c.Addr, c.Port),
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
	// 先应用 default tag 填充默认端口 3306，再由调用方显式设置覆盖；
	// defaults.Set 仅在缺省（零值）字段上生效，失败即为 default tag 格式错误（致命不变量）。
	if err := defaults.Set(cfg); err != nil {
		panic(fmt.Sprintf("mysql: apply defaults: %v", err))
	}
	dc := cfg.toDriverConfig()
	key := mysqlKey(dc, cfg.Namespace)
	mysqlClients.register(key, func() (*sql.DB, error) {
		return sql.Open("mysql", dc.FormatDSN())
	})
	return &MySQLPool{sqlDBPool: sqlDBPool{reg: mysqlClients, key: key}, db: dc.DBName}
}

// CloseAllMySQLClients 关闭全部 mysql db 池并释放底层连接。供接入方在进程优雅关闭阶段调用。
// CloseAllMySQLClients closes every mysql db pool and destroys live objects, releasing
// underlying connections. It is meant for graceful shutdown.
func CloseAllMySQLClients(ctx context.Context) {
	mysqlClients.closeAll(ctx)
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
