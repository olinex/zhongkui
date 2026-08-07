package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/tagengine"
)

func newTagEngineProvider() do.Provider[*tagengine.Engine] {
	return func(i do.Injector) (*tagengine.Engine, error) {
		db, err := InvokeCoreDatabase(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		logger, err := InvokeDatabaseLogger(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return tagengine.NewEngine(db, logger), nil
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
