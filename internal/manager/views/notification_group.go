// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知目标组（NotificationGroup）管理相关的 API 路由处理器，
// 提供组的创建、详情、更新、批量删除、默认组设置与目标集合整体替换能力，
// 以及目标与渠道的关联查询。通知组为命名空间级资源，同一命名空间内名称唯一，
// 且每命名空间至多一个默认组（由部分唯一索引 uq_notification_groups_namespace_default 保证）。
package views

import (
	// stdlib
	"net/http"
	"strconv"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// NotificationGroupCreateIn 定义创建通知目标组的请求输入结构。
// 组名在命名空间内唯一（由唯一索引保证），命名空间取自请求上下文。
type NotificationGroupCreateIn struct {
	// Name 组名称，必填，长度 1~128
	Name string `json:"name" binding:"required,min=1,max=128"`
	// IsDefault 是否为命名空间默认组，默认 false
	IsDefault bool `json:"isDefault" default:"false"`
	// Description 组描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
	// TargetIds 创建时随组绑定的目标 ID 列表（可选，均须属于当前命名空间），
	// 缺省或空表示不绑定；在创建事务内完成绑定，保证组与关联原子落库
	TargetIds []uint64 `json:"targetIds"`
}

// NotificationGroupUpdateIn 定义更新通知目标组的请求输入结构。
// 组名称与默认标记不可在此更新，仅允许调整描述与绑定目标。
type NotificationGroupUpdateIn struct {
	// Description 组描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
	// TargetIds 目标 ID 列表（可选）：缺省（null）时保持既有绑定不变，
	// 提供数组（可为空）时整集替换，空数组表示解绑全部
	TargetIds *[]uint64 `json:"targetIds"`
}

// NotificationGroupTargetOut 定义通知详情中单条绑定目标的输出结构，
// 聚合目标与其所属渠道的名称与类型信息。
type NotificationGroupTargetOut struct {
	// ID 目标 ID
	ID uint `json:"id"`
	// Name 目标名称
	Name string `json:"name"`
	// Type 目标渠道类型键快照
	Type string `json:"type"`
	// ChannelID 目标所属的通知渠道 ID
	ChannelID uint `json:"channelId"`
	// ChannelName 目标所属渠道名称
	ChannelName string `json:"channelName"`
	// ChannelType 目标所属渠道类型键
	ChannelType string `json:"channelType"`
	// Enabled 目标是否启用
	Enabled bool `json:"enabled"`
	// Language 目标偏好的消息语言（base.Language）
	Language base.Language `json:"language"`
}

// NotificationGroupDetailOut 定义通知目标组详情的输出结构，
// 在基础审计字段之上附加业务字段与绑定目标列表。
type NotificationGroupDetailOut struct {
	utils.BaseItemOut
	// Name 组名称
	Name string `json:"name"`
	// IsDefault 是否为命名空间默认组
	IsDefault bool `json:"isDefault"`
	// Description 组描述
	Description string `json:"description"`
	// Targets 组内绑定的目标列表
	Targets []NotificationGroupTargetOut `json:"targets"`
}

// NotificationGroupListOut 定义通知目标组列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计，TargetCount 汇总组内目标数。
type NotificationGroupListOut struct {
	utils.BaseItemOut
	// Name 组名称
	Name string `json:"name"`
	// IsDefault 是否为命名空间默认组
	IsDefault bool `json:"isDefault"`
	// Description 组描述
	Description string `json:"description"`
	// TargetCount 组内绑定的目标总数
	TargetCount int64 `json:"targetCount"`
	// 分页总数统计
	utils.BaseTotalOut
}

// NotificationGroupBatchIn 批量操作通知目标组的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type NotificationGroupBatchIn struct {
	// Ids 组 ID 列表
	Ids FlexList[uint64] `json:"ids"`
}

