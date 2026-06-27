package manager

import (
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

func OtelMiddleware() gin.HandlerFunc {
	return otel.GinMiddleware(PREFIX)
}
