package provider

import (
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func NewGin(i do.Injector) (*gin.Engine, error) {
	if base.Env.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()

	return engine, nil
}

func InjectGin(i do.Injector) {
	do.Provide(i, NewGin)
}

func InvokeGin(i do.Injector) (*gin.Engine, error) {
	return do.Invoke[*gin.Engine](i)
}

func MustInvokeGin(i do.Injector) *gin.Engine {
	return do.MustInvoke[*gin.Engine](i)
}
