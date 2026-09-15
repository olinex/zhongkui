// Package vtab（本文件）提供基于 SQLite 文件的 vtable 数据源连接层。
// 前端上传的 .db 文件经 S3 加密持久化（见 internal/pkg/s3/objectcache.go）；注册时由通用
// 对象缓存 ObjectCache.Materialize 按需拉取解密为本地明文文件（<= 阈值落内存目录 /dev/shm，
// 超阈暂时落盘），再把该明文路径交给 SQLiteFileDatasource 打开。连接模型复用共享
// sqlDBPool（见 sqldb.go）：按文件路径派生 Key，懒建 *sql.DB（github.com/glebarez/go-sqlite
// 纯 Go 驱动，免 cgo；驱动名 sqlite，与项目既有 gorm sqlite 方言一致），Open 借出、取数即还。
// sqlite 源与 sqlitevtab 同为 SQLite 语义，LIKE/GLOB 可比 MySQL/PG 更安全地下推（见
// sqlite_file_table.go）。
//
// This file provides the SQLite-file vtable datasource connection layer. The .db file
// uploaded by the frontend is stored encrypted in S3 (see internal/pkg/s3/objectcache.go);
// on registration the generic ObjectCache.Materialize fetches and decrypts it on demand into
// a local plaintext file (<= threshold in a memory dir such as /dev/shm, over threshold
// spilled to disk), then hands that path to SQLiteFileDatasource to open. The connection
// model reuses the shared sqlDBPool (see sqldb.go): a Key derived from the file path lazily
// builds a *sql.DB (github.com/glebarez/go-sqlite pure-Go driver, cgo-free; driver name
// "sqlite", matching the project's existing gorm sqlite dialect), borrowed on Open and
// returned right after fetching. Because the source is itself SQLite, LIKE/GLOB can be pushed
// more safely than in MySQL/PG (see sqlite_file_table.go).
package vtab

import (
	// stdlib
	"context"
	"database/sql"
	"fmt"

	// third-party
	_ "github.com/glebarez/go-sqlite"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// SQLiteFileConfig 描述一个 SQLite 文件实例的接入配置。
// Path 是通用对象缓存 ObjectCache.Materialize 拉取解密得到的本地明文文件绝对路径（进程内缓存，
// <= 内存阈值时位于 tmpfs /dev/shm 内存目录）；注册流程负责在构造前完成 Materialize。
// SQLiteFileConfig describes how to reach one SQLite file instance. Path is the absolute
// local plaintext file path produced by the generic ObjectCache.Materialize (process-cached,
// and located in the tmpfs /dev/shm memory dir when within the memory threshold); the
// registration flow materializes it before construction.
type SQLiteFileConfig struct {
	// Path 已 Materialize 的数据库明文文件绝对路径，如 /dev/shm/…sqlite；查询限定于该文件。
	// Path the materialized plaintext database file path, e.g. /dev/shm/…sqlite; queries are
	// scoped to it.
	Path string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与连接串）。
	// Namespace the datasource namespace; only participates in pool-key derivation.
	Namespace string
}

// sqliteFileClients 是 SQLite 文件的 db 池注册表（独立 Key 空间）。
// sqliteFileClients is the SQLite-file db pool registry (its own Key space).
var sqliteFileClients = newSQLDBRegistry()

// SQLiteObjectKey 计算 sqlite 数据源在 S3 中的对象键。数据源 name 不可变，故对象键在
// 配置生命周期内稳定，创建时写入、更新时覆盖、删除时清理。对象键语义随类型而定，其它
// 数据源接入时应各自派生专属键。
// SQLiteObjectKey derives the S3 object key for a sqlite datasource. Since datasource name
// is immutable, the key is stable for the config's lifetime: written at create, overwritten
// at update, removed at delete. The key semantics are per-type; other datasources should
// derive their own keys.
func SQLiteObjectKey(namespace, name string) string {
	return fmt.Sprintf("datasource/%s/%s.sqlite", namespace, name)
}

// sqliteFileKey 由配置规范化出实例 Key（Namespace + Path）。
// sqliteFileKey derives the instance Key from config (Namespace + Path).
func sqliteFileKey(c *SQLiteFileConfig) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s", c.Namespace, c.Path))
}

// NewSQLiteFilePool 注入一个 SQLite 文件实例并返回其池句柄。db 惰性创建，构造本身不
// 触发文件/连接操作。
// NewSQLiteFilePool injects one SQLite file instance and returns its pool handle.
// Databases are created lazily, so construction opens no file or connection.
func NewSQLiteFilePool(cfg *SQLiteFileConfig) *SQLiteFilePool {
	key := sqliteFileKey(cfg)
	sqliteFileClients.register(key, func() (*sql.DB, error) {
		return sql.Open("sqlite", cfg.Path)
	})
	return &SQLiteFilePool{sqlDBPool: sqlDBPool{reg: sqliteFileClients, key: key}}
}

// SQLiteFilePool 是 SQLite 文件的 sqlDBPool 句柄，绑定一个文件路径派生的 Key。
// SQLiteFilePool is the SQLite-file sqlDBPool handle bound to a Key derived from a file
// path.
type SQLiteFilePool struct {
	sqlDBPool
}

// CloseAllSQLiteClients 关闭全部 SQLite 文件 db 池并释放底层连接。供进程优雅关闭调用。
// CloseAllSQLiteClients closes every SQLite-file db pool and releases connections, meant
// for graceful shutdown.
func CloseAllSQLiteClients(ctx context.Context) {
	sqliteFileClients.closeAll(ctx)
}
