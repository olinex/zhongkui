package middlewares

import (
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/pkg/s3"
)

// S3ClientCtxName 是 S3 客户端在 Gin 上下文中存储的键名。
// 通过此键将 *s3.S3Client 实例注入到请求上下文中，供后续处理函数读取。
const S3ClientCtxName = "s3"

// S3ClientMiddleware 创建一个 Gin 中间件，将 S3 客户端实例注入到请求上下文中。
// 该中间件在请求处理链的入口处调用 c.Set 将 client 实例存储到上下文中，
// 使得后续处理函数可以通过 GetS3Client 函数获取到同一 S3 客户端。
//
// 参数:
//   - client: S3 客户端实例，通常在应用启动时创建
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func S3ClientMiddleware(client *s3.S3Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(S3ClientCtxName, client)
		c.Next()
	}
}

// GetS3Client 从 Gin 请求上下文中获取 S3 客户端实例。
//
// 注意: 如果上下文中不存在 S3 客户端实例，该函数会触发 panic，
// 这通常意味着未正确注册 S3ClientMiddleware 中间件。
//
// 参数:
//   - c: Gin 请求上下文
//
// 返回:
//   - *s3.S3Client: S3 客户端实例
func GetS3Client(c *gin.Context) *s3.S3Client {
	if client, ok := c.Get(S3ClientCtxName); ok {
		return client.(*s3.S3Client)
	}
	panic("s3 client does not exists")
}
