// Package notification 提供通知域（渠道 / 目标 / 组 / 模板 / 记录）的 API 处理器与视图 In/Out 结构体。
package notification

import (
	// 标准库

	"slices"
	"strconv"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	message "zhongkuisec.com/zhongkui/internal/pkg/message"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// NotificationChannelCreateIn 定义创建通知渠道的请求输入结构。
// 命名空间取自请求上下文，渠道名称在同一命名空间内唯一；渠道类型必须为
// message.Types() 中已注册的类型之一；Config 为渠道级配置的 JSON 文本。
type NotificationChannelCreateIn struct {
	// Name 渠道名称，必填，长度 1~128
	Name string `json:"name" binding:"required,min=1,max=128"`
	// Type 渠道类型键，必填，长度 1~32，必须为已注册类型之一（如 email / webhook）
	Type string `json:"type" binding:"required,max=32"`
	// Enabled 是否启用，默认为启用
	Enabled bool `json:"enabled" default:"true"`
	// Config 渠道级配置（JSON 文本），必填，结构随渠道类型不同而不同
	Config string `json:"config" binding:"required"`
	// Description 渠道描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// NotificationChannelUpdateIn 定义更新通知渠道的请求输入结构。
// 渠道名称与类型不可变（不接受二者），仅允许调整启用状态、配置与描述。
type NotificationChannelUpdateIn struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Config 渠道级配置（JSON 文本）
	Config string `json:"config"`
	// Description 渠道描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// NotificationChannelDetailOut 定义通知渠道的详情输出结构。
// 嵌入 BaseItemOut 携带审计字段，业务字段与模型字段一一对应。
type NotificationChannelDetailOut struct {
	utils.BaseItemOut
	// Name 渠道名称
	Name string `json:"name"`
	// Type 渠道类型键
	Type string `json:"type"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Config 渠道级配置的密文。密文由 gorm 模型经 crypto 序列化器落库/读取，
	// 本 Out 结构直接复用模型 *serializer.Crypto，经其 MarshalJSON 输出 base64 密文；
	// 字段经 form 标签标记为 crypto，由 meta options 告知前端该字段已加密（明文经明文权限接口查看）
	Config *serializer.Crypto `json:"config" form:"kind=crypto"`
	// Description 渠道描述
	Description string `json:"description"`
}

// NotificationChannelListOut 定义通知渠道列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type NotificationChannelListOut struct {
	NotificationChannelDetailOut
	utils.BaseTotalOut
}

// NotificationChannelBatchIn 批量操作通知渠道的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type NotificationChannelBatchIn struct {
	// Ids 待批量操作的渠道 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
}

// NotificationChannelTypeConfigSchema 描述某渠道类型的配置 schema。
// Channel 为渠道级配置字段列表，Target 为目标级配置字段列表；字段描述复用
// views.FieldMeta（与 OPTIONS 元信息同一反射事实来源），展示文案由前端依据
// 字段名与渠道类型自行翻译。
type NotificationChannelTypeConfigSchema struct {
	// Channel 渠道级配置字段列表
	Channel []*views.FieldMeta `json:"channel"`
	// Target 目标级配置字段列表
	Target []*views.FieldMeta `json:"target"`
}

// NotificationChannelTypeOut 描述单个通知渠道类型的枚举项。
type NotificationChannelTypeOut struct {
	// Type 渠道类型键
	Type string `json:"type"`
	// Name 渠道类型显示名称（当前为类型键本身）
	Name string `json:"name"`
	// Placeholder 是否为「占位符」渠道：渠道级配置无字段（如 webhook），仅标明目标类型，
	// 作为固定数据随命名空间自动种子化，前端应隐藏其创建入口且不可删除
	Placeholder bool `json:"placeholder"`
	// Config 该类型的渠道级 / 目标级配置 schema
	Config NotificationChannelTypeConfigSchema `json:"config"`
}

// NotificationChannelTypesOut 定义通知渠道类型枚举的输出结构。
type NotificationChannelTypesOut struct {
	// Types 已注册的通知渠道类型列表（升序）
	Types []NotificationChannelTypeOut `json:"types"`
}

// NotificationChannelTypesGet 处理获取通知渠道类型枚举的请求。
// 返回 message.Types() 中已注册的每个类型及其渠道级/目标级配置 schema，供前端
// 动态渲染创建/编辑表单。schema 依据 message 包各配置结构体的 binding/form 标签
// 经 views.GetStructFields 反射生成（与 OPTIONS 元信息同一事实来源）；字段展示
// 文案由前端依据字段名与渠道类型自行翻译。
func NotificationChannelTypesGet(c *gin.Context) {
	types := message.Types()
	results := make([]NotificationChannelTypeOut, 0, len(types))
	for _, typ := range types {
		out := NotificationChannelTypeOut{
			Type:        typ,
			Name:        typ,
			Placeholder: message.IsPlaceholderType(typ),
		}
		if fs, ok := message.FormStructsFor(typ); ok {
			out.Config.Channel = views.GetStructFields(fs.Channel)
			out.Config.Target = views.GetStructFields(fs.Target)
		}
		results = append(results, out)
	}
	response.SimpleOK.AbortWithGinW(c, NotificationChannelTypesOut{Types: results})
}

// NotificationChannelListGet 处理获取通知渠道列表的请求。
// 支持分页、排序与过滤（时间/名称/类型/启用状态），仅返回当前命名空间内的渠道。
// 查询按 gorm 模型扫描（config 经 crypto 序列化器读取），再映射为接口层 Out 结构。
func NotificationChannelListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in views.NotificationChannelListGetSearchIn
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
		core.NotificationChannel
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&core.NotificationChannel{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*NotificationChannelListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		detail := newNotificationChannelDetailOut(&rows[i].NotificationChannel)
		items = append(items, &NotificationChannelListOut{
			NotificationChannelDetailOut: detail,
			BaseTotalOut:                 utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **NotificationChannelListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// NotificationChannelCreate 处理创建通知渠道的请求。
// 创建前校验渠道类型必须为 message.Types() 中已注册的类型；渠道归属当前请求命名空间。
func NotificationChannelCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationChannelCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if !isSupportedNotificationChannelType(in.Type) {
		response.CreateBadBodyError(nil).AbortWithGinH(c)
		return
	}
	if message.IsPlaceholderType(in.Type) {
		// 占位符渠道（如 webhook）为系统固定数据，随命名空间自动种子化，禁止前端重复创建
		response.CreateBadBodyError(nil).AbortWithGinH(c)
		return
	}
	// 渠道级配置须能被渠道工厂反序列化并校验通过（如 email 的 port 必须是数字、
	// host/from 必填），避免非法配置（如以字符串 "465" 存 port）落库导致后继发送失败
	if _, err := message.New(in.Type, []byte(in.Config)); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          in.Name,
		Type:          in.Type,
		Enabled:       in.Enabled,
		Config:        serializer.NewCrypto([]byte(in.Config)),
		Description:   in.Description,
	}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationChannelDetailGet 处理获取通知渠道详情的请求。
// 仅允许访问当前命名空间内的渠道，不存在时返回 404。
func NotificationChannelDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := core.NotificationChannel{}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("id = ? AND namespace_name = ?", id, namespace).
		First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newNotificationChannelDetailOut(&instance))
}

// NotificationChannelUpdate 处理更新通知渠道的请求。
// 渠道名称与类型不可变，仅更新启用状态、配置与描述；仅允许更新当前命名空间内的渠道。
func NotificationChannelUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationChannel{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var in NotificationChannelUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 更新的配置同样须通过渠道工厂校验（类型不可变，沿用库内实例的类型），
	// 禁止非法配置（如字符串 port）覆盖已入库内容
	if _, err := message.New(instance.Type, []byte(in.Config)); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
	update := core.NotificationChannel{
		Enabled:     in.Enabled,
		Config:      serializer.NewCrypto([]byte(in.Config)),
		Description: in.Description,
	}
	if err := db.Model(&core.NotificationChannel{}).
		Where("id = ? AND namespace_name = ?", instance.ID, namespace).
		Select("enabled", "config", "description").
		Updates(&update).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationChannelBatchDelete 批量删除通知渠道。
// 逐条复用原 notificationChannelDelete 语义；渠道不存在（或不属于当前命名空间）
// 视为成功跳过（幂等）。
func NotificationChannelBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationChannelBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	views.BatchExecute(c, ids,
		func(id uint64) string { return strconv.FormatUint(id, 10) },
		func(id uint64) error {
			instance := core.NotificationChannel{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if message.IsPlaceholderType(instance.Type) {
				// 占位符渠道为系统固定数据，禁止删除
				return errors.New(response.LocalizeDetail(response.GetLocalizer(c), "notificationChannelBuiltin", instance.Name))
			}
			return db.WithContext(c).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&core.NotificationChannel{}).Error
		},
	)
}

// NotificationChannelBatchSetEnabled 批量启用/停用通知渠道。
// 逐条复用原 notificationChannelSetEnabled 语义：幂等短路（已处于目标状态直接成功）+ Update；
// 渠道不存在（或不属于当前命名空间）视为成功跳过（幂等）。
func NotificationChannelBatchSetEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, ok := middlewares.RequireNamespace(c)
		if !ok {
			return
		}
		var in NotificationChannelBatchIn
		if err := c.ShouldBindBodyWithJSON(&in); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		ids := []uint64(in.Ids)
		if !views.ValidFlexLen(c, ids) {
			return
		}
		db := middlewares.GetCoreDatabase(c)
		views.BatchExecute(c, ids,
			func(id uint64) string { return strconv.FormatUint(id, 10) },
			func(id uint64) error {
				instance := core.NotificationChannel{}
				if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil // 不存在视为成功跳过
					}
					return errors.WithStack(err)
				}
				if instance.Enabled == enabled {
					return nil // 幂等短路
				}
				return db.WithContext(c).Model(&core.NotificationChannel{}).
					Where("id = ? AND namespace_name = ?", instance.ID, namespace).
					Update("enabled", enabled).Error
			},
		)
	}
}

// isSupportedNotificationChannelType 判断渠道类型是否为 message.Types() 中已注册的类型。
func isSupportedNotificationChannelType(typ string) bool {
	return slices.Contains(message.Types(), typ)
}

// newNotificationChannelDetailOut 将 NotificationChannel 转换为基础的详情输出结构。
func newNotificationChannelDetailOut(channel *core.NotificationChannel) NotificationChannelDetailOut {
	return NotificationChannelDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        channel.ID,
			CreatedAt: channel.CreatedAt,
			UpdatedAt: channel.UpdatedAt,
		},
		Name:        channel.Name,
		Type:        channel.Type,
		Enabled:     channel.Enabled,
		Config:      channel.Config,
		Description: channel.Description,
	}
}

// init 注册通知渠道列表出参（NotificationChannelListOut.type）的渠道类型枚举类绑定，
// 枚举类由 message 包经 enum.RegisterSource 注册（类名 channelType，见 internal/pkg/message/enum.go）。
func init() {
	views.AddEnumBinding("NotificationChannelListOut", "type", "channelType")
}
