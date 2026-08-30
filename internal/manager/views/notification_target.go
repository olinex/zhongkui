// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知目标（NotificationTarget）管理相关的 API 路由处理器，
// 提供目标的创建、详情查询、列表查询、更新、批量删除、批量启停与 test-send（测试发送）能力。
// 通知目标归属于通知渠道（NotificationChannel），其 Type 为渠道类型键快照（创建时派生）。
package views

import (
	// stdlib
	"encoding/json"
	"net/http"
	"strconv"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	core "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	notifyhandler "zhongkui.com/zhongkui/internal/worker/handlers/notify"
)

// NotificationTargetCreateIn 定义创建通知目标的请求输入结构。
// Type 不入参，类型由所选渠道（ChannelID）派生并在创建时快照到目标。
// Config 为目标级配置（JSON 文本），其结构随渠道类型不同而不同
// （如 email 目标为 {"to": ...}，webhook 目标为 {"url": ..., "headers": {...}}）。
type NotificationTargetCreateIn struct {
	// Name 目标名称，必填，长度 1~128（同一命名空间内唯一）
	Name string `json:"name" binding:"required,min=1,max=128"`
	// ChannelID 目标所属的通知渠道 ID，必填
	ChannelID uint `json:"channelId" binding:"required"`
	// Enabled 是否启用，默认启用
	Enabled bool `json:"enabled" default:"true"`
	// Config 目标级配置（JSON 文本），必填
	Config string `json:"config" binding:"required"`
	// Language 目标偏好的消息语言（如 en、zh-Hans），最长 32 字符，空表示平台默认
	Language string `json:"language" binding:"max=32"`
	// Description 目标描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// NotificationTargetUpdateIn 定义更新通知目标的请求输入结构。
// 目标名称与所属渠道不可变，仅允许调整启用状态、目标配置、消息语言与描述。
type NotificationTargetUpdateIn struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Config 目标级配置（JSON 文本）
	Config string `json:"config" binding:"required"`
	// Language 目标偏好的消息语言，最长 32 字符
	Language string `json:"language" binding:"max=32"`
	// Description 目标描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// NotificationTargetDetailOut 定义通知目标详情的输出结构。
// 嵌入 BaseItemOut 携带审计字段，并附带所属渠道的快照信息（ChannelName/ChannelType/ChannelEnabled）。
type NotificationTargetDetailOut struct {
	utils.BaseItemOut
	// Name 目标名称
	Name string `json:"name"`
	// ChannelID 目标所属的通知渠道 ID
	ChannelID uint `json:"channelId"`
	// ChannelName 目标所属渠道的名称
	ChannelName string `json:"channelName"`
	// ChannelType 目标所属渠道的类型键（如 email、webhook）
	ChannelType string `json:"channelType"`
	// ChannelEnabled 目标所属渠道是否启用
	ChannelEnabled bool `json:"channelEnabled"`
	// Type 目标类型键快照（创建时由渠道派生）
	Type string `json:"type"`
	// Enabled 目标是否启用
	Enabled bool `json:"enabled"`
	// Config 目标级配置的密文。密文由 gorm 模型经 crypto 序列化器落库/读取，
	// 本 Out 结构直接复用模型 *serializer.Crypto，经其 MarshalJSON 输出 base64 密文；
	// 字段经 form 标签标记为 crypto，由 meta options 告知前端该字段已加密（明文经明文权限接口查看）
	Config *serializer.Crypto `json:"config" form:"kind:crypto"`
	// Language 目标偏好的消息语言，空表示平台默认
	Language string `json:"language"`
	// Description 目标描述
	Description string `json:"description"`
}

// NotificationTargetListOut 定义通知目标列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type NotificationTargetListOut struct {
	NotificationTargetDetailOut
	utils.BaseTotalOut
}

// NotificationTargetBatchIn 批量操作通知目标的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type NotificationTargetBatchIn struct {
	Ids FlexList[uint64] `json:"ids"`
}

