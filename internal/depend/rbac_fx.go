package depend

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type RbacPolicy = casbin.Enforcer

type RbacFxIn struct {
	fx.In
	CoreDatabaseFxIn
}

func NewRbac[M gorm.Model](in RbacFxIn) (*RbacPolicy, error) {
	mdl, err := model.NewModelFromString(base.RBAC_MODEL)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	gormadapter.TurnOffAutoMigrate(in.CoreDB)
	adapter, err := gormadapter.NewAdapterByDBWithCustomTable(in.CoreDB, &M{}, "rbac_policies")
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