// NotificationGroupListGet 处理获取通知目标组列表的请求。
// 支持分页、排序与过滤（时间/名称/默认标记），仅返回当前命名空间内的组，
// 并以单条聚合查询填充各组的目标计数。
func NotificationGroupListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationGroupListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	var items []*NotificationGroupListOut
	if query, err := utils.BuildSearchCountQuery(db.Model(&core.NotificationGroup{}).Where("namespace_name = ?", namespace), &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 单条聚合查询统计各组目标数，避免逐组查询
	if len(items) > 0 {
		groupIDs := make([]uint, 0, len(items))
		for _, item := range items {
			groupIDs = append(groupIDs, item.ID)
		}
		var counts []struct {
			GroupID uint  `gorm:"column:group_id"`
			Count   int64 `gorm:"column:count"`
		}
		if err := db.Model(&core.NotificationGroupTarget{}).
			Select("group_id, COUNT(*) AS count").
			Where("group_id IN ?", groupIDs).
			Group("group_id").
			Scan(&counts).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		countByGroup := make(map[uint]int64, len(counts))
		for _, entry := range counts {
			countByGroup[entry.GroupID] = entry.Count
		}
		for _, item := range items {
			item.TargetCount = countByGroup[item.ID]
		}
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **NotificationGroupListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// NotificationGroupDetailGet 处理获取通知目标组详情的请求。
// 返回组的业务字段与绑定目标列表（含关联渠道名称/类型）；
// 仅允许访问当前命名空间内的组。
func NotificationGroupDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationGroup{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var links []*core.NotificationGroupTarget
	if err := db.WithContext(c).Where("group_id = ?", instance.ID).Find(&links).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	targetOuts := make([]NotificationGroupTargetOut, 0, len(links))
	if len(links) > 0 {
		targetIDs := make([]uint, 0, len(links))
		for _, link := range links {
			targetIDs = append(targetIDs, link.TargetID)
		}
		var targets []*core.NotificationTarget
		if err := db.WithContext(c).Where("namespace_name = ? AND id IN ?", namespace, targetIDs).Find(&targets).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		targetByID := make(map[uint]*core.NotificationTarget, len(targets))
		channelIDs := make([]uint, 0, len(targets))
		for _, tgt := range targets {
			targetByID[tgt.ID] = tgt
			channelIDs = append(channelIDs, tgt.ChannelID)
		}
		// 批量加载目标所属渠道，映射 ID → 渠道信息
		channelByID := map[uint]*core.NotificationChannel{}
		if len(channelIDs) > 0 {
			var channels []*core.NotificationChannel
			if err := db.WithContext(c).Where("namespace_name = ? AND id IN ?", namespace, channelIDs).Find(&channels).Error; err != nil {
				response.CreateAccidentError(err).AbortWithGinH(c)
				return
			}
			for _, ch := range channels {
				channelByID[ch.ID] = ch
			}
		}
		for _, link := range links {
			tgt := targetByID[link.TargetID]
			if tgt == nil {
				continue
			}
			out := NotificationGroupTargetOut{
				ID:        tgt.ID,
				Name:      tgt.Name,
				Type:      tgt.Type,
				ChannelID: tgt.ChannelID,
				Enabled:   tgt.Enabled,
				Language:  parseTargetLanguage(tgt.Language),
			}
			if ch := channelByID[tgt.ChannelID]; ch != nil {
				out.ChannelName = ch.Name
				out.ChannelType = ch.Type
			}
			targetOuts = append(targetOuts, out)
		}
	}
	response.SimpleOK.AbortWithGinW(c, NotificationGroupDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        instance.ID,
			CreatedAt: instance.CreatedAt,
			UpdatedAt: instance.UpdatedAt,
		},
		Name:        instance.Name,
		IsDefault:   instance.IsDefault,
		Description: instance.Description,
		Targets:     targetOuts,
	})
}

