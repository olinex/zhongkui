// Package middlewares 提供了 Web 应用中常用的中间件和上下文工具函数，
// 用于在 Gin 框架的请求上下文中安全地存取数据库连接和 etcd 客户端等依赖。
// 本文件定义按命名空间隔离的 sqlite vtable 池持有者（vtab.Pools）的请求上下文注入。
package middlewares

import (
	"github.com/gin-gonic/gin"

	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// SQLiteVTabPoolsCtxName sqlite vtable 池持有者在 Gin 上下文中存储的键名。
// 通过此键将 *vtab.Pools 实例注入请求上下文，供标签事件第三方数据源 CRUD 处理器
// 在请求内按命名空间注册/注销 vtable（Pools.Register / Pools.Unregister）。
const SQLiteVTabPoolsCtxName = "sqlite-vtab-pools"

// SQLiteVTabPoolsMiddleware 创建一个 Gin 中间件，将 sqlite vtable 池持有者实例注入
// 请求上下文。该中间件在请求处理链中调用 c.Set 将池持有者存储到上下文中，使得后续
// 处理函数可以通过 GetSQLiteVTabPools 获取同一实例。
func SQLiteVTabPoolsMiddleware(pools *vtab.Pools) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(SQLiteVTabPoolsCtxName, pools)
		c.Next()
	}
}

// GetSQLiteVTabPools 从 Gin 请求上下文中获取 sqlite vtable 池持有者实例。
// 如果上下文中不存在该实例，则触发 panic（通常意味着未正确注册 SQLiteVTabPoolsMiddleware）。
func GetSQLiteVTabPools(c *gin.Context) *vtab.Pools {
	if pools, ok := c.Get(SQLiteVTabPoolsCtxName); ok {
		return pools.(*vtab.Pools)
	}
	panic("sqlite vtab pools does not exists")
}
