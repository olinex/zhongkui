package middlewares

import (
	"github.com/gin-gonic/gin"

	"zhongkuisec.com/zhongkui/internal/pkg/s3"
)

// ObjectCacheCtxName 是通用对象缓存在 Gin 上下文中存储的键名。
// 通过此键将 *s3.ObjectCache 实例注入到请求上下文中，供知识库原件上传/下载/清理等
// 处理器读取。
const ObjectCacheCtxName = "objectcache"

// ObjectCacheMiddleware 创建一个 Gin 中间件，将通用对象缓存实例注入到请求上下文中。
// 该中间件在请求处理链的入口处调用 c.Set 将 cache 实例存储到上下文中，
// 使得后续处理函数可以通过 GetObjectCache 函数获取到同一通用对象缓存。
//
// 参数:
//   - cache: 通用对象缓存实例，通常在应用启动时创建
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func ObjectCacheMiddleware(cache *s3.ObjectCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ObjectCacheCtxName, cache)
		c.Next()
	}
}

// GetObjectCache 从 Gin 请求上下文中获取通用对象缓存实例。
//
// 注意: 如果上下文中不存在通用对象缓存实例，该函数会触发 panic（与 GetS3Client 语义一致），
// 这通常意味着未正确注册 ObjectCacheMiddleware 中间件。
//
// 参数:
//   - c: Gin 请求上下文
//
// 返回:
//   - *s3.ObjectCache: 通用对象缓存实例
func GetObjectCache(c *gin.Context) *s3.ObjectCache {
	if cache, ok := c.Get(ObjectCacheCtxName); ok {
		return cache.(*s3.ObjectCache)
	}
	panic("object cache does not exists")
}
