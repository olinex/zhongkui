package provider

import (
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/tagengine"
)

func newTagEngineProvider() do.Provider[*tagengine.Engine] {
	return func(i do.Injector) (*tagengine.Engine, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			// 绑定 etcd 客户端：引擎作为 runtime.Service 可启动协程监听
			// 规则变更并对外广播变更事件，保证多实例下缓存一致。
			return tagengine.NewEngine(db, logger, client), nil
		}
	}
}

func InjectTagEngine(i do.Injector) {
	do.Provide(i, newTagEngineProvider())
}

func InvokeTagEngine(i do.Injector) (*tagengine.Engine, error) {
	return do.Invoke[*tagengine.Engine](i)
}

func MustInvokeTagEngine(i do.Injector) *tagengine.Engine {
	return do.MustInvoke[*tagengine.Engine](i)
}
