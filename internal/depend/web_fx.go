package depend

import (
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func NewGin() *gin.Engine {
	if base.DEBUG_ENV_VAR {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	return gin.New()
}
