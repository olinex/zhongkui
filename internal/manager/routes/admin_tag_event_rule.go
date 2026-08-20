// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签事件规则管理相关的 API 路由处理器，
// 提供规则的 CRUD、启停、表达式语法校验、dry-run 与版本列表/回滚能力。
//
// 一致性设计（写路径）：
//   - 任何规则写操作（创建/更新/回滚）都必须先通过逐条件表达式校验；
//   - 更新/回滚前先将当前内容（规则头 + 全部条件 JSON 快照）保存为一条新版本记录
//     （版本号 = MAX + 1），超出 MaxTagEventRuleVersions 上限时自动裁剪最旧版本。
package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// TagEventRuleValidationError 描述单条标签事件条件的校验失败详情。
// Index 为条件在规则中的下标，Message 为该条件表达式校验失败的错误消息。
type TagEventRuleValidationError struct {
	// Index 条件在规则中的下标（从 0 开始）
	Index int `json:"index"`
	// Message 表达式校验失败的错误消息
	Message string `json:"message"`
}

// invalidTagEventRuleError 构造标签事件规则校验失败的错误响应（HTTP 400，携带逐条件校验错误详情）。
func invalidTagEventRuleError(errs []TagEventRuleValidationError) *manager.CodeError {
	if errs == nil {
		errs = []TagEventRuleValidationError{}
	}
	return manager.CreateCodeError(http.StatusBadRequest, manager.CodeTagEventRuleInvalid, errs)
}

// TagEventRuleConditionIn 定义单条标签事件条件的请求输入结构。
// 表达式为标签标识符的布尔组合（AND / OR / NOT / 括号），标签由 TagRule 识别产生。
type TagEventRuleConditionIn struct {
	// Model 目标模型类型键过滤条件，为空表示不限制（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Model string `json:"model" binding:"max=255"`
	// Dimension 标签维度类型过滤条件，为空表示不限制（如 "fp_tcp_client_sig"）
	Dimension string `json:"dimension" binding:"max=128"`
	// Item 标签维度具体值过滤条件，为空表示不限制（如具体的指纹签名串）
	Item string `json:"item" binding:"max=255"`
	// Name 条件名称，必填，长度 1~255
	Name string `json:"name" binding:"required,min=1,max=255"`
	// Window 时间窗口，必须大于 0
	Window serializer.DurationNS `json:"window" binding:"gt=0"`
	// Threshold 阈值，必须大于 0
	Threshold int `json:"threshold" binding:"gt=0"`
	// Expression 标签组合布尔表达式，必填，最长 1024 字符
	Expression string `json:"expression" binding:"required,max=1024"`
	// Enabled 是否启用该条件，默认启用
	Enabled bool `json:"enabled" default:"true"`
	// Description 条件描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// TagEventRuleCreateIn 定义创建标签事件规则的请求输入结构。
// 命名空间取自请求上下文，规则名无需在命名空间内唯一（条件维度进一步区分触发语义）。
type TagEventRuleCreateIn struct {
	// Name 规则名称，必填，长度 1~255
	Name string `json:"name" binding:"required,min=1,max=255"`
	// Description 规则描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
	// MatchAll 是否要求规则下所有条件均命中才触发事件；为 false（默认）时任一条件命中即触发
	MatchAll bool `json:"matchAll" default:"false"`
	// Enabled 是否启用，默认启用
	Enabled bool `json:"enabled" default:"true"`
	// Conditions 条件列表，至少一条
	Conditions []*TagEventRuleConditionIn `json:"conditions" binding:"required,min=1,dive"`
}

// TagEventRuleUpdateIn 定义更新标签事件规则的请求输入结构。
// 规则名称不可变，仅允许调整描述、匹配策略、启用状态与条件集合。
type TagEventRuleUpdateIn struct {
	// Description 规则描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
	// MatchAll 是否要求规则下所有条件均命中才触发事件
	MatchAll bool `json:"matchAll"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Conditions 条件列表，至少一条（更新后完全替换既有条件）
	Conditions []*TagEventRuleConditionIn `json:"conditions" binding:"required,min=1,dive"`
}

// TagEventRuleConditionOut 定义单条标签事件条件的输出结构。
// 嵌入 BaseItemOut 携带审计字段，业务字段与 TagEventRuleConditionIn 一一对应。
type TagEventRuleConditionOut struct {
	utils.BaseItemOut
	// Model 目标模型类型键过滤条件
	Model string `json:"model"`
	// Dimension 标签维度类型过滤条件
	Dimension string `json:"dimension"`
	// Item 标签维度具体值过滤条件
	Item string `json:"item"`
	// Name 条件名称
	Name string `json:"name"`
	// Window 时间窗口
	Window serializer.DurationNS `json:"window"`
	// Threshold 阈值
	Threshold int `json:"threshold"`
	// Expression 标签组合布尔表达式
	Expression string `json:"expression"`
	// Enabled 是否启用该条件
	Enabled bool `json:"enabled"`
	// Description 条件描述
	Description string `json:"description"`
}

// TagEventRuleBaseDetailOut 定义标签事件规则的基础详情输出结构。
// 包含规则的标识字段与审计字段，用于列表和详情场景的公共部分。
type TagEventRuleBaseDetailOut struct {
	utils.BaseItemOut
	// Name 规则名称
	Name string `json:"name"`
	// Description 规则描述
	Description string `json:"description"`
	// MatchAll 是否要求所有条件均命中才触发事件
	MatchAll bool `json:"matchAll"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
}

// TagEventRuleDetailOut 定义标签事件规则的完整详情输出结构。
// 在基础详情之上附加全部条件与版本统计信息。
type TagEventRuleDetailOut struct {
	TagEventRuleBaseDetailOut
	// Conditions 规则下的全部条件
	Conditions []*TagEventRuleConditionOut `json:"conditions"`
	// Version 最新历史版本号，0 表示尚无历史版本
	Version uint `json:"version"`
	// VersionCount 历史版本总数
	VersionCount int64 `json:"versionCount"`
}

// TagEventRuleListOut 定义标签事件规则列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type TagEventRuleListOut struct {
	TagEventRuleBaseDetailOut
	utils.BaseTotalOut
}

