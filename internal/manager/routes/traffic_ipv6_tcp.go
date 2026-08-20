package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// Ipv6TcpListGetListItemOut IPv6 TCP 包列表输出项
// 嵌入完整模型直接返回全部字段（payload 已转存 S3，单条返回字节量可控），
// BaseTotalOut 承接 COUNT(*) OVER() 的总数
type Ipv6TcpListGetListItemOut struct {
	utils.BaseTotalOut
	network.PacketIPv6TCP
}

// ipv6TcpListGet 处理 IPv6 TCP 包列表查询请求
// 通过 Ipv6TcpListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的包数据，
// 输出 Ipv6TcpListGetListItemOut（嵌入完整模型，含全部字段）
func ipv6TcpListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in Ipv6TcpListGetSearchIn
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
	var items []*Ipv6TcpListGetListItemOut

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := manager.GetRawDatabase(c).Model(&network.PacketIPv6TCP{})

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
