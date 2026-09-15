// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签识别规则管理相关的 API 路由处理器，
// 提供规则的 CRUD、启停、语法校验、dry-run、版本列表与回滚能力。
//
// 一致性设计（写路径）：
//   - 任何规则写操作（创建/更新/删除/启停/回滚）都必须先通过规则校验；
//   - DB 事务提交后，先本地失效引擎缓存（Engine.Invalidate），
//     再通过 Engine.BroadcastChange 广播受影响分组的变更事件，
//     各进程引擎监听到事件后失效对应分组缓存并按 DB 最新规则懒重编译。
package views

import (
	// stdlib

	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/open-policy-agent/opa/v1/rego"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// invalidTagRuleError 构造标签规则校验失败的错误响应（HTTP 400，携带结构化校验错误详情）。
// RegoError 明细按请求语言（Accept-Language）本地化后输出。
func invalidTagRuleError(c *gin.Context, errs []tagengine.RegoError) *response.CodeError {
	if errs == nil {
		errs = []tagengine.RegoError{}
	}
	return response.CreateCodeError(http.StatusBadRequest, response.CodeTagRuleInvalid, localizeRegoErrors(response.GetLocalizer(c), errs))
}

// localizeRegoErrors 按请求语言的 Localizer 翻译 RegoError 列表的 Message/Hint。
// tagengine 生成的条目 Message/Hint 为 i18n 消息键：Message 翻译后与 Args 渲染，
// Hint 翻译后直接替换；OPA 原生消息不是消息键，翻译不命中时原样保留。
func localizeRegoErrors(lc *i18n.Localizer, errs []tagengine.RegoError) []tagengine.RegoError {
	out := make([]tagengine.RegoError, len(errs))
	for i, e := range errs {
		if localized, err := lc.Localize(&i18n.LocalizeConfig{MessageID: e.Message}); err == nil && localized != "" {
			if len(e.Args) > 0 {
				e.Message = fmt.Sprintf(localized, e.Args...)
			} else {
				e.Message = localized
			}
		}
		if e.Hint != "" {
			if hint, err := lc.Localize(&i18n.LocalizeConfig{MessageID: e.Hint}); err == nil && hint != "" {
				e.Hint = hint
			}
		}
		out[i] = e
	}
	return out
}

// localizeInputFields 按请求语言的 Localizer 翻译输入字段元信息的 Description。
// tagengine 为每个字段标注 i18n 消息键（input.<typeKey>.<field>），这里按请求语言
// 翻译后填充 Description；翻译失败时保留空串。
func localizeInputFields(lc *i18n.Localizer, fields []tagengine.InputFieldMeta) []tagengine.InputFieldMeta {
	out := make([]tagengine.InputFieldMeta, len(fields))
	for i, field := range fields {
		if field.DescriptionKey != "" {
			if desc, err := lc.Localize(&i18n.LocalizeConfig{MessageID: field.DescriptionKey}); err == nil && desc != "" {
				field.Description = desc
			}
		}
		out[i] = field
	}
	return out
}

// localizeDimensionOptions 把模型注册的合法维度取值列表转换为下拉选项。
// 仅返回 dimension 取值本身（Value，即 snake_case 列名）；展示 label 由前端统一经
// tagEventDimension 枚举类按 enum.tagEventDimension.<取值> 翻译（与详情列/通知模板一致），
// 未命中时由前端回退展示取值。
func localizeDimensionOptions(dims []string) []TagRuleModelDimensionOut {
	out := make([]TagRuleModelDimensionOut, 0, len(dims))
	for _, d := range dims {
		out = append(out, TagRuleModelDimensionOut{Value: d})
	}
	return out
}

// invalidTagRuleRuntimeError 构造标签规则运行期样例校验失败的错误响应（HTTP 400，携带样例错误列表）。
func invalidTagRuleRuntimeError(issues []tagengine.SampleRuntimeError) *response.CodeError {
	return response.CreateCodeError(http.StatusBadRequest, response.CodeTagRuleRuntimeInvalid, issues)
}

// invalidTagRuleModelNotRegisteredError 构造标签规则目标模型未注册的错误响应（HTTP 400）。
// 未注册模型的规则引擎永不加载（按实际数据实例的类型键分组），写入时直接拒绝避免死规则。
func invalidTagRuleModelNotRegisteredError(model string) *response.CodeError {
	return response.CreateCodeError(http.StatusBadRequest, response.CodeTagRuleModelNotRegistered, model)
}

// TagRuleCreateIn 定义创建标签规则的请求输入结构。
// 规则名称在同一个命名空间内的分组（model）中唯一，命名空间取自请求上下文。
type TagRuleCreateIn struct {
	// Name 规则名称，仅允许字母、数字、下划线、连字符，长度 1~128
	Name string `json:"name" binding:"required,min=1,max=128,alphanumspecial=_-"`
	// Description 规则描述，说明规则意图，最长 512 字符
	Description string `json:"description" binding:"max=512"`
	// Model 目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Model string `json:"model" binding:"required,max=255"`
	// Rego Rego 规则源码，遵循 package zki + tags contains 契约，最长 64KiB
	Rego string `json:"rego" binding:"required,max=65536"`
	// Enabled 是否启用，默认启用
	Enabled bool `json:"enabled" default:"true"`
}

// TagRuleUpdateIn 定义更新标签规则的请求输入结构。
// 规则的分组标识（Name/Model）不可变，仅允许调整描述、内容与启用状态。
type TagRuleUpdateIn struct {
	// Description 规则描述，最长 512 字符
	Description string `json:"description" binding:"max=512"`
	// Rego Rego 规则源码，遵循 package zki + tags contains 契约，最长 64KiB
	Rego string `json:"rego" binding:"required,max=65536"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
}

// TagRuleBaseDetailOut 定义标签规则的基础详情输出结构。
// 包含规则的标识字段与审计字段，用于列表和详情场景的公共部分。
type TagRuleBaseDetailOut struct {
	utils.BaseItemOut
	// Name 规则名称
	Name string `json:"name"`
	// Description 规则描述
	Description string `json:"description"`
	// Model 目标模型类型键
	Model string `json:"model"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
}

// TagRuleDetailOut 定义标签规则的完整详情输出结构。
// 在基础详情之上附加 Rego 全文与版本统计信息。
type TagRuleDetailOut struct {
	TagRuleBaseDetailOut
	// Rego Rego 规则源码全文
	Rego string `json:"rego"`
	// Version 最新历史版本号，0 表示尚无历史版本
	Version uint `json:"version"`
	// VersionCount 历史版本总数
	VersionCount int64 `json:"versionCount"`
}

// TagRuleListOut 定义标签规则列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type TagRuleListOut struct {
	TagRuleBaseDetailOut
	utils.BaseTotalOut
}

