package router

import (
	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerTrafficRoutes 注册 /api/v1/traffic 原始流量审计接口路由。
// 只读查询 raw 库（ClickHouse）中的原始流量数据。
// 三种数据类型（ipv4-tcp / ipv6-tcp / http-transport）的数据结构与业务逻辑差异较大，
// 拆分为三组独立路由各自演进；认证 / RBAC / raw 库 / S3 / 命名空间等公共中间件挂在父组上继承。
// 每组仅提供：
//   - POST /list      分页查询流量全字段（payload/body 已转存 S3，单条返回字节量可控）
//
// 三种流量数据均无唯一性 ID，条件查询无法可靠定位单条记录，故不提供 detail 接口；
// 加密字段以 base64 密文、S3 body 以对象键形式返回，
// 明文查看走全局通用接口 /api/v1/plaintext/crypto 与 /api/v1/plaintext/s3（见 plaintext.go）。
func registerTrafficRoutes(ctx *RegisterContext) {
	trafficGroup := ctx.v1Group.Group(
		"/traffic",                      // 流量审计接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.rawDatabaseMiddleware,       // raw 库（ClickHouse）连接
		ctx.s3Middleware,                // S3 客户端（body 明文读取）
		ctx.namespaceMiddleware,         // 命名空间隔离
	)

	// ========== IPv4 TCP 包：/api/v1/traffic/ipv4-tcp ==========
	{
		ipv4TcpGroup := trafficGroup.Group("/ipv4-tcp")
		// 流量列表查询 - 分页查询 IPv4 TCP 包全字段
		ipv4TcpGroup.POST("/list", views.Ipv4TcpListGet)
		// 流量列表元信息
		ipv4TcpGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.Ipv4TcpListGetSearchIn, *views.Ipv4TcpListGetListItemOut]())
	}

	// ========== IPv6 TCP 包：/api/v1/traffic/ipv6-tcp ==========
	{
		ipv6TcpGroup := trafficGroup.Group("/ipv6-tcp")
		// 流量列表查询 - 分页查询 IPv6 TCP 包全字段
		ipv6TcpGroup.POST("/list", views.Ipv6TcpListGet)
		// 流量列表元信息
		ipv6TcpGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.Ipv6TcpListGetSearchIn, *views.Ipv6TcpListGetListItemOut]())
	}

	// ========== HTTP 传输：/api/v1/traffic/http-transport ==========
	{
		httpTransportGroup := trafficGroup.Group("/http-transport")
		// 流量列表查询 - 分页查询 HTTP 传输全字段
		httpTransportGroup.POST("/list", views.HttpTransportListGet)
		// 流量列表元信息
		httpTransportGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.HttpTransportListGetSearchIn, *views.HttpTransportListGetListItemOut]())
		// 关联网络堆栈列表查询 - 以 HttpTransport 的 traceId 分页查询组成该 TCP 流的网络堆栈
		httpTransportGroup.POST("/stack/list", views.HttpTransportStackListGet)
		// 关联网络堆栈列表元信息
		httpTransportGroup.OPTIONS("/stack/list", views.ListMetaGinHandler[*views.NetworkStackListGetSearchIn, *views.NetworkStackListGetListItemOut]())
	}
}