// TagEventRuleVersionOut 定义标签事件规则历史版本的输出结构。
// 存储规则在快照时刻的完整内容，条件从 JSON 文本快照解析而来。
type TagEventRuleVersionOut struct {
	utils.BaseItemOut
	// RuleID 所属标签事件规则 ID
	RuleID uint `json:"ruleId"`
	// Version 版本号，同一规则内从 1 递增
	Version uint `json:"version"`
	// Name 快照时刻的规则名称
	Name string `json:"name"`
	// MatchAll 快照时刻是否要求所有条件均命中才触发事件
	MatchAll bool `json:"matchAll"`
	// Description 快照时刻的规则描述
	Description string `json:"description"`
	// Conditions 快照时刻的全部条件（从 JSON 文本快照解析）
	Conditions []*TagEventRuleConditionOut `json:"conditions"`
}

// TagEventRuleValidateIn 定义标签事件规则表达式语法校验的请求输入结构。
type TagEventRuleValidateIn struct {
	// Expression 待校验的标签组合布尔表达式
	Expression string `json:"expression" binding:"required"`
}

// TagEventRuleValidateOut 定义标签事件规则表达式语法校验的结果输出结构。
type TagEventRuleValidateOut struct {
	// Valid 是否通过语法校验
	Valid bool `json:"valid"`
	// Refined 校验通过时精炼后的标准表达式（去除多余空格）
	Refined string `json:"refined"`
	// Error 校验失败时的错误消息，合法时为空
	Error string `json:"error"`
}

