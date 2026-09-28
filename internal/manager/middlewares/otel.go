package middlewares

import (
	"context"

	"github.com/gin-gonic/gin"
	o "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"zhongkui.com/zhongkui/internal/manager/config"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

// OtelMiddleware 返回为 API 管理层接入 OpenTelemetry 链路追踪的 Gin 中间件。
func OtelMiddleware() gin.HandlerFunc {
	return otel.GinMiddleware(config.Prefix)
}

func startSpan(c *gin.Context, scope, name string) (context.Context, trace.Span) {
	tracer := o.Tracer(scope)
	ctx, span := tracer.Start(c, name)
	c.Request = c.Request.WithContext(ctx) // 让后续 handler 使用带子 span 的 ctx
	return ctx, span
}

// StartMiddlewareSpan 在当前请求上下文中创建一个 gin-middleware 作用域的子 span。
func StartMiddlewareSpan(c *gin.Context, name string) (context.Context, trace.Span) {
	return startSpan(c, "gin-middleware", name)
}

// StartHandlerSpan 在当前请求上下文中创建一个 gin-handler 作用域的子 span。
func StartHandlerSpan(c *gin.Context, name string) (context.Context, trace.Span) {
	return startSpan(c, "gin-handler", name)
}