// TagRuleVersionOut 定义标签规则历史版本的输出结构。
// 存储规则在快照时刻的完整内容。
type TagRuleVersionOut struct {
	utils.BaseItemOut
	// RuleID 所属标签规则 ID
	RuleID uint `json:"ruleId"`
	// Version 版本号，同一规则内从 1 递增
	Version uint `json:"version"`
	// Name 快照时刻的规则名称
	Name string `json:"name"`
	// Description 快照时刻的规则描述
	Description string `json:"description"`
	// Model 快照时刻的目标模型类型键
	Model string `json:"model"`
	// Rego 快照时刻的规则源码
	Rego string `json:"rego"`
}

// TagRuleValidateIn 定义标签规则语法校验的请求输入结构。
type TagRuleValidateIn struct {
	// Rego Rego 规则源码，最长 64KiB
	Rego string `json:"rego" binding:"required,max=65536"`
	// Model 目标模型类型键，可空；非空且已注册时启用字段感知 lint（未知 input 字段进入 warnings）
	Model string `json:"model"`
}

// TagRuleValidateOut 定义标签规则语法校验的结果输出结构。
type TagRuleValidateOut struct {
	// Valid 是否通过语法与契约校验
	Valid bool `json:"valid"`
	// Errors 校验错误列表（带行列号），合法时为空数组
	Errors []tagengine.RegoError `json:"errors"`
	// Warnings 字段感知 lint 的告警列表（warning 级），合法时为空数组
	Warnings []tagengine.RegoError `json:"warnings"`
}

// TagRuleModelIn 定义标签规则目标模型相关接口的请求输入结构。
type TagRuleModelIn struct {
	// Model 目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Model string `json:"model" binding:"required"`
}

// TagRuleModelMetadataOut 定义标签规则目标模型输入字段元信息的输出结构。
type TagRuleModelMetadataOut struct {
	// Model 目标模型类型键
	Model string `json:"model"`
	// Fields 输入文档扁平化字段元信息列表（按字段名排序）
	Fields []tagengine.InputFieldMeta `json:"fields"`
	// Dimensions 该模型合法标签维度选项列表（供前端 dimension 下拉使用），
	// 与 tagengine 注册表及 ClickHouse 物化视图的展开列保持一致的单一事实来源
	Dimensions []TagRuleModelDimensionOut `json:"dimensions"`
}

// TagRuleModelDimensionOut 标签维度下拉选项：Value 为 dimension 取值
// （snake_case 列名，对应 tag_summings.dimension 的存储值）。
// 展示名不在此输出：前端持有目标模型对应的 i18n 命名空间，按 Value 自行拼键翻译，
// 避免后端烤入具体命名空间前缀造成前后端耦合。
type TagRuleModelDimensionOut struct {
	Value string `json:"value"`
}

// TagRuleModelSampleOut 定义标签规则目标模型系统样例的输出结构。
type TagRuleModelSampleOut struct {
	// Name 样例名称
	Name string `json:"name"`
	// Description 样例说明
	Description string `json:"description"`
	// Input 样例输入文档（目标模型实例的 JSON 反序列化结果）
	Input map[string]any `json:"input"`
}

