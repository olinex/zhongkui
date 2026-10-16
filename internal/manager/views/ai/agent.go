// Package ai 提供 AI 知识域（Agent / 知识库 / 会话）的 HTTP 处理器与视图 In/Out 结构体，
// 向内封装 internal/ai 的组件能力。
package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/sse"
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
		if errors.Is(err, ai.ErrSessionNotFound) {
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
	Slot modelsai.Slot `json:"slot"`
	// Status 会话状态（active/archived）
	Status modelsai.SessionStatus `json:"status"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego）
	AgentKind modelsai.AgentKind `json:"agentKind"`
	// ReviewValve 会话级审核阀门（each/auto/none），与本会话内 client 工具调用的审核重要级合成，
	// 决定调用是否需人工审核
	ReviewValve modelsai.ReviewValve `json:"reviewValve"`
	// Summary 会话滚动摘要，无摘要时为空串
	Summary string `json:"summary"`
	// ContextTokens 会话最近一条助手消息的输入 token 数（上下文占用度量；无助手消息为 0）
	ContextTokens int `json:"contextTokens"`
	// ContextLimit 会话有效档位的上下文 token 容量（≤ 0 表示该档位关闭占用展示与自动压缩）
	ContextLimit int `json:"contextLimit"`
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
	Slot modelsai.Slot `json:"slot"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego），可选；缺省 knowledge，非法取值以 400 +
	// 英文错误拒绝
	AgentKind modelsai.AgentKind `json:"agentKind"`
	// ReviewValve 会话级审核阀门（each/auto/none），可选；缺省经 defaults 库以 each（逐一审核）兜底，
	// 空值或取值空间外的取值以 400 + 英文错误拒绝
	ReviewValve modelsai.ReviewValve `json:"reviewValve" default:"each" binding:"enum=reviewValve"`
}

// AgentSessionValveIn 定义更新 AI 助手会话审核阀门的请求输入。
type AgentSessionValveIn struct {
	// ReviewValve 会话新审核阀门（each/auto/none），必填；非取值空间内的取值以 400 + 英文错误拒绝
	ReviewValve modelsai.ReviewValve `json:"reviewValve" binding:"required,enum=reviewValve"`
}

// AgentSessionRenameIn 定义重命名 AI 助手会话的请求输入。
type AgentSessionRenameIn struct {
	// ID 会话 ID，必填
	ID uint `json:"id" binding:"required"`
	// Title 会话新标题，必填
	Title string `json:"title" binding:"required"`
}

// AgentMessageItemOut 定义 AI 助手会话消息的输出项结构。
type AgentMessageItemOut struct {
	// ID 消息 ID
	ID uint `json:"id"`
	// Role 消息角色（user/assistant/system）
	Role modelsai.MessageRole `json:"role"`
	// Content 消息正文内容
	Content string `json:"content"`
	// Reasoning 助手回复的思考过程全文，无则省略
	Reasoning string `json:"reasoning,omitempty"`
	// Draft 助手回复的结构化草稿载荷（JSON 对象），无草稿时省略
	Draft json.RawMessage `json:"draft,omitempty"`
	// Citations 助手回复的检索引用来源列表，无引用时省略
	Citations []ai.RetrievalResult `json:"citations,omitempty"`
	// PromptTokens 本轮助手回答全部模型调用的输入 token 累加（用户消息为 0）
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 本轮助手回答全部模型调用的输出 token 累加（用户消息为 0）
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 本轮助手回答全部模型调用的 token 总量累加（用户消息为 0）
	TotalTokens int `json:"totalTokens"`
	// ElapsedMs 本轮助手回答的总耗时（毫秒，用户消息为 0）
	ElapsedMs int64 `json:"elapsedMs"`
	// Phase 本条助手消息所属运行的生命周期终态阶段（done/failed/cancelled），空表示尚未终结
	// （用户消息与在途运行的阶段性落库行）；前端据 failed/cancelled 展示「本轮被中断」提示
	Phase modelsai.AgentRunPhase `json:"phase,omitempty"`
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
	// UserMessageIndex 折叠边界的用户消息序号（从 1 起，仅计用户消息、按 seq 升序）：折叠该轮提问
	// 连同其助手回复以及更早的尚未摘要消息；为 0 时保持全量语义（折叠全部尚未摘要消息）。
	UserMessageIndex int `json:"userMessageIndex"`
}

