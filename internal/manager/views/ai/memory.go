package ai

import (
	"zhongkuisec.com/zhongkui/internal/ai/memory"
	// 标准库
	"strconv"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// AgentMemoryItemOut 定义长期记忆列表项的输出结构（独立于内部模型层，逐字段映射）。
type AgentMemoryItemOut struct {
	// ID 记忆 ID
	ID uint `json:"id"`
	// AgentKind 记忆所属 Agent 类型（knowledge/rego）
	AgentKind string `json:"agentKind"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	Kind string `json:"kind"`
	// Title 记忆标题
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	Status string `json:"status"`
	// UseCount 被检索命中的次数
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索召回的时间，可空
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentMemoryListOut 定义长期记忆列表输出（嵌入 BaseTotalOut 支持分页总数）。
type AgentMemoryListOut struct {
	AgentMemoryItemOut
	utils.BaseTotalOut
}

// AgentMemoryDetailOut 定义长期记忆详情的输出结构。
type AgentMemoryDetailOut struct {
	// ID 记忆 ID
	ID uint `json:"id"`
	// NamespaceName 所属命名空间
	NamespaceName string `json:"namespaceName"`
	// AgentKind 记忆所属 Agent 类型（knowledge/rego）
	AgentKind string `json:"agentKind"`
	// SessionID 来源会话 ID（记忆按会话隔离，仅在来源会话内可见）
	SessionID uint `json:"sessionID"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	Kind string `json:"kind"`
	// Title 记忆标题
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	Status string `json:"status"`
	// UseCount 被检索命中的次数
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索召回的时间，可空
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentMemoryBatchIn 定义批量删除长期记忆的请求输入；Ids 接受标量或数组。
type AgentMemoryBatchIn struct {
	// Ids 待批量删除的记忆 ID 列表
	Ids views.FlexList[uint] `json:"ids"`
}

// AgentMemoryStatusIn 定义启停长期记忆的请求输入。
type AgentMemoryStatusIn struct {
	// Status 目标状态（active/archived），必填且须为合法取值
	Status string `json:"status" binding:"required,enum=memoryStatus"`
}

// newAgentMemoryItemOut 将记忆模型映射为列表项输出。
func newAgentMemoryItemOut(m *modelsai.AgentMemory) AgentMemoryItemOut {
	return AgentMemoryItemOut{
		ID:         m.ID,
		AgentKind:  string(m.AgentKind),
		Kind:       string(m.Kind),
		Title:      m.Title,
		Content:    string(m.Content.Value()),
		Status:     string(m.Status),
		UseCount:   m.UseCount,
		LastUsedAt: m.LastUsedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

// newAgentMemoryDetailOut 将记忆模型映射为详情输出。
func newAgentMemoryDetailOut(m *modelsai.AgentMemory) AgentMemoryDetailOut {
	return AgentMemoryDetailOut{
		ID:            m.ID,
		NamespaceName: m.NamespaceName,
		AgentKind:     string(m.AgentKind),
		SessionID:     m.SessionID,
		Kind:          string(m.Kind),
		Title:         m.Title,
		Content:       string(m.Content.Value()),
		Status:        string(m.Status),
		UseCount:      m.UseCount,
		LastUsedAt:    m.LastUsedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// AgentMemoryListGet 处理长期记忆列表查询的请求。
// 仅返回当前命名空间内属于当前用户的记忆，支持分页、keyword/agentKind/kind/status 过滤、排序与
// 字段投影；查询构建与总数统计统一委托 utils.BuildSearchCountQuery，视图层不自写过滤 SQL。
func AgentMemoryListGet(c *gin.Context) {
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
	var in views.AgentMemoryListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 以局部扫描行承载记忆模型与注入的 total 列，随后映射为 Out 结构
	var rows []struct {
		modelsai.AgentMemory
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&modelsai.AgentMemory{}).
		Where("namespace_name = ? AND user_id = ?", namespace, userID)
	query, err := utils.BuildSearchCountQuery(db, &in)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*AgentMemoryListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		items = append(items, &AgentMemoryListOut{
			AgentMemoryItemOut: newAgentMemoryItemOut(&rows[i].AgentMemory),
			BaseTotalOut:       utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **AgentMemoryListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// AgentMemoryDetailGet 处理长期记忆详情查询的请求。
// 记忆不存在或不属于当前命名空间时返回 404。
func AgentMemoryDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	mem, err := comps.Memory.Get(c, namespace, userID, uint(id))
	if err != nil {
		if errors.Is(err, memory.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentMemoryDetailOut(mem))
}

// AgentMemoryBatchDelete 批量软删除长期记忆。
// 逐条按 (id, namespace) 确认记忆存在（不存在记录失败明细），再软删；返回批量结果汇总。
func AgentMemoryBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	var in AgentMemoryBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	views.BatchExecute(c, ids,
		func(id uint) string { return strconv.FormatUint(uint64(id), 10) },
		func(id uint) error {
			if _, err := comps.Memory.Get(c, namespace, userID, id); err != nil {
				if errors.Is(err, memory.ErrMemoryNotFound) {
					return errors.Newf("memory %d not found", id)
				}
				return errors.WithStack(err)
			}
			return comps.Memory.Delete(c, namespace, userID, []uint{id})
		},
	)
}

// AgentMemorySetStatus 处理启停长期记忆的请求。
// 记忆不存在或不属于当前命名空间时返回 404；目标状态经枚举校验（active/archived）。
func AgentMemorySetStatus(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in AgentMemoryStatusIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Memory.SetStatus(c, namespace, userID, uint(id), modelsai.MemoryStatus(in.Status)); err != nil {
		if errors.Is(err, memory.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
