package provider

import (
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// NewGin 创建 Gin 引擎，并按环境配置调试或发布模式。
func NewGin(i do.Injector) (*gin.Engine, error) {
	if base.Env.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()

	return engine, nil
}

// InjectGin 向依赖注入器注册 Gin 引擎 provider。
func InjectGin(i do.Injector) {
	do.Provide(i, NewGin)
}

// InvokeGin 从依赖注入器解析 Gin 引擎，错误交由调用方处理。
func InvokeGin(i do.Injector) (*gin.Engine, error) {
	return do.Invoke[*gin.Engine](i)
}

// MustInvokeGin 从依赖注入器解析 Gin 引擎，失败时 panic。
func MustInvokeGin(i do.Injector) *gin.Engine {
	return do.MustInvoke[*gin.Engine](i)
}