// AgentStreamIn 定义 AI 助手流式生成的请求输入。
type AgentStreamIn struct {
	// Query 用户本次提问文本，必填
	Query string `json:"query" binding:"required"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData ai.BusinessData `json:"businessData"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码，为空跳过规则校验
	RegoRule string `json:"regoRule"`
	// Thinking 思考模式，取 on/off，空表示不注入思考开关
	Thinking string `json:"thinking" binding:"omitempty,oneof=on off"`
	// Slot 本次生成的显式档位（flash/standard/advanced），可选；为空时依次回退到会话档位与
	// agent.defaultSlot 参数
	Slot modelsai.Slot `json:"slot"`
	// After 订阅起点序号（仅转发序号 > after 的事件）；0 表示从头转发，供重连续播时跳过已收事件
	After uint64 `json:"after"`
}

// AgentStreamResumeIn 定义续播某会话在途运行的请求输入（经 URL 查询参数传入）。
type AgentStreamResumeIn struct {
	// After 回放起点序号（仅回放 seq > after 的事件）；0 表示从头回放
	After uint64 `json:"after" form:"after"`
}

// AgentPendingCallOut 定义一条待执行的工具调用（供刷新后重建待审批现场）。
type AgentPendingCallOut struct {
	// CallID 与模型 tool call id 一致
	CallID string `json:"callID"`
	// ToolName 待执行的工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级（取值集合与展示文案由后端枚举类 auditLevel 下发）
	AuditLevel ai.AuditLevel `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：false 表示无需人工审批
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属（client / backend）：client 由前端在用户自身凭据下执行，backend 由后端
	// 在恢复续跑时自行执行（已批准的 builtin / MCP 工具）
	ExecOwner ai.ToolExecOwner `json:"execOwner"`
}

// AgentLiveRunOut 定义一次驻留运行的当前状态快照（供刷新后发现在途运行与待审批现场）。
type AgentLiveRunOut struct {
	// RunID 运行标识
	RunID serializer.HexUUID `json:"runID"`
	// Phase 运行生命周期阶段（running/done/failed/cancelled/suspended）
	Phase modelsai.AgentRunPhase `json:"phase"`
	// LastSeq 已发布的最大事件序号（续播起点）
	LastSeq uint64 `json:"lastSeq"`
	// AwaitingApproval 是否挂起待审批（phase 为 suspended 时为 true）
	AwaitingApproval bool `json:"awaitingApproval"`
	// PendingCalls 挂起待审批时的待执行调用列表，非挂起时为空数组
	PendingCalls []AgentPendingCallOut `json:"pendingCalls"`
}

// AgentCancelOut 定义取消某会话在途运行的输出。
type AgentCancelOut struct {
	// Cancelled 是否已受理取消（运行已收敛终态时幂等返回 true）
	Cancelled bool `json:"cancelled"`
}

// AgentStatusIn 定义查询 AI 助手状态的请求输入。
type AgentStatusIn struct {
	// SessionID 需要查询在途运行的会话 ID，可选；为 0 时不查询任何会话的在途运行
	SessionID uint `json:"sessionID"`
}

// AgentToolResultItemIn 定义单条客户端工具调用结果：与挂起时下发的 tool_request 按 callID 对应。
type AgentToolResultItemIn struct {
	// CallID 挂起时下发的调用 ID，必填
	CallID string `json:"callID" binding:"required"`
	// Approved 用户是否批准执行该调用
	Approved bool `json:"approved"`
	// Result 前端以用户自身凭据执行成功后的结果文本
	Result string `json:"result"`
	// Error 前端执行失败时的错误文本
	Error string `json:"error"`
}

// AgentToolResultIn 定义恢复挂起运行、回传客户端工具调用结果的请求输入。
type AgentToolResultIn struct {
	// Results 本次回传的执行结果列表；须覆盖挂起时下发的全部 callID（缺失者按「未提供结果」回灌）
	Results []AgentToolResultItemIn `json:"results" binding:"required,dive"`
}

// AgentSlotStatusOut 定义单个档位的绑定模型与可用性输出项。
type AgentSlotStatusOut struct {
	// Slot 档位标识（flash/standard/advanced/embedding）
	Slot modelsai.Slot `json:"slot"`
	// ModelName 绑定模型在注册表中的本地标识（无绑定时为空）
	ModelName string `json:"modelName"`
	// ProviderModel 供应商侧的模型名（无绑定时为空）
	ProviderModel string `json:"providerModel"`
	// Kind 绑定模型类型（chat/embedding；无绑定时为空）
	Kind llm.ModelKind `json:"kind"`
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
	DefaultSlot modelsai.Slot `json:"defaultSlot"`
	// AssistantAgentKind 通用助手创建会话时使用的 Agent 类型（前端据此创建会话，避免声明枚举字面量）
	AssistantAgentKind modelsai.AgentKind `json:"assistantAgentKind"`
	// DefaultReviewValve 会话未创建或未记录阀门时使用的默认审核阀门档位，供前端在会话尚未创建时
	// 为阀门下拉取初值，避免前端自持枚举字面量
	DefaultReviewValve modelsai.ReviewValve `json:"defaultReviewValve"`
	// ParamOverrideCount 当前值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
	ParamOverrideCount int `json:"paramOverrideCount"`
	// CompactThresholdRatio 触发自动压缩的占用比例阈值
	CompactThresholdRatio float64 `json:"compactThresholdRatio"`
	// Reason AI 能力不可用的原因说明（按请求语言本地化；可用时为空）
	Reason string `json:"reason"`
	// LiveRun 指定会话当前在途运行的状态快照（请求未指定会话或该会话无在途运行时为 nil），
	// 供刷新后前端发现在跑 / 待审批的运行
	LiveRun *AgentLiveRunOut `json:"liveRun,omitempty"`
}

// newAgentSessionItemOut 将会话模型映射为视图层输出项：上下文占用（contextTokens）由调用方按
// 场景提供（新建/分支为 0，列表批量取、详情等单条取）；容量（contextLimit）取自运行时快照中
// 会话有效档位绑定模型的上下文容量，有效档位为会话记录的 TargetModel，为空时回落 agent.defaultSlot。
func newAgentSessionItemOut(s *modelsai.AgentSession, snap *ai.Snapshot, contextTokens int) AgentSessionItemOut {
	slot := s.TargetModel
	if slot == "" {
		slot = snap.DefaultSlot()
	}
	return AgentSessionItemOut{
		ID:            s.ID,
		Title:         s.Title,
		Slot:          s.TargetModel,
		Status:        s.Status,
		AgentKind:     s.AgentKind,
		ReviewValve:   s.ReviewValve,
		Summary:       string(s.Summary.Value()),
		ContextTokens: contextTokens,
		ContextLimit:  snap.SlotContextLimit(slot),
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

// newAgentMessageItemOut 将消息模型映射为视图层输出项：
// 草稿与引用载荷经 JSON 反序列化暴露，空或非法时留空（omitempty 省略）。
func newAgentMessageItemOut(m *modelsai.AgentMessage) AgentMessageItemOut {
	out := AgentMessageItemOut{
		ID:               m.ID,
		Role:             m.Role,
		Content:          string(m.Content.Value()),
		Reasoning:        string(m.Reasoning.Value()),
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		ElapsedMs:        m.ElapsedMs,
		Phase:            m.Phase,
		CreatedAt:        m.CreatedAt,
	}
	if draft := m.DraftJSON.Value(); len(draft) > 0 {
		out.Draft = json.RawMessage(draft)
	}
	if citationsJSON := m.CitationsJSON.Value(); len(citationsJSON) > 0 {
		var citations []ai.RetrievalResult
		if err := json.Unmarshal(citationsJSON, &citations); err == nil {
			out.Citations = citations
		}
	}
	return out
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
	if value := modelsai.Slot(strings.TrimSpace(string(in.Slot))); value != "" {
		parsed, ok := modelsai.ParseChatSlot(value)
		if !ok {
			response.CreateBadBodyError(modelsai.InvalidChatSlotError(value)).AbortWithGinH(c)
			return
		}
		slot = parsed
	}
	agentKind, err := modelsai.ParseAgentKind(in.AgentKind)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	query := strings.TrimSpace(in.Query)
	topic := ai.SessionTopicFallback(query)
	if comps.Memory != nil {
		topic = comps.Memory.GenerateTopic(c, query)
	}
	session, err := comps.Sessions.CreateSession(c, namespace, topic, slot, agentKind, userID)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 会话初始审核阀门按请求落库：缺省值 each（逐一审核）已由 defaults 库在绑定前填充，此处必为合法取值。
	updated, err := comps.Sessions.UpdateSessionValve(c, session.ID, namespace, userID, in.ReviewValve)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	session = updated
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session, middlewares.GetAiRuntime(c).Snapshot(), 0))
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
	// 上下文占用为展示性度量：批量查询异常时降级为全 0，不影响列表返回。
	contextTokens, _ := comps.Sessions.ContextTokensBySession(c, userID, sessionIDs)
	out := make(AgentSessionListOut, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, newAgentSessionItemOut(s, middlewares.GetAiRuntime(c).Snapshot(), contextTokens[s.ID]))
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
	contextTokens, err := comps.Sessions.ContextTokens(c, session.ID, userID)
	if err != nil {
		contextTokens = 0
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session, middlewares.GetAiRuntime(c).Snapshot(), contextTokens))
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
	updated, err := comps.Sessions.UpdateSessionValve(c, uint(sessionID), namespace, userID, in.ReviewValve)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 上下文占用取失败（含无助手消息）按 0 处理，不因此失败请求。
	contextTokens, err := comps.Sessions.ContextTokens(c, updated.ID, userID)
	if err != nil {
		contextTokens = 0
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(updated, middlewares.GetAiRuntime(c).Snapshot(), contextTokens))
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
	if err := comps.Sessions.DeleteSession(c, uint(id), namespace, userID); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionCompact 处理压缩 AI 助手会话上下文并抽取长期记忆的请求（SSE，路由
// POST /session/:id/compact）。
// 先校验会话归属（404 语义同详情接口），再按可选的 userMessageIndex 确定折叠上界：折叠该轮提问
// 连同其助手回复以及更早的尚未摘要消息（userMessageIndex=0 时折叠全部尚未摘要消息，保持全量
// 语义）。压缩事件经 SSE 流写出：以事件类型为事件名，依次下发 compress_start / compress_delta /
// compress_done，data 为事件载荷；摘要在脱管上下文（context.WithoutCancel）中完成并落库，客户端
// 断连不取消摘要生成与落库，写 SSE 失败仅吞掉。该轮序号越界或其后尚无助手回复沿用既有本地化
// 404（与消息截断一致，避免泄露会话内容细节）。
func AgentSessionCompact(c *gin.Context) {
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
	var in AgentSessionCompactIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 折叠上界须在构造 SSE 写手之前解析：NewSSEWriter 一旦构造即写出响应头，此后无法再返回 JSON
	// 错误（本地化 404 / 意外错误）。
	var uptoSeq uint64
	if in.UserMessageIndex > 0 {
		seq, err := comps.Sessions.AssistantSeqAtUserMessage(c, session.ID, userID, in.UserMessageIndex)
		if err != nil {
			if errors.Is(err, ai.ErrMessageNotFound) {
				response.ErrDefaultPageNotFound.AbortWithGinH(c)
				return
			}
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		uptoSeq = seq
	}
	messages, err := comps.Sessions.ListMessagesAfter(c, session.ID, userID, session.SummaryUptoSeq)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 折叠集合：uptoSeq 为 0 时取全量尚未摘要消息；否则按 seq 升序收集 seq <= uptoSeq 的消息
	// （消息已按 seq 升序，遇到第一条 seq > uptoSeq 即止），更晚的消息不折叠。
	var evicted []*modelsai.AgentMessage
	if uptoSeq == 0 {
		evicted = messages
	} else {
		for _, m := range messages {
			if m.Seq > uptoSeq {
				break
			}
			evicted = append(evicted, m)
		}
	}
	// 脱管上下文：客户端断连不取消摘要生成与落库（仿 RunDetached 理念）。
	ctx := context.WithoutCancel(c.Request.Context())
	// 构造 SSE 写手以设置 text/event-stream 响应头；此后不再能发 JSON 错误。事件经 WriteSSE 写出
	// （事件名取事件类型），写失败仅吞掉——保证客户端断连时摘要仍完整生成并落库。
	sse.NewSSEWriter(c.Writer)
	report := func(ev ai.CompressEvent) error {
		_ = sse.WriteSSE(c.Writer, sse.SSEEvent{Event: sse.SSEEventValue(ev.Type), Data: ev})
		return nil
	}
	comps.Agent.CompressSession(ctx, session, evicted, "manual", session.TargetModel, report)
}

// AgentPromptOptimizeIn 定义提示词优化的请求输入。
type AgentPromptOptimizeIn struct {
	// Query 待优化的用户诉求，必填
	Query string `json:"query" binding:"required"`
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
	if query == "" {
		response.CreateBadBodyError(errors.New("query is required")).AbortWithGinH(c)
		return
	}
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
	messages, err := comps.Sessions.ListMessages(c, uint(id), userID, 0, 0)
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
		}
	}
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace, userID); session == nil {
		return
	}
	var in AgentMessageTruncateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Sessions.TruncateMessagesFromUserMessage(c, uint(id), userID, in.UserMessageIndex); err != nil {
		if errors.Is(err, ai.ErrMessageNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
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
		if errors.Is(err, ai.ErrMessageNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(branched, middlewares.GetAiRuntime(c).Snapshot(), 0))
}

// AgentStream 处理 AI 助手流式生成的请求（SSE，路由 POST /agent/:sessionID/stream）。
// 先校验会话归属（404 语义同详情接口），再经驻留运行执行器 RunHub 启动一次运行：运行在独立
// goroutine 中以脱管上下文执行，浏览器断开（关抽屉 / 刷新 / 断网）不中止运行；本接口随即以
// after 为起点订阅该运行并把回放 + 实时事件透传为带序号的 SSE 事件（事件语义 delta/stage/
// tool_request/done/error 与旧实现一致）。本次生成的档位由 Agent 按「请求 slot → 会话档位 →
// agent.defaultSlot」解析，指定档位未绑定时以 error 事件返回，不静默回落。
func AgentStream(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "sessionID")
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Hub == nil {
		response.CreateAccidentError(errors.New("ai: the run hub is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID)
	if session == nil {
		return
	}
	var in AgentStreamIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// RunHub.Start 是唯一可能以 JSON 中断的一步（组件缺失 / 登记失败），须在构造 SSE 写手之前
	// 完成：NewSSEWriter 一旦构造即写出响应头，此后无法再返回 JSON 错误。
	if _, err := comps.Hub.Start(ai.NewDetachedContext(c.Request.Context()), ai.AgentInput{
		Query:         in.Query,
		NamespaceName: namespace,
		DocIDs:        in.DocIDs,
		SessionID:     uint(sessionID),
		UserID:        userID,
		BusinessData:  in.BusinessData,
		RegoRule:      in.RegoRule,
		Thinking:      in.Thinking,
		Slot:          modelsai.Slot(strings.TrimSpace(string(in.Slot))),
		DeclaredSlot:  session.TargetModel,
	}, uint(sessionID), userID, namespace, nil); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	streamLiveRun(c, comps.Hub, uint(sessionID), userID, in.After)
}

// streamLiveRun 订阅某会话的在途运行并把回放 + 实时事件透传为 SSE：订阅失败（不存在 / 已过期 /
// 归属不符）以 JSON 错误中断（此时尚未构造 SSE 写手）；成功后才构造写手并持续转发，直到运行
// 收敛（实时通道关闭）或客户端断开（写失败）。
func streamLiveRun(c *gin.Context, hub *ai.RunHub, sessionID uint, userID uint, after uint64) {
	replay, live, cancel, err := hub.Subscribe(c.Request.Context(), sessionID, userID, after)
	if err != nil {
		mapAgentLiveRunError(c, err)
		return
	}
	defer cancel()
	pumpLiveRun(sse.NewSSEWriter(c.Writer), replay, live)
}

// pumpLiveRun 先转发回放事件，再持续转发实时事件，直到实时通道关闭（运行收敛）或写失败
// （客户端断开）为止。
func pumpLiveRun(sseWriter *sse.SSEWriter, replay []ai.Published, live <-chan ai.Published) {
	for _, ev := range replay {
		if err := writePublished(sseWriter, ev); err != nil {
			return
		}
	}
	for ev := range live {
		if err := writePublished(sseWriter, ev); err != nil {
			return
		}
	}
}

// writePublished 把一条已发布事件写为带序号（id:）的 SSE 事件，序号供前端以 ?after= 续播时
// 跳过已收事件。
func writePublished(sseWriter *sse.SSEWriter, ev ai.Published) error {
	return sseWriter.SendEvent(sse.SSEIDEvent{
		ID: ev.Seq,
		SSEEvent: sse.SSEEvent{
			Event: sse.SSEEventValue(ev.Type),
			Data:  json.RawMessage(ev.Payload),
		},
	})
}

// mapAgentLiveRunError 把驻留运行的订阅 / 取消失败映射为业务错误码：不存在与归属不符同样返回
// 404（避免泄露运行存在性），登记过期返回 400；其余归为意外错误。
func mapAgentLiveRunError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ai.ErrAgentLiveRunNotFound):
		response.CreateCodeError(http.StatusNotFound, response.CodeAgentLiveRunNotFound, nil).AbortWithGinH(c)
	case errors.Is(err, ai.ErrAgentLiveRunForbidden):
		response.CreateCodeError(http.StatusNotFound, response.CodeAgentLiveRunForbidden, nil).AbortWithGinH(c)
	case errors.Is(err, ai.ErrAgentLiveRunExpired):
		response.CreateCodeError(http.StatusBadRequest, response.CodeAgentLiveRunExpired, nil).AbortWithGinH(c)
	default:
		response.CreateAccidentError(err).AbortWithGinH(c)
	}
}

// newAgentLiveRunOut 把驻留运行状态快照映射为视图层输出项：挂起态（suspended）标记待审批并
// 携带待执行调用列表，其余阶段待审批列表为空数组。
func newAgentLiveRunOut(st *ai.LiveRunStatus) *AgentLiveRunOut {
	out := &AgentLiveRunOut{
		RunID:            st.RunID,
		Phase:            st.Phase,
		LastSeq:          st.LastSeq,
		AwaitingApproval: st.Phase == modelsai.AgentRunPhaseSuspended,
	}
	out.PendingCalls = make([]AgentPendingCallOut, 0, len(st.PendingCalls))
	for _, call := range st.PendingCalls {
		out.PendingCalls = append(out.PendingCalls, AgentPendingCallOut{
			CallID:           call.CallID,
			ToolName:         call.ToolName,
			Arguments:        call.Arguments,
			AuditLevel:       call.AuditLevel,
			ApprovalRequired: call.ApprovalRequired,
			ExecOwner:        call.ExecOwner,
		})
	}
	return out
}

// AgentStreamResume 处理续播某会话在途运行的请求（SSE，路由 GET /agent/:sessionID/stream）。
// 校验会话归属后以查询参数 after 为起点订阅运行：先回放 seq > after 的已落库 / 已缓冲事件，
// 再挂接实时广播（运行已结束时回放后即以 EOF 结束）。运行不存在 / 已过期 / 归属不符以 JSON 中断。
func AgentStreamResume(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "sessionID")
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Hub == nil {
		response.CreateAccidentError(errors.New("ai: the run hub is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	if session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID); session == nil {
		return
	}
	var in AgentStreamResumeIn
	if raw := strings.TrimSpace(c.Query("after")); raw != "" {
		after, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			response.CreateBadQueryError("badUrlParam", "after", err).AbortWithGinH(c)
			return
		}
		in.After = after
	}
	streamLiveRun(c, comps.Hub, uint(sessionID), userID, in.After)
}

// AgentRunCancel 处理取消某会话在途运行的请求（路由 POST /agent/:sessionID/run/cancel）。
// 校验会话归属后经 RunHub.Cancel 立即取消运行（落 cancelled 终态，回放可见）；运行不存在 /
// 归属不符以 JSON 中断，已收敛终态时幂等返回 cancelled=true。
func AgentRunCancel(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "sessionID")
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Hub == nil {
		response.CreateAccidentError(errors.New("ai: the run hub is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	if session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID); session == nil {
		return
	}
	if err := comps.Hub.Cancel(c.Request.Context(), uint(sessionID), userID, namespace); err != nil {
		mapAgentLiveRunError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinW(c, AgentCancelOut{Cancelled: true})
}

// pumpAgentStream 消费 Agent 输出流并把事件透传为 SSE：带增量的输出项发 delta 事件，完成输出项
// 发 done 事件并结束，非 EOF 错误发 error 事件并结束；挂起（本次无 done）时流以 io.EOF 正常结束，
// 不发 error（此时前端已收到 tool_request，等待其回传结果后续跑）。供 AgentStream 与恢复端点复用，
// 保证两侧事件语义一致。
func pumpAgentStream(sseWriter *sse.SSEWriter, reader *schema.StreamReader[ai.AgentOutput]) {
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			_ = sseWriter.SendError(err)
			return
		}
		if item.Delta != "" {
			_ = sseWriter.SendDelta(item.Delta)
		}
		if item.Reasoning != "" {
			_ = sseWriter.SendReasoning(item.Reasoning)
		}
		if item.Done {
			_ = sseWriter.SendDone(item)
			return
		}
	}
}

// AgentToolResult 处理回传客户端工具调用结果并续跑挂起运行的请求（SSE，路由
// /agent/:sessionID/tool-result）。
// 挂起运行由 Agent 命中 client 来源工具时落库，本接口把前端在用户自身凭据下、经人工审批后得到的
// 执行结果回灌，从中断处续跑。流程：
//  1. 校验命名空间、会话 ID、AI 功能与会话归属（404 语义同其它接口，避免泄露资源存在性）；
//  2. 绑定请求体（须覆盖挂起时下发的全部 callID；同一轮多个调用一次性回传）；
//  3. 调用 ResumeRun 认领运行现场（校验归属/状态/过期并原子消费）并重建续跑流；
//  4. 仅当 ResumeRun 成功后才构造 SSE 写手：NewSSEWriter 一旦构造即写入响应头，此后无法再返回
//     JSON 错误，故失败时按 errors.Is 映射业务码以 JSON 中断（此时尚未构造 SSE 写手）；
//  5. 成功后以与流式生成一致的事件语义透传续跑输出（delta/stage/done），再次命中 client 工具时
//     再次下发 tool_request 并以 io.EOF 正常结束。
func AgentToolResult(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "sessionID")
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
	var in AgentToolResultIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	results := make([]ai.ToolResult, 0, len(in.Results))
	for _, item := range in.Results {
		results = append(results, ai.ToolResult{
			CallID:   item.CallID,
			Approved: item.Approved,
			Result:   item.Result,
			Error:    item.Error,
		})
	}
	// sseWriter 只能在 ResumeRun 成功后构造（构造即写响应头，之后无法再返回 JSON 错误），故先
	// 声明变量，成功后赋值；阶段事件/工具请求闭包内判空后调用，挂起下发与续跑输出均经此写手。
	var sseWriter *sse.SSEWriter
	report := func(ev ai.StageEvent) error {
		if sseWriter == nil {
			return nil
		}
		if ev.Delta != "" {
			return sseWriter.SendStageDelta(string(ev.Stage), ev.Delta)
		}
		return sseWriter.SendStage(ev)
	}
	reader, err := comps.Agent.ResumeRun(c, ai.ResumeInput{
		SessionID:     uint(sessionID),
		NamespaceName: namespace,
		UserID:        userID,
		Results:       results,
		ToolReporter:  report,
		ToolRequests: func(req ai.ToolRequest) error {
			if sseWriter == nil {
				return nil
			}
			return sseWriter.SendToolRequest(req)
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrAgentRunNotFound):
			response.CreateCodeError(http.StatusNotFound, response.CodeAgentRunNotFound, nil).AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunConsumed):
			response.CreateCodeError(http.StatusBadRequest, response.CodeAgentRunConsumed, nil).AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunExpired):
			response.CreateCodeError(http.StatusBadRequest, response.CodeAgentRunExpired, nil).AbortWithGinH(c)
		default:
			response.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}
	sseWriter = sse.NewSSEWriter(c.Writer)
	defer reader.Close()
	pumpAgentStream(sseWriter, reader)
}

// AgentStatusGet 处理获取 AI 助手状态的请求（不强制启用，始终 200）。
// 从运行时快照读取 AI 能力是否启用、各档位绑定的模型与可用性、生效的参数覆盖数量；
// 未启用时给出明确原因（按请求语言本地化）。启用时额外查询数据库 knowledge_chunks.embedding 与
// agent_memories.embedding 两列的实际维度并与绑定模型维度对比，不一致时提示风险；请求体携带
// 可选 SessionID 时再附上该会话当前在途运行的快照（供刷新后前端发现在跑 / 待审批的运行）。
func AgentStatusGet(c *gin.Context) {
	// 请求体可选：无会话 ID 时保持零值（不查询任何会话的在途运行），全程不因缺体而失败。
	var in AgentStatusIn
	_ = c.ShouldBindBodyWithJSON(&in)
	rt := middlewares.GetAiRuntime(c)
	snap := rt.Snapshot()
	out := AgentStatusOut{Enabled: rt.Enabled(), Reason: snap.DisabledReason(middlewares.GetLanguage(c))}
	out.DefaultSlot = snap.DefaultSlot()
	out.AssistantAgentKind = modelsai.AgentKindGeneral
	out.DefaultReviewValve = modelsai.DefaultReviewValve
	slots := snap.Slots()
	out.Slots = make([]AgentSlotStatusOut, 0, len(modelsai.AllSlots))
	for _, slot := range modelsai.AllSlots {
		item := AgentSlotStatusOut{Slot: slot}
		if info, ok := slots[slot]; ok {
			item.ModelName = info.Name
			item.ProviderModel = info.Model
			item.Kind = info.Kind
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
		// 该档位的上下文 token 容量（取自绑定模型的 context_tokens；embedding 等非对话档位为 0）
		item.ContextTokenLimit = snap.SlotContextLimit(slot)
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
	if out.Enabled {
		db := middlewares.GetCoreDatabase(c)
		// 维度自检覆盖两列受治理的向量列：knowledge_chunks.embedding 与 agent_memories.embedding；
		// 任一列可读且与绑定模型维度不一致即报出 dimensionMismatch（读不出时交由迁移建列）。
		if n, found, err := ai.ChunkEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: knowledge_chunks.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		} else if n, found, err := ai.MemoryEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: agent_memories.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		}
	}
	// 指定会话且 AI 已启用时附上该会话当前在途运行的快照（供刷新后前端发现「在跑 / 待审批」）；
	// 无在途运行（ErrAgentLiveRunNotFound）视为无 LiveRun，查询异常同样降级为无 LiveRun，不影响状态响应。
	// 在途运行按 (namespace, user_id) 作用域隔离：未认证（GetUser 失败）时跳过，不泄露他人运行。
	if out.Enabled && in.SessionID != 0 {
		if hub := rt.RunHub(); hub != nil {
			if user, codeErr := middlewares.GetUser(c); codeErr == nil {
				if st, err := hub.LiveRun(c.Request.Context(), in.SessionID, user.ID); err == nil {
					out.LiveRun = newAgentLiveRunOut(st)
				}
			}
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
