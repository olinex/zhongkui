// Package middlewares 提供 Web 应用中常用的中间件和上下文工具函数，
// 用于在 Gin 框架的请求上下文中安全地存取数据库连接和 etcd 客户端等依赖。
package middlewares

import (
	"github.com/gin-gonic/gin"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// TagRuleEngineCtxName 是标签引擎在 Gin 上下文中存储的键名。
const TagRuleEngineCtxName = "tag-engine"

// TagRuleEngineMiddleware 创建一个 Gin 中间件，将标签引擎注入到请求上下文中，
// 供标签规则写路径处理函数读取（本地缓存失效 + 广播变更事件）。
//
// 参数:
//   - engine: 标签识别引擎，写路径后调用 Invalidate 保证本进程即时一致，
//     并通过 BroadcastChange 通知其他进程
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func TagRuleEngineMiddleware(engine *tagengine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(TagRuleEngineCtxName, engine)
		c.Next()
	}
}

// GetTagRuleEngine 从 Gin 请求上下文中获取标签引擎。
//
// 注意: 如果上下文中不存在该实例，该函数会触发 panic，
// 这通常意味着未正确注册 TagRuleEngineMiddleware 中间件。
//
// 返回:
//   - *tagengine.Engine: 标签识别引擎
func GetTagRuleEngine(c *gin.Context) *tagengine.Engine {
	if v, ok := c.Get(TagRuleEngineCtxName); ok {
		return v.(*tagengine.Engine)
	}
	panic("tag rule engine does not exists")
}
