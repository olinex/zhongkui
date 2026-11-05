package middlewares

import (
	"github.com/gin-gonic/gin"

	"zhongkuisec.com/zhongkui/internal/pkg/redis"
)

// PasswordResetLimiterCtxName 密码重置申请限流器在 Gin 上下文中存储的键名。
// 通过此键将 *redis.FixedWindowLimiter 注入请求上下文，供忘记密码处理器按请求来源
// 维度判定是否超限。
const PasswordResetLimiterCtxName = "password-reset-limiter"

// PasswordResetLimiterMiddleware 创建密码重置申请限流中间件：将限流器注入请求上下文，
// 供处理器按请求来源维度调用其 Allow 判定是否超限。
func PasswordResetLimiterMiddleware(limiter *redis.FixedWindowLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(PasswordResetLimiterCtxName, limiter)
		c.Next()
	}
}

// GetPasswordResetLimiter 从 Gin 请求上下文中获取密码重置申请限流器。
// 上下文中不存在时触发 panic，通常意味着未注册 PasswordResetLimiterMiddleware。
func GetPasswordResetLimiter(c *gin.Context) *redis.FixedWindowLimiter {
	if limiter, ok := c.Get(PasswordResetLimiterCtxName); ok {
		return limiter.(*redis.FixedWindowLimiter)
	}
	panic("password reset limiter does not exists")
}
