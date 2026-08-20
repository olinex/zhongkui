// Package routes 提供 API 路由层的通用基础设施，包括：
//   - 抓包器（Capturer）的 CRUD 路由处理器
//   - 原始流量审计的只读列表处理器（traffic*.go）
//
// 原始流量审计按数据类型拆分为三组独立接口（/api/v1/traffic/{ipv4-tcp,ipv6-tcp,http-transport}）：
//   - traffic_ipv4_tcp.go        IPv4 TCP 包独立接口
//   - traffic_ipv6_tcp.go        IPv6 TCP 包独立接口
//   - traffic_http_transport.go  HTTP 传输独立接口
//
// 三组的数据结构与业务逻辑差异较大，各自独立演进，互不耦合；
// 各组的列表输出结构体（*ListItemOut）与 listGet 处理器按各自表实际存在的列手写在各类型文件内，
// 仅绑定各自的 SearchIn（生成于 search.go）与查询模型。
//
// 安全设计：
//   - 三种流量数据均无唯一性 ID，条件查询无法可靠定位单条记录，故不提供 detail 接口；
//     list 查询直接返回全部字段（payload/body 已转存 S3，单条返回字节量可控），
//     行级查看在前端直接使用列表行数据
//   - 加密字段输出 base64 密文（serializer.Crypto 的 MarshalJSON），S3 body 字段输出对象键
//   - 明文查看走全局通用接口 /api/v1/plaintext/crypto 与 /api/v1/plaintext/s3
//     （见 internal/manager/plaintext.go），按字段构成权限点 object 做 RBAC 静态授权；
//     权限点格式规范见 internal/pkg/base/consts.go
//   - 查询依赖 SearchIn 的 timestamp / created_at 必填过滤，防全表扫描
package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// Ipv4TcpListGetListItemOut IPv4 TCP 包列表输出项
// 嵌入完整模型直接返回全部字段（payload 已转存 S3，单条返回字节量可控），
// BaseTotalOut 承接 COUNT(*) OVER() 的总数
type Ipv4TcpListGetListItemOut struct {
	utils.BaseTotalOut
	network.PacketIPv4TCP
}

// ipv4TcpListGet 处理 IPv4 TCP 包列表查询请求
// 通过 Ipv4TcpListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的包数据，
// 输出 Ipv4TcpListGetListItemOut（嵌入完整模型，含全部字段）
func ipv4TcpListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in Ipv4TcpListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 准备查询结果容器
	var items []*Ipv4TcpListGetListItemOut

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := manager.GetRawDatabase(c).Model(&network.PacketIPv4TCP{})

	// 构建搜索查询，应用分页、排序和过滤条件
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建搜索输出结果，包含数据和分页信息
	out := utils.NewSearchOut(items)

	// 返回成功的 JSON 响应
	manager.SimpleOK.AbortWithGinW(c, out)
}