// TagRuleModelSamplesOut 定义标签规则目标模型系统样例列表的输出结构。
type TagRuleModelSamplesOut struct {
	// Model 目标模型类型键
	Model string `json:"model"`
	// Items 样例列表
	Items []*TagRuleModelSampleOut `json:"items"`
}

// TagRuleFunctionOut 定义自定义 OPA 内置函数的元信息输出项。
// Description 已按请求语言翻译；Example 为可插入编辑器的用法示例文本。
type TagRuleFunctionOut struct {
	// Name 函数全名（如 sqlite.query），rego 规则中直接引用
	Name string `json:"name"`
	// Description 函数说明（按请求语言翻译后的文本）
	Description string `json:"description"`
	// Signature 人类可读的调用签名（如 sqlite.query(sql, params)）
	Signature string `json:"signature"`
	// Example 用法示例文本
	Example string `json:"example"`
}

// TagRuleFunctionsOut 定义自定义 OPA 内置函数元信息列表的输出结构。
// 供前端 Rego 编辑器实现自定义函数的关键词高亮、悬停签名提示与自动补全。
type TagRuleFunctionsOut struct {
	// Functions 自定义函数元信息列表
	Functions []TagRuleFunctionOut `json:"functions"`
}

// TagRuleSqliteColumnOut 定义 sqlite vtable 数据源的一个列元信息。
type TagRuleSqliteColumnOut struct {
	// Name 列名（与数据源 schema 声明一致）
	Name string `json:"name"`
	// Type 值类型（string/number/boolean/blob/any）
	Type string `json:"type"`
}

// TagRuleSqliteDatasourceOut 定义已注册为 sqlite vtable 的第三方数据源元信息。
type TagRuleSqliteDatasourceOut struct {
	// Name 数据源名（rego 中 sqlite.query 引用的裸名表名）
	Name string `json:"name"`
	// Columns 该数据源的列定义列表
	Columns []TagRuleSqliteColumnOut `json:"columns"`
}

// TagRuleSqliteMetaOut 定义 sqlite 语句编辑器元信息的输出结构。
// 供前端 Rego 编辑器对 sqlite.query 内的 SQL 语句实现表名/列名补全与提示。
type TagRuleSqliteMetaOut struct {
	// Datasources 当前命名空间已注册数据源的 schema 列表
	Datasources []TagRuleSqliteDatasourceOut `json:"datasources"`
}

// TagRuleDryRunRegoIn 定义 dry-run 的单条候选规则输入结构。
// Name 允许为空：dry-run 面向草稿阶段，规则可能尚未命名，为空时经 defaults 库以占位名兜底。
type TagRuleDryRunRegoIn struct {
	// Name 候选规则名称（可为空，dry-run 不落库，缺省以 "draft" 兜底）
	Name string `json:"name" default:"draft" binding:"omitempty,min=1,max=128,alphanumspecial=_-"`
	// Rego 候选规则 Rego 源码，最长 64KiB
	Rego string `json:"rego" binding:"required,max=65536"`
}

// TagRuleDryRunRuleIn 定义标签规则单规则 dry-run 的请求输入结构。
// 仅针对一条候选规则（不落库、不影响引擎缓存），经 CompileRule 编译后评估，
// input 为规则评估所需的 JSON 输入文档。
type TagRuleDryRunRuleIn struct {
	TagRuleDryRunRegoIn
	// Model 目标模型类型键
	Model string `json:"model" binding:"required"`
	// Input 规则评估的 JSON 输入文档
	Input map[string]any `json:"input" binding:"required"`
}

// TagRuleDryRunGroupIn 定义标签规则规则组 dry-run 的请求输入结构。
// 针对一组候选规则（不落库、不影响引擎缓存），经 CompileGroup 编译后评估，
// input 为规则评估所需的 JSON 输入文档。
type TagRuleDryRunGroupIn struct {
	// Model 目标模型类型键
	Model string `json:"model" binding:"required"`
	// Rules 候选规则列表
	Rules []*TagRuleDryRunRegoIn `json:"rules" binding:"required,min=1"`
	// Input 规则评估的 JSON 输入文档
	Input map[string]any `json:"input" binding:"required"`
}

// TagRuleDryRunRuleOut 单条规则的 dry-run 评估输出。
type TagRuleDryRunRuleOut struct {
	// Tags 该规则产出的标签（去重排序）
	Tags []string `json:"tags"`
	// Duration 该规则评估耗时
	Duration serializer.DurationNS `json:"duration"`
	// Error 该规则评估的错误消息，为空表示评估成功
	Error string `json:"error"`
}

// TagRuleDryRunOut 定义标签规则 dry-run 的结果输出结构。
// 仅暴露错误消息文本，不返回错误对象，避免错误堆栈等实现细节泄露。
type TagRuleDryRunOut struct {
	// Tags 全部规则产出的合并标签（去重排序）
	Tags []string `json:"tags"`
	// Error 组级合并评估的错误消息，为空表示评估成功
	Error string `json:"error"`
	// Rules 逐规则评估详情，键为规则名
	Rules map[string]*TagRuleDryRunRuleOut `json:"rules"`
}

