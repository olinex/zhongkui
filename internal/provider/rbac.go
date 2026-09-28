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
			// TODO: 启用 zap 日志
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return enforcer, nil
		}
	}
}

// NewRbac 返回基于 casbin 的 RBAC 执行器的依赖注入 provider。
func NewRbac() do.Provider[*casbin.Enforcer] {
	return newRbacProvider()
}

// InjectRbac 将 RBAC 执行器 provider 注册到依赖注入容器。
func InjectRbac(i do.Injector) {
	do.Provide(i, NewRbac())
}

// InvokeRbac 从依赖注入容器获取 RBAC 执行器实例。
func InvokeRbac(i do.Injector) (*casbin.Enforcer, error) {
	return do.Invoke[*casbin.Enforcer](i)
}

// MustInvokeRbac 从依赖注入容器获取 RBAC 执行器实例，失败时 panic。
func MustInvokeRbac(i do.Injector) *casbin.Enforcer {
	return do.MustInvoke[*casbin.Enforcer](i)
}
