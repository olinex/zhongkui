// Package manager 提供了 Web 应用中常用的中间件和上下文工具函数，
// 用于在 Gin 框架的请求上下文中安全地存取数据库连接和 etcd 客户端等依赖。
package manager

import (
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
)

// DatabaseCtxName 是数据库实例在 Gin 上下文中存储的键名。
// 通过此键将 *database.DB 实例注入到请求上下文中，供后续处理函数读取。
const DatabaseCtxName = "db"

// EtcdCtxName 是 etcd 客户端在 Gin 上下文中存储的键名。
// 通过此键将 *etcd.EtcdClient 实例注入到请求上下文中，供后续处理函数读取。
const EtcdCtxName = "etcd"

// DatabaseMiddleware 创建一个 Gin 中间件，将数据库连接实例注入到请求上下文中。
// 该中间件在请求处理链的入口处调用 c.Set 将 db 实例存储到上下文中，
// 使得后续处理函数可以通过 GetDatabase 函数获取到同一数据库连接。
//
// 参数:
//   - db: 数据库连接实例，通常在应用启动时创建
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func DatabaseMiddleware(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(DatabaseCtxName, db)
		c.Next()
	}
}

// GetDatabase 从 Gin 请求上下文中获取数据库连接实例。
// 该函数使用当前请求的 context.Context 调用 WithContext 方法，
// 确保数据库操作与请求生命周期绑定，支持请求取消时的自动超时控制。
//
// 注意: 如果上下文中不存在数据库实例，该函数会触发 panic，
// 这通常意味着未正确注册 DatabaseMiddleware 中间件。
//
// 参数:
//   - c: Gin 请求上下文
//
// 返回:
//   - *database.DB: 绑定了当前请求上下文的数据库连接实例
func GetDatabase(c *gin.Context) *database.DB {
	if db, ok := c.Get(DatabaseCtxName); ok {
		return db.(*database.DB).WithContext(c.Request.Context())
	}
	panic("database does not exists")
}

// EtcdClientMiddleware 创建一个 Gin 中间件，将 etcd 客户端实例注入到请求上下文中。
// 该中间件在请求处理链的入口处调用 c.Set 将 client 实例存储到上下文中，
// 使得后续处理函数可以通过 GetEtcdClient 函数获取到同一 etcd 客户端。
//
// 参数:
//   - client: etcd 客户端实例，通常在应用启动时创建
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件处理函数
func EtcdClientMiddleware(client *etcd.EtcdClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(EtcdCtxName, client)
		c.Next()
	}
}

// GetEtcdClient 从 Gin 请求上下文中获取 etcd 客户端实例。
//
// 注意: 如果上下文中不存在 etcd 客户端实例，该函数会触发 panic，
// 这通常意味着未正确注册 EtcdClientMiddleware 中间件。
//
// 参数:
//   - c: Gin 请求上下文
//
// 返回:
//   - *etcd.EtcdClient: etcd 客户端实例
func GetEtcdClient(c *gin.Context) *etcd.EtcdClient {
	if e, ok := c.Get(EtcdCtxName); ok {
		return e.(*etcd.EtcdClient)
	}
	panic("etcd client does not exists")
}