// TagRuleListGet 处理获取标签规则列表的请求。
// 支持分页、排序与过滤（时间/名称/目标类型/启用状态），仅返回当前命名空间内的规则。
func TagRuleListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagRuleListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*TagRuleListOut
	db := middlewares.GetCoreDatabase(c).Model(&tag.TagRule{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **TagRuleListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// TagRuleDetailGet 处理获取标签规则详情的请求。
// 返回规则的完整内容与最新历史版本号、版本总数；仅允许访问当前命名空间内的规则。
func TagRuleDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := tag.TagRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 单条 SQL 同时统计历史版本总数与最新版本号（Where 条件一致，避免两次查询）
	var stat struct {
		VersionCount int64 `gorm:"column:version_count"`
		MaxVersion   uint  `gorm:"column:max_version"`
	}
	if err := db.WithContext(c).Model(&tag.TagRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", instance.ID, namespace).
		Select("COUNT(*) AS version_count, COALESCE(MAX(version), 0) AS max_version").
		Scan(&stat).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, TagRuleDetailOut{
		TagRuleBaseDetailOut: newTagRuleBaseDetailOut(&instance),
		Rego:                 instance.Rego,
		Version:              stat.MaxVersion,
		VersionCount:         stat.VersionCount,
	})
}

// TagRuleCreate 处理创建标签规则的请求。
// 创建前执行语法校验与组级编译校验；规则归属当前请求命名空间；成功后本地失效缓存并发布 etcd 变更事件。
func TagRuleCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagRuleCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := &tag.TagRule{
		NamespaceName: namespace,
		Name:          in.Name,
		Description:   in.Description,
		Model:         in.Model,
		Rego:          in.Rego,
		Enabled:       in.Enabled,
	}
	// 语法与组级校验：任何调整都必须通过规则校验，让客户及早得知规则是否正确
	if !validateTagRuleCandidate(c, instance) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	if err := db.WithContext(c).Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 写路径后置动作：本地失效 + 全局广播
	notifyTagRuleChange(c, instance)
	response.SimpleOK.AbortWithGinH(c)
}

// TagRuleUpdate 处理更新标签规则的请求。
// 更新前先将当前内容快照为新版本；随后校验并落库；成功后失效缓存并广播变更。
// 仅允许更新当前命名空间内的规则。
func TagRuleUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	var in TagRuleUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := tag.TagRule{}
	db := middlewares.GetCoreDatabase(c)
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 构造候选内容用于校验
	candidate := instance
	candidate.Description = in.Description
	candidate.Rego = in.Rego
	candidate.Enabled = in.Enabled
	if !validateTagRuleCandidate(c, &candidate) {
		return
	}
	// 事务内：快照当前内容 → 更新规则 → 裁剪超限版本
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if err := snapshotTagRuleVersion(tx, &instance); err != nil {
			return err
		}
		if err := tx.Model(&tag.TagRule{}).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Updates(map[string]any{
			"description": candidate.Description,
			"rego":        candidate.Rego,
			"enabled":     candidate.Enabled,
		}).Error; err != nil {
			return err
		}
		return pruneTagRuleVersions(tx, instance.ID, namespace)
	}); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	notifyTagRuleChange(c, &candidate)
	response.SimpleOK.AbortWithGinH(c)
}

// TagRuleBatchIn 批量操作标签规则的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type TagRuleBatchIn struct {
	Ids FlexList[uint64] `json:"ids"`
}

// TagRuleBatchDelete 批量删除标签规则。
// 逐条复用原 tagRuleDelete 的事务级联删版本 + notifyTagRuleChange 语义；
// 规则不存在（或不属于当前命名空间）视为成功跳过（幂等）。
func TagRuleBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagRuleBatchIn
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
			instance := tag.TagRule{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
				// 显式级联删除历史版本
				if err := tx.Where("rule_id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagRuleVersion{}).Error; err != nil {
					return err
				}
				return tx.Where("id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&tag.TagRule{}).Error
			}); err != nil {
				return err
			}
			notifyTagRuleChange(c, &instance)
			return nil
		},
	)
}

// TagRuleBatchSetEnabled 批量启用/停用标签规则。
// 逐条复用原 tagRuleSetEnabled 语义：
//   - 规则不存在（或不属于当前命名空间）→ 视为成功跳过（幂等）；
//   - 已处于目标状态 → 幂等成功；
//   - 启用时校验"变更后的组"可编译，校验失败该条记入 Failed（跳过）；
//   - 更新成功后 notifyTagRuleChange。
func TagRuleBatchSetEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, ok := middlewares.RequireNamespace(c)
		if !ok {
			return
		}
		var in TagRuleBatchIn
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
				instance := tag.TagRule{}
				if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil // 不存在视为成功跳过
					}
					return errors.WithStack(err)
				}
				if instance.Enabled == enabled {
					return nil // 幂等短路
				}
				// 启用时该规则将进入组编译集合，需校验合并组可编译；校验失败记入失败原因
				if enabled {
					candidate := instance
					candidate.Enabled = true
					if err := validateTagRuleCandidateBatch(c, &candidate); err != nil {
						return err
					}
				}
				if err := db.WithContext(c).Model(&tag.TagRule{}).
					Where("id = ? AND namespace_name = ?", instance.ID, namespace).
					Update("enabled", enabled).Error; err != nil {
					return err
				}
				instance.Enabled = enabled
				notifyTagRuleChange(c, &instance)
				return nil
			},
		)
	}
}

