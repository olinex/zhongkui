// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签事件记录管理相关的 API 路由处理器，
// 提供标签事件记录的手动关闭（批量置为 closed）、列表查询与详情查询能力，
// 详情中关联展示该事件触发的多条通知发送记录（来自 notification_records）。
package views

import (
	// stdlib
	"time"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"

	core "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/models/tag"
)


// TagEventRecordListOut 定义标签事件记录列表的输出项结构。
// 嵌入 BaseItemOut 携带审计字段、BaseTotalOut 支持分页总数统计；
// 展示触发规则快照、是否全命中与事件状态等核心信息。
type TagEventRecordListOut struct {
	utils.BaseItemOut
	// RuleID 所属标签事件规则 ID
	RuleID uint `json:"ruleId"`
	// RuleName 规则名称快照
	RuleName string `json:"ruleName"`
	// MatchAll 规则 MatchAll 快照
	MatchAll bool `json:"matchAll"`
	// Status 事件状态：pending/alerting/suppressed/closed
	Status string `json:"status"`
	// DetectedAt 检测时间
	DetectedAt time.Time `json:"detectedAt"`
	utils.BaseTotalOut
}

// TagEventRecordHitOut 定义标签事件记录命中明细输出项。
// 展示某条事件条件在时间窗口内对特定维度值的命中统计。
type TagEventRecordHitOut struct {
	// ID 命中明细 ID
	ID uint `json:"id"`
	// ConditionName 条件名称快照
	ConditionName string `json:"conditionName"`
	// Model 目标模型类型键
	Model string `json:"model"`
	// Dimension 标签维度类型
	Dimension string `json:"dimension"`
	// Item 维度具体值
	Item string `json:"item"`
	// Count 分组命中次数
	Count uint64 `json:"count"`
	// Threshold 条件阈值
	Threshold int `json:"threshold"`
	// Window 条件时间窗口（可读字符串，如 "5s"）
	Window serializer.DurationNS `json:"window"`
}

// TagEventRecordSendOut 定义标签事件关联的通知发送记录输出项。
// 数据来自 notification_records（event_type=tag_event_record 且 event_id=事件记录ID），
// 目标名/渠道名为尽力补全，相关目标或渠道已删除时为空字符串。
type TagEventRecordSendOut struct {
	// ID 发送记录 ID
	ID uint `json:"id"`
	// TargetID 通知目标 ID
	TargetID uint `json:"targetId"`
	// TargetName 通知目标名称（尽力补全）
	TargetName string `json:"targetName"`
	// ChannelID 发送通知的渠道 ID
	ChannelID uint `json:"channelId"`
	// ChannelName 渠道名称（尽力补全）
	ChannelName string `json:"channelName"`
	// ChannelType 渠道类型键快照
	ChannelType string `json:"channelType"`
	// Status 通知发送状态：pending/success/failed
	Status string `json:"status"`
	// MessageID 渠道返回的消息 ID
	MessageID string `json:"messageId"`
	// Error 发送失败时的错误信息，成功时为空字符串
	Error string `json:"error"`
	// Payload 通知负载（JSON 文本）快照
	Payload string `json:"payload"`
	// NotifiedAt 通知实际发送成功的时间，nil 表示尚未发送成功
	NotifiedAt *time.Time `json:"notifiedAt"`
}

// TagEventRecordDetailOut 定义标签事件记录详情输出结构。
// 嵌入 BaseItemOut 携带审计字段；除事件基础信息外，还携带命中明细（Hits）
// 及其关联的多条通知发送记录（Sends）。
type TagEventRecordDetailOut struct {
	utils.BaseItemOut
	// RuleID 所属标签事件规则 ID
	RuleID uint `json:"ruleId"`
	// RuleName 规则名称快照
	RuleName string `json:"ruleName"`
	// MatchAll 规则 MatchAll 快照
	MatchAll bool `json:"matchAll"`
	// Status 事件状态：pending/alerting/suppressed/closed
	Status string `json:"status"`
	// DedupKey 抑制指纹（SHA-256 十六进制）
	DedupKey string `json:"dedupKey"`
	// DetectedAt 检测时间
	DetectedAt time.Time `json:"detectedAt"`
	// Hits 该事件记录下的命中明细
	Hits []TagEventRecordHitOut `json:"hits"`
	// Sends 关联的通知发送记录列表
	Sends []TagEventRecordSendOut `json:"sends"`
}

// TagEventRecordListGet 处理获取标签事件记录列表的请求。
// 支持分页、排序与过滤（时间/规则名/状态/是否全命中），仅返回当前命名空间内的记录。
func TagEventRecordListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagEventRecordListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*TagEventRecordListOut
	db := middlewares.GetCoreDatabase(c).Model(&tag.TagEventRecord{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchOut(items))
}