// NotificationTargetSendIn 定义"向单个通知目标发送通知"的请求输入结构。
// 调用方指定目标（经 URL :id）+ 事件引用 + 消息模板，投递 notify:send:target 异步任务，
// 复用通知全链路（模板渲染、渠道基座投递与发送记录写入）。
// NotificationTargetSendIn is the request input of "send a notification to a single
// target". The caller supplies the target (via URL :id) plus an event reference and a
// message template; the handler enqueues a notify:send:target async task that reuses
// the full notification pipeline (template rendering, channel delivery and record write).
type NotificationTargetSendIn struct {
	// EventType 事件类型键（如 tag_event），与 EventID 组合定位通知记录（必填）
	// EventType the event type key (e.g. tag_event), locating the notification record with EventID (required)
	EventType string `json:"eventType" binding:"required"`
	// EventID 触发通知的事件 ID，须为非零值以保证幂等去重（必填）
	// EventID the ID of the event that triggered the notification, must be non-zero for idempotent dedup (required)
	EventID uint `json:"eventID" binding:"required"`
	// Template 消息模板键，经目标偏好语言渲染为标题/正文（必填）
	// Template the message template key, rendered into title/content in the target-preferred language (required)
	Template string `json:"template" binding:"required"`
	// Params 模板参数，供 locales 模板渲染
	// Params the template params used to render the locale template
	Params map[string]any `json:"params"`
}

// NotificationTargetSendOut 定义"向单个通知目标发送通知"的输出结构。
// 回显本次异步投递请求所定位的目标与事件引用。
// NotificationTargetSendOut is the output of "send a notification to a single target",
// echoing the target and event reference of this async send request.
type NotificationTargetSendOut struct {
	// TargetID 本次投递的目标 ID
	// TargetID the target ID this send addressed
	TargetID uint `json:"targetId"`
	// EventType 本次投递的事件类型键
	// EventType the event type key of this send
	EventType string `json:"eventType"`
	// EventID 本次投递的事件 ID
	// EventID the event ID of this send
	EventID uint `json:"eventId"`
}

// NotificationTargetTestSendOut 定义通知目标 test-send 的输出结构。
// 返回渠道实际生成的 MessageID、按目标语言渲染后的标题/正文，以及本次通知行为
// 实际使用的渠道与目标参数（Config 为 base64 密文，前端可经 plaintext 接口解密
// 核对，保证「所见即所发」）；结构对渠道类型无感知，新渠道无需改动本结构即可复用。
type NotificationTargetTestSendOut struct {
	// MessageID 渠道返回的消息 ID
	MessageID string `json:"messageId"`
	// Title 渲染后的消息标题
	Title string `json:"title"`
	// Content 渲染后的消息正文
	Content string `json:"content"`
	// Language 本次发送实际使用的消息语言
	Language string `json:"language"`
	// Channel 本次使用的渠道参数
	Channel NotificationTargetTestSendChannelOut `json:"channel"`
	// Target 本次使用的目标参数
	Target NotificationTargetTestSendTargetOut `json:"target"`
}

// NotificationTargetTestSendChannelOut 定义 test-send 的渠道参数输出结构。
type NotificationTargetTestSendChannelOut struct {
	// Name 渠道名称
	Name string `json:"name"`
	// Type 渠道类型键
	Type string `json:"type"`
	// Enabled 渠道是否启用
	Enabled bool `json:"enabled"`
	// Config 渠道级配置密文（base64），前端可经 plaintext 接口解密
	Config *serializer.Crypto `json:"config" form:"kind:crypto"`
}

// NotificationTargetTestSendTargetOut 定义 test-send 的目标参数输出结构。
type NotificationTargetTestSendTargetOut struct {
	// Name 目标名称
	Name string `json:"name"`
	// Language 目标偏好语言
	Language string `json:"language"`
	// Enabled 目标是否启用
	Enabled bool `json:"enabled"`
	// Config 目标级配置密文（base64），前端可经 plaintext 接口解密
	Config *serializer.Crypto `json:"config" form:"kind:crypto"`
}

