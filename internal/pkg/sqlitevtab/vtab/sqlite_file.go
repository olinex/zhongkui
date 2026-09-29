// Package vtab（本文件）提供基于 SQLite 文件的 vtable 数据源连接层。
// 前端上传的 .db 文件经 S3 加密持久化（见 internal/pkg/s3/objectcache.go）；注册时由通用
// 对象缓存 ObjectCache.Materialize 按需拉取解密为本地明文文件（<= 阈值落内存目录 /dev/shm，
// 超阈暂时落盘），再把该明文路径交给 SQLiteFileDatasource 打开。连接模型复用共享
// sqlDBPool（见 sqldb.go）：按文件路径派生 Key，懒建 *sql.DB（github.com/glebarez/go-sqlite
// 纯 Go 驱动，免 cgo；驱动名 sqlite，与项目既有 gorm sqlite 方言一致），Open 借出、取数即还。
// sqlite 源与 sqlitevtab 同为 SQLite 语义，LIKE/GLOB 可比 MySQL/PG 更安全地下推（见
// sqlite_file_table.go）。

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"fmt"
	"io"
	"reflect"

	// 第三方
	_ "github.com/glebarez/go-sqlite"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// SQLiteFileConfig 描述一个 SQLite 文件实例的接入配置。
// Path 是通用对象缓存 ObjectCache.Materialize 拉取解密得到的本地明文文件绝对路径（进程内缓存，
// <= 内存阈值时位于 tmpfs /dev/shm 内存目录）；注册流程负责在构造前完成 Materialize。
type SQLiteFileConfig struct {
	// Path 已 Materialize 的数据库明文文件绝对路径，如 /dev/shm/…sqlite；查询限定于该文件。
	Path string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与连接串）。
	Namespace string
}

// sqliteFileClients 是 SQLite 文件的 db 池注册表（独立 Key 空间）。
var sqliteFileClients = newSQLDBRegistry()

// SQLiteObjectKey 计算 sqlite 数据源在 S3 中的对象键。数据源 name 不可变，故对象键在
// 配置生命周期内稳定，创建时写入、更新时覆盖、删除时清理。对象键语义随类型而定，其它
// 数据源接入时应各自派生专属键。
func SQLiteObjectKey(namespace, name string) string {
	return fmt.Sprintf("datasource/%s/%s.sqlite", namespace, name)
}

// sqliteFileKey 由配置规范化出实例 Key（Namespace + Path）。
func sqliteFileKey(c *SQLiteFileConfig) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s", c.Namespace, c.Path))
}

// NewSQLiteFilePool 注入一个 SQLite 文件实例并返回其池句柄。db 惰性创建，构造本身不
// 触发文件/连接操作。
func NewSQLiteFilePool(cfg *SQLiteFileConfig) *SQLiteFilePool {
	key := sqliteFileKey(cfg)
	sqliteFileClients.register(key, func() (*sql.DB, error) {
		return sql.Open("sqlite", cfg.Path)
	})
	return &SQLiteFilePool{sqlDBPool: sqlDBPool{reg: sqliteFileClients, key: key}}
}

// SQLiteFilePool 是 SQLite 文件的 sqlDBPool 句柄，绑定一个文件路径派生的 Key。
type SQLiteFilePool struct {
	sqlDBPool
}

// CloseAllSQLiteClients 关闭全部 SQLite 文件 db 池并释放底层连接。供进程优雅关闭调用。
func CloseAllSQLiteClients(ctx context.Context) {
	sqliteFileClients.closeAll(ctx)
}

// sqliteFilePlugin 实现 DatasourcePlugin，把 sqlite_file 数据源类型接入插件注册表。
type sqliteFilePlugin struct{}

// Key 返回数据源类型键。
func (sqliteFilePlugin) Key() string { return DatasourceTypeSQLiteFile }

// ConfigType 返回配置值反射类型。
func (sqliteFilePlugin) ConfigType() reflect.Type { return reflect.TypeFor[SQLiteFileTableConfig]() }

// NewConfig 返回全新一份 *SQLiteFileTableConfig。
func (sqliteFilePlugin) NewConfig() any { return &SQLiteFileTableConfig{} }

// ValidateConfig 结构已经解码校验，这里无需额外操作。
func (sqliteFilePlugin) ValidateConfig(_ any) error { return nil }

// TestConn sqlite 文件无网络端点，返回「不支持连接测试」错误。
func (sqliteFilePlugin) TestConn(_ context.Context, _ any) error {
	return errors.New("sqlite_file: connection test is not supported (no network endpoint)")
}

// IntrospectSchema 对前端上传的 .db 明文文件流做 PRAGMA table_info 自省；未传文件报错。
func (sqliteFilePlugin) IntrospectSchema(ctx context.Context, cfg any, file io.Reader) ([]DatasourceColumn, error) {
	c := cfg.(*SQLiteFileTableConfig)
	if file == nil {
		return nil, errors.New("sqlite_file: schema introspection requires an uploaded .db file")
	}
	return introspectSQLiteFile(ctx, c.Table, file)
}

// Register 要求 schema 非空与 S3 对象存储已启用，再 Materialize 明文文件建池构造数据源。
func (sqliteFilePlugin) Register(ctx context.Context, _ *sqlitevtab.SQLitePool, store *s3.ObjectCache, namespace, name string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*SQLiteFileTableConfig)
	if len(c.Schema) == 0 {
		return nil, errors.Newf("%s: schema must be provided; run introspection from the frontend", DatasourceTypeSQLiteFile)
	}
	if store == nil {
		return nil, errors.New("tag datasource: sqlite_file requires S3 object storage (ObjectCache) to be enabled")
	}
	// 先从 S3 拉取并解密为本地明文文件，再把该明文路径交给连接池打开；对象键由命名空间+
	// 数据源名派生，随生命周期稳定。
	objectKey := SQLiteObjectKey(namespace, name)
	path, err := store.Materialize(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	poolHandle := NewSQLiteFilePool(&SQLiteFileConfig{Path: path, Namespace: namespace})
	cols, err := parseDatasourceColumns(c.Schema)
	if err != nil {
		return nil, err
	}
	return NewSQLiteFileDatasourceWithSchema(poolHandle, c.Table, cols)
}

// init 自注册 sqlite_file 数据源类型。
func init() {
	RegisterDatasourceType(sqliteFilePlugin{})
}