// TagEventRuleDryRunIn 定义标签事件规则 dry-run 的请求输入结构。
// 针对一条候选规则（不落库），在指定时间点执行一次事件检测并返回命中事件。
type TagEventRuleDryRunIn struct {
	// Name 候选规则名称，必填，长度 1~255
	Name string `json:"name" binding:"required,min=1,max=255"`
	// MatchAll 是否要求所有条件均命中才触发事件，默认 false
	MatchAll bool `json:"matchAll" default:"false"`
	// Enabled 是否启用，默认启用；停用时直接返回空事件列表
	Enabled bool `json:"enabled" default:"true"`
	// Time 检测时间点，可选；零值时使用服务器当前时间
	Time time.Time `json:"time"`
	// Conditions 候选条件列表，至少一条
	Conditions []*TagEventRuleConditionIn `json:"conditions" binding:"required,min=1,dive"`
}

// TagEventRuleDryRunOut 定义标签事件规则 dry-run 的结果输出结构。
type TagEventRuleDryRunOut struct {
	// RuleName 规则名称
	RuleName string `json:"ruleName"`
	// MatchAll 是否要求所有条件均命中才触发事件
	MatchAll bool `json:"matchAll"`
	// DetectedAt 检测时间点
	DetectedAt time.Time `json:"detectedAt"`
	// Events 检测命中的标签事件列表
	Events []*tagengine.TagEvent `json:"events"`
}

