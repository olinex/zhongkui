// Package ai 提供 AI 知识域（Agent / 知识库 / 会话）的 HTTP 处理器与视图 In/Out 结构体，
// 向内封装 internal/ai 的组件能力。
package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	// 第三方
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
	// （无可用测量为 0，0 表示「尚无测量」而非「占用为 0」，见 ai.SessionStore.ContextTokens）
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

// AgentMessageItemOut 定义 AI 助手会话消息的输出项结构。
type AgentMessageItemOut struct {
	// ID 消息 ID
	ID uint `json:"id"`
	// Role 消息角色（user/assistant/system）
	Role string `json:"role"`
	// Content 消息正文内容
	Content string `json:"content"`
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
	Phase string `json:"phase,omitempty"`
	// Compress 该消息所锚定的历史压缩记录（压缩游标 upto_seq 等于本条消息 seq 时给出），
	// 无则为 nil；供前端刷新 / 重进会话后回显过往的压缩推进（含静默滚动 rolling）
	Compress *AgentMessageCompressOut `json:"compress,omitempty"`
	// Blocks 该助手消息的有序内容块（按 block_seq 升序）：思考段 / 工具调用组 / 授权待审 / 主动提问 /
	// 正文段各按其真实产生时序排列，是消息体的唯一展示来源，与流式 block 事件同源；无块时省略
	Blocks []AgentMessageBlockOut `json:"blocks,omitempty"`
	// PendingCalls 该助手消息若承载「仍待用户输入」的挂起现场（问卷待作答 / 工具待授权），此处给出该
	// 现场的待执行调用清单：回退重开（重新回答 / 重新授权）只重置挂起现场、不产生在途运行，故刷新 /
	// 重进会话时无从经在途快照（AgentLiveRunOut）重建可交互面板，改由此处随消息下发（提交 / 取消续跑
	// 需原样回传其中的 callID）；其余消息不带此字段
	PendingCalls []AgentPendingCallOut `json:"pendingCalls,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// AgentMessageBlockOut 定义助手消息所承载的**一块**内容块的展示项：块在消息内的序号、块类型与该块的
// 明文 JSON 载荷。载荷不经视图层再解码（原样透传块表明文 JSON），与前端的块渲染、流式 block 事件的
// payload 同源，故前端刷新 / 重进会话与实时会话由同一数据源驱动。
type AgentMessageBlockOut struct {
	// Seq 块在所属消息内的单调序号（从 1 起），即内容的真实产生顺序
	Seq uint64 `json:"seq"`
	// Kind 块类型：reasoning（思考段）/ tool_calls（工具调用组）/ approval（授权待审）/
	// questionnaire（主动提问）/ content（正文段）/ next_steps（推荐的下一步）
	Kind string `json:"kind"`
	// Payload 该块的明文 JSON 载荷（对象）：按块类型为 reasoning/content 的文本、tool_calls 的调用记录
	// 列表、approval 的待审批清单、questionnaire 的题目与作答或 next_steps 的推荐项列表，形状与流式
	// block 事件的 payload 一致
	Payload json.RawMessage `json:"payload"`
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
	// 不可用时为 0）：供刷新 / 重进会话后仍回显压缩前后对比；为 0 时前端回落为仅展示压缩前占用与
	// 折叠条数
	TokensAfter int `json:"tokensAfter"`
}

// AgentQuestionnaireOut 定义助手消息所承载的**一轮**问卷展示项：题目、用户作答与是否仍待回答。供前端
// 把该轮渲染成一个独立问答框，并提供「重新回答」入口（仅末轮）与在刷新 / 重进后据 phase=suspended 重建
// 问卷面板。
type AgentQuestionnaireOut struct {
	// Questions 本轮问卷题目列表（每题含候选项与可选说明）
	Questions []ai.QuestionnaireQuestion `json:"questions"`
	// Answers 本轮用户作答列表（未作答或取消回答时为空）
	Answers []ai.QuestionnaireAnswer `json:"answers,omitempty"`
	// Awaiting 本轮是否仍待用户回答：仅可能落在末轮——消息 phase 为 suspended，且末轮仍存在未作答的题目；
	// 由 AgentAssistantInputRerun 对「回退重开」回合另行显式置位
	Awaiting bool `json:"awaiting"`
	// PendingCalls 承载该问卷的运行现场中的待执行调用：在「重新回答」接口出参中给出（供重答后经既有
	// tool-result 原样回传续跑）；现场不可用时（已消费 / 已过期）不携带，前端据此把面板降级为不可用
	PendingCalls []AgentPendingCallOut `json:"pendingCalls,omitempty"`
}

// AgentAssistantInputRerunIn 定义「回退到助手待输入回合」的请求输入。
type AgentAssistantInputRerunIn struct {
	// MessageID 助手待输入回合的锚点消息 ID（会话消息列表出参的 id），必填
	MessageID uint `json:"messageID" binding:"required"`
}

// AgentAssistantInputRerunOut 定义「回退到助手待输入回合」的出参：按回合种类给出对应载荷，并附上
// 回退后的会话交互主状态（与 StateOf 同口径，供前端立即刷新状态驱动 UI）。Kind 与回退后的会话
// 交互子状态同值（answer=问卷作答 / approval=工具授权）。
type AgentAssistantInputRerunOut struct {
	// Kind 回合种类：answer（问卷作答）/ approval（工具授权）
	Kind string `json:"kind"`
	// InteractionState 回退后的会话交互主状态（供前端立即刷新状态驱动 UI）
	InteractionState string `json:"interactionState"`
	// Questionnaires 问卷回合按轮次先后并列的各轮载荷（题目 + 上次答案 + 待执行调用），仅问卷回合携带
	Questionnaires []AgentQuestionnaireOut `json:"questionnaires,omitempty"`
	// PendingCalls 该回合的待执行调用清单，授权回合据此重建审批面板
	PendingCalls []AgentPendingCallOut `json:"pendingCalls,omitempty"`
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

// AgentStreamIn 定义 AI 助手流式生成的请求输入。
type AgentStreamIn struct {
	// Query 用户本次提问文本，必填
	Query string `json:"query" binding:"required"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData map[string]any `json:"businessData"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码，为空跳过规则校验
	RegoRule string `json:"regoRule"`
	// Thinking 思考模式，取 on/off，空表示不注入思考开关
	Thinking string `json:"thinking" binding:"omitempty,oneof=on off"`
	// Slot 本次生成的显式档位（flash/standard/advanced），可选；为空时依次回退到会话档位与
	// agent.defaultSlot 参数
	Slot string `json:"slot"`
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
	AuditLevel string `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：false 表示无需人工审批
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属（client / backend）：client 由前端在用户自身凭据下执行，backend 由后端
	// 在恢复续跑时自行执行（已批准的 builtin / MCP 工具）
	ExecOwner string `json:"execOwner"`
}

