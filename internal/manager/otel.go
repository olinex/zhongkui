package manager

import (
	"context"

	"github.com/gin-gonic/gin"
	o "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

func OtelMiddleware() gin.HandlerFunc {
	return otel.GinMiddleware(PREFIX)
}

func StartMiddlewareSpan(c *gin.Context, name string) (context.Context, trace.Span) {
	tracer := o.Tracer("gin-middleware")
	ctx, span := tracer.Start(c.Request.Context(), name)
	c.Request = c.Request.WithContext(ctx) // 让后续 handler 使用带子 span 的 ctx
	return ctx, span
}
