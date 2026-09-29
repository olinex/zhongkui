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

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	// 第三方

	"github.com/creasty/defaults"
	"github.com/go-sql-driver/mysql"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// mysqlIdentPattern 合法标识符（表名/列名）校验模式，与 sqlitevtab 的列名校验一致。
var mysqlIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// MySQLConfig 描述一个 MySQL/MariaDB 实例的接入配置。组件化形态便于多实例的动态
// 注入与按规范化字段派生彼此独立的连接池 Key。
type MySQLConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1。
	Addr string
	// Port 端口，默认 3306。
	Port int `default:"3306"`
	// User 用户名。
	User string
	// Password 密码。
	Password string
	// DBName 默认数据库名；本实例下的 vtable 查询均限定于该库。
	DBName string
	// Namespace 数据源所属命名空间。仅参与连接池 Key 派生（不参与 DSN），
	// 用于按命名空间隔离连接会话：同一实例在不同命名空间下派生不同 Key、绝不共用连接。
	Namespace string
}

// mysqlClients 是进程级 mysql db 池注册表（独立 Key 空间，复用通用 *sql.DB 基座）。
// 前端配置经 NewMySQLPool 注入后，对应 Key 的 *sql.DB 即可在任意 datasource 中借出。
var mysqlClients = newSQLDBRegistry()

// mysqlBase 是各 MySQL vtable 数据源的共享基座：持有连接池、schema、列值类型与
// 可下推（非 Any）列索引。
type mysqlBase struct {
	pool       *MySQLPool
	schema     sqlitevtab.Schema
	valueTypes []sqlitevtab.ValueType
	filterable []int
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
func (b *mysqlBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// MySQLPool 是 mysql 的 sqlDBPool 句柄，绑定一个实例配置派生的 Key（复用通用 *sql.DB
// 基座）。*sql.DB 并发安全，可被局部借出使用；数据源自省表结构时以 pool.db 限定到库。
type MySQLPool struct {
	sqlDBPool
	db string
}

// toDriverConfig 将组件化配置编为 go-sql-driver 的 *mysql.Config，用于 FormatDSN。
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
func mysqlKey(c *mysql.Config, namespace string) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s:%s/%s", namespace, c.Addr, c.User, c.DBName))
}

// NewMySQLPool 注入一份 mysql/mariadb 实例配置并返回其池句柄。db 惰性创建（借出时
// 才建立），故构造本身不触发网络连接。同一实例配置（Key）重复注入会覆盖。
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
func CloseAllMySQLClients(ctx context.Context) {
	mysqlClients.closeAll(ctx)
}

// quoteIdent 用反引号包裹标识符（表名/列名），并转义其中的反引号，用于把数据源与列名
// 安全地嵌入 SQL，防止注入。
func quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// convertCell 将 driver 扫描值按列的 ValueType 归一化为 sqlite 值；nil 保持 nil，
// 转换失败的异常值按 nil 处理（sqlite 复查仍以 Omit=false 兜底，真实匹配行不会被丢弃）。
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
type mysqlRows struct {
	rows [][]any
	idx  int
}

// newMySQLRows 构造内存行游标。
func newMySQLRows(rows [][]any) *mysqlRows {
	return &mysqlRows{rows: rows}
}

// Next 返回下一行（直接引用，不拷贝；调用方按需保留时自行拷贝）。
func (r *mysqlRows) Next() ([]any, error) {
	if r.idx >= len(r.rows) {
		return nil, io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	return row, nil
}

// Close 释放行集引用（幂等）。
func (r *mysqlRows) Close() error {
	r.rows = nil
	return nil
}

// mysqlPlugin 实现 DatasourcePlugin，把 mysql 数据源类型接入插件注册表。
type mysqlPlugin struct{}

// Key 返回数据源类型键。
func (mysqlPlugin) Key() string { return DatasourceTypeMySQL }

// ConfigType 返回配置值反射类型。
func (mysqlPlugin) ConfigType() reflect.Type { return reflect.TypeFor[MySQLTableConfig]() }

// NewConfig 返回全新一份 *MySQLTableConfig。
func (mysqlPlugin) NewConfig() any { return &MySQLTableConfig{} }

// ValidateConfig 结构已经解码校验，这里无需额外操作。
func (mysqlPlugin) ValidateConfig(_ any) error { return nil }

// TestConn 建池借出 connection、PingContext 探活后归还。
func (mysqlPlugin) TestConn(ctx context.Context, cfg any) error {
	c := cfg.(*MySQLTableConfig)
	pool := NewMySQLPool(&MySQLConfig{Addr: c.Addr, Port: c.Port, User: c.User, Password: c.Password, DBName: c.DBName})
	db, err := pool.Acquire(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	pingErr := db.PingContext(ctx)
	pool.Release(ctx, db)
	if pingErr != nil {
		return errors.WithStack(pingErr)
	}
	return nil
}

// IntrospectSchema 建池、NewMySQLDatasource 自省表结构后返回列定义。
func (mysqlPlugin) IntrospectSchema(ctx context.Context, cfg any, _ io.Reader) ([]DatasourceColumn, error) {
	c := cfg.(*MySQLTableConfig)
	pool := NewMySQLPool(&MySQLConfig{Addr: c.Addr, Port: c.Port, User: c.User, Password: c.Password, DBName: c.DBName})
	ds, err := NewMySQLDatasource(ctx, pool, c.Table)
	if err != nil {
		return nil, err
	}
	schema, err := schemaOf(ds)
	if err != nil {
		return nil, err
	}
	return schemaColumnsOut(schema), nil
}

// Register 校验 schema 非空 → 建池 → parseDatasourceColumns → NewMySQLDatasourceWithSchema。
func (mysqlPlugin) Register(ctx context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, namespace, _ string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*MySQLTableConfig)
	if len(c.Schema) == 0 {
		return nil, errors.Newf("%s: schema must be provided; run introspection from the frontend", DatasourceTypeMySQL)
	}
	poolHandle := NewMySQLPool(&MySQLConfig{
		Addr:      c.Addr,
		Port:      c.Port,
		User:      c.User,
		Password:  c.Password,
		DBName:    c.DBName,
		Namespace: namespace,
	})
	cols, err := parseDatasourceColumns(c.Schema)
	if err != nil {
		return nil, err
	}
	return NewMySQLDatasourceWithSchema(poolHandle, c.Table, cols)
}

// init 自注册 mysql 数据源类型。
func init() {
	RegisterDatasourceType(mysqlPlugin{})
}