// tagEventRuleListGet 处理获取标签事件规则列表的请求。
// 支持分页、排序与过滤（时间/名称/匹配策略/启用状态），仅返回当前命名空间内的规则。
func tagEventRuleListGet(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagEventRuleListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*TagEventRuleListOut
	db := manager.GetCoreDatabase(c).Model(&tag.TagEventRule{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinW(c, utils.NewSearchOut(items))
}

// tagEventRuleDetailGet 处理获取标签事件规则详情的请求。
// 返回规则的完整内容（含全部条件）与最新历史版本号、版本总数；仅允许访问当前命名空间内的规则。
func tagEventRuleDetailGet(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	instance := tag.TagEventRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	conds, err := loadTagEventRuleConditions(db, instance.ID, namespace)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 单条 SQL 同时统计历史版本总数与最新版本号（Where 条件一致，避免两次查询）
	var stat struct {
		VersionCount int64 `gorm:"column:version_count"`
		MaxVersion   uint  `gorm:"column:max_version"`
	}
	if err := db.WithContext(c).Model(&tag.TagEventRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", instance.ID, namespace).
		Select("COUNT(*) AS version_count, COALESCE(MAX(version), 0) AS max_version").
		Scan(&stat).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	conditionOuts := make([]*TagEventRuleConditionOut, 0, len(conds))
	for _, cond := range conds {
		out := newTagEventRuleConditionOut(cond)
		conditionOuts = append(conditionOuts, &out)
	}
	manager.SimpleOK.AbortWithGinW(c, TagEventRuleDetailOut{
		TagEventRuleBaseDetailOut: newTagEventRuleBaseDetailOut(&instance),
		Conditions:                conditionOuts,
		Version:                   stat.MaxVersion,
		VersionCount:              stat.VersionCount,
	})
}

// tagEventRuleCreate 处理创建标签事件规则的请求。
// 创建前逐条件校验表达式语法；规则归属当前请求命名空间，条件在同一事务内随规则一并落库。
func tagEventRuleCreate(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagEventRuleCreateIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := &tag.TagEventRule{
		NamespaceName: namespace,
		Name:          in.Name,
		Description:   in.Description,
		MatchAll:      in.MatchAll,
		Enabled:       in.Enabled,
	}
	// 逐条件表达式校验：任何非法表达式都拒绝创建，让客户及早得知规则是否正确
	if !validateTagEventRuleConditions(c, in.Conditions) {
		return
	}
	conds := newTagEventRuleConditions(in.Conditions)
	db := manager.GetCoreDatabase(c)
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if err := tx.Create(instance).Error; err != nil {
			return err
		}
		for _, cond := range conds {
			cond.TagEventRuleID = instance.ID
			cond.NamespaceName = namespace
			if err := tx.Create(cond).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagEventRuleUpdate 处理更新标签事件规则的请求。
// 更新前先将当前内容（含全部条件）快照为新版本；随后逐条件校验并重建条件集合。
// 仅允许更新当前命名空间内的规则。
func tagEventRuleUpdate(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	instance := tag.TagEventRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var in TagEventRuleUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 构造候选内容用于校验
	candidate := instance
	candidate.Description = in.Description
	candidate.MatchAll = in.MatchAll
	candidate.Enabled = in.Enabled
	if !validateTagEventRuleConditions(c, in.Conditions) {
		return
	}
	newConds := newTagEventRuleConditions(in.Conditions)
	// 事务内：快照当前内容 → 更新规则头 → 重建条件集合 → 裁剪超限版本
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		currentConds, err := loadTagEventRuleConditions(tx, instance.ID, namespace)
		if err != nil {
			return err
		}
		if err := snapshotTagEventRuleVersion(tx, &instance, currentConds); err != nil {
			return err
		}
		if err := tx.Model(&tag.TagEventRule{}).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Updates(map[string]any{
			"description": candidate.Description,
			"match_all":   candidate.MatchAll,
			"enabled":     candidate.Enabled,
		}).Error; err != nil {
			return err
		}
		// 显式删除既有条件（兼容未开启外键约束的数据库，如测试用 sqlite 内存库）
		if err := tx.Where("tag_event_rule_id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagEventCondition{}).Error; err != nil {
			return err
		}
		for _, cond := range newConds {
			cond.TagEventRuleID = instance.ID
			cond.NamespaceName = namespace
		}
		if err := tx.Create(&newConds).Error; err != nil {
			return err
		}
		return pruneTagEventRuleVersions(tx, instance.ID, namespace)
	}); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagEventRuleDelete 处理删除标签事件规则的请求。
// 在事务内先显式删除条件与历史版本，再删除规则本身。
// 仅允许删除当前命名空间内的规则。
func tagEventRuleDelete(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	instance := tag.TagEventRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		// 显式级联删除条件与历史版本，兼容未开启外键约束的数据库（如测试用 sqlite 内存库）
		if err := tx.Where("tag_event_rule_id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagEventCondition{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagEventRuleVersion{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagEventRule{}).Error
	}); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagEventRuleSetEnabled 返回启用/停用标签事件规则的处理函数。
// 幂等返回：规则已处于目标状态时直接成功；事件规则不涉及引擎编译，直接更新启用状态。
// 仅允许操作当前命名空间内的规则。
func tagEventRuleSetEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, ok := manager.RequireNamespace(c)
		if !ok {
			return
		}
		id, ok := getUint64Param(c, "id")
		if !ok {
			return
		}
		db := manager.GetCoreDatabase(c)
		instance := tag.TagEventRule{}
		if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				manager.ErrDefaultPageNotFound.AbortWithGinH(c)
				return
			}
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		if instance.Enabled == enabled {
			manager.SimpleOK.AbortWithGinH(c)
			return
		}
		if err := db.WithContext(c).Model(&tag.TagEventRule{}).
			Where("id = ? AND namespace_name = ?", instance.ID, namespace).
			Update("enabled", enabled).Error; err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		manager.SimpleOK.AbortWithGinH(c)
	}
}

// tagEventRuleValidate 处理标签事件规则表达式语法校验的请求。
// 仅校验传入的表达式，返回精炼后的标准表达式或错误消息；HTTP 200 表达业务结果。
func tagEventRuleValidate(c *gin.Context) {
	var in TagEventRuleValidateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	out := TagEventRuleValidateOut{}
	if expr, err := tag.ParseBoolExpr(in.Expression); err != nil {
		out.Valid = false
		out.Error = err.Error()
	} else {
		out.Valid = true
		out.Refined = expr.String()
	}
	manager.SimpleOK.AbortWithGinW(c, out)
}

// tagEventRuleVersionList 处理获取标签事件规则历史版本列表的请求。
// 返回该规则的全部历史版本，按版本号倒序；仅允许访问当前命名空间内的规则版本。
func tagEventRuleVersionList(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	// 确认规则存在且属于当前命名空间
	var count int64
	if err := db.WithContext(c).Model(&tag.TagEventRule{}).
		Where("id = ? AND namespace_name = ?", id, namespace).Count(&count).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count == 0 {
		manager.ErrDefaultPageNotFound.AbortWithGinH(c)
		return
	}
	var records []*tag.TagEventRuleVersion
	if err := db.WithContext(c).Model(&tag.TagEventRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", id, namespace).
		Order("version DESC").
		Find(&records).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	versions := make([]*TagEventRuleVersionOut, 0, len(records))
	for _, record := range records {
		conds, err := unmarshalConditions(record.Conditions)
		if err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		versions = append(versions, &TagEventRuleVersionOut{
			BaseItemOut: utils.BaseItemOut{
				ID:        record.ID,
				CreatedAt: record.CreatedAt,
				UpdatedAt: record.UpdatedAt,
			},
			RuleID:      record.RuleID,
			Version:     record.Version,
			Name:        record.Name,
			MatchAll:    record.MatchAll,
			Description: record.Description,
			Conditions:  conds,
		})
	}
	manager.SimpleOK.AbortWithGinW(c, gin.H{"items": versions})
}

// tagEventRuleVersionRollback 处理标签事件规则回滚的请求。
// 将规则回滚到指定历史版本：回滚前先快照当前内容，再从目标版本解析并校验条件，
// 校验通过后落库并重建条件集合。
// 仅允许回滚当前命名空间内的规则。
func tagEventRuleVersionRollback(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	versionStr := c.Param("version")
	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		manager.CreateBadUrlParamError("badUrlParam", "version", err).AbortWithGinH(c)
		return
	}
	db := manager.GetCoreDatabase(c)
	instance := tag.TagEventRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	target := tag.TagEventRuleVersion{}
	if err := db.WithContext(c).
		Where("rule_id = ? AND version = ? AND namespace_name = ?", id, version, namespace).
		First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 从目标版本快照解析条件并逐条件校验表达式语法
	targetConds, err := unmarshalConditions(target.Conditions)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	condIns := make([]*TagEventRuleConditionIn, 0, len(targetConds))
	for _, cond := range targetConds {
		condIns = append(condIns, &TagEventRuleConditionIn{
			Model:       cond.Model,
			Dimension:   cond.Dimension,
			Item:        cond.Item,
			Name:        cond.Name,
			Window:      cond.Window,
			Threshold:   cond.Threshold,
			Expression:  cond.Expression,
			Enabled:     cond.Enabled,
			Description: cond.Description,
		})
	}
	if !validateTagEventRuleConditions(c, condIns) {
		return
	}
	// 构造回滚后的规则头与条件集合
	candidate := instance
	candidate.Name = target.Name
	candidate.MatchAll = target.MatchAll
	candidate.Description = target.Description
	newConds := newTagEventRuleConditions(condIns)
	// 事务内：快照当前内容 → 回滚目标版本内容 → 重建条件集合 → 裁剪超限版本
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		currentConds, err := loadTagEventRuleConditions(tx, instance.ID, namespace)
		if err != nil {
			return err
		}
		if err := snapshotTagEventRuleVersion(tx, &instance, currentConds); err != nil {
			return err
		}
		if err := tx.Model(&tag.TagEventRule{}).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Updates(map[string]any{
			"name":        candidate.Name,
			"match_all":   candidate.MatchAll,
			"description": candidate.Description,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("tag_event_rule_id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagEventCondition{}).Error; err != nil {
			return err
		}
		for _, cond := range newConds {
			cond.TagEventRuleID = instance.ID
			cond.NamespaceName = namespace
		}
		if err := tx.Create(&newConds).Error; err != nil {
			return err
		}
		return pruneTagEventRuleVersions(tx, instance.ID, namespace)
	}); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagEventRuleDryRun 处理标签事件规则 dry-run 的请求。
// 针对一条候选规则（不落库），先逐条件校验表达式，再于指定时间点执行一次事件检测；
// 仅将启用条件映射为 DetectionCondition 参与检测，禁用或空条件时直接返回空事件列表。
func tagEventRuleDryRun(c *gin.Context) {
	namespace, ok := manager.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagEventRuleDryRunIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 先校验全部条件表达式，失败直接返回，避免无效表达式进入检测
	if !validateTagEventRuleConditions(c, in.Conditions) {
		return
	}
	now := in.Time
	if now.IsZero() {
		now = time.Now()
	}
	out := TagEventRuleDryRunOut{
		RuleName:   in.Name,
		MatchAll:   in.MatchAll,
		DetectedAt: now,
		Events:     []*tagengine.TagEvent{},
	}
	if !in.Enabled {
		manager.SimpleOK.AbortWithGinW(c, out)
		return
	}
	// 仅启用条件参与检测；RuleID 置 0 表示 dry-run 候选规则
	conds := make([]tagengine.DetectionCondition, 0, len(in.Conditions))
	for index, cond := range in.Conditions {
		if !cond.Enabled {
			continue
		}
		conds = append(conds, tagengine.DetectionCondition{
			RuleID:        0,
			RuleName:      in.Name,
			MatchAll:      in.MatchAll,
			ConditionID:   uint(index + 1),
			ConditionName: cond.Name,
			Namespace:     namespace,
			Model:         cond.Model,
			Dimension:     cond.Dimension,
			Item:          cond.Item,
			Window:        cond.Window,
			Threshold:     cond.Threshold,
			Expression:    cond.Expression,
		})
	}
	if len(conds) == 0 {
		manager.SimpleOK.AbortWithGinW(c, out)
		return
	}
	events, err := tagengine.Detect(c.Request.Context(), manager.GetRawDatabase(c), conds, now)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if events == nil {
		events = []*tagengine.TagEvent{}
	}
	out.Events = events
	manager.SimpleOK.AbortWithGinW(c, out)
}

// newTagEventRuleBaseDetailOut 将 TagEventRule 转换为基础详情输出结构。
func newTagEventRuleBaseDetailOut(rule *tag.TagEventRule) TagEventRuleBaseDetailOut {
	return TagEventRuleBaseDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        rule.ID,
			CreatedAt: rule.CreatedAt,
			UpdatedAt: rule.UpdatedAt,
		},
		Name:        rule.Name,
		Description: rule.Description,
		MatchAll:    rule.MatchAll,
		Enabled:     rule.Enabled,
	}
}

