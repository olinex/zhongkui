package provider

import (
	// third-party
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/datasource"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// newDatasourceWatcherProvider 创建数据源变更监听器（datasource.Watcher）provider。
// 依赖方向为 provider → datasource.Watcher →（database / sqlitevtab / etcd / logging）：
// db 用于重读数据源配置，pools 用于收敛 vtable，client 用于监听 etcd 变更键，
// logger 记录同步结果；作为 runtime.Service 由消费方（worker 进程）编排启动。
func newDatasourceWatcherProvider() do.Provider[*datasource.Watcher] {
	return func(i do.Injector) (*datasource.Watcher, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else if pools, err := InvokeSQLiteVTabPools(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return datasource.NewWatcher(db, pools, client, logger), nil
		}
	}
}

// InjectDatasourceWatcher 注册数据源变更监听器 provider。
// InjectDatasourceWatcher registers the datasource change watcher provider.
func InjectDatasourceWatcher(i do.Injector) {
	do.Provide(i, newDatasourceWatcherProvider())
}

// InvokeDatasourceWatcher 取得数据源变更监听器。
// InvokeDatasourceWatcher resolves the datasource change watcher.
func InvokeDatasourceWatcher(i do.Injector) (*datasource.Watcher, error) {
	return do.Invoke[*datasource.Watcher](i)
}

// MustInvokeDatasourceWatcher panic 版本取得数据源变更监听器。
// MustInvokeDatasourceWatcher is the panicking variant to resolve the datasource
// change watcher.
func MustInvokeDatasourceWatcher(i do.Injector) *datasource.Watcher {
	return do.MustInvoke[*datasource.Watcher](i)
}
