package ai

import (
	// 标准库
	"fmt"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// AgentMemoryItemOut 定义长期记忆列表项的输出结构（独立于内部模型层，逐字段映射）。
type AgentMemoryItemOut struct {
	// ID 记忆 ID
	ID uint `json:"id"`
	// AgentKind 记忆所属 Agent 类型（knowledge/rego）
	AgentKind modelsai.AgentKind `json:"agentKind"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	Kind modelsai.MemoryKind `json:"kind"`
	// Title 记忆标题
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	Status modelsai.MemoryStatus `json:"status"`
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
	AgentKind modelsai.AgentKind `json:"agentKind"`
	// SessionID 来源会话 ID（记忆按会话隔离，仅在来源会话内可见）
	SessionID uint `json:"sessionId"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	Kind modelsai.MemoryKind `json:"kind"`
	// Title 记忆标题
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	Status modelsai.MemoryStatus `json:"status"`
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
	Status modelsai.MemoryStatus `json:"status" binding:"required,oneof=active archived"`
}

// newAgentMemoryItemOut 将记忆模型映射为列表项输出。
func newAgentMemoryItemOut(m *modelsai.AgentMemory) AgentMemoryItemOut {
	return AgentMemoryItemOut{
		ID:         m.ID,
		AgentKind:  m.AgentKind,
		Kind:       m.Kind,
		Title:      m.Title,
		Content:    string(m.Content.Value()),
		Status:     m.Status,
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
		AgentKind:     m.AgentKind,
		SessionID:     m.SessionID,
		Kind:          m.Kind,
		Title:         m.Title,
		Content:       string(m.Content.Value()),
		Status:        m.Status,
		UseCount:      m.UseCount,
		LastUsedAt:    m.LastUsedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// AgentMemoryListGet 处理长期记忆列表查询的请求。
// 仅返回当前命名空间内的记忆，支持分页、按 keyword/agentKind/kind/status 过滤与字段投影；
// 过滤与落库语义复用 ai.Memory.List（MemoryStore），视图层不自写 SQL。
func AgentMemoryListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
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
	query := ai.MemoryQuery{
		NamespaceName: namespace,
		Offset:        int((in.Page - 1) * in.PageSize),
		Limit:         int(in.PageSize),
	}
	if fb := in.FilterBy; fb != nil {
		if fb.Keyword != nil {
			query.Keyword = strings.TrimSpace(fb.Keyword.Value)
		}
		if fb.AgentKind != nil {
			query.AgentKind = modelsai.AgentKind(strings.TrimSpace(fb.AgentKind.Value))
		}
		if fb.Kind != nil {
			query.Kind = modelsai.MemoryKind(strings.TrimSpace(fb.Kind.Value))
		}
		if fb.Status != nil {
			query.Status = modelsai.MemoryStatus(strings.TrimSpace(fb.Status.Value))
		}
	}
	items, total, err := comps.Memory.List(c, query)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	outs := make([]AgentMemoryListOut, 0, len(items))
	for _, m := range items {
		outs = append(outs, AgentMemoryListOut{AgentMemoryItemOut: newAgentMemoryItemOut(m)})
	}
	projected := utils.ProjectSearchItems(outs, &in, func(o *AgentMemoryListOut) any { return *o })
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
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
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	mem, err := comps.Memory.Get(c, namespace, uint(id))
	if err != nil {
		if errors.Is(err, ai.ErrMemoryNotFound) {
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
			if _, err := comps.Memory.Get(c, namespace, id); err != nil {
				if errors.Is(err, ai.ErrMemoryNotFound) {
					return fmt.Errorf("memory %d not found", id)
				}
				return errors.WithStack(err)
			}
			return comps.Memory.Delete(c, namespace, []uint{id})
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
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in AgentMemoryStatusIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Memory.SetStatus(c, namespace, uint(id), in.Status); err != nil {
		if errors.Is(err, ai.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
