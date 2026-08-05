package provider

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func newRbacProvider() do.Provider[*casbin.Enforcer] {
	return func(i do.Injector) (*casbin.Enforcer, error) {
		if mdl, err := model.NewModelFromString(base.RbacModel); err != nil {
			return nil, errors.WithStack(err)
		} else if coreDB, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			gormadapter.TurnOffAutoMigrate(coreDB)
			ins := &core.RbacPolicy{}
			adapter, err := gormadapter.NewAdapterByDBWithCustomTable(coreDB, ins, "rbac_policies")
			if err != nil {
				return nil, errors.WithStack(err)
			}
			enforcer, err := casbin.NewEnforcer(mdl, adapter)
			// TODO: enable zap logger
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return enforcer, nil
		}
	}
}

func NewRbac() do.Provider[*casbin.Enforcer] {
	return newRbacProvider()
}

func InjectRbac(i do.Injector) {
	do.Provide(i, NewRbac())
}

func InvokeRbac(i do.Injector) (*casbin.Enforcer, error) {
	return do.Invoke[*casbin.Enforcer](i)
}

func MustInvokeRbac(i do.Injector) *casbin.Enforcer {
	return do.MustInvoke[*casbin.Enforcer](i)
}