// NotificationTargetListGet 处理获取通知目标列表的请求。
// 支持分页、排序与过滤（时间/名称/渠道/类型/启用状态/语言），仅返回当前命名空间内的目标；
// 按 gorm 模型扫描（config 经 crypto 序列化器读取）再映射为接口层 Out 结构，
// 第二趟按渠道 ID 批量加载所属渠道信息（ChannelName/ChannelType/ChannelEnabled）回填。
func NotificationTargetListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTargetListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 以局部扫描行承载 gorm 模型与注入的 total 列（用于分页总数），随后映射为 Out 结构
	var rows []struct {
		core.NotificationTarget
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&core.NotificationTarget{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*NotificationTargetListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		detail := notificationTargetDetailOut(&rows[i].NotificationTarget, nil)
		items = append(items, &NotificationTargetListOut{
			NotificationTargetDetailOut: detail,
			BaseTotalOut:                utils.BaseTotalOut{Total: total},
		})
	}
	if len(items) > 0 {
		notificationTargetsFillChannelInfo(c, namespace, items)
	}
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchOut(items))
}

// notificationTargetsFillChannelInfo 批量加载通知目标所属渠道信息并回填到输出结构。
// 按去重后的渠道 ID 一次性查询当前命名空间的渠道，映射 id -> channel；
// 渠道缺失（已被删除）时对应渠道字段保持空值。
func notificationTargetsFillChannelInfo(c *gin.Context, namespace string, items []*NotificationTargetListOut) {
	seen := make(map[uint]struct{}, len(items))
	channelIDs := make([]uint, 0, len(items))
	for _, item := range items {
		if item.ChannelID == 0 {
			continue
		}
		if _, ok := seen[item.ChannelID]; ok {
			continue
		}
		seen[item.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, item.ChannelID)
	}
	if len(channelIDs) == 0 {
		return
	}
	var channels []*core.NotificationChannel
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("namespace_name = ? AND id IN ?", namespace, channelIDs).
		Find(&channels).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	channelByID := make(map[uint]*core.NotificationChannel, len(channels))
	for _, ch := range channels {
		channelByID[ch.ID] = ch
	}
	for _, item := range items {
		if ch, ok := channelByID[item.ChannelID]; ok {
			item.ChannelName = ch.Name
			item.ChannelType = ch.Type
			item.ChannelEnabled = ch.Enabled
		}
	}
}

// NotificationTargetCreate 处理创建通知目标的请求。
// 校验所选渠道属于当前命名空间（不存在则返回 404）；目标 Type 由渠道类型键派生并快照入库。
func NotificationTargetCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTargetCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	channel := core.NotificationChannel{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", in.ChannelID, namespace).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	instance := &core.NotificationTarget{
		NamespaceName: namespace,
		Name:          in.Name,
		ChannelID:     in.ChannelID,
		Type:          channel.Type,
		Enabled:       in.Enabled,
		Config:        serializer.NewCrypto([]byte(in.Config)),
		Language:      in.Language,
		Description:   in.Description,
	}
	if err := db.WithContext(c).Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationTargetDetailGet 处理获取通知目标详情的请求。
// 返回目标全部字段并附带所属渠道信息；仅允许访问当前命名空间内的目标。
func NotificationTargetDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationTarget{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 加载所属渠道信息（渠道缺失时渠道字段保持空值）
	channel := core.NotificationChannel{}
	_ = db.WithContext(c).Where("id = ? AND namespace_name = ?", instance.ChannelID, namespace).First(&channel).Error
	response.SimpleOK.AbortWithGinW(c, notificationTargetDetailOut(&instance, &channel))
}