// newTagEventRuleConditionOut 将 TagEventCondition 转换为条件输出结构。
func newTagEventRuleConditionOut(cond *tag.TagEventCondition) TagEventRuleConditionOut {
	return TagEventRuleConditionOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        cond.ID,
			CreatedAt: cond.CreatedAt,
			UpdatedAt: cond.UpdatedAt,
		},
		Model:       cond.Model,
		Dimension:   cond.Dimension,
		Item:        cond.Item,
		Name:        cond.Name,
		Window:      cond.Window,
		Threshold:   cond.Threshold,
		Expression:  cond.Expression,
		Enabled:     cond.Enabled,
		Description: cond.Description,
	}
}

// newTagEventRuleConditions 将请求输入的条件列表转换为标签事件条件模型列表。
// 归属字段（TagEventRuleID / NamespaceName）由调用方在落库前填充。
func newTagEventRuleConditions(in []*TagEventRuleConditionIn) []*tag.TagEventCondition {
	conds := make([]*tag.TagEventCondition, 0, len(in))
	for _, cond := range in {
		conds = append(conds, &tag.TagEventCondition{
			Model:       cond.Model,
			Dimension:   cond.Dimension,
			Item:        cond.Item,
			Name:        cond.Name,
			Window:      cond.Window,
			Threshold:   cond.Threshold,
			Expression:  cond.Expression,
			Enabled:     cond.Enabled,
			Description: cond.Description,
		})
	}
	return conds
}