// NotificationGroupCreate 处理创建通知目标组的请求。
// 组归属当前请求命名空间；若标记为默认组，则在事务内先取消命名空间内其他组的
// 默认标记，再创建新组（IsDefault=true 语义：新默认组替换旧默认组）。
func NotificationGroupCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationGroupCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := &core.NotificationGroup{
		NamespaceName: namespace,
		Name:          in.Name,
		IsDefault:     in.IsDefault,
		Description:   in.Description,
	}
	db := middlewares.GetCoreDatabase(c)
	// 事务内完成默认标记维护、组创建与目标绑定，保证组及其关联原子落库
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if in.IsDefault {
			// 设置新默认组需先取消其他组的默认标记，保证命名空间内至多一个默认组
			if err := tx.Model(&core.NotificationGroup{}).Where("namespace_name = ?", namespace).
				Update("is_default", false).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		if err := tx.Create(instance).Error; err != nil {
			return errors.WithStack(err)
		}
		// 创建时随组绑定目标（可选）
		if len(in.TargetIds) > 0 {
			if err := replaceNotificationGroupTargetsTx(tx, namespace, instance.ID, in.TargetIds); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, errNotificationGroupTargetInvalid) {
			// 目标缺失或跨命名空间：CreateBadBodyError 仅接受 validator.ValidationErrors，此处直接用 CodeBadBody 构造 400 响应
			response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, nil).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationGroupUpdate 处理更新通知目标组的请求。
// 更新描述字段；若请求携带 targetIds 则同步整集替换绑定目标（缺省保持既有绑定不变）；
// 仅允许更新当前命名空间内的组。
func NotificationGroupUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	var in NotificationGroupUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationGroup{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if err := tx.Model(&core.NotificationGroup{}).
			Where("id = ? AND namespace_name = ?", instance.ID, namespace).
			Updates(map[string]any{"description": in.Description}).Error; err != nil {
			return err
		}
		// 编辑时若携带 targetIds 则整集替换绑定目标（缺省不改动既有绑定）
		if in.TargetIds != nil {
			if err := replaceNotificationGroupTargetsTx(tx, namespace, instance.ID, *in.TargetIds); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, errNotificationGroupTargetInvalid) {
			// 目标缺失或跨命名空间：CreateBadBodyError 仅接受 validator.ValidationErrors，此处直接用 CodeBadBody 构造 400 响应
			response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, nil).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationGroupBatchDelete 批量删除通知目标组。
// 逐条复用事务删除语义：先显式删除组目标关联行，再删除组本身；组不存在（或不属于当前命名空间）
// 视为成功跳过（幂等）。
func NotificationGroupBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationGroupBatchIn
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
			instance := core.NotificationGroup{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			return db.WithContext(c).Transaction(func(tx *database.DB) error {
				// 显式删除组目标关联行
				if err := tx.Where("group_id = ?", instance.ID).Delete(&core.NotificationGroupTarget{}).Error; err != nil {
					return err
				}
				return tx.Where("id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&core.NotificationGroup{}).Error
			})
		},
	)
}

// NotificationGroupSetDefault 设置指定通知组为命名空间的默认组。
// 事务内先取消命名空间内其他组的默认标记，再将该组置为默认；仅允许操作当前命名空间内的组。
func NotificationGroupSetDefault(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationGroup{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if err := tx.Model(&core.NotificationGroup{}).Where("namespace_name = ?", namespace).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&core.NotificationGroup{}).
			Where("id = ? AND namespace_name = ?", instance.ID, namespace).
			Update("is_default", true).Error
	}); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// errNotificationGroupTargetInvalid 通知组绑定的目标 ID 不全部属于当前命名空间。
// 由 replaceNotificationGroupTargetsTx 返回，调用方映射为 400 业务错误。
var errNotificationGroupTargetInvalid = errors.New("notification group target not in namespace")

// replaceNotificationGroupTargetsTx 在事务内整集替换通知组的绑定目标集合。
// 目标 ID 全部必须属于当前命名空间，否则返回 errNotificationGroupTargetInvalid 且不产生任何写入；
// 空列表表示解绑全部既有目标。该逻辑被创建/编辑/设置目标三个入口复用，保证语义一致。
func replaceNotificationGroupTargetsTx(tx *database.DB, namespace string, groupID uint, targetIds []uint64) error {
	if len(targetIds) == 0 {
		return tx.Where("group_id = ?", groupID).Delete(&core.NotificationGroupTarget{}).Error
	}
	// 去重：同一命名空间内一组目标不重复绑定（与唯一索引语义一致）
	seen := make(map[uint64]struct{}, len(targetIds))
	dedup := make([]uint64, 0, len(targetIds))
	for _, tid := range targetIds {
		if _, exists := seen[tid]; exists {
			continue
		}
		seen[tid] = struct{}{}
		dedup = append(dedup, tid)
	}
	var found []*core.NotificationTarget
	if err := tx.Where("namespace_name = ? AND id IN ?", namespace, dedup).Find(&found).Error; err != nil {
		return err
	}
	if len(found) != len(dedup) {
		return errNotificationGroupTargetInvalid
	}
	// 事务内整集替换：删除既有关联行后插入新集合
	if err := tx.Where("group_id = ?", groupID).Delete(&core.NotificationGroupTarget{}).Error; err != nil {
		return err
	}
	for _, tid := range dedup {
		if err := tx.Create(&core.NotificationGroupTarget{GroupID: groupID, TargetID: uint(tid)}).Error; err != nil {
			return err
		}
	}
	return nil
}