// NotificationTargetUpdate 处理更新通知目标的请求。
// 仅更新启用状态、目标配置、消息语言与描述；目标名称与所属渠道不可变。
// 仅允许更新当前命名空间内的目标。
func NotificationTargetUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationTarget{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var in NotificationTargetUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
	update := core.NotificationTarget{
		Enabled:     in.Enabled,
		Config:      serializer.NewCrypto([]byte(in.Config)),
		Language:    in.Language,
		Description: in.Description,
	}
	if err := db.WithContext(c).Model(&core.NotificationTarget{}).Where("id = ? AND namespace_name = ?", instance.ID, namespace).
		Select("enabled", "config", "language", "description").
		Updates(&update).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationTargetBatchDelete 批量删除通知目标。
// 目标不存在（或不属于当前命名空间）视为成功跳过（幂等）。
func NotificationTargetBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTargetBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !validFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	batchExecute(c, ids,
		func(id uint64) string { return strconv.FormatUint(id, 10) },
		func(id uint64) error {
			instance := core.NotificationTarget{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			return db.WithContext(c).Where("id = ? AND namespace_name = ?", instance.ID, namespace).
				Delete(&core.NotificationTarget{}).Error
		},
	)
}

// NotificationTargetBatchSetEnabled 批量启用/停用通知目标。
// 幂等短路（已处于目标状态直接成功）+ Update；目标不存在（或不属于当前命名空间）视为成功跳过。
func NotificationTargetBatchSetEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, ok := middlewares.RequireNamespace(c)
		if !ok {
			return
		}
		var in NotificationTargetBatchIn
		if err := c.ShouldBindBodyWithJSON(&in); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		ids := []uint64(in.Ids)
		if !validFlexLen(c, ids) {
			return
		}
		db := middlewares.GetCoreDatabase(c)
		batchExecute(c, ids,
			func(id uint64) string { return strconv.FormatUint(id, 10) },
			func(id uint64) error {
				instance := core.NotificationTarget{}
				if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil // 不存在视为成功跳过
					}
					return errors.WithStack(err)
				}
				if instance.Enabled == enabled {
					return nil // 幂等短路
				}
				return db.WithContext(c).Model(&core.NotificationTarget{}).
					Where("id = ? AND namespace_name = ?", instance.ID, namespace).
					Update("enabled", enabled).Error
			},
		)
	}
}

// NotificationTargetTestSend 处理通知目标 test-send（测试发送）的请求。
// 按目标所属渠道实际类型构建渠道实例，以固定模板（notify.test.title / notify.test.content）
// 在目标偏好的语言下渲染测试消息，并经渠道实际发送一次；响应返回渠道 MessageID、
// 渲染后的标题/正文、实际使用的语言，以及本次通知行为所用渠道与目标的参数
// （config 为 base64 密文，前端可经 plaintext 接口解密）。
// 发起通知行为的任何报错（渠道构建、目标配置解析、实际投递失败）均以正常业务错误
// （400 + CodeNotificationSendFailed，details 携带具体原因）返回，供 UI 直接展示；
// 仅数据库查询等未预期内部错误保持 500。
func NotificationTargetTestSend(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	target := core.NotificationTarget{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).Preload("Channel").First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 渠道构建失败（类型未注册、渠道级配置非法）同样作为通知行为报错正常返回
	ch, err := notify.New(target.Channel.Type, json.RawMessage(target.Channel.ConfigData()))
	if err != nil {
		notificationTestSendFailed(c, err)
		return
	}
	// 按目标偏好的语言渲染测试消息，语言缺失或非法时回退平台默认语言
	lang := base.DefaultLanguage
	if target.Language != "" {
		if parsed, err := base.ParseLanguage(target.Language); err == nil {
			lang = parsed
		}
	}
	params := map[string]any{
		"ChannelName": target.Channel.Name,
		"ChannelType": target.Channel.Type,
		"TargetName":  target.Name,
	}
	title := translate.Localize(lang, "notify.test.title", params)
	content := translate.Localize(lang, "notify.test.content", params)
	msgID, err := ch.Send(c, &notify.Target{
		Name:     target.Name,
		Language: target.Language,
		Config:   json.RawMessage(target.ConfigData()),
	}, &notify.RenderedMessage{Title: title, Content: content})
	if err != nil {
		notificationTestSendFailed(c, err)
		return
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTargetTestSendOut{
		MessageID: msgID,
		Title:     title,
		Content:   content,
		Language:  lang.String(),
		Channel: NotificationTargetTestSendChannelOut{
			Name:    target.Channel.Name,
			Type:    target.Channel.Type,
			Enabled: target.Channel.Enabled,
			Config:  target.Channel.Config,
		},
		Target: NotificationTargetTestSendTargetOut{
			Name:     target.Name,
			Language: target.Language,
			Enabled:  target.Enabled,
			Config:   target.Config,
		},
	})
}