// TagEventRecordDetailGet 处理获取标签事件记录详情的请求。
// 仅允许访问当前命名空间内的记录；返回事件基础信息、命中明细（Hits）
// 以及该事件触发的关联通知发送记录（Sends，来自 notification_records，
// 目标名/渠道名为尽力补全）。
func TagEventRecordDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := tag.TagEventRecord{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := newTagEventRecordDetailOut(&instance)

	// 查询命中明细
	var hits []*tag.TagEventRecordHit
	if err := db.WithContext(c).Where("tag_event_record_id = ?", id).Order("id ASC").Find(&hits).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out.Hits = make([]TagEventRecordHitOut, 0, len(hits))
	for _, hit := range hits {
		out.Hits = append(out.Hits, newTagEventRecordHitOut(hit))
	}

	// 查询关联的通知发送记录
	var sends []*core.NotificationRecord
	if err := db.WithContext(c).
		Where("namespace_name = ? AND event_type = ? AND event_id = ?", namespace, core.EventTypeTagEventRecord, id).
		Order("id ASC").Find(&sends).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out.Sends = make([]TagEventRecordSendOut, 0, len(sends))
	for _, send := range sends {
		out.Sends = append(out.Sends, newTagEventRecordSendOut(send))
	}
	enrichTagEventRecordSendNames(c, db, namespace, out.Sends)
	response.SimpleOK.AbortWithGinW(c, out)
}

// newTagEventRecordDetailOut 将 TagEventRecord 转换为详情输出结构的基础事件字段。
// 命中明细与关联发送记录的组装由调用方 TagEventRecordDetailGet 完成。
func newTagEventRecordDetailOut(record *tag.TagEventRecord) TagEventRecordDetailOut {
	return TagEventRecordDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        record.ID,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		},
		RuleID:     record.RuleID,
		RuleName:   record.RuleName,
		MatchAll:   record.MatchAll,
		Status:     record.Status,
		DedupKey:   record.DedupKey,
		DetectedAt: record.DetectedAt,
	}
}

// newTagEventRecordHitOut 将 TagEventRecordHit 转换为命中明细输出结构。
func newTagEventRecordHitOut(hit *tag.TagEventRecordHit) TagEventRecordHitOut {
	return TagEventRecordHitOut{
		ID:            hit.ID,
		ConditionName: hit.ConditionName,
		Model:         hit.Model,
		Dimension:     hit.Dimension,
		Item:          hit.Item,
		Count:         hit.Count,
		Threshold:     hit.Threshold,
		Window:        hit.Window,
	}
}

// newTagEventRecordSendOut 将 NotificationRecord 转换为关联发送记录输出结构。
func newTagEventRecordSendOut(record *core.NotificationRecord) TagEventRecordSendOut {
	return TagEventRecordSendOut{
		ID:          record.ID,
		TargetID:    record.TargetID,
		ChannelID:   record.ChannelID,
		ChannelType: record.ChannelType,
		Status:      record.Status,
		MessageID:   record.MessageID,
		Error:       record.Error,
		Payload:     record.Payload,
		NotifiedAt:  record.NotifiedAt,
	}
}

// enrichTagEventRecordSendNames 批量补全关联发送记录的目标名称与渠道名称。
// 一次性收集全部去重目标/渠道 ID 分别查询，缺失（目标或渠道已删除）的名称保持空字符串，
// 补全为尽力而为，查询失败仅记录、不中断详情响应。
func enrichTagEventRecordSendNames(c *gin.Context, db *gorm.DB, namespace string, sends []TagEventRecordSendOut) {
	targetIDs := make([]uint, 0, len(sends))
	targetSeen := make(map[uint]bool)
	channelIDs := make([]uint, 0, len(sends))
	channelSeen := make(map[uint]bool)
	for i := range sends {
		if sends[i].TargetID != 0 && !targetSeen[sends[i].TargetID] {
			targetSeen[sends[i].TargetID] = true
			targetIDs = append(targetIDs, sends[i].TargetID)
		}
		if sends[i].ChannelID != 0 && !channelSeen[sends[i].ChannelID] {
			channelSeen[sends[i].ChannelID] = true
			channelIDs = append(channelIDs, sends[i].ChannelID)
		}
	}
	if len(targetIDs) > 0 {
		targetNames := make(map[uint]string)
		var targets []*notificationTargetRow
		if err := db.WithContext(c).Model(&core.NotificationTarget{}).
			Select("id, name").
			Where("namespace_name = ? AND id IN ?", namespace, targetIDs).
			Find(&targets).Error; err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "enrich tag event record target names failed, err=%v", err)
		} else {
			for _, t := range targets {
				targetNames[t.ID] = t.Name
			}
			for i := range sends {
				sends[i].TargetName = targetNames[sends[i].TargetID]
			}
		}
	}
	if len(channelIDs) > 0 {
		channelNames := make(map[uint]string)
		var channels []*notificationChannelRow
		if err := db.WithContext(c).Model(&core.NotificationChannel{}).
			Select("id, name").
			Where("namespace_name = ? AND id IN ?", namespace, channelIDs).
			Find(&channels).Error; err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "enrich tag event record channel names failed, err=%v", err)
		} else {
			for _, ch := range channels {
				channelNames[ch.ID] = ch.Name
			}
			for i := range sends {
				sends[i].ChannelName = channelNames[sends[i].ChannelID]
			}
		}
	}
}

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
