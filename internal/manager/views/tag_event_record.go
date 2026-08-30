// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签事件记录管理相关的 API 路由处理器，
// 提供标签事件记录的手动关闭（批量置为 closed）能力，
// 后续事件列表/详情等能力在此扩展。
package views

import (
	// third-party
	"github.com/gin-gonic/gin"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/tag"
)

// TagEventRecordBatchClose 批量关闭标签事件记录（置为 closed）。
// 仅关闭当前命名空间内未关闭（status != closed）的事件记录；不存在或已关闭的 ID 幂等跳过，
// 全部视为成功并返回 BatchResultOut 汇总（成功条数 = 请求 ID 数）。
func TagEventRecordBatchClose(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagEventRuleBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !validFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	if err := db.WithContext(c).Model(&tag.TagEventRecord{}).
		Where("namespace_name = ? AND id IN ? AND status != ?", namespace, ids, tag.EventStatusClosed).
		Update("status", tag.EventStatusClosed).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, BatchResultOut{Succeeded: len(ids), Failed: []BatchFailureOut{}})
}
