// Package middlewares 提供了 Web 应用中常用的中间件和上下文工具函数，
// 用于在 Gin 框架的请求上下文中安全地存取数据库连接和 etcd 客户端等依赖。
package middlewares

import (
	"github.com/gin-gonic/gin"

	"zhongkui.com/zhongkui/internal/pkg/message"
)

// MessagePoolCtxName 消息渠道连接池在 Gin 上下文中存储的键名。
// 通过此键将 *message.Pool 实例注入请求上下文，供通知 test-send、模板渲染预览等
// 处理器经统一渠道资源池实际投递/渲染。
const MessagePoolCtxName = "message-pool"

// MessagePoolMiddleware 创建一个 Gin 中间件，将消息渠道连接池实例注入请求上下文。
// 该中间件在请求处理链中调用 c.Set 将池实例存储到上下文中，使得后续处理函数可以
// 通过 GetMessagePool 获取同一连接池实例。
func MessagePoolMiddleware(pool *message.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(MessagePoolCtxName, pool)
		c.Next()
	}
}

// GetMessagePool 从 Gin 请求上下文中获取消息渠道连接池实例。
// 如果上下文中不存在该实例，则触发 panic（通常意味着未正确注册 MessagePoolMiddleware）。
func GetMessagePool(c *gin.Context) *message.Pool {
	if pool, ok := c.Get(MessagePoolCtxName); ok {
		return pool.(*message.Pool)
	}
	panic("message pool does not exists")
}
