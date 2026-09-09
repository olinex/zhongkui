// Package datasource 提供第三方数据源（通用能力）的启动重载与跨进程热同步：
// 启动时扫描全部启用数据源，逐个注册到各自命名空间的 SQLite vtable 池，
// 使前端动态配置在进程重启后恢复注册状态；运行期写路径（增删改/启停）经 etcd
// 广播变更事件，worker 等消费进程由 Watcher 监听并按 (Namespace, Name) 精准同步
// 本地池，实现配置热更新、无需重启。
//
// 命名空间隔离：每个命名空间独立持有自己的 SQLitePool（vtab.Pools），vtable 以裸名
// （{name}）注册到对应命名空间的池，rego 规则直接引用裸名；连接池 Key 仍含命名空间
// 维度，不同命名空间即使链接同一外部实例也不复用连接。
//
// The datasource package reloads third-party datasources (a general capability) at
// startup and hot-syncs them across processes: it scans all enabled datasources and
// registers each onto its namespace's own SQLite vtable pool, so front-end dynamic
// config is restored after a restart; during runtime the write path (create/update/
// delete/enable) broadcasts change events via etcd, and consumer processes such as
// workers watch them through a Watcher, precisely syncing their local pools by
// (Namespace, Name) for hot updates without a restart.
//
// Namespace isolation: each namespace owns its own SQLitePool (vtab.Pools) and
// vtables are registered under their bare name ({name}) on their namespace's pool,
// which rego rules reference directly; pool keys still carry the namespace dimension,
// so different namespaces never share connections even to the same external instance.
package datasource

import (
	// stdlib
	"context"

	// third-party
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// Reload 扫描全部已启用的第三方数据源并逐个注册到其命名空间的 SQLite vtable 池。
// 逐行按 (NamespaceName, Name, Type, 明文 Config) 调用 pools.Register，注册身份为
// 该命名空间池内的裸名 {name}。单个命名空间注册失败不中断后续数据源，错误聚合并返回
// （with stack），由启动路径按既有错误处理上报。
//
// 本函数只注册当前库中启用的数据源，不清理进程重启/Down 期间已不存在的陈旧 vtable
// （存量注册身份在重新注册/注销时自然收敛）；若后续需要主动 diff 清理，可在此处扩展。
// Reload scans all enabled third-party datasources and registers each onto its
// namespace's SQLite vtable pool. Each row calls pools.Register with (NamespaceName,
// Name, Type, plaintext Config); the registration identity is the bare name {name}
// within that namespace's pool. A registration failure does not stop subsequent
// datasources; errors are aggregated and returned (with stack) for the startup path
// to handle as usual.
//
// It only registers currently-enabled datasources and does not clean stale vtables
// that no longer exist after a restart/down (existing identities converge naturally
// on re-registration/unregistration); an explicit diff-based cleanup can be added
// here later.
func Reload(ctx context.Context, db *gorm.DB, pools *vtab.Pools) error {
	var rows []core.Datasource
	if err := db.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return errors.WithStack(err)
	}
	var errs []error
	for i := range rows {
		row := &rows[i]
		if err := pools.Register(ctx, row.NamespaceName, row.Name, row.Type, row.ConfigData()); err != nil {
			errs = append(errs, errors.WithStack(errors.Newf(
				"tag datasource: reload failed for namespace %q name %q (type %s): %v",
				row.NamespaceName, row.Name, row.Type, err,
			)))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// SyncOne 按命名空间 + 裸名从数据库重读单个数据源并收敛本地池：
// 记录存在且启用 → pools.Register 覆盖注册（配置解析/建连接失败返回错误）；
// 记录不存在或停用 → pools.Unregister 幂等注销。
// 供 worker 进程收到 etcd 数据源变更事件后精准同步单个数据源，
// 避免全量重载的成本，同时保证停用/删除条目即时从池中移除。
//
// SyncOne re-reads a single datasource by namespace + bare name and converges the
// local pool: an existing enabled row is registered (overwrite) via pools.Register;
// a missing or disabled row is unregistered idempotently. It is driven by the etcd
// datasource change watcher so worker processes sync a single datasource precisely,
// avoiding the cost of a full reload while keeping disabled/deleted entries removed.
func SyncOne(ctx context.Context, db *gorm.DB, pools *vtab.Pools, namespace, name string) error {
	var row core.Datasource
	if err := db.WithContext(ctx).Where("namespace_name = ? AND name = ?", namespace, name).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pools.Unregister(ctx, namespace, name)
		}
		return errors.WithStack(err)
	}
	if !row.Enabled {
		return pools.Unregister(ctx, namespace, name)
	}
	return pools.Register(ctx, row.NamespaceName, row.Name, row.Type, row.ConfigData())
}
