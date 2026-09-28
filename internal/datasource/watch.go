package datasource

import (
	// 标准库
	"context"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// Watcher 数据源变更监听器，实现 runtime.Service 生命周期。
// 监听 etcd 数据源变更键，收到事件后按 (Namespace, Name) 从 DB 重读并精准同步
// 本地池（SyncOne），使 worker 等消费进程在数据源增删改/启停后无需重启即可查询
// 最新 vtable；事件负载无法解析时回退全量重载保证不遗漏。
// 底层复用 etcd 通用 Watcher（断线自动重连、ctx 取消退出、单事件回调失败仅告警），
// 未绑定 etcd 客户端时直接返回（无监听能力）。
// 启动前的存量数据源由 datasource.Reload 在进程启动路径加载。
type Watcher struct {
	db          *database.DB
	pools       *vtab.Pools
	logger      *logging.Logger
	etcdWatcher *etcd.Watcher
}

// NewWatcher 创建数据源变更监听器。
// 依赖注入方向为 provider → datasource.Watcher：db 用于重读数据源配置，
// pools 用于收敛 vtable，client 用于监听 etcd 变更键（可为 nil，此时无监听能力），
// logger 记录同步结果；变更处理逻辑由 datasourceChangeHandler 承担。
func NewWatcher(db *database.DB, pools *vtab.Pools, client *etcd.EtcdClient, logger *logging.Logger) *Watcher {
	return &Watcher{
		db:     db,
		pools:  pools,
		logger: logger,
		etcdWatcher: etcd.NewWatcher(client, datasourceChangeKey, &datasourceChangeHandler{
			db:     db,
			pools:  pools,
			logger: logger,
		}, logger),
	}
}

// Start 阻塞运行数据源变更监听，直到 ctx 取消。实现 runtime.Service 生命周期。
// 未绑定 etcd 客户端（etcdWatcher 缺失）时直接返回（无监听能力）。
func (w *Watcher) Start(ctx context.Context) error {
	if w.etcdWatcher == nil {
		w.logger.Ctx(ctx).Info("datasource change watch skipped: no etcd client")
		return nil
	}
	return w.etcdWatcher.Start(ctx)
}

// Shutdown 停止数据源变更监听。
// 由 runtime 编排器在 Start 返回后调用，实际停止依赖 Start 的 ctx 取消。
func (w *Watcher) Shutdown(ctx context.Context) error {
	if w.etcdWatcher == nil {
		return nil
	}
	return w.etcdWatcher.Shutdown(ctx)
}

// datasourceChangeHandler 数据源变更事件处理器，实现 etcd.ChangeHandler。
// 解析负载后按 (Namespace, Name) 精准同步本地池；负载无法解析时回退全量重载。
type datasourceChangeHandler struct {
	db     *database.DB
	pools  *vtab.Pools
	logger *logging.Logger
}

// HandleChange 处理单个数据源变更事件负载。
// 解析失败时回退全量重载并上抛错误（由通用层记录告警，不中断后续事件）；
// 单个数据源同步失败仅记录告警，返回 nil。
func (h *datasourceChangeHandler) HandleChange(ctx context.Context, value []byte) error {
	event, err := parseDatasourceChangeEvent(value)
	if err != nil {
		h.logger.Ctx(ctx).Warnf("datasource change event unparsable, fallback to full reload: %v", err)
		return Reload(ctx, h.db, h.pools)
	}
	if err := SyncOne(ctx, h.db, h.pools, event.Namespace, event.Name); err != nil {
		h.logger.Ctx(ctx).Warnf("datasource sync failed for %s/%s: %v", event.Namespace, event.Name, err)
	}
	return nil
}
