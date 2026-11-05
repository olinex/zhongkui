// 报告中心视图层：周期性报告计划、运行记录、关注关系与信息流的 HTTP 接口。
// 全部 In/Out 结构独立定义并按字段显式映射，不引用 internal/models 结构体；计划与运行记录的
// 写入、单条读取与名称回填委托 internal/ai/reports/periodic 领域层，列表查询统一复用 views 搜索契约
// （utils.BuildSearchCountQuery + 字段投影），视图层只负责绑定、命名空间隔离与任务投递。
package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// PeriodicReportSkillInputFieldOut 定义报告可用技能输入字段的输出结构（独立于模型层，逐字段映射）。
type PeriodicReportSkillInputFieldOut struct {
	// Name 字段名称，作为计划 inputValues 中的键
	Name string `json:"name"`
	// Type 字段类型，取值为 date / number / string / enum / boolean；nowTime 是字段名，其类型为 date
	Type string `json:"type"`
	// Required 该字段是否必填
	Required bool `json:"required"`
	// Description 字段用途说明，可为空
	Description string `json:"description"`
}

// PeriodicReportSkillOptionOut 定义可作为周期性报告来源的技能选项输出。
type PeriodicReportSkillOptionOut struct {
	// ID 技能 ID（agent_skills.id）
	ID uint `json:"id"`
	// Name 技能名称
	Name string `json:"name"`
	// Description 技能说明，可为空
	Description string `json:"description"`
	// InputFields 技能输入字段，供前端渲染计划输入表单
	InputFields []PeriodicReportSkillInputFieldOut `json:"inputFields"`
	// FixedFields 技能固定参数，保留原始 JSON 类型；前端只读展示并作为计划偏置默认值
	FixedFields map[string]json.RawMessage `json:"fixedFields"`
}

// PeriodicReportSkillOptionListOut 定义报告技能选项列表输出。
type PeriodicReportSkillOptionListOut struct {
	// Items 可选择作为报告来源的技能
	Items []PeriodicReportSkillOptionOut `json:"items"`
}

