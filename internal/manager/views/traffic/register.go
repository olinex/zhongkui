// Package traffic 提供流量审计域（IPv4-TCP / IPv6-TCP / HTTP 传输 / 网络栈）的 API 处理器与
// 视图 In/Out 结构体。
//
// RegisterRoutes 注册 /api/v1/traffic 原始流量审计接口路由。
// 只读查询 raw 库（ClickHouse）中的原始流量数据。
// 三种数据类型（ipv4-tcp / ipv6-tcp / http-transport）的数据结构与业务逻辑差异较大，
// 拆分为三组独立路由各自演进；认证 / RBAC / raw 库 / S3 / 命名空间等公共中间件挂在父组上继承。
// 每组仅提供：
//   - POST /list      分页查询流量全字段（payload/body 已转存 S3，单条返回字节量可控）
//   - POST /detail    按 namespace + id 查询单条全字段记录（行详情/收藏样例/堆栈下钻用）
//
// 三种流量数据均带行唯一 ID（uuid），列表投影后仅返回可见列 + id，
// 行详情、收藏样例、堆栈下钻等消费方经 detail 接口按 namespace + id 补齐全量字段；
// 加密字段以 base64 密文、S3 body 以对象键形式返回，
// 明文查看走全局通用接口 /api/v1/plaintext/crypto 与 /api/v1/plaintext/s3（见 plaintext.go）。
package traffic

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/traffic 原始流量审计接口路由到共享注册上下文。
func RegisterRoutes(ctx *views.RegisterContext) {
	trafficGroup := ctx.V1Group.Group(
		"/traffic",            // 流量审计接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.RawDatabase,       // raw 库（ClickHouse）连接
	)

	// ========== IPv4 TCP 包：/api/v1/traffic/ipv4-tcp ==========
	{
		ipv4TcpGroup := trafficGroup.Group("/ipv4-tcp")
		// 流量列表查询 - 分页查询 IPv4 TCP 包全字段
		ipv4TcpGroup.POST("/list", Ipv4TcpListGet)
		// 流量列表元信息
		views.RegisterListApi[*views.Ipv4TcpListGetSearchIn, *Ipv4TcpListGetListItemOut](ipv4TcpGroup, "/list", "流量列表查询 - 分页查询 IPv4 TCP 包全字段",
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.ipv4.list",
				Domain:    "traffic",
				Intent:    "查询 IPv4 TCP 流量列表",
				Entities:  []string{"packet_ipv4_tcps"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 行详情查询 - 按 namespace + id 返回单条全字段记录
		ipv4TcpGroup.POST("/detail", Ipv4TcpDetailGet)
		// 行详情元信息
		views.RegisterApi(ipv4TcpGroup, "/detail", "行详情查询 - 按 namespace + id 返回单条全字段记录",
			views.WithMethod(http.MethodPost, views.WithBodyInput[Ipv4TcpDetailGetIn](), views.WithBodyOutput[Ipv4TcpListGetListItemOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.ipv4.detail",
				Domain:    "traffic",
				Intent:    "查看单条 IPv4 TCP 流量的全字段详情",
				Entities:  []string{"packet_ipv4_tcps"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
	}

	// ========== IPv6 TCP 包：/api/v1/traffic/ipv6-tcp ==========
	{
		ipv6TcpGroup := trafficGroup.Group("/ipv6-tcp")
		// 流量列表查询 - 分页查询 IPv6 TCP 包全字段
		ipv6TcpGroup.POST("/list", Ipv6TcpListGet)
		// 流量列表元信息
		views.RegisterListApi[*views.Ipv6TcpListGetSearchIn, *Ipv6TcpListGetListItemOut](ipv6TcpGroup, "/list", "流量列表查询 - 分页查询 IPv6 TCP 包全字段",
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.ipv6.list",
				Domain:    "traffic",
				Intent:    "查询 IPv6 TCP 流量列表",
				Entities:  []string{"packet_ipv6_tcps"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 行详情查询 - 按 namespace + id 返回单条全字段记录
		ipv6TcpGroup.POST("/detail", Ipv6TcpDetailGet)
		// 行详情元信息
		views.RegisterApi(ipv6TcpGroup, "/detail", "行详情查询 - 按 namespace + id 返回单条全字段记录",
			views.WithMethod(http.MethodPost, views.WithBodyInput[Ipv6TcpDetailGetIn](), views.WithBodyOutput[Ipv6TcpListGetListItemOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.ipv6.detail",
				Domain:    "traffic",
				Intent:    "查看单条 IPv6 TCP 流量的全字段详情",
				Entities:  []string{"packet_ipv6_tcps"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
	}

	// ========== HTTP 传输：/api/v1/traffic/http-transport ==========
	{
		httpTransportGroup := trafficGroup.Group("/http-transport")
		// 流量列表查询 - 分页查询 HTTP 传输全字段
		httpTransportGroup.POST("/list", HttpTransportListGet)
		// 流量列表元信息
		views.RegisterListApi[*views.HttpTransportListGetSearchIn, *HttpTransportListGetListItemOut](httpTransportGroup, "/list", "流量列表查询 - 分页查询 HTTP 传输全字段",
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.list",
				Domain:    "traffic",
				Intent:    "查询 HTTP 流量列表（按时间、主机、路径、方法等筛选）",
				Entities:  []string{"http_transports"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 行详情查询 - 按 namespace + id 返回单条全字段记录
		httpTransportGroup.POST("/detail", HttpTransportDetailGet)
		// 行详情元信息
		views.RegisterApi(httpTransportGroup, "/detail", "行详情查询 - 按 namespace + id 返回单条全字段记录",
			views.WithMethod(http.MethodPost, views.WithBodyInput[HttpTransportDetailGetIn](), views.WithBodyOutput[HttpTransportListGetListItemOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.detail",
				Domain:    "traffic",
				Intent:    "查看单条 HTTP 流量的全字段详情",
				Entities:  []string{"http_transports"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 关联网络堆栈列表查询 - 以 HttpTransport 的 traceId 分页查询组成该 TCP 流的网络堆栈
		httpTransportGroup.POST("/stack/list", HttpTransportStackListGet)
		// 关联网络堆栈列表元信息
		views.RegisterListApi[*views.NetworkStackListGetSearchIn, *NetworkStackListGetListItemOut](httpTransportGroup, "/stack/list", "关联网络堆栈列表查询 - 以 HttpTransport 的 traceId 分页查询组成该 TCP 流的网络堆栈",
			views.WithCapability(views.CapabilityMeta{
				ID:        "traffic.stack",
				Domain:    "traffic",
				Intent:    "查询某条流量关联的网络协议栈",
				Entities:  []string{"network_stacks"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
	}
}