// AgentLiveRunOut 定义一次驻留运行的当前状态快照（供刷新后发现在途运行与待审批现场）。
type AgentLiveRunOut struct {
	// RunID 运行标识
	RunID serializer.HexUUID `json:"runID"`
	// Phase 运行生命周期阶段（running/done/failed/cancelled/suspended）
	Phase string `json:"phase"`
	// LastSeq 已发布的最大事件序号（续播起点）
	LastSeq uint64 `json:"lastSeq"`
	// AwaitingApproval 是否挂起待审批（phase 为 suspended 时为 true）
	AwaitingApproval bool `json:"awaitingApproval"`
	// PendingCalls 挂起待审批时的待执行调用列表，非挂起时为空数组
	PendingCalls []AgentPendingCallOut `json:"pendingCalls"`
	// InteractionState 该会话当前交互主状态（与 AgentSessionItemOut 同口径）
	InteractionState string `json:"interactionState"`
	// InteractionSubstate 该会话当前交互子状态
	InteractionSubstate string `json:"interactionSubstate"`
	// Locked 该会话是否已锁定
	Locked bool `json:"locked"`
	// UserTurns 该会话有效轮次
	UserTurns int `json:"userTurns"`
	// MaxUserTurns 该会话轮次上限（参数 session.maxUserTurns 的生效值）
	MaxUserTurns int `json:"maxUserTurns"`
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
	Slot string `json:"slot"`
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
	// LiveRun 指定会话当前在途运行的状态快照（请求未指定会话或该会话无在途运行时为 nil），
	// 供刷新后前端发现在跑 / 待审批的运行
	LiveRun *AgentLiveRunOut `json:"liveRun,omitempty"`
}

