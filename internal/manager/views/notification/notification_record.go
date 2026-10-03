package notification

import (
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"

	core "zhongkuisec.com/zhongkui/internal/models/core"
)

// NotificationRecordDetailOut 定义通知记录详情输出结构。
// 嵌入 BaseItemOut 携带审计字段；除记录本身字段外，还尽力补全目标名称与渠道名称。
type NotificationRecordDetailOut struct {
	utils.BaseItemOut
	// EventType 事件类型，标识 EventID 指向的事件类型
	EventType string `json:"eventType"`
	// EventID 触发通知的事件 ID
	EventID uint `json:"eventId"`
	// GroupID 发送时所属的通知组 ID，0 表示未绑定组
	GroupID uint `json:"groupId"`
	// TargetID 通知目标 ID
	TargetID uint `json:"targetId"`
	// TargetName 通知目标名称（尽力补全，目标已删除时为空字符串）
	TargetName string `json:"targetName"`
	// ChannelID 发送通知的渠道 ID
	ChannelID uint `json:"channelId"`
	// ChannelName 渠道名称（尽力补全，渠道已删除时为空字符串）
	ChannelName string `json:"channelName"`
	// ChannelType 渠道类型键快照
	ChannelType string `json:"channelType"`
	// Status 通知发送状态：pending/success/failed
	Status string `json:"status"`
	// Error 发送失败时的错误信息，成功时为空字符串
	Error string `json:"error"`
	// MessageID 渠道返回的消息 ID
	MessageID string `json:"messageId"`
	// Payload 通知负载（JSON 文本）快照
	Payload string `json:"payload"`
	// NotifiedAt 通知实际发送成功的时间，nil 表示尚未发送成功
	NotifiedAt *time.Time `json:"notifiedAt"`
}

// NotificationRecordListOut 定义通知记录列表的输出项结构。
// 嵌入 BaseItemOut 携带审计字段、BaseTotalOut 支持分页总数统计；
// 列表仅补全目标名称，不携带渠道名称等详情字段。
type NotificationRecordListOut struct {
	utils.BaseItemOut
	// EventType 事件类型，标识 EventID 指向的事件类型
	EventType string `json:"eventType"`
	// EventID 触发通知的事件 ID
	EventID uint `json:"eventId"`
	// TargetID 通知目标 ID
	TargetID uint `json:"targetId"`
	// TargetName 通知目标名称（尽力补全，目标已删除时为空字符串）
	TargetName string `json:"targetName"`
	// ChannelType 渠道类型键快照
	ChannelType string `json:"channelType"`
	// Status 通知发送状态：pending/success/failed
	Status string `json:"status"`
	// MessageID 渠道返回的消息 ID
	MessageID string `json:"messageId"`
	// NotifiedAt 通知实际发送成功的时间，nil 表示尚未发送成功
	NotifiedAt *time.Time `json:"notifiedAt"`
	utils.BaseTotalOut
}

// notificationTargetRow 补全目标名称查询使用的精简行结构。
type notificationTargetRow struct {
	ID   uint   `gorm:"column:id"`
	Name string `gorm:"column:name"`
}

// notificationChannelRow 补全渠道名称查询使用的精简行结构。
type notificationChannelRow struct {
	ID   uint   `gorm:"column:id"`
	Name string `gorm:"column:name"`
}

