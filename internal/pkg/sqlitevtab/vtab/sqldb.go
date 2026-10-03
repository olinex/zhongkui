// Package vtab（本文件）提供基于 database/sql *sql.DB 的数据源（ClickHouse、SQLite
// 文件）共享的连接池基座。
// 与 MySQL/PostgreSQL 各自维护一份连接池注册表不同，*sql.DB 类的驱动数据源连模型
// 完全一致：配置→懒建 *sql.DB（sql.Open/sql.OpenDB）→借还。本文将这份样板收敛为一个
// 共享 sqlDBPool，避免 ClickHouse 与 SQLite 文件重复 MySQL/PG 的池注册表代码。
// resourcepool 的 Scope 注释虽建议库自托管的 *sql.DB 不额外套池，但本项目 mysql/pg 已
// 统一采用该模式（池化的是 *sql.DB 实例而非物理连接，按 Key 隔离多个实例配置），
// 此处保持一致（详见 postgres.go 的说明）。
//
// 超集契约、借还的取数即还语义与 MySQL/PostgreSQL 相同；各数据源用独立的
// sqlDBRegistry 实例维护自己的 Key 空间与 CloseAll。

package vtab

import (
	// 标准库
	"context"
	"database/sql"
	"sync"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/resourcepool"
)

// sqlDBConfig 描述一个 Key 对应的 *sql.DB 惰性构造器。
type sqlDBConfig struct {
	// open 懒建 *sql.DB（对 sql.Open 而言不触发网络连接；sql.OpenDB 直接包驱动连接器）。
	open func() (*sql.DB, error)
}

// sqlDBRegistry 按 Key 维护 *sql.DB 池，供单个 *sql.DB 类数据源专用。
type sqlDBRegistry struct {
	mu       sync.RWMutex
	manager  *resourcepool.Manager[*sql.DB]
	cfgs     map[resourcepool.Key]*sqlDBConfig
	override map[resourcepool.Key]*sql.DB // 仅供测试注入 mock db
}

// newSQLDBRegistry 创建注册表并装配进程级 Manager。
func newSQLDBRegistry() *sqlDBRegistry {
	r := &sqlDBRegistry{
		cfgs:     make(map[resourcepool.Key]*sqlDBConfig),
		override: make(map[resourcepool.Key]*sql.DB),
	}
	r.manager = resourcepool.NewManager(r.makeDBFor, resourcepool.NewDefaultConfig())
	return r
}

// makeDBFor 依据 Key 惰性构造 *sql.DB（resourcepool.Make）；测试经 override 注入 mock
// db 时直接返回之。
func (r *sqlDBRegistry) makeDBFor(key resourcepool.Key) (*sql.DB, error) {
	r.mu.RLock()
	if db, ok := r.override[key]; ok {
		r.mu.RUnlock()
		return db, nil
	}
	cfg := r.cfgs[key]
	r.mu.RUnlock()
	if cfg == nil {
		return nil, errors.Newf("sqldb: config not registered for key %q", key)
	}
	return cfg.open()
}

// register 注入一个 Key 的懒构造器（重复注入覆盖）。
func (r *sqlDBRegistry) register(key resourcepool.Key, open func() (*sql.DB, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfgs[key] = &sqlDBConfig{open: open}
}

// overrideDB 设置/清除指定 Key 的 mock db 注入（仅供测试）。
func (r *sqlDBRegistry) overrideDB(key resourcepool.Key, db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if db == nil {
		delete(r.override, key)
	} else {
		r.override[key] = db
	}
}

// sqlDBPool 是基于 sqlDBRegistry 的池句柄，绑定一个实例配置派生的 Key。
type sqlDBPool struct {
	reg *sqlDBRegistry
	key resourcepool.Key
}

// Acquire 借出并返回一个可用的 *sql.DB（懒建连）。
func (p *sqlDBPool) Acquire(ctx context.Context) (*sql.DB, error) {
	return p.reg.manager.Acquire(ctx, p.key)
}

// Release 归还 *sql.DB 以便复用。
func (p *sqlDBPool) Release(ctx context.Context, db *sql.DB) {
	p.reg.manager.Release(ctx, p.key, db)
}

// Close 销毁该实例池内的闲置 db（借出中的不受影响）。
func (p *sqlDBPool) Close(ctx context.Context) {
	p.reg.manager.Clear(ctx, p.key)
}

// closeAll 关闭该注册表全部池。由各数据源封装为 CloseAllXxxClients。
func (r *sqlDBRegistry) closeAll(ctx context.Context) {
	r.manager.CloseAll(ctx)
}