// newAgentSessionItemOut 将会话模型映射为视图层输出项：上下文占用（contextTokens）由调用方按
// 场景提供（新建/分支为 0，列表批量取、详情等单条取）；容量（contextLimit）取自运行时快照中
// 会话有效档位绑定模型的上下文容量，有效档位为会话记录的 TargetModel，为空时回落 agent.defaultSlot；
// 交互状态五项（state/substate/locked/userTurns/maxUserTurns）由调用方按 interactionSnapshot 取快照填入，
// 与守卫判定同口径。
func newAgentSessionItemOut(s *modelsai.AgentSession, snap *ai.Snapshot, contextTokens int, interaction ai.InteractionSnapshot) AgentSessionItemOut {
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

// newAgentMessageItemOut 将消息模型与其有序内容块映射为视图层输出项：
// 草稿与引用载荷经 JSON 反序列化暴露，空或非法时留空（omitempty 省略）；块由调用方批量读取后传入
// （见 AgentMessageListGet），按 block_seq 升序原样透传其块表明文载荷，避免逐消息查块的 N+1。
func newAgentMessageItemOut(m *modelsai.AgentMessage, blocks []*modelsai.AgentMessageBlock) AgentMessageItemOut {
	out := AgentMessageItemOut{
		ID:               m.ID,
		Role:             string(m.Role),
		Content:          string(m.Content.Value()),
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		ElapsedMs:        m.ElapsedMs,
		Phase:            string(m.Phase),
		Blocks:           newAgentMessageBlockOuts(blocks),
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

// newAgentMessageBlockOuts 把某条消息的有序内容块映射为视图层输出项（按 block_seq 升序，输入已有序）：
// 载荷原样透传块表明文 JSON 字节，不做二次解码——与流式 block 事件的 payload 及前端块渲染同源。
// 无块时返回 nil（omitempty 省略）。
func newAgentMessageBlockOuts(blocks []*modelsai.AgentMessageBlock) []AgentMessageBlockOut {
	if len(blocks) == 0 {
		return nil
	}
	outs := make([]AgentMessageBlockOut, 0, len(blocks))
	for _, b := range blocks {
		if b == nil {
			continue
		}
		out := AgentMessageBlockOut{Seq: b.BlockSeq, Kind: string(b.Kind)}
		if payload := b.Payload.Value(); len(payload) > 0 {
			out.Payload = json.RawMessage(payload)
		}
		outs = append(outs, out)
	}
	if len(outs) == 0 {
		return nil
	}
	return outs
}

// newAgentQuestionnaireOuts 把锚点行落库的问卷载荷（按轮次先后并列）映射为视图层输出项：每轮各自成一份，
// 供前端渲染成各自独立的问答框并把最新一轮沉在底部。「仍待回答（awaiting）」只可能落在**末轮**——同一回合
// 内模型总是拿到上一轮答案后才提出下一轮，故仅当 phase 为挂起、且末轮仍存在未作答题目时置为 true（非问卷
// 挂起时末轮已作答，恒为 false、不误判）；更早各轮恒为 false，前端据此把它们渲染为只读的已答卡片。
// 本函数只据「phase 挂起 + 末轮未作答」给出保守判定；而「回退重开的回合」由调用方另行把末轮 awaiting 显式置位
// （见 AgentAssistantInputRerun）。
func newAgentQuestionnaireOuts(rounds []ai.QuestionnairePayload, phase modelsai.AgentRunPhase) []AgentQuestionnaireOut {
	if len(rounds) == 0 {
		return nil
	}
	outs := make([]AgentQuestionnaireOut, 0, len(rounds))
	for i, r := range rounds {
		outs = append(outs, AgentQuestionnaireOut{
			Questions: r.Questions,
			Answers:   r.Answers,
			Awaiting:  phase == modelsai.AgentRunPhaseSuspended && i == len(rounds)-1 && !r.FullyAnswered(),
		})
	}
	return outs
}

// awaitingQuestionnaireIndex 返回各轮问卷中「仍待回答」那一轮的序号：awaiting 只可能落在末轮（见
// newAgentQuestionnaireOuts），故命中即返回末轮序号；无轮次或无末轮 awaiting 时返回 -1，调用方据此
// 跳过「下发待执行调用」的分支。
func awaitingQuestionnaireIndex(rounds []AgentQuestionnaireOut) int {
	if len(rounds) == 0 {
		return -1
	}
	if last := rounds[len(rounds)-1]; last.Awaiting {
		return len(rounds) - 1
	}
	return -1
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
	// 上下文占用取「末次真实测量与更晚的压缩后估算中更晚者」（见 ai.SessionStore.ContextUsage），
	// 使压缩后尚无生成落定的会话不再显示压缩前的高占用。该度量是列表展示的一部分，故取失败与
	// 会话列表查询同等对待——以意外错误中断，绝不静默降级为 0（静默归零会被读成「占用已清空」）。
	contextTokens, err := comps.Sessions.ContextUsageBySession(c, userID, sessionIDs)
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
	contextTokens, err := comps.Sessions.ContextUsage(c, session.ID, userID)
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
	contextTokens, err := comps.Sessions.ContextUsage(c, updated.ID, userID)
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
	if err := comps.Sessions.DeleteSession(c, uint(id), namespace, userID); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionCompact 处理压缩 AI 助手会话上下文并抽取长期记忆的请求（SSE，路由
// POST /session/:id/compact）。
// 口径（Task 8）：入参以选中的助手回答为折叠边界——折叠「当前滚动摘要游标 summary_upto_seq（下界）
// 之后、到该助手回答 seq（上界）为止」的尚未摘要消息并把游标推进到该答案，更晚的消息保持原文；
// 边界为 0 时折叠全部尚未摘要消息（全量语义）。压缩本身是一次在途活动（RunHub.StartActivity）：
// 期间会话为 运行中/上下文压缩中、可中止，完成后经活动终局自然回落 暂停；失败仅告警且不产生半截检查点。
// 处理顺序严格：校验命名空间 / AI 功能 / 属主 / 会话归属（404 语义同详情接口）→ 交互状态守卫（仅
// 暂停 / 已锁定 可发起，运行中 / 等待中 → 409+1125）→ 绑定请求体 → 解析边界 seq（消息不存在 /
// 非助手消息 → 本地化 404）→ 读未摘要消息并选出折叠集合 → 启动在途活动 → 订阅透传 SSE。所有可能
// 返回 JSON 错误的步骤（尤其边界解析与读库）都必须在启动活动与构造 SSE 写手之前完成，被拒时尚未
// 产生在途活动。
func AgentSessionCompact(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
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
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	session := getOwnedAgentSession(c, comps, uint(id), namespace, userID)
	if session == nil {
		return
	}
	// 压缩守卫：仅 `暂停` / `已锁定` 允许，`运行中` / `等待中` 以 409 + 1125 拒绝（口径与回滚一致）。
	// 须在绑定请求体与读库之前生效，绝不产生半截压缩。
	if rejectGuardError(c, ai.CanCompact(agentInteractionFacts(c, comps, session))) {
		return
	}
	var in AgentSessionCompactIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 折叠上界：AssistantMessageID > 0 时按助手消息 ID 定位其 seq；为 0 时保持全量语义（uptoSeq=0）。
	// 边界解析是可能返回 JSON 错误的步骤，必须在启动在途活动与构造 SSE 写手之前完成。
	var uptoSeq uint64
	if in.AssistantMessageID > 0 {
		seq, err := comps.Sessions.AssistantSeqAtMessage(c, session.ID, userID, in.AssistantMessageID)
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
	// 下界 = 当前 summary_upto_seq（只取尚未摘要消息），上界 = 选中的助手回答 seq。读库失败按意外错误
	// 中断（同样须在启动活动之前）。
	messages, err := comps.Sessions.ListMessagesAfter(c, session.ID, userID, session.SummaryUptoSeq)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 折叠集合：下界为会话当前游标（messages 已是该游标之后的尚未摘要消息），上界为选中的助手回答 seq。
	evicted := compactFoldSet(messages, uptoSeq)
	// 启动通用在途活动（与生成共用登记互斥、运行根 context、取消、终局化与落库）：此处的返回错误
	// （组件缺失 / 登记互斥失败）是最后一处可能返回 JSON 的步骤，故必须在 streamLiveRun 构造 SSE 写手之前。
	if _, err := comps.Hub.StartActivity(ai.NewDetachedContext(c.Request.Context()), ai.ActivityInput{
		SessionID: session.ID, UserID: userID, Namespace: namespace,
		Run: func(runCtx context.Context, reportFn func(typ modelsai.AgentRunEventType, payload any) error) error {
			// 事件名由 report 的首参表达（CompressEvent.Type 带 json:"-"），报告载荷即事件本身。
			report := func(ev ai.CompressEvent) error {
				return reportFn(ev.Type, ev)
			}
			comps.Agent.CompressSession(runCtx, session, evicted, "manual", session.TargetModel, report)
			return nil
		},
	}, nil); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 订阅并透传该活动的 SSE：compress_* 事件与 state 事件（运行中/上下文压缩中 → 终局回落暂停）由此下发。
	streamLiveRun(c, comps.Hub, session.ID, userID, 0)
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
			TokensAfter:  cp.TokensAfter,
		}
	}
	// 有序内容块：一次性批量读取本会话全部消息的块（单条 IN 查询），随消息出参与流式 block 事件同源，
	// 且避免逐消息查块的 N+1；无块的消息不出现在结果中（其 blocks 留空、omitempty 省略）。
	messageIDs := make([]uint, 0, len(messages))
	for _, m := range messages {
		messageIDs = append(messageIDs, m.ID)
	}
	blocksByMessage, err := comps.Sessions.ListBlocksByMessages(c, messageIDs, namespace, userID)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(AgentMessageListOut, 0, len(messages))
	for _, m := range messages {
		item := newAgentMessageItemOut(m, blocksByMessage[m.ID])
		item.Compress = compressBySeq[m.Seq]
		out = append(out, item)
	}
	// 仍待用户输入的挂起现场（问卷待作答 / 工具待授权）不产生在途运行：回退重开（重新回答 / 重新授权）
	// 只重置挂起现场，故刷新 / 重进会话时无从经在途快照重建可交互面板。按其锚点消息把现场的待执行调用
	// 一并附加到该消息出参上（供前端重建提交 / 取消所需的 callID）；无可用现场时不附加。读取失败降级为
	// 不附加——消息列表本身仍完整返回，不因该辅助信息影响列表可用性。
	if anchorID, calls, cerr := comps.Sessions.PendingInputCalls(c, uint(id), userID); cerr == nil && anchorID != 0 {
		for i := range out {
			if out[i].ID == anchorID {
				out[i].PendingCalls = newAgentPendingCallOuts(calls)
				break
			}
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentMessageTruncate 处理从某一轮用户消息起截断会话消息的请求（路由
// POST /session/:id/messages/truncate）。
// 供前端「回退 / 删除到某一轮对话」使用：按 UserMessageIndex（仅计用户消息）定位该轮提问，删除其
// 自身及之后的全部消息（含助手回复）。会话不存在 / 归属不符返回 404；目标用户消息不存在（序号越界）
// 同样返回 404，避免泄露会话内容细节。
// 状态守卫（口径来源见 ai.CanRollback）：回滚是一次「改动有效轮次」的既成事实操作，仅在 `暂停` / `已锁定`
// 允许发起——已锁定非死路，回滚把轮次降到上限以下即自动解锁；`运行中` / `等待中` 无稳定截断对象，一律
// 以 409 + 业务码 1125 拒绝（守卫在绑定请求体与调用 store 之前生效，绝不产生半截截断）。
// 截断同时作废该轮及之后的挂起现场（见 store）；内存中的在途运行再显式取消一次，作为「读取事实」与
// 「执行截断」之间竞态窗口的兜底（并发发送可能恰在守卫判定后启动一次运行）——否则客户端刷新 / 重开抽屉时
// 会探测到这个仍在「运行中」的 live-run 并续播，把已删除回合的回答重新拼回消息区。取消为尽力而为，
// 失败不影响截断结果。
// 截断成功后重算并向该会话在内存运行广播交互状态（须走 broadcastInteractionState 同一入口，不得就地拼
// 状态）：回滚改变有效轮次，可能由 `已锁定` 派生回落 `暂停`。
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
	// 会话在函数作用域捕获：后续守卫判定与状态广播都需要复用同一份会话事实。
	session := getOwnedAgentSession(c, comps, uint(id), namespace, userID)
	if session == nil {
		return
	}
	// 回滚守卫：仅 `暂停` / `已锁定` 允许，`运行中` / `等待中` 以 409 + 1125 拒绝（守卫在绑定请求体
	// 与调用 store 之前生效）。
	if rejectGuardError(c, ai.CanRollback(agentInteractionFacts(c, comps, session))) {
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
	// 竞态兜底：守卫已排除运行中 / 等待中，但「读取事实」与「执行截断」之间仍有一个窗口——并发发送可能
	// 恰在守卫判定后启动一次运行。此时截断已把其锚点消息删除，运行继续产出只会写回一条已不存在的回合，
	// 故再显式取消一次；运行不存在（服务已重启 / 已收敛）或归属不符都不影响截断结果。
	if comps.Hub != nil {
		_ = comps.Hub.Cancel(c.Request.Context(), uint(id), userID, namespace)
	}
	// 事实变更（消息与现场已截断）后重算并广播交互状态：回滚可能使 `已锁定` 回落 `暂停`，须走同一入口。
	broadcastInteractionState(c, comps, session)
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
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(branched, middlewares.GetAiRuntime(c).Snapshot(), 0, interactionSnapshot(c, comps, branched)))
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
	// 发送守卫：以交互状态推导为唯一口径拒绝本次提问（已锁定 → 1124；运行中 / 等待中 → 1125），
	// 须在构造 SSE 写手与 Hub.Start 之前完成——一旦启动运行即已产生在途活动，无法再回退为 JSON 错误。
	if rejectIfInteractionBusy(c, agentInteractionFacts(c, comps, session)) {
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
		BusinessData:  ai.BusinessData(in.BusinessData),
		RegoRule:      in.RegoRule,
		Thinking:      in.Thinking,
		Slot:          modelsai.Slot(strings.TrimSpace(in.Slot)),
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
// 携带待执行调用列表，其余阶段待审批列表为空数组；交互状态五项由调用方按 interactionSnapshot
// 取快照填入（与会话出参同口径）。
func newAgentLiveRunOut(st *ai.LiveRunStatus, interaction ai.InteractionSnapshot) *AgentLiveRunOut {
	out := &AgentLiveRunOut{
		RunID:               st.RunID,
		Phase:               string(st.Phase),
		LastSeq:             st.LastSeq,
		AwaitingApproval:    st.Phase == modelsai.AgentRunPhaseSuspended,
		InteractionState:    string(interaction.State),
		InteractionSubstate: string(interaction.Substate),
		Locked:              interaction.Locked,
		UserTurns:           interaction.UserTurns,
		MaxUserTurns:        interaction.MaxUserTurns,
	}
	out.PendingCalls = newAgentPendingCallOuts(st.PendingCalls)
	return out
}

// newAgentPendingCallOuts 把待执行调用映射为视图层输出项（供续播快照与问卷「重新回答」出参共用）。
func newAgentPendingCallOuts(calls []ai.PendingCall) []AgentPendingCallOut {
	out := make([]AgentPendingCallOut, 0, len(calls))
	for _, call := range calls {
		out = append(out, AgentPendingCallOut{
			CallID:           call.CallID,
			ToolName:         call.ToolName,
			Arguments:        call.Arguments,
			AuditLevel:       string(call.AuditLevel),
			ApprovalRequired: call.ApprovalRequired,
			ExecOwner:        string(call.ExecOwner),
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
	session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID)
	if session == nil {
		return
	}
	// 等待中（待授权 / 待回答）：此刻后端内存中已无生成 goroutine 可取消，中止的语义是「作废挂起现场」——
	// 把该现场置为已消费、锚点助手消息终局置 cancelled，使会话派生回落 `暂停/用户中止`。
	if ai.StateOf(agentInteractionFacts(c, comps, session)).State == modelsai.AgentInteractionStateWaiting {
		if err := comps.Sessions.AbortPendingRun(c.Request.Context(), session.ID, session.UserID); err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		// 事实变更（挂起现场作废）后重算并向该会话在内存运行广播状态：此刻无内存运行（等待中后端
		// 无生成 goroutine），广播为 no-op，用户可见状态随本响应 HTTP 出参下发。
		broadcastInteractionState(c, comps, session)
		response.SimpleOK.AbortWithGinW(c, AgentCancelOut{Cancelled: true})
		return
	}
	// 运行中：经 RunHub.Cancel 取消根 context 并等待生成 goroutine 真正退出；暂停 / 已锁定无在途活动，
	// Hub.Cancel 幂等返回（重复取消不报错）。
	if err := comps.Hub.Cancel(c.Request.Context(), uint(sessionID), userID, namespace); err != nil {
		mapAgentLiveRunError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinW(c, AgentCancelOut{Cancelled: true})
}

// AgentToolResult 处理回传客户端工具调用结果并续跑挂起运行的请求（SSE，路由
// /agent/:sessionID/tool-result）。
// 挂起运行由 Agent 命中 client 来源工具时落库，本接口把前端在用户自身凭据下、经人工审批后得到的
// 执行结果回灌，从中断处续跑。流程：
//  1. 校验命名空间、会话 ID、AI 功能与会话归属（404 语义同其它接口，避免泄露资源存在性）；
//  2. 绑定请求体（须覆盖挂起时下发的全部 callID；同一轮多个调用一次性回传）；
//  3. 经驻留运行执行器 RunHub 启动续跑（StartResume）：它先同步认领运行现场（校验归属/状态/过期并
//     原子消费），再以与首轮生成一致的登记 / 广播 / 收敛路径执行——因此续跑同样拥有独立的在途登记并
//     在收敛时写入终态，不会残留挂起现场留下的 suspended 登记（否则前端刷新后会据它误判「仍在等待
//     审批」，并以陈旧的挂起事件回放覆盖已落库的正文与思考）；认领失败按 errors.Is 映射业务码以 JSON
//     中断（此时尚未构造 SSE 写手）；
//  4. 成功后订阅该运行并把回放 + 实时事件透传为带序号的 SSE 事件（事件语义 delta/stage/done 与首轮
//     生成一致），再次命中 client 工具时再次下发 tool_request 并以运行收敛结束。
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
	// StartResume 是唯一可能以 JSON 中断的一步（组件缺失 / 认领现场失败 / 登记失败），须在构造 SSE
	// 写手之前完成：NewSSEWriter 一旦构造即写出响应头，此后无法再返回 JSON 错误。
	if _, err := comps.Hub.StartResume(ai.NewDetachedContext(c.Request.Context()), ai.ResumeInput{
		SessionID:     uint(sessionID),
		NamespaceName: namespace,
		UserID:        userID,
		Results:       results,
	}, uint(sessionID), userID, namespace, nil); err != nil {
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
	streamLiveRun(c, comps.Hub, uint(sessionID), userID, 0)
}

// AgentAssistantInputRerun 处理「回退到助手待输入回合」的请求（路由
// POST /agent/:sessionID/input/rerun）。
// 泛化既有「问卷重新回答」语义，同时覆盖两类历史待输入回合（由该回合现场中的挂起调用判定）：
//   - 问卷回合（含 ask_user 调用）：删除锚点之后的全部消息、清空锚点本轮产出并重置为 suspended，把承载
//     现场重置为 pending（刷新存活期限），出参以 Questionnaire 返回题目 + 上次答案，供前端预填重开问卷；
//   - 工具授权回合（其余调用）：同样截断与重置，出参以 PendingCalls 返回待审批调用清单，供前端重建
//     审批面板完成重新授权。
//
// 两类回合均使改答 / 重新授权后经既有 tool-result 接口按新决策续跑、写回同一锚点行。出参附带回退后的
// 交互主状态（与 StateOf 同口径），供前端立即刷新状态驱动 UI。
//
// 处理顺序：命名空间 → 会话 ID → AI 启用 → 属主 → 会话归属（404 语义同详情接口）→ 状态守卫
// （CanRerunUserInput，运行中 / 等待中被拒、已锁定放行；在绑定请求体与任何写库之前，被拒时不得产生任何
// 改动）→ 绑定请求体 → 执行回退。锚点消息不存在 / 不是待输入回合、或承载现场不存在均返回 404（避免泄露
// 资源存在性）；现场已过期按既有「已过期」语义以 400 拒绝（前端按后端本地化提示）。
func AgentAssistantInputRerun(c *gin.Context) {
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
	session := getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID)
	if session == nil {
		return
	}
	// 状态守卫须在任何写库之前：运行中 / 等待中被拒（409 + 1125），已锁定放行（回滚类操作非死路）。
	if rejectGuardError(c, ai.CanRerunUserInput(agentInteractionFacts(c, comps, session))) {
		return
	}
	var in AgentAssistantInputRerunIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	turn, err := comps.Sessions.RerunAssistantInput(c, uint(sessionID), userID, in.MessageID)
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrMessageNotFound), errors.Is(err, ai.ErrAgentRunNotFound):
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunExpired):
			response.CreateCodeError(http.StatusBadRequest, response.CodeAgentRunExpired, nil).AbortWithGinH(c)
		default:
			response.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}
	// 事实（消息截断、锚点与现场重置）已变更，重算交互主状态写入出参，使前端立即按「等待中」驱动 UI。
	out := AgentAssistantInputRerunOut{
		Kind:             string(turn.Kind),
		InteractionState: string(interactionSnapshot(c, comps, session).State),
	}
	if turn.Kind == modelsai.AgentInteractionSubstateAnswer {
		// 锚点此刻已重置为 suspended（挂起待输入），故按挂起相位映射各轮：更早各轮（已作答）为只读卡片、
		// 末轮 awaiting=true；末轮即重答目标，把待执行调用一并挂在末轮上供前端经既有 tool-result 回传续跑。
		out.Questionnaires = newAgentQuestionnaireOuts(turn.Questionnaires, modelsai.AgentRunPhaseSuspended)
		// 回退即把该回合重置为「等待回答」：承载问卷的现场已复位为 pending，末轮此刻回到待作答。即便锚点行保留
		// 上次答案（供面板预填，FullyAnswered 因而为真），也必须显式把末轮补置 awaiting=true，否则出参按「已作答」
		// 判定、前端会渲染成已答卡片而非重开可交互问卷（见 newAgentQuestionnaireOuts 的现场置位约定）。
		if len(out.Questionnaires) > 0 {
			out.Questionnaires[len(out.Questionnaires)-1].Awaiting = true
		}
		if awaiting := awaitingQuestionnaireIndex(out.Questionnaires); awaiting >= 0 {
			out.Questionnaires[awaiting].PendingCalls = newAgentPendingCallOuts(turn.Calls)
		}
	} else {
		out.PendingCalls = newAgentPendingCallOuts(turn.Calls)
	}
	broadcastInteractionState(c, comps, session)
	response.SimpleOK.AbortWithGinW(c, out)
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
	out.MaxUserTurns = snap.Params().Int("session.maxUserTurns")
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
		hub := rt.RunHub()
		comps := rt.Current()
		if hub != nil && comps.Sessions != nil {
			if user, codeErr := middlewares.GetUser(c); codeErr == nil {
				namespace, _ := middlewares.GetNamespace(c)
				// 先解出会话再取在途运行：交互状态五项须由 interactionSnapshot 推导（其内部经
				// agentInteractionFacts 装配事实并与 SessionStore 同口径读库），而事实装配需要会话
				// 本体；取不到会话（不存在 / 归属不符 / 已删除）时不附在途快照——避免给出空状态。
				if session, serr := comps.Sessions.GetSession(c.Request.Context(), in.SessionID, namespace, user.ID); serr == nil {
					if st, err := hub.LiveRun(c.Request.Context(), in.SessionID, user.ID); err == nil {
						out.LiveRun = newAgentLiveRunOut(st, interactionSnapshot(c, comps, session))
					}
				}
			}
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