// NotificationRecordListGet 处理获取通知记录列表的请求。
// 支持分页、排序与过滤（时间/状态/事件类型/渠道类型），仅返回当前命名空间内的记录；
// 列表批量补全目标名称（尽力而为，缺失置空字符串）。
func NotificationRecordListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in views.NotificationRecordListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*NotificationRecordListOut
	db := middlewares.GetCoreDatabase(c).Model(&core.NotificationRecord{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	enrichNotificationRecordListTargetNames(c, namespace, items)
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **NotificationRecordListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// enrichNotificationRecordListTargetNames 批量补全通知记录列表的目标名称。
// 一次性收集全部去重目标 ID 查询目标表，缺失（目标已删除）的名称保持空字符串。
func enrichNotificationRecordListTargetNames(c *gin.Context, namespace string, items []*NotificationRecordListOut) {
	targetIDs := make([]uint, 0, len(items))
	seen := make(map[uint]bool)
	for _, item := range items {
		if item.TargetID == 0 || seen[item.TargetID] {
			continue
		}
		seen[item.TargetID] = true
		targetIDs = append(targetIDs, item.TargetID)
	}
	if len(targetIDs) == 0 {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	nameMap := make(map[uint]string)
	var targets []*notificationTargetRow
	if err := db.WithContext(c).Model(&core.NotificationTarget{}).
		Select("id, name").
		Where("namespace_name = ? AND id IN ?", namespace, targetIDs).
		Find(&targets).Error; err != nil {
		// 补全为尽力而为：查询失败仅记录，不中断列表响应，目标名称保持空字符串
		middlewares.GetLogger(c).Warnf(c, nil, "enrich notification record target names failed, err=%v", err)
		return
	}
	for _, target := range targets {
		nameMap[target.ID] = target.Name
	}
	for _, item := range items {
		item.TargetName = nameMap[item.TargetID]
	}
}

// NotificationRecordDetailGet 处理获取通知记录详情的请求。
// 返回记录的完整发送内容；仅允许访问当前命名空间内的记录；
// 目标名称与渠道名称尽力补全（相关记录可能已删除，缺失置空字符串）。
func NotificationRecordDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationRecord{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := newNotificationRecordDetailOut(&instance)
	out.TargetName = loadNotificationTargetName(c, db, namespace, instance.TargetID)
	out.ChannelName = loadNotificationChannelName(c, db, namespace, instance.ChannelID)
	response.SimpleOK.AbortWithGinW(c, out)
}

// newNotificationRecordDetailOut 将 NotificationRecord 转换为详情输出结构。
func newNotificationRecordDetailOut(record *core.NotificationRecord) NotificationRecordDetailOut {
	return NotificationRecordDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        record.ID,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		},
		EventType:   record.EventType,
		EventID:     record.EventID,
		GroupID:     record.GroupID,
		TargetID:    record.TargetID,
		ChannelID:   record.ChannelID,
		ChannelType: record.ChannelType,
		Status:      record.Status,
		Error:       record.Error,
		MessageID:   record.MessageID,
		Payload:     record.Payload,
		NotifiedAt:  record.NotifiedAt,
	}
}

// loadNotificationTargetName 尽力补全通知目标名称。
// 目标已删除或查询失败时返回空字符串，不中断主请求。
func loadNotificationTargetName(c *gin.Context, db *gorm.DB, namespace string, targetID uint) string {
	if targetID == 0 {
		return ""
	}
	target := notificationTargetRow{}
	if err := db.WithContext(c).Model(&core.NotificationTarget{}).
		Select("id, name").
		Where("namespace_name = ? AND id = ?", namespace, targetID).
		First(&target).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			middlewares.GetLogger(c).Warnf(c, nil, "load notification target name failed, target=%d, err=%v", targetID, err)
		}
		return ""
	}
	return target.Name
}

// loadNotificationChannelName 尽力补全通知渠道名称。
// 渠道已删除或查询失败时返回空字符串，不中断主请求。
func loadNotificationChannelName(c *gin.Context, db *gorm.DB, namespace string, channelID uint) string {
	if channelID == 0 {
		return ""
	}
	channel := notificationChannelRow{}
	if err := db.WithContext(c).Model(&core.NotificationChannel{}).
		Select("id, name").
		Where("namespace_name = ? AND id = ?", namespace, channelID).
		First(&channel).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			middlewares.GetLogger(c).Warnf(c, nil, "load notification channel name failed, channel=%d, err=%v", channelID, err)
		}
		return ""
	}
	return channel.Name
}

// init 注册通知记录列表出参（NotificationRecordListOut.channelType / eventType）的枚举类绑定：
// channelType 对应 enum 注册表 channelType 类（message.Types()）、
// eventType 对应 notificationEvent 类（notify.TemplateSpecs()）。
func init() {
	views.AddEnumBinding("NotificationRecordListOut", "channelType", "channelType")
	views.AddEnumBinding("NotificationRecordListOut", "eventType", "notificationEvent")
}