// validateTagEventRuleConditions 逐条件校验表达式语法。
// 全部合法返回 true；任一非法时构造逐条件错误详情（Index=条件下标、Message=错误消息）
// 并中断请求后返回 false。
func validateTagEventRuleConditions(c *gin.Context, conds []*TagEventRuleConditionIn) bool {
	errs := make([]TagEventRuleValidationError, 0)
	for index, cond := range conds {
		if _, err := tagengine.TranslateExpression(cond.Expression); err != nil {
			errs = append(errs, TagEventRuleValidationError{Index: index, Message: err.Error()})
		}
	}
	if len(errs) == 0 {
		return true
	}
	invalidTagEventRuleError(errs).AbortWithGinH(c)
	return false
}

// tagEventConditionSnapshot 标签事件条件的 JSON 快照结构。
// json tag 与 TagEventRuleConditionOut 的业务字段一致，仅记录业务字段，不含审计字段。
type tagEventConditionSnapshot struct {
	Model       string              `json:"model"`
	Dimension   string              `json:"dimension"`
	Item        string              `json:"item"`
	Name        string              `json:"name"`
	Window      serializer.DurationNS `json:"window"`
	Threshold   int                 `json:"threshold"`
	Expression  string              `json:"expression"`
	Enabled     bool                `json:"enabled"`
	Description string              `json:"description"`
}

