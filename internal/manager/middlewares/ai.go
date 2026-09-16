package middlewares

import (
	// third-party
	"github.com/gin-gonic/gin"

	// internal
	"zhongkui.com/zhongkui/internal/ai"
)

// AiCtxName 是 AI 助手组件在 Gin 上下文中存储的键名。
// AiCtxName is the key under which the AI assistant components are stored in the Gin context.
const AiCtxName = "ai"

// AiMiddleware 创建一个 Gin 中间件，将 AI 助手组件（*ai.Components）注入到请求上下文中，
// 供 AI 相关处理器读取。AI 未启用时组件仅携带 Config，处理器经 Enabled 统一判断。
// AiMiddleware creates a Gin middleware that injects the AI assistant components
// (*ai.Components) into the request context for AI-related handlers to read. When AI
// is disabled the components carry only Config; handlers check Enabled uniformly.
func AiMiddleware(comps *ai.Components) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AiCtxName, comps)
		c.Next()
	}
}

// GetAi 从 Gin 请求上下文中获取 AI 助手组件。
// GetAi fetches the AI assistant components from the Gin request context.
//
// 注意: 如果上下文中不存在该实例，该函数会触发 panic，
// 这通常意味着未正确注册 AiMiddleware 中间件。
//
// Note: this function panics when the instance is absent from the context, which
// usually means the AiMiddleware is not registered properly.
func GetAi(c *gin.Context) *ai.Components {
	if v, ok := c.Get(AiCtxName); ok {
		return v.(*ai.Components)
	}
	panic("ai components does not exists")
}
