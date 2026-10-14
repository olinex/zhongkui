// Package middlewares 提供系统管理模块的核心功能，包括认证、授权、
// 中间件、配置管理、数据库操作和任务队列管道等。
package middlewares

import (
	"github.com/gin-gonic/gin"

	"zhongkuisec.com/zhongkui/internal/manager/features"
)

// FeatureRegistryCtxName 功能开关注册表在 Gin 上下文中存储的键名。
// 通过此键将 *features.Registry 注入请求上下文，供公共接口（如功能开关列表、
// 密码重置申请）统一判断各项功能是否可用。
const FeatureRegistryCtxName = "feature-registry"

// FeatureRegistryMiddleware 创建一个 Gin 中间件，将功能开关注册表注入请求上下文。
// 该中间件将传入的注册表实例存储在 Gin 上下文中，后续处理器可通过
// GetFeatureRegistry 获取，用于查询功能开关（如判断密码重置功能是否可用）。
func FeatureRegistryMiddleware(registry *features.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(FeatureRegistryCtxName, registry)
		c.Next()
	}
}

// GetFeatureRegistry 从 Gin 请求上下文中获取功能开关注册表实例。
func GetFeatureRegistry(c *gin.Context) *features.Registry {
	if registry, ok := c.Get(FeatureRegistryCtxName); ok {
		return registry.(*features.Registry)
	}
	panic("feature registry does not exists")
}