// marshalConditions 将条件模型列表序列化为 JSON 文本快照。
func marshalConditions(conds []*tag.TagEventCondition) (string, error) {
	snapshots := make([]*tagEventConditionSnapshot, 0, len(conds))
	for _, cond := range conds {
		snapshots = append(snapshots, &tagEventConditionSnapshot{
			Model:       cond.Model,
			Dimension:   cond.Dimension,
			Item:        cond.Item,
			Name:        cond.Name,
			Window:      cond.Window,
			Threshold:   cond.Threshold,
			Expression:  cond.Expression,
			Enabled:     cond.Enabled,
			Description: cond.Description,
		})
	}
	data, err := json.Marshal(snapshots)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(data), nil
}

// unmarshalConditions 将 JSON 文本快照解析为条件输出结构列表。
// 快照由 marshalConditions 生成，解析失败返回错误由调用方兜底。
func unmarshalConditions(text string) ([]*TagEventRuleConditionOut, error) {
	var conds []*TagEventRuleConditionOut
	if err := json.Unmarshal([]byte(text), &conds); err != nil {
		return nil, errors.WithStack(err)
	}
	return conds, nil
}

// snapshotTagEventRuleVersion 在事务内将规则当前内容（含全部条件）快照为一条新版本记录。
// 版本号 = COALESCE(MAX(version), 0) + 1，按规则与命名空间隔离；条件序列化失败返回错误。
func snapshotTagEventRuleVersion(tx *database.DB, rule *tag.TagEventRule, conds []*tag.TagEventCondition) error {
	var maxVersion uint
	if err := tx.Model(&tag.TagEventRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", rule.ID, rule.NamespaceName).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error; err != nil {
		return err
	}
	conditions, err := marshalConditions(conds)
	if err != nil {
		return err
	}
	version := &tag.TagEventRuleVersion{
		RuleID:        rule.ID,
		NamespaceName: rule.NamespaceName,
		Version:       maxVersion + 1,
		Name:          rule.Name,
		MatchAll:      rule.MatchAll,
		Description:   rule.Description,
		Conditions:    conditions,
	}
	return tx.Create(version).Error
}

// pruneTagEventRuleVersions 裁剪规则的历史版本，仅保留最新 MaxTagEventRuleVersions 条。
func pruneTagEventRuleVersions(tx *database.DB, ruleID uint, namespace string) error {
	var versions []uint
	if err := tx.Model(&tag.TagEventRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", ruleID, namespace).
		Order("version DESC").
		Offset(tag.MaxTagEventRuleVersions).
		Pluck("version", &versions).Error; err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}
	return tx.Where("rule_id = ? AND namespace_name = ? AND version IN ?", ruleID, namespace, versions).
		Delete(&tag.TagEventRuleVersion{}).Error
}

// loadTagEventRuleConditions 加载规则下的全部条件，按 ID 升序返回。
func loadTagEventRuleConditions(db *database.DB, ruleID uint, namespace string) ([]*tag.TagEventCondition, error) {
	var conds []*tag.TagEventCondition
	if err := db.Where("tag_event_rule_id = ? AND namespace_name = ?", ruleID, namespace).
		Order("id ASC").
		Find(&conds).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return conds, nil
}
