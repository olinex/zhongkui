package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// HttpTransportListGetListItemOut HTTP 传输列表输出项
// 嵌入完整模型直接返回全部字段（body 已转存 S3，单条返回字节量可控），
// BaseTotalOut 承接 COUNT(*) OVER() 的总数
type HttpTransportListGetListItemOut struct {
	utils.BaseTotalOut
	network.HttpTransport
}

// httpTransportListGet 处理 HTTP 传输列表查询请求
// 通过 HttpTransportListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的传输数据，
// 输出 HttpTransportListGetListItemOut（嵌入完整模型，含全部字段）
func httpTransportListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in HttpTransportListGetSearchIn
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
	var items []*HttpTransportListGetListItemOut

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := manager.GetRawDatabase(c).Model(&network.HttpTransport{})

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
