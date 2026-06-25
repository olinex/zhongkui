package manager

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func OtelMiddleware() gin.HandlerFunc {
	return otelgin.Middleware(PREFIX)
}
