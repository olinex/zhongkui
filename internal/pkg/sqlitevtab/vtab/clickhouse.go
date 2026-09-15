// Package vtab（本文件）提供基于 ClickHouse 的 vtable 数据源连接层。
// 每个业务表经 ClickHouseDatasource 在构造期 DESCRIBE 自省表结构后注册为一张独立
// vtable（详见 clickhouse_table.go）。连接模型复用共享的 sqlDBPool（详见 sqldb.go）：
// 按实例配置派生 Key，懒建 *sql.DB（clickhouse-go/v2 原生协议），Open 借出、取数即还。
// 数据源标识符用双引号包裹（复用 quotePgIdent），查询占位符为 ?。
//
// 超集契约与下推取舍见 clickhouse_table.go：ClickHouse 的 LIKE 大小写敏感（与 sqlite
// 不敏感不一致）而跳过下推，仅非 Any 列的比较/等值谓词安全下推。
//
// This file provides the ClickHouse vtable datasource connection layer. Each business
// table is introspected via DESCRIBE at construction into an independent vtable (see
// clickhouse_table.go). The connection model reuses the shared sqlDBPool (see
// sqldb.go): a Key derived from the instance config lazily builds a *sql.DB
// (clickhouse-go/v2 native protocol), borrowed on Open and returned right after
// fetching. Identifiers are double-quoted (reusing quotePgIdent) and query placeholders
// are ?. The superset contract and pushdown trade-off are discussed in
// clickhouse_table.go: ClickHouse LIKE is case-sensitive (inconsistent with sqlite) and
// so is not pushed; only comparison/equality predicates on non-Any columns are safely
// pushed.
package vtab

import (
	// stdlib
	"context"
	"database/sql"
	"fmt"
	"time"

	// third-party
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/creasty/defaults"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// ClickHouseConfig 描述一个 ClickHouse 实例的接入配置。
// ClickHouseConfig describes how to reach one ClickHouse instance.
type ClickHouseConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1。
	// Addr the host address without port, e.g. 127.0.0.1.
	Addr string
	// Port 端口，默认 9000（原生协议）。
	// Port the port, default 9000 (native protocol).
	Port int `default:"9000"`
	// User 用户名。
	// User the user name.
	User string
	// Password 密码。
	// Password the password.
	Password string
	// DBName 默认数据库名；本实例下的 vtable 查询均限定于该库。
	// DBName the default database; vtable queries under this instance are scoped to it.
	DBName string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与 DSN）。
	// Namespace the datasource namespace; only participates in pool-key derivation.
	Namespace string
	// DialTimeout 建连超时，默认 5s。
	// DialTimeout the connection dial timeout, default 5s.
	DialTimeout time.Duration `default:"5s"`
}

// clickhouseClients 是 ClickHouse 的 db 池注册表（独立 Key 空间）。
// clickhouseClients is the ClickHouse db pool registry (its own Key space).
var clickhouseClients = newSQLDBRegistry()

// clickhouseKey 由配置规范化出实例 Key（Namespace + Addr + User + DBName）。
// clickhouseKey derives the instance Key from config (Namespace + Addr + User + DBName).
func clickhouseKey(c *ClickHouseConfig) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s:%s/%s", c.Namespace, c.Addr, c.User, c.DBName))
}

// NewClickHousePool 注入一份 ClickHouse 实例配置并返回其池句柄。db 惰性创建，
// 构造本身不触发网络连接。（address 为空时跳过 addr 字段，交由驱动默认。）
// NewClickHousePool injects one ClickHouse instance config and returns its pool handle.
// Databases are created lazily, so construction initiates no network connection.
func NewClickHousePool(cfg *ClickHouseConfig) *ClickHousePool {
	// 先应用 default tag 填充默认值（端口 9000、建连超时 5s），再由调用方显式设置覆盖；
	// defaults.Set 仅在缺省（零值）字段上生效，失败即为 default tag 格式错误（致命不变量）。
	if err := defaults.Set(cfg); err != nil {
		panic(fmt.Sprintf("clickhouse: apply defaults: %v", err))
	}
	addr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)
	key := clickhouseKey(cfg)
	clickhouseClients.register(key, func() (*sql.DB, error) {
		opts := &clickhouse.Options{
			Addr:        []string{addr},
			Auth:        clickhouse.Auth{Database: cfg.DBName, Username: cfg.User, Password: cfg.Password},
			DialTimeout: cfg.DialTimeout,
		}
		return clickhouse.OpenDB(opts), nil
	})
	return &ClickHousePool{sqlDBPool: sqlDBPool{reg: clickhouseClients, key: key}, db: cfg.DBName}
}

// ClickHousePool 是 ClickHouse 的 sqlDBPool 句柄，绑定一个实例配置派生的 Key。
// ClickHousePool is the ClickHouse sqlDBPool handle bound to a Key derived from an
// instance config.
type ClickHousePool struct {
	sqlDBPool
	db string
}

// CloseAllClickHouseClients 关闭全部 ClickHouse db 池并释放底层连接。供进程优雅关闭调用。
// CloseAllClickHouseClients closes every ClickHouse db pool and releases connections,
// meant for graceful shutdown.
func CloseAllClickHouseClients(ctx context.Context) {
	clickhouseClients.closeAll(ctx)
}