// notificationTestSendFailed 以正常业务错误响应通知发送失败。
// 使用 400 + CodeNotificationSendFailed，details 为具体失败原因（渠道错误消息），
// 供前端直接在 UI 中向用户展示，而非 500 未知错误。
func notificationTestSendFailed(c *gin.Context, err error) {
	response.CreateCodeError(http.StatusBadRequest, response.CodeNotificationSendFailed, err).AbortWithGinH(c)
}

// NotificationTargetSend 处理"向单个通知目标发送"的请求。
// 按 (id, namespace) 预加载渠道校验目标存在且属于当前命名空间（缺失 404），
// 目标或所属渠道被禁用时返回 400 + CodeNotificationSendFailed（复用 notificationTestSendFailed），
// 通过后组装 notify.SendTargetInput 并投递 notify:send:target 异步任务（投递失败 500）。
// NotificationTargetSend handles the "send a notification to a single target" request.
// It validates the target exists and belongs to the current namespace via (id, namespace)
// with its channel preloaded (404 when missing), returns 400 + CodeNotificationSendFailed
// (reusing notificationTestSendFailed) when the target or its channel is disabled,
// then builds a notify.SendTargetInput and enqueues a notify:send:target async task
// (500 on enqueue failure).
func NotificationTargetSend(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	target := core.NotificationTarget{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).Preload("Channel").First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 目标自身被禁用，或所属渠道缺失 / 被禁用时，目标不可投递，以正常业务错误返回
	if !target.Enabled || target.Channel.ID == 0 || !target.Channel.Enabled {
		notificationTestSendFailed(c, errors.New("notification: target or its channel is disabled"))
		return
	}
	var in NotificationTargetSendIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	input := notify.SendTargetInput{
		Namespace: namespace,
		TargetID:  target.ID,
		EventType: in.EventType,
		EventID:   in.EventID,
		Message: notify.Message{
			Template: in.Template,
			Params:   in.Params,
		},
	}
	task, err := notifyhandler.NewSendTargetTask(input)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	enq := middlewares.GetPipelineClient(c)
	if _, err = enq.EnqueueContext(c, task, asynq.MaxRetry(3)); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTargetSendOut{
		TargetID:  target.ID,
		EventType: in.EventType,
		EventID:   in.EventID,
	})
}

// notificationTargetDetailOut 将通知目标与所属渠道转换为详情输出结构。
func notificationTargetDetailOut(target *core.NotificationTarget, channel *core.NotificationChannel) NotificationTargetDetailOut {
	out := NotificationTargetDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        target.ID,
			CreatedAt: target.CreatedAt,
			UpdatedAt: target.UpdatedAt,
		},
		Name:        target.Name,
		ChannelID:   target.ChannelID,
		Type:        target.Type,
		Enabled:     target.Enabled,
		Config:      target.Config,
		Language:    target.Language,
		Description: target.Description,
	}
	if channel != nil && channel.ID != 0 {
		out.ChannelName = channel.Name
		out.ChannelType = channel.Type
		out.ChannelEnabled = channel.Enabled
	}
	return out
}
