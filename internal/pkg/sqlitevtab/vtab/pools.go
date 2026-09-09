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
	"sync"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// Pools 是按命名空间持有 SQLitePool 的进程级持有者。粒度：namespace → *SQLitePool。
// 并发安全：Pool 在读锁命中快速返回，未命中时进入写锁临界区内 double-check 建池，
// 防止并发首建泄漏重复池。
// Pools is the process-wide holder of per-namespace SQLitePools, keyed by namespace.
// It is concurrency-safe: Pool returns fast under the read lock on a hit, and on a
// miss creates the pool inside the write-lock critical section with a double-check,
// preventing leaked pools from concurrent first-creation.
type Pools struct {
	mu    sync.RWMutex
	pools map[string]*sqlitevtab.SQLitePool
}

// NewPools 创建空的 Pools 持有者。
// NewPools creates an empty Pools holder.
func NewPools() *Pools {
	return &Pools{pools: make(map[string]*sqlitevtab.SQLitePool)}
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
	return registerInto(ctx, p.Pool(ns), ns, name, typ, cfgData)
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