// PeriodicReportPlanItemOut 定义报告计划列表项的输出结构（独立于模型层，逐字段映射）。
type PeriodicReportPlanItemOut struct {
	// ID 计划 ID
	ID uint `json:"id"`
	// Name 计划名称
	Name string `json:"name"`
	// Description 计划说明，可为空
	Description string `json:"description"`
	// SkillID 计划使用的技能 ID
	SkillID uint `json:"skillID"`
	// SkillName 计划关联的技能名称（按 skill_id 关联读取，不落库快照）
	SkillName string `json:"skillName"`
	// CronSpec 计划执行的 cron 表达式
	CronSpec string `json:"cronSpec"`
	// InputValues 技能输入字段的实参（键为字段名），保留原始 JSON 类型；
	// 列表按 JSON 展示（form kind=json，经 OPTIONS 自省下发前端）
	InputValues map[string]json.RawMessage `json:"inputValues" form:"kind=json"`
	// Enabled 计划是否启用
	Enabled bool `json:"enabled"`
	// RetentionDays 报告保留天数
	RetentionDays int `json:"retentionDays"`
	// ExpiresAt 计划失效时间，nil 表示长期有效
	ExpiresAt *time.Time `json:"expiresAt"`
	// NextRunAt 下一次计划执行时间，nil 表示待分发器计算
	NextRunAt *time.Time `json:"nextRunAt"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// PeriodicReportPlanListOut 定义报告计划列表项输出（嵌入 BaseTotalOut 支持分页总数）。
// 字段与 search.toml 的 PeriodicReportPlanListGet.selectFields 一一对应。
type PeriodicReportPlanListOut struct {
	PeriodicReportPlanItemOut
	utils.BaseTotalOut
}

// PeriodicReportPlanCreateIn 定义创建报告计划的请求输入：字段格式与保留天数范围由视图层 binding
// 与 default 标签统一校验，技能资格、cron 与输入偏置由领域层校验。
type PeriodicReportPlanCreateIn struct {
	// Name 计划名称，必填（不超过 128 个字符）
	Name string `json:"name" binding:"required,max=128"`
	// Description 计划说明，可选（不超过 512 个字符）
	Description string `json:"description" binding:"max=512"`
	// SkillID 计划使用的技能 ID，必填
	SkillID uint `json:"skillID" binding:"required"`
	// CronSpec 计划执行的 cron 表达式，必填、不超过 64 个字符且须通过 cron 语法校验
	CronSpec string `json:"cronSpec" binding:"required,max=64,cron"`
	// InputValues 技能输入字段的实参（键为字段名），保留原始 JSON 类型
	InputValues map[string]json.RawMessage `json:"inputValues"`
	// RetentionDays 报告保留天数，缺省 90 天，允许 1..3650 天
	RetentionDays int `json:"retentionDays" default:"90" binding:"min=1,max=3650"`
	// ExpiresAt 计划失效时间，空表示长期有效
	ExpiresAt *time.Time `json:"expiresAt"`
}

// PeriodicReportPlanUpdateIn 定义更新报告计划的请求输入（可编辑字段整体覆盖）。
type PeriodicReportPlanUpdateIn struct {
	// Name 计划名称，必填（不超过 128 个字符）
	Name string `json:"name" binding:"required,max=128"`
	// Description 计划说明，可选（不超过 512 个字符）
	Description string `json:"description" binding:"max=512"`
	// SkillID 计划使用的技能 ID，必填
	SkillID uint `json:"skillID" binding:"required"`
	// CronSpec 计划执行的 cron 表达式，必填、不超过 64 个字符且须通过 cron 语法校验
	CronSpec string `json:"cronSpec" binding:"required,max=64,cron"`
	// InputValues 技能输入字段的实参（键为字段名），保留原始 JSON 类型
	InputValues map[string]json.RawMessage `json:"inputValues"`
	// RetentionDays 报告保留天数，缺省 90 天，允许 1..3650 天
	RetentionDays int `json:"retentionDays" default:"90" binding:"min=1,max=3650"`
	// ExpiresAt 计划失效时间，空表示长期有效
	ExpiresAt *time.Time `json:"expiresAt"`
}

// PeriodicReportPlanToggleIn 定义启停报告计划的请求输入。
type PeriodicReportPlanToggleIn struct {
	// Enabled 目标启用状态（true 启用 / false 停用）
	Enabled bool `json:"enabled"`
}

// PeriodicReportRunItemOut 定义报告运行记录列表项的输出结构（独立于模型层，逐字段映射）。
type PeriodicReportRunItemOut struct {
	// ID 运行记录 ID
	ID uint `json:"id"`
	// PlanID 所属报告计划 ID
	PlanID uint `json:"planID"`
	// Status 运行状态（running/success/failed）
	Status string `json:"status"`
	// Title 生成报告的标题
	Title string `json:"title"`
	// ObjectKey 报告对象的存储键，无产出时为空
	ObjectKey string `json:"objectKey"`
	// SizeBytes 生成报告的字节大小
	SizeBytes int64 `json:"sizeBytes"`
	// Error 运行失败原因，成功时为空
	Error string `json:"error"`
	// TriggerAt 计划触发时间点
	TriggerAt time.Time `json:"triggerAt"`
	// SessionID 本次运行专属的 Agent 会话 ID，0 表示尚未创建
	SessionID uint `json:"sessionID"`
	// StartedAt 实际开始执行时间，nil 表示尚未开始
	StartedAt *time.Time `json:"startedAt"`
	// FinishedAt 执行结束时间，nil 表示尚未结束
	FinishedAt *time.Time `json:"finishedAt"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// PeriodicReportRunDetailOut 定义报告运行详情输出：在列表项基础上补充报告正文。
type PeriodicReportRunDetailOut struct {
	PeriodicReportRunItemOut
	// Content 报告正文；无产出或未生成时为空
	Content string `json:"content"`
}

// PeriodicReportRunListOut 定义报告运行记录列表项输出（嵌入 BaseTotalOut 支持分页总数）。
// 字段与 search.toml 的 PeriodicReportRunListGet.selectFields 一一对应。
type PeriodicReportRunListOut struct {
	PeriodicReportRunItemOut
	utils.BaseTotalOut
}

// PeriodicReportFeedItemOut 定义报告中心信息流的条目输出。
type PeriodicReportFeedItemOut struct {
	// RunID 运行记录 ID
	RunID uint `json:"runID"`
	// PlanID 所属报告计划 ID
	PlanID uint `json:"planID"`
	// PlanName 所属计划名称快照；计划已删除时为空
	PlanName string `json:"planName"`
	// Title 生成报告的标题
	Title string `json:"title"`
	// Status 运行状态（running/success/failed）
	Status string `json:"status"`
	// TriggerAt 计划触发时间点
	TriggerAt time.Time `json:"triggerAt"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// PeriodicReportFeedListOut 定义报告中心信息流列表项输出（嵌入 BaseTotalOut 支持分页总数）。
// 字段与 search.toml 的 PeriodicReportFeedListGet.selectFields 一一对应。
type PeriodicReportFeedListOut struct {
	PeriodicReportFeedItemOut
	utils.BaseTotalOut
}

// PeriodicReportWatchPlanItemOut 定义关注计划候选列表项输出（独立于模型层，逐字段映射）。
// 字段与 search.toml 的 PeriodicReportWatchListGet.selectFields 一一对应。
type PeriodicReportWatchPlanItemOut struct {
	// ID 计划 ID
	ID uint `json:"id"`
	// Name 计划名称
	Name string `json:"name"`
	// Description 计划说明，可为空
	Description string `json:"description"`
	// Enabled 计划是否启用
	Enabled bool `json:"enabled"`
	// Watched 当前用户是否已关注该计划（处理器按关注关系回填）
	Watched bool `json:"watched"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// PeriodicReportWatchListOut 定义关注计划候选列表项输出（嵌入 BaseTotalOut 支持分页总数）。
type PeriodicReportWatchListOut struct {
	PeriodicReportWatchPlanItemOut
	utils.BaseTotalOut
}

// PeriodicReportWatchSetIn 定义设置计划关注状态的请求输入。
type PeriodicReportWatchSetIn struct {
	// PlanID 计划 ID，必填
	PlanID uint `json:"planID" binding:"required"`
	// Watched 目标关注状态（true 关注 / false 取消关注）
	Watched bool `json:"watched"`
}

// newPeriodicReportSkillInputFieldOut 把模型层输入字段映射为视图层等价结构。
func newPeriodicReportSkillInputFieldOut(field modelsai.SkillInputField) PeriodicReportSkillInputFieldOut {
	return PeriodicReportSkillInputFieldOut{
		Name:        field.Name,
		Type:        string(field.Type),
		Required:    field.Required,
		Description: field.Description,
	}
}

// newPeriodicReportSkillOptionOut 把技能模型映射为报告技能选项输出；输入字段解析失败时按空列表
// 处理，固定参数解析失败时按空对象处理。
func newPeriodicReportSkillOptionOut(m *modelsai.AgentSkill) PeriodicReportSkillOptionOut {
	fields, _ := modelsai.ParseSkillInputFields(m.InputFields)
	outs := make([]PeriodicReportSkillInputFieldOut, 0, len(fields))
	for _, field := range fields {
		outs = append(outs, newPeriodicReportSkillInputFieldOut(field))
	}
	fixedFields, _ := modelsai.ParseSkillFixedFields(m.FixedFields)
	if fixedFields == nil {
		fixedFields = map[string]json.RawMessage{}
	}
	return PeriodicReportSkillOptionOut{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		InputFields: outs,
		FixedFields: fixedFields,
	}
}

// newPeriodicReportPlanItemOut 把报告计划模型映射为列表项输出；input_values 解码失败时置为空映射。
func newPeriodicReportPlanItemOut(m *modelsai.PeriodicReportPlan) PeriodicReportPlanItemOut {
	values, err := periodic.DecodeInputValues(m.InputValues)
	if err != nil {
		values = map[string]json.RawMessage{}
	}
	return PeriodicReportPlanItemOut{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		SkillID:       m.SkillID,
		CronSpec:      m.CronSpec,
		InputValues:   values,
		Enabled:       m.Enabled,
		RetentionDays: m.RetentionDays,
		ExpiresAt:     m.ExpiresAt,
		NextRunAt:     m.NextRunAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// fillPlanSkillNames 按 skill_id 批量回填计划列表项的技能名称（避免逐条查询）；名称不落库，
// 恒取技能表当前值，技能软删除后仍能显示原名称。
func fillPlanSkillNames(ctx context.Context, store *periodic.Store, outs []PeriodicReportPlanItemOut) error {
	if len(outs) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(outs))
	for _, out := range outs {
		ids = append(ids, out.SkillID)
	}
	names, err := store.SkillNamesByIDs(ctx, ids)
	if err != nil {
		return errors.WithStack(err)
	}
	for i := range outs {
		outs[i].SkillName = names[outs[i].SkillID]
	}
	return nil
}

// newPeriodicReportPlanItemWithSkill 映射单条计划并按 skill_id 回填技能名称。
func newPeriodicReportPlanItemWithSkill(ctx context.Context, store *periodic.Store, m *modelsai.PeriodicReportPlan) (PeriodicReportPlanItemOut, error) {
	out := newPeriodicReportPlanItemOut(m)
	outs := []PeriodicReportPlanItemOut{out}
	if err := fillPlanSkillNames(ctx, store, outs); err != nil {
		return PeriodicReportPlanItemOut{}, err
	}
	return outs[0], nil
}

// newPeriodicReportRunItemOut 把报告运行记录模型映射为列表项输出。
func newPeriodicReportRunItemOut(m *modelsai.PeriodicReportRun) PeriodicReportRunItemOut {
	var sessionID uint
	if m.SessionID != nil {
		sessionID = *m.SessionID
	}
	return PeriodicReportRunItemOut{
		ID:         m.ID,
		PlanID:     m.PlanID,
		Status:     string(m.Status),
		Title:      m.Title,
		ObjectKey:  m.ObjectKey,
		SizeBytes:  m.SizeBytes,
		Error:      m.Error,
		TriggerAt:  m.TriggerAt,
		SessionID:  sessionID,
		StartedAt:  m.StartedAt,
		FinishedAt: m.FinishedAt,
		CreatedAt:  m.CreatedAt,
	}
}

// newPeriodicReportFeedItemOut 把运行记录模型与第二趟回填的计划名称映射为信息流条目输出。
func newPeriodicReportFeedItemOut(run *modelsai.PeriodicReportRun, planName string) PeriodicReportFeedItemOut {
	return PeriodicReportFeedItemOut{
		RunID:     run.ID,
		PlanID:    run.PlanID,
		PlanName:  planName,
		Title:     run.Title,
		Status:    string(run.Status),
		TriggerAt: run.TriggerAt,
		CreatedAt: run.CreatedAt,
	}
}

// periodicReportStore 从请求上下文构造报告领域存储；构造失败时以意外错误中断请求并返回 ok=false。
func periodicReportStore(c *gin.Context) (*periodic.Store, bool) {
	store, err := periodic.NewStore(middlewares.GetCoreDatabase(c))
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return nil, false
	}
	return store, true
}

// abortPeriodicReportError 把报告领域层错误映射为 HTTP 响应并中断请求。
func abortPeriodicReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, periodic.ErrPlanNotFound), errors.Is(err, periodic.ErrRunNotFound):
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
	case errors.Is(err, periodic.ErrSkillNotEligible):
		response.CreateBadBodyMessageError("periodicReportSkillNotEligible").AbortWithGinH(c)
	case errors.Is(err, periodic.ErrScheduleInvalid):
		response.CreateBadBodyMessageError("periodicReportScheduleInvalid").AbortWithGinH(c)
	case errors.Is(err, periodic.ErrInputMissing):
		response.CreateBadBodyMessageError("periodicReportInputMissing").AbortWithGinH(c)
	case errors.Is(err, periodic.ErrInputInvalid):
		response.CreateBadBodyMessageError("periodicReportInputInvalid").AbortWithGinH(c)
	default:
		response.CreateAccidentError(err).AbortWithGinH(c)
	}
}

