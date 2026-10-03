package admin

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// newTestRbacEnforcer 基于测试数据库创建 Casbin enforcer
// 与 provider/rbac.go 的构造方式保持一致（同一 rbac_policies 表 + 关闭自动迁移），
// 供 handler 测试中经 middlewares.GetRbacPolicy 获取使用。
func newTestRbacEnforcer(db *database.DB) *casbin.Enforcer {
	if mdl, err := model.NewModelFromString(base.RbacModel); err != nil {
		panic("failed to create rbac model: " + err.Error())
	} else {
		gormadapter.TurnOffAutoMigrate(db)
		adapter, err := gormadapter.NewAdapterByDBWithCustomTable(db, &core.RbacPolicy{}, "rbac_policies")
		if err != nil {
			panic("failed to create rbac adapter: " + err.Error())
		}
		enforcer, err := casbin.NewEnforcer(mdl, adapter)
		if err != nil {
			panic("failed to create rbac enforcer: " + err.Error())
		}
		return enforcer
	}
}
