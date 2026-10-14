// Package vtab（本文件）提供基于 ClickHouse 的 vtable 数据源连接层。
// 每个业务表经 ClickHouseDatasource 在构造期 DESCRIBE 自省表结构后注册为一张独立
// vtable（详见 clickhouse_table.go）。连接模型复用共享的 sqlDBPool（详见 sqldb.go）：
// 按实例配置派生 Key，懒建 *sql.DB（clickhouse-go/v2 原生协议），Open 借出、取数即还。
// 数据源标识符用双引号包裹（复用 quotePgIdent），查询占位符为 ?。
//
// 超集契约与下推取舍见 clickhouse_table.go：ClickHouse 的 LIKE 大小写敏感（与 sqlite
// 不敏感不一致）而跳过下推，仅非 Any 列的比较/等值谓词安全下推。

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"fmt"
	"io"
	"reflect"
	"time"

	// 第三方
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/creasty/defaults"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/resourcepool"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// ClickHouseConfig 描述一个 ClickHouse 实例的接入配置。
type ClickHouseConfig struct {
	// Addr 主机地址（不含端口），如 127.0.0.1。
	Addr string
	// Port 端口，默认 9000（原生协议）。
	Port int `default:"9000"`
	// User 用户名。
	User string
	// Password 密码。
	Password string
	// DBName 默认数据库名；本实例下的 vtable 查询均限定于该库。
	DBName string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与 DSN）。
	Namespace string
	// DialTimeout 建连超时，默认 5s。
	DialTimeout time.Duration `default:"5s"`
}

// clickhouseClients 是 ClickHouse 的 db 池注册表（独立 Key 空间）。
var clickhouseClients = newSQLDBRegistry()

// clickhouseKey 由配置规范化出实例 Key（Namespace + Addr + User + DBName）。
func clickhouseKey(c *ClickHouseConfig) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s:%s/%s", c.Namespace, c.Addr, c.User, c.DBName))
}

// NewClickHousePool 注入一份 ClickHouse 实例配置并返回其池句柄。db 惰性创建，
// 构造本身不触发网络连接。（address 为空时跳过 addr 字段，交由驱动默认。）
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
type ClickHousePool struct {
	sqlDBPool
	// db 实例默认数据库名。
	db string
}

// CloseAllClickHouseClients 关闭全部 ClickHouse db 池并释放底层连接。供进程优雅关闭调用。
func CloseAllClickHouseClients(ctx context.Context) {
	clickhouseClients.closeAll(ctx)
}

// clickhousePlugin 实现 DatasourcePlugin，把 clickhouse 数据源类型接入插件注册表。
type clickhousePlugin struct{}

// Key 返回数据源类型键。
func (clickhousePlugin) Key() string { return DatasourceTypeClickHouse }

// ConfigType 返回配置值反射类型。
func (clickhousePlugin) ConfigType() reflect.Type { return reflect.TypeFor[ClickHouseTableConfig]() }

// NewConfig 返回全新一份 *ClickHouseTableConfig。
func (clickhousePlugin) NewConfig() any { return &ClickHouseTableConfig{} }

// ValidateConfig 结构已经解码校验，这里无需额外操作。
func (clickhousePlugin) ValidateConfig(_ any) error { return nil }

// TestConn 建池借出 connection、PingContext 探活后归还。
func (clickhousePlugin) TestConn(ctx context.Context, cfg any) error {
	c := cfg.(*ClickHouseTableConfig)
	pool := NewClickHousePool(&ClickHouseConfig{Addr: c.Addr, Port: c.Port, User: c.User, Password: c.Password, DBName: c.DBName})
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

// IntrospectSchema 建池、NewClickHouseDatasource 自省表结构后返回列定义。
func (clickhousePlugin) IntrospectSchema(ctx context.Context, cfg any, _ io.Reader) ([]DatasourceColumn, error) {
	c := cfg.(*ClickHouseTableConfig)
	pool := NewClickHousePool(&ClickHouseConfig{Addr: c.Addr, Port: c.Port, User: c.User, Password: c.Password, DBName: c.DBName})
	ds, err := NewClickHouseDatasource(ctx, pool, c.Table)
	if err != nil {
		return nil, err
	}
	schema, err := schemaOf(ds)
	if err != nil {
		return nil, err
	}
	return schemaColumnsOut(schema), nil
}

// Register 校验 schema 非空 → 建池 → parseDatasourceColumns → NewClickHouseDatasourceWithSchema。
func (clickhousePlugin) Register(ctx context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, namespace, _ string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*ClickHouseTableConfig)
	if len(c.Schema) == 0 {
		return nil, errors.Newf("%s: schema must be provided; run introspection from the frontend", DatasourceTypeClickHouse)
	}
	poolHandle := NewClickHousePool(&ClickHouseConfig{
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
	return NewClickHouseDatasourceWithSchema(poolHandle, c.Table, cols)
}

// init 自注册 clickhouse 数据源类型。
func init() {
	RegisterDatasourceType(clickhousePlugin{})
}
