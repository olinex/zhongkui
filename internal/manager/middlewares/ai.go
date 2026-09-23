package middlewares

import (
	// third-party
	"github.com/gin-gonic/gin"

	// internal
	"zhongkui.com/zhongkui/internal/ai"
)

// AiRuntimeCtxName 是 AI 助手运行时在 Gin 上下文中存储的键名。
// AiRuntimeCtxName is the key under which the AI assistant runtime is stored in the Gin context.
const AiRuntimeCtxName = "aiRuntime"

// AiMiddleware 创建一个 Gin 中间件，将 AI 助手运行时（*ai.Runtime）注入到请求上下文
// 中，供 AI 相关处理器读取。运行时按请求实时给出当前组件快照（Current），配置变更
// （etcd 广播热重载）后无需重启即可生效；AI 未启用时组件仅携带参数解析器，处理器经
// Enabled 统一判断。
// AiMiddleware creates a Gin middleware that injects the AI assistant runtime
// (*ai.Runtime) into the request context for AI-related handlers to read. The runtime
// hands out the current components snapshot per request (Current), so a config change (an
// etcd broadcast hot reload) takes effect without a restart; when AI is disabled the
// components carry only the parameter resolver and handlers check Enabled uniformly.
func AiMiddleware(rt *ai.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AiRuntimeCtxName, rt)
		c.Next()
	}
}

// GetAiRuntime 从 Gin 请求上下文中获取 AI 助手运行时（供需要快照/档位等运行时能力的处理器使用）。
// GetAiRuntime fetches the AI assistant runtime from the Gin request context (for handlers
// needing runtime capabilities such as the snapshot or the slots).
//
// 注意: 如果上下文中不存在该实例，该函数会触发 panic，
// 这通常意味着未正确注册 AiMiddleware 中间件。
//
// Note: this function panics when the instance is absent from the context, which
// usually means the AiMiddleware is not registered properly.
func GetAiRuntime(c *gin.Context) *ai.Runtime {
	if v, ok := c.Get(AiRuntimeCtxName); ok {
		return v.(*ai.Runtime)
	}
	panic("ai runtime does not exists")
}

// GetAi 从 Gin 请求上下文中获取当前 AI 助手组件快照（等价于 GetAiRuntime(c).Current()）。
// GetAi fetches the current AI assistant components snapshot from the Gin request context
// (equivalent to GetAiRuntime(c).Current()).
//
// 注意: 如果上下文中不存在该实例，该函数会触发 panic，
// 这通常意味着未正确注册 AiMiddleware 中间件。
//
// Note: this function panics when the instance is absent from the context, which
// usually means the AiMiddleware is not registered properly.
func GetAi(c *gin.Context) *ai.Components {
	return GetAiRuntime(c).Current()
}