// newPeriodicReportPlanInput 把创建 / 更新请求输入显式映射为领域层输入；两个 In 结构字段一致，共用本映射。
func newPeriodicReportPlanInput(in PeriodicReportPlanCreateIn) periodic.PlanInput {
	return periodic.PlanInput{
		Name:          in.Name,
		Description:   in.Description,
		SkillID:       in.SkillID,
		CronSpec:      in.CronSpec,
		InputValues:   in.InputValues,
		RetentionDays: in.RetentionDays,
		ExpiresAt:     in.ExpiresAt,
	}
}

// PeriodicReportSkillOptionsGet 处理报告技能选项查询的请求。
// 只返回当前命名空间内可周期执行的自定义技能（含时间窗口字段），供前端渲染计划创建表单。
func PeriodicReportSkillOptionsGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	rows, err := store.EligibleSkills(c, namespace)
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	items := make([]PeriodicReportSkillOptionOut, 0, len(rows))
	for _, row := range rows {
		items = append(items, newPeriodicReportSkillOptionOut(row))
	}
	response.SimpleOK.AbortWithGinW(c, PeriodicReportSkillOptionListOut{Items: items})
}

// PeriodicReportPlanListGet 处理报告计划列表查询的请求。
// 仅返回当前命名空间内的计划，支持 keyword（计划名称模糊）与 enabled 过滤、排序、分页与字段投影；
// 查询构建与总数统计统一委托 utils.BuildSearchCountQuery。skillName 不落库，按 skill_id 第二趟批量回填。
func PeriodicReportPlanListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in views.PeriodicReportPlanListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 以局部扫描行承载计划模型与注入的 total 列，随后映射为 Out 结构
	var rows []struct {
		modelsai.PeriodicReportPlan
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&modelsai.PeriodicReportPlan{}).
		Where("namespace_name = ?", namespace)
	query, err := utils.BuildSearchCountQuery(db, &in)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	outs := make([]PeriodicReportPlanItemOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		outs = append(outs, newPeriodicReportPlanItemOut(&rows[i].PeriodicReportPlan))
	}
	if err := fillPlanSkillNames(c, store, outs); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*PeriodicReportPlanListOut, 0, len(outs))
	for i := range outs {
		items = append(items, &PeriodicReportPlanListOut{
			PeriodicReportPlanItemOut: outs[i],
			BaseTotalOut:              utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **PeriodicReportPlanListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// PeriodicReportPlanCreate 处理创建报告计划的请求。
// 校验与落库由领域层完成；下次执行时间留空，由分发器按 cron 计算。
func PeriodicReportPlanCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in PeriodicReportPlanCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	plan, err := store.CreatePlan(c, namespace, userID, newPeriodicReportPlanInput(in))
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	out, err := newPeriodicReportPlanItemWithSkill(c, store, plan)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// PeriodicReportPlanUpdate 处理更新报告计划的请求。
// 按 (id, namespace) 定位计划，不存在返回 404；更新会清空下次执行时间，交由分发器重新计算。
func PeriodicReportPlanUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in PeriodicReportPlanUpdateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	plan, err := store.UpdatePlan(c, namespace, uint(id), newPeriodicReportPlanInput(PeriodicReportPlanCreateIn(in)))
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	out, err := newPeriodicReportPlanItemWithSkill(c, store, plan)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// PeriodicReportPlanToggle 处理启停报告计划的请求。
// 按 (id, namespace) 定位计划，不存在返回 404；启用时清空下次执行时间，交由分发器重新计算。
func PeriodicReportPlanToggle(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in PeriodicReportPlanToggleIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	plan, err := store.SetPlanEnabled(c, namespace, uint(id), in.Enabled)
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	out, err := newPeriodicReportPlanItemWithSkill(c, store, plan)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// PeriodicReportPlanDelete 处理删除报告计划的请求。
// 按 (id, namespace) 定位并软删除计划，分发器不再选取已删除计划。
func PeriodicReportPlanDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	if _, err := store.DeletePlan(c, namespace, uint(id)); err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// PeriodicReportPlanRun 处理立即执行报告计划的请求。
// 计划不存在返回 404；计划已过期返回 400（periodicReportPlanExpired）；否则立即投递一条生成任务。
func PeriodicReportPlanRun(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	plan, err := store.GetPlan(c, namespace, uint(id))
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	if plan.ExpiresAt != nil && time.Now().After(*plan.ExpiresAt) {
		response.CreateBadBodyMessageError("periodicReportPlanExpired").AbortWithGinH(c)
		return
	}
	if err := periodic.EnqueuePlanRun(c, middlewares.GetPipelineClient(c), plan.ID, time.Now()); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// PeriodicReportRunListGet 处理报告运行记录列表查询的请求。
// 仅返回当前命名空间内的运行记录，支持 planID / status 过滤、排序、分页与字段投影；
// 查询构建与总数统计统一委托 utils.BuildSearchCountQuery。
func PeriodicReportRunListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	var in views.PeriodicReportRunListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 以局部扫描行承载运行记录模型与注入的 total 列，随后映射为 Out 结构
	var rows []struct {
		modelsai.PeriodicReportRun
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&modelsai.PeriodicReportRun{}).
		Where("namespace_name = ?", namespace)
	query, err := utils.BuildSearchCountQuery(db, &in)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*PeriodicReportRunListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		items = append(items, &PeriodicReportRunListOut{
			PeriodicReportRunItemOut: newPeriodicReportRunItemOut(&rows[i].PeriodicReportRun),
			BaseTotalOut:             utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **PeriodicReportRunListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// PeriodicReportRunDetailGet 处理报告运行详情查询的请求。
// 运行记录不存在或跨命名空间返回 404；无产出时正文为空；对象缺失返回 404
// （periodicReportRunObjectMissing），其余读取失败返回意外错误。
func PeriodicReportRunDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	run, err := store.GetRun(c, namespace, uint(id))
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	out := PeriodicReportRunDetailOut{PeriodicReportRunItemOut: newPeriodicReportRunItemOut(run)}
	if run.ObjectKey == "" {
		response.SimpleOK.AbortWithGinW(c, out)
		return
	}
	rc, err := middlewares.GetObjectCache(c).Open(c, run.ObjectKey)
	if err != nil {
		if s3.IsNotExists(err) {
			response.CreateCodeError(http.StatusNotFound, http.StatusNotFound,
				&response.DetailContext{MsgID: "periodicReportRunObjectMissing"}).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 正文为统一 JSON 信封 {"content":...,"draft":...}，由前端按约定解析 content 后渲染 Markdown。
	out.Content = string(content)
	response.SimpleOK.AbortWithGinW(c, out)
}

// PeriodicReportRunDelete 处理删除报告运行记录的请求。
// 按 (id, namespace) 定位记录：先删除对象存储中的报告正文（对象已不存在视为已删除），再软删除
// 运行记录。运行中与已结束的记录都允许删除；删除后生成任务的结果不再回写该记录。
func PeriodicReportRunDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	run, err := store.GetRun(c, namespace, uint(id))
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	if run.ObjectKey != "" {
		if err := middlewares.GetObjectCache(c).Remove(c, run.ObjectKey); err != nil && !s3.IsNotExists(err) {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}
	if err := store.DeleteRun(c, run.ID); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// PeriodicReportWatchListGet 处理关注计划候选列表查询的请求。
// 仅返回当前命名空间内的报告计划，支持 keyword / enabled 过滤、排序、分页与字段投影；
// 查询构建与总数统计统一委托 utils.BuildSearchCountQuery。watched 不落库，按当前用户的
// 关注关系第二趟回填。
func PeriodicReportWatchListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in views.PeriodicReportWatchListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	planIDs, err := store.ListWatchedPlanIDs(c, namespace, userID)
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	watched := make(map[uint]struct{}, len(planIDs))
	for _, planID := range planIDs {
		watched[planID] = struct{}{}
	}
	// 以局部扫描行承载计划模型与注入的 total 列，随后映射为 Out 结构
	var rows []struct {
		modelsai.PeriodicReportPlan
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&modelsai.PeriodicReportPlan{}).
		Where("namespace_name = ?", namespace)
	query, err := utils.BuildSearchCountQuery(db, &in)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*PeriodicReportWatchListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		plan := &rows[i].PeriodicReportPlan
		_, isWatched := watched[plan.ID]
		items = append(items, &PeriodicReportWatchListOut{
			PeriodicReportWatchPlanItemOut: PeriodicReportWatchPlanItemOut{
				ID:          plan.ID,
				Name:        plan.Name,
				Description: plan.Description,
				Enabled:     plan.Enabled,
				Watched:     isWatched,
				CreatedAt:   plan.CreatedAt,
				UpdatedAt:   plan.UpdatedAt,
			},
			BaseTotalOut: utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **PeriodicReportWatchListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// PeriodicReportWatchSet 处理设置计划关注状态的请求。
// 关注时校验计划属于当前命名空间（不存在 404）；幂等设置，重复关注不会重复插入。
func PeriodicReportWatchSet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in PeriodicReportWatchSetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := store.SetWatch(c, namespace, userID, in.PlanID, in.Watched); err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// PeriodicReportFeedListGet 处理报告中心信息流查询的请求。
// 只返回当前用户关注计划的成功运行记录，支持排序、分页与字段投影；查询构建与总数统计统一委托
// utils.BuildSearchCountQuery。planName 按 plan_id 第二趟批量回填，未关注任何计划时直接返回空列表。
func PeriodicReportFeedListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if requireAiEnabled(c) == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	store, ok := periodicReportStore(c)
	if !ok {
		return
	}
	var in views.PeriodicReportFeedListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	planIDs, err := store.ListWatchedPlanIDs(c, namespace, userID)
	if err != nil {
		abortPeriodicReportError(c, err)
		return
	}
	if len(planIDs) == 0 {
		response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(0, []map[string]any{}))
		return
	}
	// 以局部扫描行承载运行记录模型与注入的 total 列，随后映射为 Out 结构
	var rows []struct {
		modelsai.PeriodicReportRun
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&modelsai.PeriodicReportRun{}).
		Where("namespace_name = ? AND plan_id IN ? AND status = ?",
			namespace, planIDs, modelsai.PeriodicReportRunStatusSuccess)
	query, err := utils.BuildSearchCountQuery(db, &in)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	names, err := store.PlanNamesByIDs(c, namespace, planIDs)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*PeriodicReportFeedListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		run := &rows[i].PeriodicReportRun
		items = append(items, &PeriodicReportFeedListOut{
			PeriodicReportFeedItemOut: newPeriodicReportFeedItemOut(run, names[run.PlanID]),
			BaseTotalOut:              utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **PeriodicReportFeedListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}