// TagRuleValidate 处理标签规则语法校验的请求。
// 仅校验传入的 Rego 源码，返回结构化错误（带行列号）；HTTP 200 表达业务结果。
// 复用公共 Rego 校验（ValidateRegoRules），与写路径/dry-run 校验语义一致。
// 请求携带已注册的目标模型（model）时，追加字段感知 lint（LintInputRefs）：
// 引用不存在的 input 字段产出 warning 级告警进入 Warnings，不阻断合法结果；
// model 为空或未注册时 Warnings 为空数组。
func TagRuleValidate(c *gin.Context) {
	var in TagRuleValidateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	rule := &tag.TagRule{Name: "candidate", Rego: in.Rego}
	errs, valid := tagengine.ValidateRegoRules(c, []*tag.TagRule{rule})
	if errs == nil {
		errs = []tagengine.RegoError{}
	}
	lc := response.GetLocalizer(c)
	out := TagRuleValidateOut{Valid: valid, Errors: localizeRegoErrors(lc, errs), Warnings: []tagengine.RegoError{}}
	if in.Model != "" && tagengine.IsRegistered(in.Model) {
		if fields, err := tagengine.InputMetadata(in.Model); err != nil {
			// 元信息推导失败仅记录告警，不阻断校验结果（lint 属于增强能力）
			middlewares.GetLogger(c).Warnf(c, nil,
				"tag rule validate: load input metadata failed, model=%s, err=%v", in.Model, err)
		} else if warnings := tagengine.LintInputRefs(in.Rego, fields); len(warnings) > 0 {
			out.Warnings = localizeRegoErrors(lc, warnings)
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// TagRuleModelMetadata 处理获取标签规则目标模型输入字段元信息的请求。
// 返回输入文档扁平化字段元信息列表（按字段名排序，Description 按请求语言翻译）
// 与合法标签维度选项列表；供前端 Rego 编辑器自动补全与 dimension 下拉使用；
// 未注册模型返回 400 + 错误码 1107。
func TagRuleModelMetadata(c *gin.Context) {
	var in TagRuleModelIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	fields, err := tagengine.InputMetadata(in.Model)
	if err != nil {
		if errors.Is(err, tagengine.ErrModelNotRegistered) {
			invalidTagRuleModelNotRegisteredError(in.Model).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	lc := response.GetLocalizer(c)
	response.SimpleOK.AbortWithGinW(c, TagRuleModelMetadataOut{
		Model:      in.Model,
		Fields:     localizeInputFields(lc, fields),
		Dimensions: localizeDimensionOptions(tagengine.Dimensions(in.Model)),
	})
}

// TagRuleModelSamples 处理获取标签规则目标模型系统样例列表的请求。
// 模型在当前命名空间下无样例时懒供给（default 零值文档，HttpTransport 额外预置 http-example）后返回；
// 样例按命名空间隔离，仅返回当前请求命名空间的样例；
// 样例输入文档反序列化为 map；未注册模型返回 400 + 错误码 1107。
func TagRuleModelSamples(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in TagRuleModelIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	samples, err := tagengine.EnsureModelSamples(c, middlewares.GetCoreDatabase(c), namespace, in.Model)
	if err != nil {
		if errors.Is(err, tagengine.ErrModelNotRegistered) {
			invalidTagRuleModelNotRegisteredError(in.Model).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*TagRuleModelSampleOut, 0, len(samples))
	for _, sample := range samples {
		input := map[string]any{}
		if err := json.Unmarshal(sample.Input, &input); err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		items = append(items, &TagRuleModelSampleOut{
			Name:        sample.Name,
			Description: sample.Description,
			Input:       input,
		})
	}
	response.SimpleOK.AbortWithGinW(c, TagRuleModelSamplesOut{Model: in.Model, Items: items})
}

// TagRuleFunctions 处理获取 rego 规则可调用的自定义 OPA 内置函数元信息的请求。
// 返回函数名、说明（按请求语言翻译）、调用签名与用法示例；
// 供前端 Rego 编辑器实现自定义函数的关键词高亮、悬停签名提示与自动补全。
func TagRuleFunctions(c *gin.Context) {
	funcs := tagengine.CustomFunctions()
	lc := response.GetLocalizer(c)
	items := make([]TagRuleFunctionOut, 0, len(funcs))
	for _, f := range funcs {
		desc := f.Description
		if text, err := lc.Localize(&i18n.LocalizeConfig{MessageID: f.Description}); err == nil && text != "" {
			desc = text
		}
		items = append(items, TagRuleFunctionOut{
			Name:        f.Name,
			Description: desc,
			Signature:   f.Signature,
			Example:     f.Example,
		})
	}
	response.SimpleOK.AbortWithGinW(c, TagRuleFunctionsOut{Functions: items})
}

// TagRuleSqliteMeta 处理获取当前命名空间 sqlite vtable 数据源 schema 元信息的请求。
// 返回已注册数据源的 {name, columns} 列表，供前端 Rego 编辑器对 sqlite.query 内的
// SQL 语句实现表名/列名补全与提示。
func TagRuleSqliteMeta(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	pool := middlewares.GetSQLiteVTabPools(c).Pool(namespace)
	schemas := pool.DatasourceSchemas()
	items := make([]TagRuleSqliteDatasourceOut, 0, len(schemas))
	for _, s := range schemas {
		cols := make([]TagRuleSqliteColumnOut, 0, len(s.Columns))
		for _, col := range s.Columns {
			cols = append(cols, TagRuleSqliteColumnOut{Name: col.Name, Type: sqliteValueTypeName(col.ValueType)})
		}
		items = append(items, TagRuleSqliteDatasourceOut{Name: s.Name, Columns: cols})
	}
	response.SimpleOK.AbortWithGinW(c, TagRuleSqliteMetaOut{Datasources: items})
}

// sqliteValueTypeName 将 sqlitevtab.ValueType 映射为前端展示用类型名。
// 整型/浮点/布尔统一归为 number（对规则作者而言都是数值比较）。
func sqliteValueTypeName(t sqlitevtab.ValueType) string {
	switch t {
	case sqlitevtab.ValueTypeString:
		return "string"
	case sqlitevtab.ValueTypeInt64, sqlitevtab.ValueTypeFloat64, sqlitevtab.ValueTypeBool:
		return "number"
	case sqlitevtab.ValueTypeBlob:
		return "blob"
	default:
		return "any"
	}
}

// withTagRuleSQLitePool 将当前请求命名空间的 sqlite vtable 池绑定到 ctx，
// 使 dry-run / 写路径运行期样例校验中的 sqlite.query 可查询该命名空间的裸名 vtable。
// 命名空间缺失或池不可用时原样返回（sqlite.query 将按未绑池语义处理）。
func withTagRuleSQLitePool(c *gin.Context) context.Context {
	namespace, err := middlewares.GetNamespace(c)
	if err != nil {
		return c
	}
	return tagengine.WithDatasourcePool(c, middlewares.GetSQLiteVTabPools(c).Pool(namespace))
}

// TagRuleDryRun 处理单规则 dry-run 的请求。
// 仅针对一条候选规则（rego），先经公共 Rego 校验（语法 + 契约 + 组级编译），
// 校验通过后再经 CompileRule 编译并评估输入文档。
// 评估前按请求命名空间绑定第三方数据源池，规则内的 sqlite.query 可查询裸名 vtable。
// 输出仅含错误消息文本，不返回错误对象，避免错误堆栈等实现细节泄露。
func TagRuleDryRun(c *gin.Context) {
	var in TagRuleDryRunRuleIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 草稿阶段规则可能尚未命名，name 缺省经 defaults 库以 "draft" 兜底（仅用于输出键，不落库）
	if err := defaults.Set(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	rule := &tag.TagRule{
		Name:    in.Name,
		Model:   in.Model,
		Rego:    in.Rego,
		Enabled: true,
	}
	if errs, ok := tagengine.ValidateRegoRules(c, []*tag.TagRule{rule}); !ok {
		invalidTagRuleError(c, errs).AbortWithGinH(c)
		return
	}
	ctx := withTagRuleSQLitePool(c)
	if _, cr, err := tagengine.CompileRule(ctx, rule); err != nil {
		invalidTagRuleError(c, tagengine.ExtractRegoErrors(err)).AbortWithGinH(c)
		return
	} else {
		res := cr.RecognizeWithDetail(ctx, rego.EvalInput(in.Input))
		response.SimpleOK.AbortWithGinW(c, &TagRuleDryRunOut{
			Tags:  res.Tags,
			Rules: map[string]*TagRuleDryRunRuleOut{in.Name: newTagRuleDryRunRuleOut(res)},
		})
	}
}

// TagRuleDryRunGroup 处理规则组 dry-run 的请求。
// 针对一组候选规则（rules），先经公共 Rego 校验（逐条语法契约 + 整组语义编译），
// 校验通过后再经 CompileGroup 编译并评估输入文档，
// 评估前按请求命名空间绑定第三方数据源池，规则内的 sqlite.query 可查询裸名 vtable。
// 输出逐规则详情与合并标签。
// 输出仅含错误消息文本，不返回错误对象，避免错误堆栈等实现细节泄露。
func TagRuleDryRunGroup(c *gin.Context) {
	var in TagRuleDryRunGroupIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	rules := make([]*tag.TagRule, 0, len(in.Rules))
	for _, item := range in.Rules {
		rules = append(rules, &tag.TagRule{
			Name:    item.Name,
			Model:   in.Model,
			Rego:    item.Rego,
			Enabled: true,
		})
	}
	if errs, ok := tagengine.ValidateRegoRules(c, rules); !ok {
		invalidTagRuleError(c, errs).AbortWithGinH(c)
		return
	}
	ctx := withTagRuleSQLitePool(c)
	group, err := tagengine.CompileGroup(ctx, rules)
	if err != nil {
		invalidTagRuleError(c, tagengine.ExtractRegoErrors(err)).AbortWithGinH(c)
		return
	}
	result := group.RecognizeWithDetail(ctx, rego.EvalInput(in.Input))
	response.SimpleOK.AbortWithGinW(c, newTagRuleDryRunOut(result))
}

// newTagRuleDryRunOut 将 tagengine 组级评估详情转换为仅含错误消息的 dry-run 输出。
func newTagRuleDryRunOut(res *tagengine.GroupDetailResult) *TagRuleDryRunOut {
	out := &TagRuleDryRunOut{
		Tags:  res.Tags,
		Rules: make(map[string]*TagRuleDryRunRuleOut, len(res.Rules)),
	}
	if res.Error != nil {
		out.Error = res.Error.Error()
	}
	for name, item := range res.Rules {
		out.Rules[name] = newTagRuleDryRunRuleOut(item)
	}
	return out
}

// newTagRuleDryRunRuleOut 将单条规则的评估详情转换为仅含错误消息的 dry-run 输出。
func newTagRuleDryRunRuleOut(res *tagengine.RuleDetailResult) *TagRuleDryRunRuleOut {
	out := &TagRuleDryRunRuleOut{
		Tags:     res.Tags,
		Duration: res.Duration,
	}
	if res.Error != nil {
		out.Error = res.Error.Error()
	}
	return out
}

// TagRuleVersionList 处理获取标签规则历史版本列表的请求。
// 返回该规则的全部历史版本，按版本号倒序；仅允许访问当前命名空间内的规则版本。
func TagRuleVersionList(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 确认规则存在且属于当前命名空间
	var count int64
	if err := db.WithContext(c).Model(&tag.TagRule{}).
		Where("id = ? AND namespace_name = ?", id, namespace).Count(&count).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count == 0 {
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
		return
	}
	var versions []*TagRuleVersionOut
	if err := db.WithContext(c).Model(&tag.TagRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", id, namespace).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, gin.H{"items": versions})
}

// TagRuleVersionRollback 处理标签规则回滚的请求。
// 将规则回滚到指定历史版本：回滚前先快照当前内容，再把目标版本内容写回，
// 校验通过后落库，最后失效缓存并广播变更。
// 仅允许回滚当前命名空间内的规则。
func TagRuleVersionRollback(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
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
		response.CreateBadUrlParamError("badUrlParam", "version", err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := tag.TagRule{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	target := tag.TagRuleVersion{}
	if err := db.WithContext(c).
		Where("rule_id = ? AND version = ? AND namespace_name = ?", id, version, namespace).
		First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 校验回滚后的内容（语法 + 合并组编译）
	candidate := instance
	candidate.Name = target.Name
	candidate.Description = target.Description
	candidate.Model = target.Model
	candidate.Rego = target.Rego
	if !validateTagRuleCandidate(c, &candidate) {
		return
	}
	// 事务内：快照当前内容 → 回滚目标版本内容
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		if err := snapshotTagRuleVersion(tx, &instance); err != nil {
			return err
		}
		return tx.Model(&tag.TagRule{}).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Updates(map[string]any{
			"name":        candidate.Name,
			"description": candidate.Description,
			"model":       candidate.Model,
			"rego":        candidate.Rego,
		}).Error
	}); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	notifyTagRuleChange(c, &candidate)
	response.SimpleOK.AbortWithGinH(c)
}

// newTagRuleBaseDetailOut 将 TagRule 转换为基础详情输出结构。
func newTagRuleBaseDetailOut(rule *tag.TagRule) TagRuleBaseDetailOut {
	return TagRuleBaseDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        rule.ID,
			CreatedAt: rule.CreatedAt,
			UpdatedAt: rule.UpdatedAt,
		},
		Name:        rule.Name,
		Description: rule.Description,
		Model:       rule.Model,
		Enabled:     rule.Enabled,
	}
}

// validateTagRuleCandidate 校验一条候选规则：先确认目标模型已注册（未注册直接拒绝），
// 再基于"变更后的组"（DB 现有启用规则 + 候选）做 Rego 校验（语法/契约/组级编译，
// 复用 ValidateRegoRules），最后做运行期样例校验。
// 任一校验失败时向客户端返回带行列号的错误详情并返回 false。
func validateTagRuleCandidate(c *gin.Context, candidate *tag.TagRule) bool {
	if err := validateTagRuleCandidateBatch(c, candidate); err != nil {
		if ce, ok := err.(*response.CodeError); ok {
			// 结构化错误（未注册模型 / Rego 校验失败 / 运行期样例失败）按原样响应
			ce.AbortWithGinH(c)
		} else {
			response.CreateAccidentError(err).AbortWithGinH(c)
		}
		return false
	}
	return true
}

// validateTagRuleCandidateBatch 校验一条候选规则并返回错误（nil 表示通过）。
// 逻辑与 validateTagRuleCandidate 完全一致，但以 error 形式返回失败原因，
// 供批量启停等"失败汇总"场景使用，避免直接中断请求。
func validateTagRuleCandidateBatch(c *gin.Context, candidate *tag.TagRule) error {
	// 目标模型必须已在引擎注册表中登记：未注册模型的规则引擎永不加载
	// （按实际数据实例的类型键分组），属于死规则，写入时直接拒绝
	if !tagengine.IsRegistered(candidate.Model) {
		return invalidTagRuleModelNotRegisteredError(candidate.Model)
	}
	db := middlewares.GetCoreDatabase(c)
	group, err := buildTagRuleGroup(c, db, candidate)
	if err != nil {
		return err
	}
	// Rego 校验（语法 + 契约 + 组级语义编译），与 validate / dry-run 统一复用公共函数
	if errs, ok := tagengine.ValidateRegoRules(c, group); !ok {
		return invalidTagRuleError(c, errs)
	}
	// 第二层校验：基于验证样例做运行期评估，任一样例评估报错即判定运行期不合法。
	// 评估前按命名空间绑定第三方数据源池，使含 sqlite.query 的规则在样例校验时可查询
	// 该命名空间的裸名 vtable。
	if issues, err := tagengine.ValidateGroupRuntime(withTagRuleSQLitePool(c), db, group); err != nil {
		if errors.Is(err, tagengine.ErrSampleNotProvisioned) {
			// 该模型尚未预置验证样例：运行期校验降级跳过，记录告警后放行
			// （规则仍受第一层语法/契约校验保障，样例补充后即恢复运行期保障）
			middlewares.GetLogger(c).Warnf(c, nil,
				"tag rule runtime validation skipped: no samples provisioned, model=%s", candidate.Model)
		} else {
			return err
		}
	} else if len(issues) > 0 {
		return invalidTagRuleRuntimeError(issues)
	}
	return nil
}

// buildTagRuleGroup 构建分组（namespace + model）下"变更后"将参与编译的启用规则集合：
// 从 DB 读取当前命名空间内同组现有启用规则（排除候选规则自身），候选规则启用时并入集合。
// 用于写路径的组级校验，与引擎实际加载的规则集合保持一致。
func buildTagRuleGroup(ctx *gin.Context, db *database.DB, candidate *tag.TagRule) ([]*tag.TagRule, error) {
	query := db.WithContext(ctx).
		Where("namespace_name = ? AND model = ? AND enabled = ?", candidate.NamespaceName, candidate.Model, true)
	if candidate.ID != 0 {
		query = query.Where("id != ?", candidate.ID)
	}
	var rules []*tag.TagRule
	if err := query.Find(&rules).Error; err != nil {
		return nil, err
	}
	if candidate.Enabled {
		rules = append(rules, candidate)
	}
	return rules, nil
}

// notifyTagRuleChange 写路径后置动作：先本地失效引擎缓存保证本进程即时一致，
// 再广播受影响分组的变更事件通知其他进程；广播失败仅记录告警，不阻断已提交的变更。
// 缓存与广播均按当前请求命名空间隔离。
func notifyTagRuleChange(c *gin.Context, rule *tag.TagRule) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	engine := middlewares.GetTagRuleEngine(c)
	engine.Invalidate(namespace, rule.Model)
	if err := engine.BroadcastChange(c, namespace, rule.Model); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "broadcast tag rule change failed, rule=%d, err=%v", rule.ID, err)
	}
}

// snapshotTagRuleVersion 在事务内将规则当前内容快照为一条新版本记录（版本号 = MAX + 1）。
// 快照同时记录所属命名空间，保证版本数据与规则同租户隔离。
func snapshotTagRuleVersion(tx *database.DB, rule *tag.TagRule) error {
	var maxVersion uint
	if err := tx.Model(&tag.TagRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", rule.ID, rule.NamespaceName).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error; err != nil {
		return err
	}
	version := &tag.TagRuleVersion{
		RuleID:        rule.ID,
		NamespaceName: rule.NamespaceName,
		Version:       maxVersion + 1,
		Name:          rule.Name,
		Description:   rule.Description,
		Model:         rule.Model,
		Rego:          rule.Rego,
	}
	return tx.Create(version).Error
}

// pruneTagRuleVersions 裁剪规则的历史版本，仅保留最新 MaxTagRuleVersions 条。
func pruneTagRuleVersions(tx *database.DB, ruleID uint, namespace string) error {
	var versions []uint
	if err := tx.Model(&tag.TagRuleVersion{}).
		Where("rule_id = ? AND namespace_name = ?", ruleID, namespace).
		Order("version DESC").
		Offset(tag.MaxTagRuleVersions).
		Pluck("version", &versions).Error; err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}
	return tx.Where("rule_id = ? AND namespace_name = ? AND version IN ?", ruleID, namespace, versions).
		Delete(&tag.TagRuleVersion{}).Error
}
