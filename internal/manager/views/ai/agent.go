// Package ai 提供 AI 知识域（Agent / 知识库 / 会话）的 HTTP 处理器与视图 In/Out 结构体，
// 向内封装 internal/ai 的组件能力。
package ai

import (
	// 标准库
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	interaction "zhongkuisec.com/zhongkui/internal/ai/interaction"
	"zhongkuisec.com/zhongkui/internal/ai/llmconfig"
	"zhongkuisec.com/zhongkui/internal/ai/llminfra"
	"zhongkuisec.com/zhongkui/internal/ai/memory"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	sessionstore "zhongkuisec.com/zhongkui/internal/ai/session"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// requireAiEnabled 获取已启用的 AI 组件；功能未启用时以 400 + 本地化错误中断请求并返回 nil。
func requireAiEnabled(c *gin.Context) *ai.Components {
	comps := middlewares.GetAi(c)
	if !comps.Enabled() {
		response.CreateBadBodyMessageError("aiFeatureDisabled").AbortWithGinH(c)
		return nil
	}
	return comps
}

// getOwnedAgentSession 按 ID 查询 AI 助手会话并校验其属于给定命名空间与属主用户：
// 会话不存在（ErrSessionNotFound）、命名空间不符或属主不符时以 404 中断请求并返回 nil。
func getOwnedAgentSession(c *gin.Context, comps *ai.Components, sessionID uint, namespace string, userID uint) *modelsai.AgentSession {
	session, err := comps.Sessions.GetSession(c, sessionID, namespace, userID)
	if err != nil {
		if errors.Is(err, sessionstore.ErrSessionNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return nil
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return nil
	}
	return session
}

// AgentSessionItemOut 定义 AI 助手会话的输出项结构。
type AgentSessionItemOut struct {
	// ID 会话 ID
	ID uint `json:"id"`
	// Title 会话标题
	Title string `json:"title"`
	// Slot 会话缺省档位（flash/standard/advanced），空表示按 agent.defaultSlot 参数解析
	Slot string `json:"slot"`
	// Status 会话状态（active/archived）
	Status string `json:"status"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego）
	AgentKind string `json:"agentKind"`
	// ReviewValve 会话级审核阀门（each/auto/none），与本会话内 client 工具调用的审核重要级合成，
	// 决定调用是否需人工审核
	ReviewValve string `json:"reviewValve"`
	// Summary 会话滚动摘要，无摘要时为空串
	Summary string `json:"summary"`
	// ContextTokens 会话的上下文占用度量：最近一条有用量记录的助手消息的输入 token 数
	// （无可用测量为 0，0 表示「尚无测量」而非「占用为 0」，见 sessionstore.Store.ContextTokens）
	ContextTokens int `json:"contextTokens"`
	// ContextLimit 会话有效档位的上下文 token 容量（≤ 0 表示该档位关闭占用展示与自动压缩）
	ContextLimit int `json:"contextLimit"`
	// InteractionState 会话交互主状态（waiting/running/paused/locked），由既有事实推导（不落库）
	InteractionState string `json:"interactionState"`
	// InteractionSubstate 会话交互子状态（随主状态取值；已锁定为空串）
	InteractionSubstate string `json:"interactionSubstate"`
	// Locked 会话是否已锁定（有效轮次达上限）；恒等于 InteractionState 为 locked
	Locked bool `json:"locked"`
	// UserTurns 会话有效轮次（当前存在的用户提问条数，被回滚删除的不计入）
	UserTurns int `json:"userTurns"`
	// MaxUserTurns 会话轮次上限（参数 session.maxUserTurns 的生效值；≤ 0 表示不设上限）
	MaxUserTurns int `json:"maxUserTurns"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentSessionListOut 定义 AI 助手会话列表输出。
type AgentSessionListOut []AgentSessionItemOut

// AgentSessionCreateIn 定义创建 AI 助手会话的请求输入。
type AgentSessionCreateIn struct {
	// Query 首个用户问题，必填；会话主题据此经 LLM 生成（失败时按其截断兜底）
	Query string `json:"query" binding:"required"`
	// Slot 会话缺省档位（flash/standard/advanced），可选；空表示按 agent.defaultSlot 参数解析
	Slot string `json:"slot"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego），可选；缺省 knowledge，非法取值以 400 +
	// 英文错误拒绝
	AgentKind string `json:"agentKind"`
	// ReviewValve 会话级审核阀门（each/auto/none），可选；缺省经 defaults 库以 each（逐一审核）兜底，
	// 空值或取值空间外的取值以 400 + 英文错误拒绝
	ReviewValve string `json:"reviewValve" default:"each" binding:"enum=reviewValve"`
}

// AgentSessionValveIn 定义更新 AI 助手会话审核阀门的请求输入。
type AgentSessionValveIn struct {
	// ReviewValve 会话新审核阀门（each/auto/none），必填；非取值空间内的取值以 400 + 英文错误拒绝
	ReviewValve string `json:"reviewValve" binding:"required,enum=reviewValve"`
}

// AgentSessionRenameIn 定义重命名 AI 助手会话的请求输入。
type AgentSessionRenameIn struct {
	// ID 会话 ID，必填
	ID uint `json:"id" binding:"required"`
	// Title 会话新标题，必填
	Title string `json:"title" binding:"required"`
}

// AgentMessageItemOut 定义 AI 助手会话消息的输出项结构。正文由消息的原生载荷（eino 消息 JSON）
// 解码得到，仅含正文的行回落为 Content；时间线由 /agent/:sessionID/render 重建，
// 本出参只承担会话消息的只读展示。
type AgentMessageItemOut struct {
	// ID 消息 ID
	ID uint `json:"id"`
	// Role 消息角色（user/assistant/system）
	Role string `json:"role"`
	// Content 消息正文内容
	Content string `json:"content"`
	// Citations 助手回复的检索引用来源列表，无引用时省略
	Citations []retrieval.RetrievalResult `json:"citations,omitempty"`
	// PromptTokens 本轮助手回答全部模型调用的输入 token 累加（用户消息为 0）
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 本轮助手回答全部模型调用的输出 token 累加（用户消息为 0）
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 本轮助手回答全部模型调用的 token 总量累加（用户消息为 0）
	TotalTokens int `json:"totalTokens"`
	// ElapsedMs 本轮助手回答的总耗时（毫秒，用户消息为 0）
	ElapsedMs int64 `json:"elapsedMs"`
	// Phase 本条助手消息所属回合的生命周期终态阶段（done/failed/cancelled），空表示尚未终结
	Phase string `json:"phase,omitempty"`
	// SkillID 本条用户消息所在轮次选用的技能 ID，0 表示未选技能（用户消息才有值）
	SkillID uint `json:"skillID,omitempty"`
	// SkillName 本条用户消息所在轮次选用的技能名称快照，空表示未选技能（用户消息才有值）
	SkillName string `json:"skillName,omitempty"`
	// Compress 该消息所锚定的历史压缩记录（压缩游标 upto_seq 等于本条消息 seq 时给出），
	// 无则为 nil；供前端刷新 / 重进会话后回显过往的压缩推进（含静默滚动 rolling）
	Compress *AgentMessageCompressOut `json:"compress,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// AgentMessageCompressOut 定义消息所锚定的历史压缩记录展示项：仅承载前端回显所需的最小字段，
// 不含摘要全文与游标本身。
type AgentMessageCompressOut struct {
	// Reason 触发本次推进的原因（rolling 静默滚动 / auto 自动压缩 / manual 手动压缩）
	Reason string `json:"reason"`
	// TokensBefore 推进前会话上下文占用 token（三种原因均为真实值）
	TokensBefore int `json:"tokensBefore"`
	// FoldedCount 本次折叠进摘要的消息条数（三种原因均为真实条数）
	FoldedCount int `json:"foldedCount"`
	// TokensAfter 本次推进后上下文占用的即时估算 token（仅 auto / manual 给出，静默滚动与估算
	// 不可用时为 0）
	TokensAfter int `json:"tokensAfter"`
}

// AgentMessageListOut 定义 AI 助手会话消息列表输出。
type AgentMessageListOut []AgentMessageItemOut

// AgentMessageTruncateIn 定义截断 AI 助手会话消息的请求输入。
type AgentMessageTruncateIn struct {
	// UserMessageIndex 目标用户消息在会话内的序号（从 1 起，仅按用户消息计数、按 seq 升序计），
	// 该条及其之后的全部消息（含助手回复）将被删除
	UserMessageIndex int `json:"userMessageIndex" binding:"required,min=1"`
}

// AgentSessionBranchIn 定义会话分支的请求输入。
type AgentSessionBranchIn struct {
	AgentMessageTruncateIn
	// Title 新会话标题，可选；为空时沿用源会话标题
	Title string `json:"title"`
}

// AgentSessionCompactIn 定义会话压缩的请求输入。
type AgentSessionCompactIn struct {
	// AssistantMessageID 折叠边界的助手消息 ID：折叠该助手回答及其之前的尚未摘要消息（下界为当前
	// 滚动摘要游标 summary_upto_seq）；为 0 时保持全量语义（折叠全部尚未摘要消息）。口径由「第 N 轮
	// 用户提问」改为「以助手回答为边界」，故入参按消息 ID 而非用户消息序号定位。
	AssistantMessageID uint `json:"assistantMessageID"`
}

// AgentChatIn 定义启动（或抢占）一轮对话的请求输入。
type AgentChatIn struct {
	// Query 用户本次提问文本，必填且不能为空白
	Query string `json:"query" binding:"required,notBlank"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData map[string]any `json:"businessData"`
	// Thinking 思考模式，取 on/off，空表示不注入思考开关
	Thinking string `json:"thinking" binding:"omitempty,oneof=on off"`
	// Slot 本次生成的显式档位（flash/standard/advanced），可选；为空时依次回退到会话档位与
	// agent.defaultSlot 参数
	Slot string `json:"slot"`
	// SkillID 本轮选用的技能 ID（内置与自定义统一），可选；为 0 表示不注入技能指引，
	// 非 0 时须存在且已启用（自定义技能还须属于当前命名空间）
	SkillID uint `json:"skillID"`
}

// AgentLiveRunOut 定义一轮会话的实时生命周期快照（turns.Manager.State）。
type AgentLiveRunOut struct {
	// RunID 在途轮次的运行标识；无在途运行时为空。前端据此订阅 /events?after= 续播。
	RunID string `json:"runID"`
	// Running 当前是否有回合正在执行
	Running bool `json:"running"`
	// Interrupted 是否挂起等待用户输入（审批 / 问卷）
	Interrupted bool `json:"interrupted"`
	// PendingInterruptIDs 挂起中的根 interrupt ID 列表
	PendingInterruptIDs []string `json:"pendingInterruptIds"`
	// UserTurns 该会话有效轮次
	UserTurns int `json:"userTurns"`
	// LastErr 最近一轮的错误文本，空表示最近一轮成功（或尚无回合）
	LastErr string `json:"lastErr,omitempty"`
	// InteractionState 该会话当前交互主状态（与 AgentSessionItemOut 同口径）
	InteractionState string `json:"interactionState"`
	// InteractionSubstate 该会话当前交互子状态
	InteractionSubstate string `json:"interactionSubstate"`
	// Locked 该会话是否已锁定
	Locked bool `json:"locked"`
	// MaxUserTurns 该会话轮次上限（参数 session.maxUserTurns 的生效值）
	MaxUserTurns int `json:"maxUserTurns"`
}

// AgentCancelOut 定义中止某会话当前回合的输出。
type AgentCancelOut struct {
	// Cancelled 是否已受理中止（无在途回合时幂等返回 true）
	Cancelled bool `json:"cancelled"`
}

// AgentStatusIn 定义查询 AI 助手状态的请求输入。
type AgentStatusIn struct {
	// SessionID 需要查询回合状态的会话 ID，可选；为 0 时不查询任何会话
	SessionID uint `json:"sessionID"`
}

// AgentSlotStatusOut 定义单个档位的绑定模型与可用性输出项。
type AgentSlotStatusOut struct {
	// Slot 档位标识（flash/standard/advanced/embedding）
	Slot string `json:"slot"`
	// ModelName 绑定模型在注册表中的本地标识（无绑定时为空）
	ModelName string `json:"modelName"`
	// ProviderModel 供应商侧的模型名（无绑定时为空）
	ProviderModel string `json:"providerModel"`
	// Kind 绑定模型类型（chat/embedding；无绑定时为空）
	Kind llm.ModelKind `json:"kind"`
	// APIMode 对话模型接入的 API 形态（chat / responses；非对话档位为空）
	APIMode llm.APIMode `json:"apiMode,omitempty"`
	// Enabled 绑定模型是否启用
	Enabled bool `json:"enabled"`
	// Bound 绑定是否可用（模型已启用且类型与档位匹配）
	Bound bool `json:"bound"`
	// Available 该档位当前是否可解析到已构建的客户端
	Available bool `json:"available"`
	// ContextTokenLimit 该档位的上下文 token 容量（≤ 0 表示该档位关闭占用展示与自动压缩）
	ContextTokenLimit int `json:"contextTokenLimit"`
	// ThinkingToggle 该档位绑定模型是否支持思考开关（思考字段与开关取值齐备时为 true）
	ThinkingToggle bool `json:"thinkingToggle"`
}

// AgentStatusOut 定义 AI 助手状态输出。
type AgentStatusOut struct {
	// Enabled AI 能力是否已完整装配启用
	Enabled bool `json:"enabled"`
	// ChatModel 默认对话档位（standard）绑定的模型名称
	ChatModel string `json:"chatModel"`
	// EmbeddingModel Embedding 模型名称
	EmbeddingModel string `json:"embeddingModel"`
	// EmbeddingDimensions 配置的向量维度
	EmbeddingDimensions int `json:"embeddingDimensions"`
	// DimensionMismatch 数据库 embedding 列实际维度与配置是否不一致
	DimensionMismatch bool `json:"dimensionMismatch"`
	// DimensionDetail 维度不一致的详细说明（一致时为空）
	DimensionDetail string `json:"dimensionDetail"`
	// Slots 各档位绑定的模型与可用性（含不可用绑定），按 modelsai.AllSlots 顺序
	Slots []AgentSlotStatusOut `json:"slots"`
	// DefaultSlot 未显式指定档位且会话未记录档位时使用的默认档位（参数 agent.defaultSlot 的
	// 生效值），供前端档位下拉取初值，避免前端自持档位字面量
	DefaultSlot string `json:"defaultSlot"`
	// AssistantAgentKind 通用助手创建会话时使用的 Agent 类型（前端据此创建会话，避免声明枚举字面量）
	AssistantAgentKind string `json:"assistantAgentKind"`
	// DefaultReviewValve 会话未创建或未记录阀门时使用的默认审核阀门档位，供前端在会话尚未创建时
	// 为阀门下拉取初值，避免前端自持枚举字面量
	DefaultReviewValve string `json:"defaultReviewValve"`
	// ParamOverrideCount 当前值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
	ParamOverrideCount int `json:"paramOverrideCount"`
	// CompactThresholdRatio 触发自动压缩的占用比例阈值
	CompactThresholdRatio float64 `json:"compactThresholdRatio"`
	// MaxUserTurns 会话轮次上限（参数 session.maxUserTurns 的生效值），供前端在尚无会话时展示
	// 闸门口径（≤ 0 表示不设上限，永不锁定）
	MaxUserTurns int `json:"maxUserTurns"`
	// Reason AI 能力不可用的原因说明（按请求语言本地化；可用时为空）
	Reason string `json:"reason"`
	// LiveRun 指定会话当前回合的状态快照（请求未指定会话或该会话尚未产生回合时为 nil），
	// 供刷新后前端发现在跑 / 挂起待输入的会话
	LiveRun *AgentLiveRunOut `json:"liveRun,omitempty"`
}

// newAgentSessionItemOut 将会话模型映射为视图层输出项：上下文占用（contextTokens）由调用方按
// 场景提供（新建/分支为 0，列表批量取、详情等单条取）；容量（contextLimit）取自运行时快照中
// 会话有效档位绑定模型的上下文容量，有效档位为会话记录的 TargetModel，为空时回落 agent.defaultSlot；
// 交互状态五项（state/substate/locked/userTurns/maxUserTurns）由调用方按 interactionSnapshot 取快照填入，
// 与守卫判定同口径。
func newAgentSessionItemOut(s *modelsai.AgentSession, snap *llmconfig.Snapshot, contextTokens int, interaction interaction.InteractionSnapshot) AgentSessionItemOut {
	slot := s.TargetModel
	if slot == "" {
		slot = snap.DefaultSlot()
	}
	return AgentSessionItemOut{
		ID:                  s.ID,
		Title:               s.Title,
		Slot:                string(s.TargetModel),
		Status:              string(s.Status),
		AgentKind:           string(s.AgentKind),
		ReviewValve:         string(s.ReviewValve),
		Summary:             string(s.Summary.Value()),
		ContextTokens:       contextTokens,
		ContextLimit:        snap.SlotContextLimit(slot),
		InteractionState:    string(interaction.State),
		InteractionSubstate: string(interaction.Substate),
		Locked:              interaction.Locked,
		UserTurns:           interaction.UserTurns,
		MaxUserTurns:        interaction.MaxUserTurns,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}
}

// newAgentMessageItemOut 将消息模型映射为视图层输出项：正文优先从原生消息载荷（Payload）解码，
// 仅含正文的行回落为 Content；引用载荷经 JSON 反序列化暴露，空或非法时留空（omitempty 省略）。
func newAgentMessageItemOut(m *modelsai.AgentMessage) AgentMessageItemOut {
	out := AgentMessageItemOut{
		ID:               m.ID,
		Role:             string(m.Role),
		Content:          agentMessageText(m),
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		ElapsedMs:        m.ElapsedMs,
		Phase:            string(m.Phase),
		SkillID:          modelsai.SkillIDFromColumn(m.SkillID),
		SkillName:        m.SkillName,
		CreatedAt:        m.CreatedAt,
	}
	if citationsJSON := m.CitationsJSON.Value(); len(citationsJSON) > 0 {
		var citations []retrieval.RetrievalResult
		if err := json.Unmarshal(citationsJSON, &citations); err == nil {
			out.Citations = citations
		}
	}
	return out
}

// agentMessageText 从消息的原生载荷解码正文，载荷缺失或不可解码时回落为 Content 列。
func agentMessageText(m *modelsai.AgentMessage) string {
	if m == nil {
		return ""
	}
	if payload := m.Payload.Value(); len(payload) > 0 {
		kind := llm.APIModeChat
		if m.MessageKind != "" {
			if parsed, err := llm.ParseAPIMode(m.MessageKind); err == nil {
				kind = parsed
			}
		}
		switch kind {
		case llm.APIModeResponses:
			if msg, err := msgops.DecodeForModel[*schema.AgenticMessage](payload, kind); err == nil {
				return msgops.Text(msg)
			}
		default:
			if msg, err := msgops.DecodeForModel[*schema.Message](payload, kind); err == nil {
				return msgops.Text(msg)
			}
		}
	}
	return string(m.Content.Value())
}

// AgentSessionCreate 处理创建 AI 助手会话的请求。
// 新会话归属当前请求命名空间，初始状态为 active；会话主题由首个用户问题经 LLM 生成，失败时
// 按其截断兜底，绝不阻断创建。可选档位字段经校验后作为该会话的缺省档位落库，可选 Agent 类型
// 字段经校验后作为该会话的归属落库（缺省 knowledge）；可选审核阀门字段经校验后作为该会话的
// 审核阀门落库（缺省经 defaults 库以 each 兜底）；三类字段的非法取值均以 400 + 英文错误拒绝
// （不静默回落）。
func AgentSessionCreate(c *gin.Context) {
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
	var in AgentSessionCreateIn
	// 缺省字段经 defaults 库在绑定前填充（reviewValve 缺省 each），再由显式传入值覆盖。
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	slot := modelsai.Slot("")
	if value := modelsai.Slot(strings.TrimSpace(in.Slot)); value != "" {
		parsed, ok := modelsai.ParseChatSlot(value)
		if !ok {
			response.CreateBadBodyError(modelsai.InvalidChatSlotError(value)).AbortWithGinH(c)
			return
		}
		slot = parsed
	}
	agentKind, err := modelsai.ParseAgentKind(modelsai.AgentKind(in.AgentKind))
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	query := strings.TrimSpace(in.Query)
	topic := memory.SessionTopicFallback(query)
	if comps.Memory != nil {
		topic = comps.Memory.GenerateTopic(c, query)
	}
	session, err := comps.Sessions.CreateSession(c, namespace, topic, slot, agentKind, userID)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 会话初始审核阀门按请求落库：缺省值 each（逐一审核）已由 defaults 库在绑定前填充，此处必为合法取值。
	updated, err := comps.Sessions.UpdateSessionValve(c, session.ID, namespace, userID, modelsai.ReviewValve(in.ReviewValve))
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	session = updated
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session, middlewares.GetAiRuntime(c).Snapshot(), 0, interactionSnapshot(c, comps, session)))
}

// AgentSessionListGet 处理获取 AI 助手会话列表的请求。
// 仅返回当前命名空间内的会话，按更新时间倒序（最近活跃在前）。
func AgentSessionListGet(c *gin.Context) {
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
	sessions, err := comps.Sessions.ListSessions(c, namespace, userID, 0, 0)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	sessionIDs := make([]uint, 0, len(sessions))
	for _, s := range sessions {
		sessionIDs = append(sessionIDs, s.ID)
	}
	// 上下文占用取「末次真实测量与更晚的压缩后估算中更晚者」（见 sessionstore.Store.ContextUsage），
	// 使压缩后尚无生成落定的会话不再显示压缩前的高占用。该度量是列表展示的一部分，故取失败与
	// 会话列表查询同等对待——以意外错误中断，绝不静默降级为 0（静默归零会被读成「占用已清空」）。
	contextTokens, err := comps.Sessions.ContextUsageBySession(c, namespace, userID, sessionIDs)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(AgentSessionListOut, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, newAgentSessionItemOut(s, middlewares.GetAiRuntime(c).Snapshot(), contextTokens[s.ID], interactionSnapshot(c, comps, s)))
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentSessionDetailGet 处理获取 AI 助手会话详情的请求。
// 会话不存在或不属于当前命名空间时返回 404。
func AgentSessionDetailGet(c *gin.Context) {
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
	session := getOwnedAgentSession(c, comps, uint(id), namespace, userID)
	if session == nil {
		return
	}
	// 上下文占用取失败（含无助手消息）按 0 处理，不因此失败请求。
	contextTokens, err := comps.Sessions.ContextUsage(c, session.ID, namespace, userID)
	if err != nil {
		contextTokens = 0
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session, middlewares.GetAiRuntime(c).Snapshot(), contextTokens, interactionSnapshot(c, comps, session)))
}

// AgentSessionRename 处理重命名 AI 助手会话的请求。
// 会话不存在或不属于当前命名空间时返回 404。
func AgentSessionRename(c *gin.Context) {
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
	var in AgentSessionRenameIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if session := getOwnedAgentSession(c, comps, in.ID, namespace, userID); session == nil {
		return
	}
	if _, err := comps.Sessions.RenameSession(c, in.ID, namespace, userID, in.Title); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionValveSet 处理更新 AI 助手会话审核阀门的请求（路由 /session/:id/valve）。
// 依次校验命名空间、会话 ID、AI 功能与会话归属（404 语义同其它接口，避免泄露资源存在性），
// 绑定请求体（阀门取值经 enum=reviewValve 绑定标签校验，非法取值以 400 拒绝、不落库），
// 随后更新该会话的审核阀门并返回更新后的会话项（与详情/列表出参同构）。
func AgentSessionValveSet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "id")
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
	if session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID); session == nil {
		return
	}
	var in AgentSessionValveIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	updated, err := comps.Sessions.UpdateSessionValve(c, uint(sessionID), namespace, userID, modelsai.ReviewValve(in.ReviewValve))
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 上下文占用取失败（含无助手消息）按 0 处理，不因此失败请求。
	contextTokens, err := comps.Sessions.ContextUsage(c, updated.ID, namespace, userID)
	if err != nil {
		contextTokens = 0
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(updated, middlewares.GetAiRuntime(c).Snapshot(), contextTokens, interactionSnapshot(c, comps, updated)))
}

// AgentSessionDelete 处理删除 AI 助手会话的请求。
// 会话不存在或不属于当前命名空间时返回 404；删除会话时其消息在同一事务内级联删除。
func AgentSessionDelete(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace, userID); session == nil {
		return
	}
	// 会话删除前先停掉并遗忘其 TurnLoop，避免循环在删除后仍有产出。
	if comps.Turns != nil {
		if err := comps.Turns.StopSession(c.Request.Context(), uint(id)); err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "ai session delete: stop turn session %d failed: %v", id, err)
		}
	}
	if err := comps.Sessions.DeleteSession(c, uint(id), namespace, userID); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionCompact 处理压缩 AI 助手会话上下文并抽取长期记忆的请求（SSE，路由
// POST /session/:id/compact）。
// 口径：入参以选中的助手回答为折叠边界——折叠「当前滚动摘要游标 summary_upto_seq（下界）之后、到该
// 助手回答 seq（上界）为止」的尚未摘要消息并把游标推进到该答案，更晚的消息保持原文；边界为 0 时折叠
// 全部尚未摘要消息（全量语义）。压缩进度以 message.compress 事件下发：开始（running）、
// 摘要增量（draft）与完成 / 失败；失败仅降级为「未产生摘要」，不中断 SSE。
// 处理顺序严格：校验命名空间 / AI 功能 / 属主 / 会话归属（404 语义同详情接口）→ 交互状态守卫（仅
// 暂停 / 已锁定 可发起，运行中 / 等待中 → 409+1306）→ 绑定请求体 → 解析边界 seq（消息不存在 →
// 本地化 404）→ 读未摘要消息并选出折叠集合 → 构造 SSE 写手并同步执行压缩。所有可能返回 JSON 错误的
// 步骤都在写出 SSE 响应头之前完成。
func AgentSessionCompact(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Agent == nil || comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the agent runtime is not assembled")).AbortWithGinH(c)
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
	session := getOwnedAgentSession(c, comps, uint(id), namespace, userID)
	if session == nil {
		return
	}
	// 压缩守卫：仅 `暂停` / `已锁定` 允许，`运行中` / `等待中` 以 409 + 1306 拒绝（口径与回滚一致）。
	// 须在绑定请求体与读库之前生效，绝不产生半截压缩。
	if rejectGuardError(c, interaction.CanCompact(agentInteractionFacts(c, comps, session))) {
		return
	}
	var in AgentSessionCompactIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 折叠上界：AssistantMessageID > 0 时按助手消息 ID 定位其 seq；为 0 时保持全量语义（uptoSeq=0）。
	var uptoSeq uint64
	if in.AssistantMessageID > 0 {
		seq, err := comps.Sessions.AssistantSeqAtMessage(c, session.ID, namespace, userID, in.AssistantMessageID)
		if err != nil {
			if errors.Is(err, sessionstore.ErrMessageNotFound) {
				response.ErrDefaultPageNotFound.AbortWithGinH(c)
				return
			}
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		uptoSeq = seq
	}
	// 下界 = 当前 summary_upto_seq（只取尚未摘要消息），上界 = 选中的助手回答 seq。
	messages, err := comps.Sessions.ListMessagesAfter(c, session.ID, namespace, userID, session.SummaryUptoSeq)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	evicted := compactFoldSet(messages, uptoSeq)

	streamCompactEvents(c, session.ID, in.AssistantMessageID, len(evicted), func(report contract.CompressReporter) {
		comps.Agent.CompressSession(c.Request.Context(), session, evicted, "manual", session.TargetModel, report)
	})
}

// compactFoldSet 从「尚未摘要的消息」（已按 seq 升序）中选出本次压缩要折叠的集合：uptoSeq 为 0 时取全量
// （全量语义），否则只取 seq <= uptoSeq 的前缀（遇到第一条 seq > uptoSeq 即止），使更晚的消息保持原文。
// 独立成纯函数以便按折叠区间口径直接断言，无需经在途活动。
func compactFoldSet(messages []*modelsai.AgentMessage, uptoSeq uint64) []*modelsai.AgentMessage {
	if uptoSeq == 0 {
		return messages
	}
	evicted := make([]*modelsai.AgentMessage, 0, len(messages))
	for _, m := range messages {
		if m.Seq > uptoSeq {
			break
		}
		evicted = append(evicted, m)
	}
	return evicted
}

// AgentPromptOptimizeIn 定义提示词优化的请求输入。
type AgentPromptOptimizeIn struct {
	// Query 待优化的用户诉求，必填且不能为空白
	Query string `json:"query" binding:"required,notBlank"`
}

// AgentPromptOptimizeOut 定义提示词优化的输出。
type AgentPromptOptimizeOut struct {
	// Query 优化后的提问文本，供前端整体替换输入框内容
	Query string `json:"query"`
}

// AgentPromptOptimize 处理优化用户提问的请求（路由 POST /prompt/optimize）。
// 单轮、非流式的轻量能力：以参数 agent.promptOptimizeSystemPrompt 驱动 flash 档位模型把当前诉求
// 改写为更清晰、更易被正确理解的提问，成功时回传改写文本供前端整体替换输入框。依次校验命名空间与
// AI 功能；绑定请求体后诉求裁剪为空返回 400（本地化），模型调用失败或返回空内容以统一错误封套返回。
func AgentPromptOptimize(c *gin.Context) {
	if _, ok := middlewares.RequireNamespace(c); !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentPromptOptimizeIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	query := strings.TrimSpace(in.Query)
	out, err := comps.Agent.OptimizePrompt(c, query)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, AgentPromptOptimizeOut{Query: out})
}

// AgentMessageListGet 处理获取 AI 助手会话消息列表的请求。
// 会话不存在或不属于当前命名空间时返回 404；消息按创建时间升序返回（与对话顺序一致）。
func AgentMessageListGet(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace, userID); session == nil {
		return
	}
	messages, err := comps.Sessions.ListMessages(c, uint(id), namespace, userID, 0, 0)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 历史压缩记录：取该会话的全部压缩检查点（含静默滚动 rolling），按压缩游标 upto_seq 归并——
	// 游标等于某条消息 seq 即把该记录锚定到这条消息上。静默滚动推进不产生实时事件，故其记录仅在
	// 刷新 / 重进会话时随历史回显。
	checkpoints, err := comps.Sessions.ListCompactions(c, uint(id), userID)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	compressBySeq := make(map[uint64]*AgentMessageCompressOut, len(checkpoints))
	for _, cp := range checkpoints {
		compressBySeq[cp.UptoSeq] = &AgentMessageCompressOut{
			Reason:       cp.Reason,
			TokensBefore: cp.TokensBefore,
			FoldedCount:  cp.FoldedCount,
			TokensAfter:  cp.TokensAfter,
		}
	}
	// 有序列的只读消息出参：正文由原生载荷解码（见 newAgentMessageItemOut），引用与用量随行给出。
	out := make(AgentMessageListOut, 0, len(messages))
	for _, m := range messages {
		item := newAgentMessageItemOut(m)
		item.Compress = compressBySeq[m.Seq]
		out = append(out, item)
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentMessageTruncate 处理从某一轮用户消息起截断会话消息的请求（路由
// POST /session/:id/messages/truncate）。
// 供前端「回退 / 删除到某一轮对话」使用：按 UserMessageIndex（仅计用户消息）定位该轮提问，删除其
// 自身及之后的全部消息（含助手回复）。会话不存在 / 归属不符返回 404；目标用户消息不存在（序号越界）
// 同样返回 404，避免泄露会话内容细节。
// 状态守卫（口径来源见 interaction.CanRollback）：回滚是一次「改动有效轮次」的既成事实操作，`暂停` / `已锁定` /
// `等待中` 均允许发起——等待中回滚会直接放弃当前待输入回合（store 在同一事务内把该会话的 pending 现场
// 置为已消费）；只有 `运行中` 无稳定截断对象，一律以 409 + 业务码 1306 拒绝（守卫在绑定请求体与调用 store
// 之前生效，绝不产生半截截断）。
// 截断同时作废该会话的挂起现场（见 store）；内存中的回合再显式中止一次，作为「读取事实」与「执行截断」
// 之间竞态窗口的兜底（并发发送可能恰在守卫判定后启动一次回合）——否则客户端刷新 / 重开抽屉时会探测到
// 这个仍在「运行中」的回合并续播已删除的内容。中止为尽力而为，失败不影响截断结果。
func AgentMessageTruncate(c *gin.Context) {
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
	session := getOwnedAgentSession(c, comps, uint(id), namespace, userID)
	if session == nil {
		return
	}
	// 回滚守卫：仅 `运行中` 以 409 + 1306 拒绝；`暂停` / `已锁定` / `等待中` 均放行（守卫在绑定请求体
	// 与调用 store 之前生效）。
	if rejectGuardError(c, interaction.CanRollback(agentInteractionFacts(c, comps, session))) {
		return
	}
	var in AgentMessageTruncateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Sessions.TruncateMessagesFromUserMessage(c, uint(id), userID, in.UserMessageIndex); err != nil {
		if errors.Is(err, sessionstore.ErrMessageNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 竞态兜底：截断已作废挂起现场；内存回合再中止一次，避免其继续写回已删除的回合。
	if comps.Turns != nil {
		if err := comps.Turns.Abort(c.Request.Context(), uint(id), userID, namespace); err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "ai session truncate: abort turn session %d failed: %v", id, err)
		}
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionBranch 处理以某一轮对话为结尾创建会话分支的请求（路由 POST /session/:id/branch）。
// 供前端「分支」使用：以第 UserMessageIndex 轮用户提问为结尾，把该轮及其之前的全部消息复制到
// 新会话（新会话归属同一命名空间与属主，标题为空时沿用源会话标题），源会话不受影响。依次校验
// 命名空间、AI 功能、属主与会话归属（会话不存在 / 归属不符返回 404，避免泄露资源存在性），
// 绑定请求体后调用 service；目标用户消息不存在（序号越界）同样返回 404，避免泄露会话内容细节。
func AgentSessionBranch(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace, userID); session == nil {
		return
	}
	var in AgentSessionBranchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	branched, err := comps.Sessions.BranchSessionFromUserMessage(c, uint(id), userID, in.UserMessageIndex, in.Title)
	if err != nil {
		if errors.Is(err, sessionstore.ErrMessageNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(branched, middlewares.GetAiRuntime(c).Snapshot(), 0, interactionSnapshot(c, comps, branched)))
}

// AgentStatusGet 处理获取 AI 助手状态的请求（不强制启用，始终 200）。
// 从运行时快照读取 AI 能力是否启用、各档位绑定的模型与可用性、生效的参数覆盖数量；
// 未启用时给出明确原因（按请求语言本地化）。启用时额外查询数据库 knowledge_chunks.embedding 与
// agent_memories.embedding 两列的实际维度并与绑定模型维度对比，不一致时提示风险；请求体携带
// 可选 SessionID 时再附上该会话当前回合状态快照（供刷新后前端发现在跑 / 挂起待输入）。
func AgentStatusGet(c *gin.Context) {
	// 请求体可选：无会话 ID 时保持零值（不查询任何会话），全程不因缺体而失败。
	var in AgentStatusIn
	// 请求体可选：仅忽略「未携带请求体」；格式非法或类型错误必须按 400 返回，不能静默当作零值。
	if err := c.ShouldBindBodyWithJSON(&in); err != nil && !errors.Is(err, io.EOF) {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	rt := middlewares.GetAiRuntime(c)
	snap := rt.Snapshot()
	comps := rt.Current()
	out := AgentStatusOut{Enabled: rt.Enabled(), Reason: snap.DisabledReason(middlewares.GetLanguage(c))}
	out.DefaultSlot = string(snap.DefaultSlot())
	out.AssistantAgentKind = string(modelsai.AgentKindGeneral)
	out.DefaultReviewValve = string(modelsai.DefaultReviewValve)
	slots := snap.Slots()
	out.Slots = make([]AgentSlotStatusOut, 0, len(modelsai.AllSlots))
	for _, slot := range modelsai.AllSlots {
		item := AgentSlotStatusOut{Slot: string(slot)}
		if info, ok := slots[slot]; ok {
			item.ModelName = info.Name
			item.ProviderModel = info.Model
			item.Kind = info.Kind
			item.APIMode = info.APIMode
			item.Enabled = info.Enabled
			item.Bound = info.Bound
		}
		// 可用性取运行时实际构建结果：对话档位查客户端表，向量化槽位查向量化客户端
		if slot == modelsai.SlotEmbedding {
			_, err := rt.Embed()
			item.Available = err == nil
		} else {
			_, err := rt.Chat(slot)
			item.Available = err == nil
		}
		// 该档位的上下文 token 容量：以 agentkit 运行时的有效档位容量为准，装配缺失时回落配置快照。
		item.ContextTokenLimit = snap.SlotContextLimit(slot)
		if comps.Agent != nil && slot != modelsai.SlotEmbedding {
			item.ContextTokenLimit = comps.Agent.ContextLimit(slot)
		}
		// 该档位绑定模型是否支持思考开关：思考字段与开关取值三者齐备才算支持（不可用绑定无配置）
		if cfg, err := snap.ModelConfig(slot); err == nil {
			item.ThinkingToggle = cfg.ThinkingField != "" && cfg.ThinkingOnValue != nil && cfg.ThinkingOffValue != nil
		}
		out.Slots = append(out.Slots, item)
	}
	if info, ok := slots[modelsai.SlotStandard]; ok {
		out.ChatModel = info.Model
	}
	if info, ok := slots[modelsai.SlotEmbedding]; ok {
		out.EmbeddingModel = info.Model
	}
	out.EmbeddingDimensions = snap.EmbeddingDimensions()
	out.ParamOverrideCount = snap.ParamOverrideCount()
	out.CompactThresholdRatio = snap.Params().Float("session.compactThresholdRatio")
	out.MaxUserTurns = snap.Params().Int("session.maxUserTurns")
	if out.Enabled {
		db := middlewares.GetCoreDatabase(c)
		// 维度自检覆盖两列受治理的向量列：knowledge_chunks.embedding 与 agent_memories.embedding；
		// 任一列可读且与绑定模型维度不一致即报出 dimensionMismatch（读不出时交由迁移建列）。
		if n, found, err := llminfra.ChunkEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: knowledge_chunks.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		} else if n, found, err := llminfra.MemoryEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: agent_memories.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		}
	}
	// 指定会话且 AI 已启用时附上该会话当前回合状态快照（供刷新后前端发现「在跑 / 待输入」）；
	// 状态按 (namespace, user_id) 作用域隔离：未认证（GetUser 失败）时跳过，不泄露他人会话。
	if out.Enabled && in.SessionID != 0 && comps.Turns != nil && comps.Sessions != nil {
		if user, codeErr := middlewares.GetUser(c); codeErr == nil {
			namespace, nsErr := middlewares.GetNamespace(c)
			// 先解出会话再取状态：交互五项须由 interactionSnapshot 推导，而事实装配需要会话本体；
			// 取不到会话（不存在 / 归属不符 / 已删除）时不附快照——避免给出空状态。
			if nsErr == nil {
				if session, serr := comps.Sessions.GetSession(c.Request.Context(), in.SessionID, namespace, user.ID); serr == nil {
					if st, err := comps.Turns.State(c.Request.Context(), in.SessionID, user.ID, namespace); err == nil {
						out.LiveRun = newAgentLiveRunOut(st, interactionSnapshot(c, comps, session))
					}
				}
			}
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// newAgentLiveRunOut 把 turns.Manager.State 映射为视图层输出项；交互状态五项由调用方按
// interactionSnapshot 取快照填入（与会话出参同口径）。
func newAgentLiveRunOut(st turns.State, snapshot interaction.InteractionSnapshot) *AgentLiveRunOut {
	return &AgentLiveRunOut{
		RunID:               st.RunID,
		Running:             st.Running,
		Interrupted:         st.Interrupted,
		PendingInterruptIDs: append([]string{}, st.PendingInterruptIDs...),
		UserTurns:           st.UserTurns,
		LastErr:             st.LastErr,
		InteractionState:    string(snapshot.State),
		InteractionSubstate: string(snapshot.Substate),
		Locked:              snapshot.Locked,
		MaxUserTurns:        snapshot.MaxUserTurns,
	}
}
