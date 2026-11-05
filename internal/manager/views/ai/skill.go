package ai

import (
	// 标准库
	"encoding/json"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// SkillInputFieldOut 定义技能输入字段的输出结构（独立于模型层，逐字段映射）。
type SkillInputFieldOut struct {
	// Name 字段名称，作为报告计划 inputValues 中的键
	Name string `json:"name"`
	// Type 字段类型，取值为 date / number / string / enum / boolean；nowTime 是字段名，其类型为 date
	Type string `json:"type"`
	// Required 该字段是否必填
	Required bool `json:"required"`
	// Description 字段用途说明，可为空
	Description string `json:"description"`
}

// AgentSkillItemOut 定义 AI 技能列表项的输出结构（独立于模型层，逐字段映射）。
// 列表不返回加密正文，正文仅在详情与创建 / 更新结果中给出。
type AgentSkillItemOut struct {
	// ID 技能 ID
	ID uint `json:"id"`
	// Name 技能名称（同一命名空间内唯一；内置技能为全局名称）
	Name string `json:"name"`
	// Source 技能来源：custom | builtin；builtin 由代码注册表声明，只读（可启停）
	Source string `json:"source"`
	// Description 技能描述（明文），便于在列表中识别技能用途
	Description string `json:"description"`
	// Enabled 技能是否启用；停用的技能不参与运行时装配
	Enabled bool `json:"enabled"`
	// PeriodicCapable 技能是否可周期执行（由用户声明）
	PeriodicCapable bool `json:"periodicCapable"`
	// InputFields 技能输入字段（由用户填写），供报告计划渲染输入表单；
	// 列表按 JSON 展示（form kind=json，经 OPTIONS 自省下发前端）
	InputFields []SkillInputFieldOut `json:"inputFields" gorm:"-" form:"kind=json"`
	// InputFieldsRaw 列表扫描用的原始 JSON 列，映射为 InputFields 后不直接对外输出
	InputFieldsRaw datatypes.JSON `json:"-" gorm:"column:input_fields"`
	// FixedFields 技能固定参数（由用户填写），保留各键的原始 JSON 类型；
	// 列表按 JSON 展示（form kind=json，经 OPTIONS 自省下发前端）
	FixedFields map[string]json.RawMessage `json:"fixedFields" gorm:"-" form:"kind=json"`
	// FixedFieldsRaw 列表扫描用的原始 JSON 列，映射为 FixedFields 后不直接对外输出
	FixedFieldsRaw datatypes.JSON `json:"-" gorm:"column:fixed_fields"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentSkillListOut 定义 AI 技能列表输出（嵌入 BaseTotalOut 支持分页总数）。
type AgentSkillListOut struct {
	AgentSkillItemOut
	utils.BaseTotalOut
}

// AgentSkillOptionOut 定义技能选择器的选项输出：内置与自定义技能统一以 agent_skills 行 ID
// 作为选择标识，选择器与聊天请求都以 ID 表达一次技能选择。
type AgentSkillOptionOut struct {
	// ID 技能 ID（agent_skills.id）
	ID uint `json:"id"`
	// Name 技能名称
	Name string `json:"name"`
	// Description 技能说明，可为空
	Description string `json:"description"`
}

// AgentSkillOptionListOut 定义技能选择器选项列表输出：内置技能在前、当前命名空间已启用
// 自定义技能在后；不返回技能正文。
type AgentSkillOptionListOut struct {
	// Items 合并后的技能选项
	Items []AgentSkillOptionOut `json:"items"`
}

// AgentSkillDetailOut 定义 AI 技能详情的输出结构（含解密后的技能正文）。
type AgentSkillDetailOut struct {
	// ID 技能 ID
	ID uint `json:"id"`
	// NamespaceName 所属命名空间
	NamespaceName string `json:"namespaceName"`
	// Source 技能来源：custom | builtin；builtin 只读（可启停）
	Source string `json:"source"`
	// Name 技能名称（同一命名空间内唯一）
	Name string `json:"name"`
	// Description 技能描述，便于在列表中识别技能用途
	Description string `json:"description"`
	// Content 技能正文内容（由 crypto 序列化器解密后输出明文）
	Content string `json:"content"`
	// Enabled 技能是否启用；停用的技能不参与运行时装配
	Enabled bool `json:"enabled"`
	// PeriodicCapable 技能是否可周期执行（由用户声明）
	PeriodicCapable bool `json:"periodicCapable"`
	// InputFields 技能输入字段（由用户填写），供报告计划渲染输入表单
	InputFields []SkillInputFieldOut `json:"inputFields"`
	// FixedFields 技能固定参数（由用户填写），保留各键的原始 JSON 类型
	FixedFields map[string]json.RawMessage `json:"fixedFields"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// SkillInputFieldIn 定义用户填写的技能输入字段：结构约束由 binding 校验，周期 nowTime 契约
// 与固定参数约束由模型层统一校验。
type SkillInputFieldIn struct {
	// Name 字段名称，必填（1..64 个字符，首尾不能有空白）
	Name string `json:"name" binding:"required,trimmed,max=64"`
	// Type 字段类型，取值为 date / number / string / enum / boolean
	Type modelsai.SkillInputFieldType `json:"type" binding:"required,oneof=date number string enum boolean"`
	// Required 该字段是否必填
	Required bool `json:"required"`
	// Description 字段用途说明，可为空（不超过 256 个字符）
	Description string `json:"description" binding:"max=256"`
}

// AgentSkillCreateIn 定义创建 AI 技能的请求输入。
// 名称须非空、首尾无空白且不超过 64 个字符；描述不超过 256 个字符；正文须含非空白内容且
// 不超过 32768 个字符。上述约束统一由 binding 校验，存储层不再重复校验。
type AgentSkillCreateIn struct {
	// Name 技能名称，必填（1..64 个字符，首尾不能有空白）
	Name string `json:"name" binding:"required,trimmed,max=64"`
	// Description 技能描述，可选（不超过 256 个字符）
	Description string `json:"description" binding:"max=256"`
	// Content 技能正文，必填（不能为纯空白，不超过 32768 个字符）
	Content string `json:"content" binding:"required,notBlank,max=32768"`
	// PeriodicCapable 技能是否可周期执行；为 true 时输入字段必须包含 name=nowTime、type=date
	PeriodicCapable bool `json:"periodicCapable"`
	// InputFields 技能输入字段（由用户填写）
	InputFields []SkillInputFieldIn `json:"inputFields" binding:"dive"`
	// FixedFields 技能固定参数（由用户填写），值为原始 JSON
	FixedFields map[string]json.RawMessage `json:"fixedFields"`
}

// AgentSkillUpdateIn 定义更新 AI 技能的请求输入（全部定义字段整体覆盖）。
type AgentSkillUpdateIn struct {
	// Name 技能名称，必填（1..64 个字符，首尾不能有空白）
	Name string `json:"name" binding:"required,trimmed,max=64"`
	// Description 技能描述，可选（不超过 256 个字符）
	Description string `json:"description" binding:"max=256"`
	// Content 技能正文，必填（不能为纯空白，不超过 32768 个字符）
	Content string `json:"content" binding:"required,notBlank,max=32768"`
	// PeriodicCapable 技能是否可周期执行；为 true 时输入字段必须包含 name=nowTime、type=date
	PeriodicCapable bool `json:"periodicCapable"`
	// InputFields 技能输入字段（由用户填写）
	InputFields []SkillInputFieldIn `json:"inputFields" binding:"dive"`
	// FixedFields 技能固定参数（由用户填写），值为原始 JSON
	FixedFields map[string]json.RawMessage `json:"fixedFields"`
}

// AgentSkillBatchIn 定义批量删除 AI 技能的请求输入；Ids 接受标量或数组。
type AgentSkillBatchIn struct {
	// Ids 待批量删除的技能 ID 列表
	Ids views.FlexList[uint] `json:"ids"`
}

// AgentSkillStatusIn 定义启停 AI 技能的请求输入。
type AgentSkillStatusIn struct {
	// Enabled 目标启用状态（true 启用 / false 停用）；前端总会显式传入，故不使用 binding 必填
	Enabled bool `json:"enabled"`
}

// newSkillInputFieldOuts 把模型层的输入字段映射为视图层等价结构；nil 入参返回空切片。
func newSkillInputFieldOuts(fields []modelsai.SkillInputField) []SkillInputFieldOut {
	outs := make([]SkillInputFieldOut, 0, len(fields))
	for _, field := range fields {
		outs = append(outs, SkillInputFieldOut{
			Name:        field.Name,
			Type:        string(field.Type),
			Required:    field.Required,
			Description: field.Description,
		})
	}
	return outs
}

// newAgentSkillDetailOut 将技能模型映射为详情输出（正文解密后以明文返回）。输入字段与固定参数
// 是落库 JSON，解析失败属于服务端数据异常，返回错误由调用方按意外错误处理，不静默降级为空值。
func newAgentSkillDetailOut(m *modelsai.AgentSkill) (AgentSkillDetailOut, error) {
	fields, err := modelsai.ParseSkillInputFields(m.InputFields)
	if err != nil {
		return AgentSkillDetailOut{}, errors.Wrapf(err, "ai: parse skill %d input fields", m.ID)
	}
	fixedFields, err := modelsai.ParseSkillFixedFields(m.FixedFields)
	if err != nil {
		return AgentSkillDetailOut{}, errors.Wrapf(err, "ai: parse skill %d fixed fields", m.ID)
	}
	return AgentSkillDetailOut{
		ID:              m.ID,
		NamespaceName:   m.NamespaceName,
		Source:          string(m.Source),
		Name:            m.Name,
		Description:     m.Description,
		Content:         string(m.Content.Value()),
		Enabled:         m.Enabled,
		PeriodicCapable: m.PeriodicCapable,
		InputFields:     newSkillInputFieldOuts(fields),
		FixedFields:     fixedFields,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}, nil
}

// AgentSkillListGet 处理 AI 技能列表查询的请求。
// 返回当前命名空间的自定义技能与全局内置技能（内置技能由代码注册表声明、fixture 注入注册行，
// 跨命名空间可见但只读），支持分页、keyword / source / enabled 过滤、排序与字段投影；与项目其它
// 列表接口一致，查询构建与总数统计统一委托 utils.BuildSearchCountQuery，视图层不自写过滤 SQL。
// 列表不返回加密正文；只读语义由 source='builtin' 直接表达，不再单独下发布尔标记。
func AgentSkillListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	var in views.AgentSkillListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*AgentSkillListOut
	if query, err := utils.BuildSearchCountQuery(
		middlewares.GetCoreDatabase(c).Model(&modelsai.AgentSkill{}).
			Where("source = ? OR (namespace_name = ? AND source = ?)",
				modelsai.SkillSourceBuiltin, namespace, modelsai.SkillSourceCustom), &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 列表扫描的原始 JSON 列映射为视图层输入字段与固定参数；解析失败记录告警并按空列表 /
	// 空对象处理，不中断列表。
	for _, item := range items {
		fields, err := modelsai.ParseSkillInputFields(item.InputFieldsRaw)
		if err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "ai skill list: parse input fields for skill %d failed, using empty list: %v", item.ID, err)
			fields = nil
		}
		item.InputFields = newSkillInputFieldOuts(fields)
		fixedFields, err := modelsai.ParseSkillFixedFields(item.FixedFieldsRaw)
		if err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "ai skill list: parse fixed fields for skill %d failed, using empty object: %v", item.ID, err)
			fixedFields = make(map[string]json.RawMessage)
		}
		item.FixedFields = fixedFields
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **AgentSkillListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// AgentSkillOptionsGet 处理技能选择器选项查询的请求。
// 先返回 agent_skills 中位于系统命名空间且已启用的内置技能注册行（按 id 升序，即 fixture
// 注入顺序；代码侧正文缺失的行按编程缺陷跳过），再返回当前命名空间已启用的自定义技能（按创建
// 时间倒序）。本接口只服务聊天框选择器，不返回技能正文；管理页仍走 AgentSkillListGet，内置
// 技能按 source='builtin' 在管理列表可见且只读。
func AgentSkillOptionsGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	items := make([]AgentSkillOptionOut, 0)
	if comps.Skills != nil {
		builtins, err := comps.Skills.ListEnabledBuiltinSkills(c.Request.Context())
		if err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		for _, row := range builtins {
			if _, declared := skill.LookupBuiltin(row.Name); !declared {
				continue
			}
			items = append(items, AgentSkillOptionOut{
				ID:          row.ID,
				Name:        row.Name,
				Description: row.Description,
			})
		}
	}
	var rows []*modelsai.AgentSkill
	if err := middlewares.GetCoreDatabase(c).Model(&modelsai.AgentSkill{}).
		Select("id", "name", "description").
		Where("namespace_name = ? AND source = ? AND enabled = ?", namespace, modelsai.SkillSourceCustom, true).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	for _, row := range rows {
		items = append(items, AgentSkillOptionOut{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	response.SimpleOK.AbortWithGinW(c, AgentSkillOptionListOut{Items: items})
}

// AgentSkillDetailGet 处理 AI 技能详情查询的请求。
// 技能不存在或不属于当前命名空间时返回 404。
func AgentSkillDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	if comps.Skills == nil {
		response.CreateAccidentError(errors.New("ai: the skill store is not assembled")).AbortWithGinH(c)
		return
	}
	item, err := comps.Skills.Get(c, namespace, uint(id))
	if err != nil {
		if errors.Is(err, skill.ErrSkillNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out, err := newAgentSkillDetailOut(item)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentSkillCreate 处理创建 AI 技能的请求。
// 新建技能默认启用；周期能力、输入字段与固定参数由用户提交，周期技能必须包含 nowTime 输入字段，
// 且固定参数必须包含 timeOffset 与 timeWindow（timeWindow 须大于 0）。
// 同命名空间下已存在同名技能返回 400（aiSkillNameExists）。名称 / 描述 / 正文与输入字段结构的
// 必填、长度与空白约束由 binding 统一校验。
func AgentSkillCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentSkillCreateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	inputFields := newSkillInputFields(in.InputFields)
	if !validateSkillDefinition(c, in.PeriodicCapable, inputFields, in.FixedFields) {
		return
	}
	item, err := comps.Skills.Create(c, namespace, skill.CreateInput{
		Name:            in.Name,
		Description:     strings.TrimSpace(in.Description),
		Content:         in.Content,
		PeriodicCapable: in.PeriodicCapable,
		InputFields:     inputFields,
		FixedFields:     in.FixedFields,
	})
	if err != nil {
		abortSkillWriteError(c, err)
		return
	}
	out, err := newAgentSkillDetailOut(item)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentSkillUpdate 处理更新 AI 技能的请求。
// 按 (id, namespace) 定位技能，不存在返回 404；其余写入错误与创建同口径映射。名称、描述、
// 正文与周期能力、输入字段、固定参数全部整体覆盖，周期技能同样必须包含 nowTime 输入字段，
// 且固定参数必须包含 timeOffset 与 timeWindow（timeWindow 须大于 0）。
func AgentSkillUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	if comps.Skills == nil {
		response.CreateAccidentError(errors.New("ai: the skill store is not assembled")).AbortWithGinH(c)
		return
	}
	builtin, err := comps.Skills.IsBuiltinSkill(c.Request.Context(), uint(id))
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if builtin {
		response.CreateBadBodyMessageError("aiSkillBuiltinReadonly").AbortWithGinH(c)
		return
	}
	var in AgentSkillUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	inputFields := newSkillInputFields(in.InputFields)
	if !validateSkillDefinition(c, in.PeriodicCapable, inputFields, in.FixedFields) {
		return
	}
	item, err := comps.Skills.Update(c, namespace, uint(id), skill.UpdateInput{
		Name:            in.Name,
		Description:     strings.TrimSpace(in.Description),
		Content:         in.Content,
		PeriodicCapable: in.PeriodicCapable,
		InputFields:     inputFields,
		FixedFields:     in.FixedFields,
	})
	if err != nil {
		abortSkillWriteError(c, err)
		return
	}
	out, err := newAgentSkillDetailOut(item)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// newSkillInputFields 把请求中的输入字段映射为模型层结构，并去除名称与说明的首尾空白。
func newSkillInputFields(fields []SkillInputFieldIn) []modelsai.SkillInputField {
	outs := make([]modelsai.SkillInputField, 0, len(fields))
	for _, field := range fields {
		outs = append(outs, modelsai.SkillInputField{
			Name:        strings.TrimSpace(field.Name),
			Type:        field.Type,
			Required:    field.Required,
			Description: strings.TrimSpace(field.Description),
		})
	}
	return outs
}

// validateSkillDefinition 校验用户填写的技能定义：固定参数须满足保留键、与输入字段不重名及
// 周期时间窗口契约（周期技能固定参数必须包含 timeOffset 与 timeWindow，timeWindow 须大于 0）；
// 周期技能必须且只能包含一个 name=nowTime、type=date 的输入字段。
// 校验失败时写入本地化 400 响应并返回 false。
func validateSkillDefinition(c *gin.Context, periodic bool, fields []modelsai.SkillInputField, fixedFields map[string]json.RawMessage) bool {
	if err := modelsai.ValidateSkillFixedFields(fixedFields, periodic, fields); err != nil {
		response.CreateBadBodyMessageError("aiSkillInvalidFields", err.Error()).AbortWithGinH(c)
		return false
	}
	if err := modelsai.ValidateSkillPeriodicInputFields(periodic, fields); err != nil {
		response.CreateBadBodyMessageError("aiSkillPeriodicNowTimeRequired").AbortWithGinH(c)
		return false
	}
	return true
}

// abortSkillWriteError 把技能写入（创建 / 更新）失败映射为本地化业务错误并中断请求：
// 同名冲突为 400，技能不存在为 404，其余为意外错误。
func abortSkillWriteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, skill.ErrSkillNameExists):
		response.CreateBadBodyMessageError("aiSkillNameExists").AbortWithGinH(c)
	case errors.Is(err, skill.ErrSkillNotFound):
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
	default:
		response.CreateAccidentError(err).AbortWithGinH(c)
	}
}

// AgentSkillBatchDelete 批量软删除 AI 技能。
// 逐条按 (id, namespace) 确认技能存在（不存在记录失败明细），再软删；返回批量结果汇总。
func AgentSkillBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentSkillBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	// 整批预检：批内存在内置技能即整批拒绝（内置技能由代码注册表声明、生命周期由代码掌握），
	// 避免出现「部分删除」的不可预期结果；与内置工具删除语义一致。
	var builtin int64
	if err := middlewares.GetCoreDatabase(c).Model(&modelsai.AgentSkill{}).
		Where("id IN ? AND source = ?", ids, modelsai.SkillSourceBuiltin).Count(&builtin).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if builtin > 0 {
		response.CreateBadBodyMessageError("aiSkillBuiltinReadonly").AbortWithGinH(c)
		return
	}
	views.BatchExecute(c, ids,
		func(id uint) string { return strconv.FormatUint(uint64(id), 10) },
		func(id uint) error {
			if _, err := comps.Skills.Get(c, namespace, id); err != nil {
				if errors.Is(err, skill.ErrSkillNotFound) {
					return errors.Newf("skill %d not found", id)
				}
				return errors.WithStack(err)
			}
			return comps.Skills.Delete(c, namespace, []uint{id})
		},
	)
}

// AgentSkillSetEnabled 处理启停 AI 技能的请求。
// 技能不存在或不属于当前命名空间时返回 404；停用的技能不参与运行时装配。
func AgentSkillSetEnabled(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in AgentSkillStatusIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Skills.SetEnabled(c, namespace, uint(id), in.Enabled); err != nil {
		if errors.Is(err, skill.ErrSkillNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
