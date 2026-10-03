package provider

import (
	"github.com/samber/do/v2"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

func newTagEngineProvider() do.Provider[*tagengine.Engine] {
	return func(i do.Injector) (*tagengine.Engine, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else if pool, err := InvokeSQLiteVTabPools(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			// 注入按命名空间池：tagengine 作为消费方自行解析其依赖，
			// 而非由 sqlitevtab 的 provider 反向注入，保持依赖方向为
			// provider → tagengine → sqlitevtab。引擎求值时按命名空间
			// 绑定对应池到求值 ctx，供 sqlite.query 查询裸名 vtable。
			// 绑定 etcd 客户端：引擎作为 runtime.Service 可启动协程监听
			// 规则变更并对外广播变更事件，保证多实例下缓存一致。
			return tagengine.NewEngine(db, logger, client, tagengine.WithDatasourcePools(pool)), nil
		}
	}
}

// InjectTagEngine 向依赖注入器注册标签识别引擎 provider。
func InjectTagEngine(i do.Injector) {
	do.Provide(i, newTagEngineProvider())
}

// InvokeTagEngine 从依赖注入器解析标签识别引擎，错误交由调用方处理。
func InvokeTagEngine(i do.Injector) (*tagengine.Engine, error) {
	return do.Invoke[*tagengine.Engine](i)
}

// MustInvokeTagEngine 从依赖注入器解析标签识别引擎，失败时 panic。
func MustInvokeTagEngine(i do.Injector) *tagengine.Engine {
	return do.MustInvoke[*tagengine.Engine](i)
}
