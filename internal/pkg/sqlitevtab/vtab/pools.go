// Package vtab（本文件）提供按命名空间隔离的 SQLite vtable 池持有者 Pools。
// 每个命名空间独立持有一个 *sqlitevtab.SQLitePool，vtable 以裸名注册，消除 rego
// 规则中冗余的 {namespace}__ 前缀，同时保证不同命名空间的连接与凭据互不串扰。
// Pools 自身不持有数据库引用：扫描启用数据源由外部的 datasource.Reload 驱动，
// 避免与模型层产生依赖倒置。
//
// This file provides the namespace-isolated SQLite vtable pool holder Pools. Each
// namespace owns an independent *sqlitevtab.SQLitePool and vtables are registered
// under their bare name, removing the redundant {namespace}__ prefix in rego rules
// while keeping connections and credentials isolated across namespaces. Pools holds
// no database reference: scanning enabled datasources is driven by the external
// datasource.Reload, avoiding an inverted dependency on the model layer.
package vtab

import (
	// stdlib
	"context"
	"io"
	"sync"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// ErrSQLiteStoreNotInjected 表示 sqlite_file 数据源所需的 S3 对象缓存未注入（S3 未启用）。
// ErrSQLiteStoreNotInjected reports that the S3 object cache required by sqlite_file
// datasources is not injected (S3 disabled).
var ErrSQLiteStoreNotInjected = errors.New("sqlite_file: S3 object cache not injected")

// Pools 是按命名空间持有 SQLitePool 的进程级持有者。粒度：namespace → *SQLitePool。
// 并发安全：Pool 在读锁命中快速返回，未命中时进入写锁临界区内 double-check 建池，
// 防止并发首建泄漏重复池。另持有 objectStore（可为 nil）：sqlite_file 数据源注册时的
// S3 对象拉取/解密与删除时的对象清理经它完成。
// Pools is the process-wide holder of per-namespace SQLitePools, keyed by namespace.
// It is concurrency-safe: Pool returns fast under the read lock on a hit, and on a
// miss creates the pool inside the write-lock critical section with a double-check,
// preventing leaked pools from concurrent first-creation. It also holds objectStore
// (may be nil): sqlite_file datasources use it to fetch/decrypt S3 objects on register
// and to remove objects on delete.
type Pools struct {
	mu          sync.RWMutex
	pools       map[string]*sqlitevtab.SQLitePool
	objectStore *s3.ObjectCache
}

// NewPools 创建 Pools 持有者，并在构造期注入通用的 S3 对象缓存 store（可为 nil，表示
// S3 未启用；此时 sqlite_file 数据源注册将报「需要启用 S3」）。
// NewPools creates a Pools holder, injecting the generic S3 object cache store at
// construction (may be nil, meaning S3 is disabled; sqlite_file datasource registration then
// fails with "requires S3").
func NewPools(store *s3.ObjectCache) *Pools {
	return &Pools{pools: make(map[string]*sqlitevtab.SQLitePool), objectStore: store}
}

// store 返回持有的通用 S3 对象缓存（私有访问器；对外的读写均经 StoreSQLiteObject/
// RemoveSQLiteObject 方法暴露，不直接泄漏对象存储）。
// store returns the held generic S3 object cache (a private accessor; reads/writes are
// exposed only through StoreSQLiteObject/RemoveSQLiteObject, never leaking the raw store).
func (p *Pools) store() *s3.ObjectCache {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.objectStore
}

// StoreSQLiteObject 把命名空间+数据源名对应 S3 对象的明文读取流流式加密上传并失效旧缓存。
// sqlite_file 数据源创建/更新时调用；store 未注入或 S3 未配置时返回 ErrSQLiteStoreNotInjected。
// StoreSQLiteObject streams the plaintext reader of the namespace+name datasource's S3 object
// and encrypts it into S3, invalidating the old local cache. Called on sqlite_file
// create/update; returns ErrSQLiteStoreNotInjected when the store is unset or S3 is disabled.
func (p *Pools) StoreSQLiteObject(ctx context.Context, ns, name string, r io.Reader) error {
	s := p.store()
	if s == nil {
		return ErrSQLiteStoreNotInjected
	}
	return s.Store(ctx, SQLiteObjectKey(ns, name), r)
}

// RemoveSQLiteObject 删除指定命名空间+数据源名对应 S3 对象的加密副本并失效本地缓存。
// 数据源删除/注销后调用，实现对象与配置共存亡。store 未注入或 S3 未配置时返回
// ErrSQLiteStoreNotInjected。
// RemoveSQLiteObject deletes the encrypted S3 object for a namespace+name datasource and
// invalidates its local cache. Called after datasource deletion/unregister so the object
// shares the config's lifecycle. Returns ErrSQLiteStoreNotInjected when the store is unset
// or S3 is not configured.
func (p *Pools) RemoveSQLiteObject(ctx context.Context, ns, name string) error {
	s := p.store()
	if s == nil {
		return ErrSQLiteStoreNotInjected
	}
	return s.Remove(ctx, SQLiteObjectKey(ns, name))
}

// Pool 返回指定命名空间的 SQLitePool，不存在则惰性创建（写锁内 double-check）。
// 返回的 *SQLitePool 已就绪可注册/查询；命名空间应为布尔非空（空命名空间由调用方
// （engine/视图层）归一化）。
// Pool returns the SQLitePool of the given namespace, lazily creating it on first
// use (double-checked under the write lock). The returned pool is ready for
// registration/query; the namespace should be non-empty (callers such as the engine
// or view layer normalize empty namespaces).
func (p *Pools) Pool(ns string) *sqlitevtab.SQLitePool {
	p.mu.RLock()
	pool, ok := p.pools[ns]
	p.mu.RUnlock()
	if ok {
		return pool
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if pool, ok = p.pools[ns]; ok {
		return pool
	}
	pool = sqlitevtab.NewSQLitePool()
	p.pools[ns] = pool
	return pool
}

// Register 在指定命名空间的池上以裸名注册一张 vtable。配置解析/建连接池逻辑见
// registerInto；注册成功后该命名空间的规则即可经 sqlite.query 以裸 name 查询。
// Register registers a vtable under its bare name on the pool of the given namespace.
// Config parsing and pool construction live in registerInto; once registered, rules
// in that namespace can query it by the bare name via sqlite.query.
func (p *Pools) Register(ctx context.Context, ns, name, typ string, cfgData []byte) error {
	return registerInto(
		ctx, 
		p.Pool(ns), 
		p.store(), 
		ns, name, typ, cfgData)
}

// Unregister 从指定命名空间的池上注销一张 vtable（裸名）。命名空间池不存在或其下
// 该数据源未注册时幂等忽略。
// Unregister removes a vtable (bare name) from the given namespace's pool. Missing
// namespace pools or already-removed datasources are ignored idempotently.
func (p *Pools) Unregister(ctx context.Context, ns, name string) error {
	p.mu.RLock()
	pool, ok := p.pools[ns]
	p.mu.RUnlock()
	if !ok {
		return nil
	}
	err := pool.UnregisterDatasource(ctx, name)
	if isNotRegistered(err) {
		return nil
	}
	return err
}

// Close 关闭全部命名空间池（幂等）。实现对 Pools 的所有权清理。
// Close closes all namespace pools (idempotent), releasing their ownership.
func (p *Pools) Close(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ns, pool := range p.pools {
		pool.Close(ctx)
		delete(p.pools, ns)
	}
}

// Shutdown 实现 do.ShutdownerWithContextAndError，供 samber/do 在优雅关闭阶段驱动
// Pools 及其名下全部 SQLitePool 关闭。
// Shutdown implements do.ShutdownerWithContextAndError so samber/do drives the
// shutdown of Pools and all its SQLitePools during graceful shutdown.
func (p *Pools) Shutdown(ctx context.Context) error {
	p.Close(ctx)
	return nil
}

// isNotRegistered 判断错误是否表示数据源未注册（用于 Unregister 幂等忽略）。
// isNotRegistered reports whether the error indicates an unregistered datasource
// (used to ignore idempotently in Unregister).
func isNotRegistered(err error) bool {
	return errors.Is(err, sqlitevtab.ErrDatasourceNotRegistered)
}
