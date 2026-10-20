package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/open-policy-agent/opa/rego"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// agentNode 常量：Eino Graph 四节点管线（上下文组装 → 业务数据清洗 → LLM 生成
// → 结构化校验）的节点名。
const (
	agentNodeContextAssembly = "context_assembly"
	agentNodeSanitize        = "sanitize_business_data"
	agentNodeLLMGeneration   = "llm_generation"
	agentNodeValidation      = "validation"
)

// agentRegoQuery 是结构化草稿 Rego 校验规则必须产出的查询名：规则文件声明
// package agent 且规则名为 validate（data.agent.validate），返回值须为布尔。
const agentRegoQuery = "data.agent.validate"

// agentSummarySectionTitle 是注入给 LLM 时「会话滚动摘要」段的标题常量（长期记忆分节的
// 标题取参数 memory.sectionTitle，分节位于摘要之后、窗口历史之前）。
const agentSummarySectionTitle = "## 会话摘要"

// ToolRequest 是一条待执行工具调用请求（对应 SSE tool_request 事件的载荷）：client 工具由前端
// 在用户自身凭据下执行，backend 工具由后端在恢复时自行执行。
type ToolRequest struct {
	// CallID 与模型 tool call id 一致，用于回传时关联
	CallID string `json:"callID"`
	// ToolName 待执行的工具名（模型给出的调用名）
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本，后端不做解释）
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级（由挂起前的批量评估产出）
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：true 表示需人工审核
	ApprovalRequired bool `json:"approvalRequired"`
	// ExecOwner 执行归属（client / backend）；为空时按 client 处理
	ExecOwner ToolExecOwner `json:"execOwner"`
	// MessageID 本次挂起所锚定的助手消息 ID（agent_messages.id）：挂起必定把本回合的助手消息行落为
	// 锚点（问卷挂起写问卷载荷、其余挂起仅置 phase=suspended），故正常挂起恒非零，前端据此在实时会话
	// 内即刻把挂起定位到具体助手消息行（「重新回答」须按它截断其后消息）。仅运行期共享状态缺失的旁路
	// 调用为 0（省略，此时无处可锚）
	MessageID uint `json:"messageID,omitempty"`
}

// ToolRequestReporter 上报一条待执行的工具调用请求；返回错误（写失败/客户端断开）时挂起中止。
type ToolRequestReporter func(ToolRequest) error

// reportToolRequests 按原序逐条上报挂起时的待执行调用；messageID 为该挂起所锚定的助手消息行 ID
// （运行期共享状态缺失的旁路调用传 0，载荷据此省略该字段）。任一条写失败即中止后续上报并返回错误（写失败/客户端断开）。
func reportToolRequests(reporter ToolRequestReporter, requests []pendingToolCall, messageID uint) error {
	if reporter == nil {
		return nil
	}
	for i := range requests {
		call := requests[i]
		if err := reporter(ToolRequest{
			CallID:           call.CallID,
			ToolName:         call.ToolName,
			Arguments:        call.Arguments,
			AuditLevel:       call.AuditLevel,
			ApprovalRequired: call.ApprovalRequired,
			ExecOwner:        call.ExecOwner,
			MessageID:        messageID,
		}); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// ToolResult 是前端回传的一次客户端工具调用执行结果（对应恢复请求的载荷），按 CallID 与挂起时
// 下发的 tool_request 关联。
type ToolResult struct {
	// CallID 与 tool_request 下发的 callID 一致
	CallID string `json:"callID"`
	// Approved 用户是否批准执行该调用
	Approved bool `json:"approved"`
	// Result 执行结果文本（批准且执行成功时有效）
	Result string `json:"result,omitempty"`
	// Error 执行错误文本（前端执行失败时有效）
	Error string `json:"error,omitempty"`
}

// CompressEvent 是一次上下文压缩过程的事件载荷：Type 决定发布的运行事件类型
// （compress_start / compress_delta / compress_done），其余字段按事件类型择一有值。
type CompressEvent struct {
	// Type 事件类型（compress_start/compress_delta/compress_done），不进入 JSON 载荷
	Type ai.AgentRunEventType `json:"-"`
	// Reason 压缩原因（auto / manual），start/done 事件携带
	Reason string `json:"reason,omitempty"`
	// Delta 摘要生成的增量文本（仅 compress_delta）
	Delta string `json:"delta,omitempty"`
	// Tokens 压缩前会话占用 token（仅 compress_start）
	Tokens int `json:"tokens,omitempty"`
	// Limit 本次有效档位的上下文容量（仅 compress_start）
	Limit int `json:"limit,omitempty"`
	// Threshold 触发压缩的占用比例阈值（仅 compress_start）
	Threshold float64 `json:"threshold,omitempty"`
	// TokensBefore 压缩前占用 token（仅 compress_done）
	TokensBefore int `json:"tokensBefore,omitempty"`
	// TokensAfter 压缩后占用 token 的即时估算（仅 compress_done）：压缩前占用 − 被折叠内容占用 +
	// 新摘要占用，优先取摘要模型用量算出的估值，用量缺失或其估值不可用时回落为按字节折算的估值
	// （见 estimatedCompactedTokens），亦即压缩完成即刻总能给出一个可展示的值——两种口径都不可用时
	// 才缺省（前端回落为仅展示压缩前占用与折叠条数）。该值仅作即时展示并与检查点同时落库，待在途
	// 或后续生成由供应商测得的真实 prompt tokens 到达后覆盖为精确值。
	TokensAfter int `json:"tokensAfter,omitempty"`
	// FoldedCount 本次折叠的消息条数（仅 compress_done）
	FoldedCount int `json:"foldedCount,omitempty"`
	// SummaryUptoSeq 压缩后滚动摘要游标（仅 compress_done）
	SummaryUptoSeq uint `json:"summaryUptoSeq,omitempty"`
}

// CompressReporter 上报一次上下文压缩事件；返回错误（写失败/客户端断开）时压缩过程应中止。
type CompressReporter func(CompressEvent) error

// AgentInput 是一次 Agent 提问的图输入。
type AgentInput struct {
	// Query 用户本次提问文本
	Query string `json:"query"`
	// NamespaceName 检索限定的命名空间（多租户隔离）
	NamespaceName string `json:"namespaceName"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs,omitempty"`
	// SessionID 会话 ID；为 0 时不读写会话（无持久化）
	SessionID uint `json:"sessionID,omitempty"`
	// UserID 本次运行的属主用户 ID（users.id），是 AI 数据跨用户隔离的维度；会话路径由
	// StreamRun / 续播从会话/上层注入。
	UserID uint `json:"-"`
	// AgentKind 本次运行所属的 Agent 类型，是长期记忆按「命名空间 + Agent 类型」隔离的维度；
	// 为空时按 knowledge 处理。会话路径由 StreamRun 从会话读取后注入。
	AgentKind ai.AgentKind `json:"agentKind,omitempty"`
	// HistoryMessages 历史消息（一般由 StreamRun 从会话加载，也可由调用方直接注入）
	HistoryMessages []*schema.Message `json:"-"`
	// Summary 本次生成使用的会话滚动摘要（由 StreamRun 从会话读取/刷新后注入），
	// 非空时作为独立摘要段插在系统提示之后、窗口历史之前
	Summary string `json:"-"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData BusinessData `json:"businessData,omitempty"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码；为空跳过规则校验
	RegoRule string `json:"regoRule,omitempty"`
	// Thinking 本次运行的思考模式，取 on/off，空表示不注入思考开关
	Thinking string `json:"thinking,omitempty"`
	// Slot 显式指定的 LLM 档位；为空时依次回退到 DeclaredSlot 与
	// agent.defaultSlot 参数（见 resolveInputSlot）
	Slot ai.Slot `json:"slot,omitempty"`
	// DeclaredSlot 调用点声明的默认档位（不对外暴露），优先级低于显式 Slot
	DeclaredSlot ai.Slot `json:"-"`
	// ToolReporter 工具调用阶段事件的上报回调（不对外暴露），由视图层注入 SSE 写入器；
	// 为 nil 时不上报。与 HistoryMessages / DeclaredSlot 同为 json:"-"。
	ToolReporter EvalReporter `json:"-"`
	// ToolRequestReporter 挂起时上报待执行工具调用请求的通道（不对外暴露），由视图层注入
	// SSE 写入器；为 nil（或本次运行无会话）时客户端工具不挂起，按工具失败回灌，后端工具一律行内
	// 执行。client 工具在具备该通道时一律挂起，backend 工具仅在审核重要级为 must 时挂起。与
	// ToolReporter 同为 json:"-"。
	ToolRequestReporter ToolRequestReporter `json:"-"`
	// ResumeMessages 恢复运行要续跑的消息链（不对外暴露）；非 nil 表示本次为恢复运行，节点 1 跳过
	// 提问非空校验、节点 2 短路直接透传该消息链（不重跑业务数据清洗与记忆召回）。
	ResumeMessages []*schema.Message `json:"-"`
	// ResumeCitations 恢复运行要产出的检索引用（不对外暴露），由挂起时刻持久化的引用预置进采集器。
	ResumeCitations []RetrievalResult `json:"-"`
	// Questionnaire 问卷挂起与续跑之间共享的运行期状态（不对外暴露）：由 StreamRun / ResumeRun 在
	// 进入图前分配，随 AgentInput 以指针形式在节点 3 与 wrapPersist 之间共享（结构体按值拷贝只复制
	// 指针，两侧指向同一对象）。节点 3 在问卷挂起时写入载荷与现场 ID；wrapPersist 据此决定「保留
	// 锚点行」还是按既有逻辑回收阶段性落库行，并在续跑时把终态回答写回锚点行。非问卷运行的指针仍然
	// 存在但载荷为 nil，行为与引入问卷前完全一致。
	Questionnaire *questionnaireState `json:"-"`
	// blocks 本次运行的块构建器（不对外暴露）：由 StreamRun / PrepareResume 在进入图前创建（续跑时以锚点
	// 消息 ID 与已有块序号重整），随 AgentInput 以指针形式在节点（工具调用 / 思考与正文增量）与 wrapPersist
	// 之间共享——结构体按值拷贝只复制指针，各处指向同一构建器，使块的时序唯一来源在本包内单点成立。
	// 与 Questionnaire / ToolReporter 同为 json:"-"。无会话运行同样具备该构建器（仅不落库，块事件仍随流下发）。
	blocks *blockBuilder `json:"-"`
	// ReviewValve 会话级审核阀门（不对外暴露），由 StreamRun / ResumeRun 从会话读取注入，不对外暴露；
	// 与每条调用的审核重要级合成得出最终是否需人工审核。
	ReviewValve ai.ReviewValve `json:"-"`
	// CompressReporter 压缩过程事件上报通道（不对外暴露），由视图层/RunHub 注入；为 nil 时不上报。
	// 与 ToolReporter / ToolRequestReporter 同为 json:"-"。
	CompressReporter CompressReporter `json:"-"`
}

// ChatResolver 按档位解析对话模型（使用者侧行为契约）：由上层装配方（Runtime）
// 实现，Agent 内部按本次运行解析出的档位调用它取回对话客户端，从而在
// 同一张图内支持按请求切换模型而不重建图。
type ChatResolver = func(slot ai.Slot) (model.BaseChatModel, error)

// requestOptionsProvider 是对话模型可选实现的「按思考模式构造请求选项」能力：thinking 取
// "on"/"off"/空，返回本次请求要注入的 eino 选项（思考字段与高级字段的合并结果）。*llm.ChatClient
// 实现了它；未实现该接口（或返回空切片）时 generate 不注入任何思考选项，保持既有行为。
type requestOptionsProvider interface {
	RequestOptions(thinking string) []model.Option
}

// questionnaireState 是本轮挂起与续跑之间共享的运行期状态：由 StreamRun / ResumeRun 在进入图前分配，
// 经 AgentInput.Questionnaire 以指针形式在节点 3 与 wrapPersist 之间共享（结构体值拷贝只复制指针，两侧
// 仍指向同一对象），使节点 3 挂起时写入的信息能被流包装层读到。名称为历史沿革（最初为问卷引入），
// 现同时承载人工审批等非问卷挂起的锚点信息。
type questionnaireState struct {
	// rounds 本回合已累计的各轮问卷载荷（每轮各自一份题目；续跑终态化时把答案并入对应轮次）。nil /
	// 空表示本回合尚无问卷挂起——非问卷挂起（人工审批等）恒不追加，锚点行不写问卷载荷
	rounds []QuestionnairePayload
	// runID 承载本次挂起的运行现场 ID（挂起时由节点 3 写入）：写好锚点消息行后据它把锚点 ID 回写到现场
	runID uint
	// anchorID 承载本回合助手消息的锚点行 ID（挂起时由 wrapPersist 写入并回写现场）：续跑时作为终态
	// 回答的写回目标（原地更新该行而非新增一行助手消息）
	anchorID uint
	// pending 本次挂起时暂存的待执行调用（挂起时由节点 3 写入）：节点 3 挂起时不即时下发，交由
	// wrapPersist 在锚点行落库、拿到 messageID 后再逐条下发，使 tool_request 载荷能带上锚点 ID。
	// 问卷挂起与人工审批挂起都经此处；仅运行期共享状态缺失的旁路调用由节点 3 即时下发（messageID 恒 0）
	pending []pendingToolCall
	// loadedRounds 本次续跑从锚点已有 questionnaire 块载入的轮次数（首次运行恒 0）。wrapPersist 据它把
	// rounds[loadedRounds:] 视为本次运行新产生的轮次（经块构建器新建 questionnaire 块），而此前各轮的
	// 块已在锚点消息里（只回填其作答，不重复建块）。
	loadedRounds int
}

// AgentState 是图运行期间的共享状态，经 compose.ProcessState 在节点间传递
// 原始输入与知识检索采集器（局部状态按次运行隔离，并发安全由框架保证）。
type AgentState struct {
	input        AgentInput          // 节点 1 写入的原始输入
	knowledge    *knowledgeCollector // 节点 1 创建的知识检索采集器（供工具循环写入、节点 4 读取）
	slot         ai.Slot             // 节点 1 写入的本次运行档位
	toolReporter EvalReporter        // 节点 1 写入的本次运行工具阶段上报回调
	toolRequests ToolRequestReporter // 节点 1 写入的本次运行待执行工具请求上报通道（节点 3 挂起时使用）
	resume       []*schema.Message   // 节点 1 写入的恢复消息链，非 nil 表示本次为恢复运行（节点 2 据此短路）
	suspended    bool                // 节点 3 在挂起时置为 true，节点 4 据此不发任何输出（不产出 done、不报错）
}

// AgentOutput 是图输出（流式项）：流中先出现若干 delta 增量项，最后出现一个
// done 完成项（含结构化产物与校验结果）；聚合后即为单次问答的完整产物。
type AgentOutput struct {
	// MessageID 本回合助手消息的落库行 ID（仅 done 项，且仅当本次运行持久化了会话时给出；无会话的
	// 旁路运行为 0）：落库后才可知，故由持久化收尾处回填。供前端实时会话把本地气泡绑定到该行——
	// 「压缩到此」等按消息 ID 定位的操作由此无需先回读历史即可发起（与 tool_request 载荷携带挂起
	// 锚点 ID 同源同义，见 reportToolRequests）。
	MessageID uint `json:"messageID,omitempty"`
	// Delta 流式增量文本（仅中间 delta 项）
	Delta string `json:"delta,omitempty"`
	// Reasoning 思考过程增量/全文（中间项为增量片段，done 项为思考过程全文）
	Reasoning string `json:"reasoning,omitempty"`
	// Content 完整回复正文（仅 done 项）
	Content string `json:"content,omitempty"`
	// Done 是否为最终完成项
	Done bool `json:"done"`
	// Citations 检索引用来源列表（仅 done 项）
	Citations []RetrievalResult `json:"citations,omitempty"`
	// Draft 结构化草稿载荷（JSON 对象）；模型未产出结构化为 null
	Draft json.RawMessage `json:"draft,omitempty"`
	// Valid 结构化产物是否通过校验（无校验规则时为 true）
	Valid bool `json:"valid"`
	// ValidationDetail 校验未通过时的说明
	ValidationDetail string `json:"validationDetail,omitempty"`
	// PromptTokens 本轮全部模型调用的输入 token 累加（仅 done 项；供应商未返回用量时为 0）
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 本轮全部模型调用的输出 token 累加（仅 done 项）
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 本轮全部模型调用的 token 总量累加（仅 done 项）
	TotalTokens int `json:"totalTokens"`
	// ElapsedMs 本轮总耗时（毫秒），从 StreamRun 开始到 done 项产出（仅 done 项）
	ElapsedMs int64 `json:"elapsedMs"`
	// Blocks 本次流项承载的「新增块事件」（仅内传，不对外序列化）：由本包在内容真实产生时点从块构建器
	// 取出并挂到对应的流项上，供 run_hub 以 block 事件下发（前端据 {seq, kind} 归并增量到当前开放块）。
	// 与 delta / reasoning 增量、done 项同源同序。
	Blocks []AgentBlockEvent `json:"-"`
}

// init 注册 AgentOutput 的流块聚合函数：图 Invoke 降级路径把 [delta..., done]
// 聚合为最后一个非零项（即完成项）。该机制是 eino 为自定义流类型提供的
// RegisterStreamChunkConcatFunc 注册点，幂等覆盖。
func init() {
	compose.RegisterStreamChunkConcatFunc(func(items []AgentOutput) (AgentOutput, error) {
		if len(items) == 0 {
			return AgentOutput{}, nil
		}
		return items[len(items)-1], nil
	})
}

// Agent 是 AI 助手的对话执行器：持有编译后的四节点 Eino Graph（Runnable），
// 提供非流式（Generate）与流式（Stream）两种执行入口，并依赖 SessionStore
// 做会话持久化（见 StreamRun）。行为参数（提示词、分节标题、上下文与截断上限）
// 一律经参数解析器 params 读取。
type Agent struct {
	resolver      ChatResolver                // 图内 LLM 节点（按档位解析对话模型）
	retriever     retriever.Retriever         // 知识检索器，供 knowledge_search 工具适配器调用（*Retriever 天然满足）
	store         *SessionStore               // 会话持久化
	params        *Params                     // 行为参数解析器（提示词、标题、上限、截断长度）
	memory        *Memory                     // 会话记忆服务（滚动摘要）；为 nil 时跳过摘要
	logger        *logging.Logger             // 日志记录器（可空：为 nil 时跳过告警）
	tools         []tool.BaseTool             // 本次装配可用的工具（无工具时为空）
	toolInfos     []*schema.ToolInfo          // 与 tools 对应的 schema 清单（无工具时为空）
	toolIndex     map[string]tool.BaseTool    // 按 Info(ctx).Name 建立的工具索引（节点 4 派发用）
	toolInfoIndex map[string]*schema.ToolInfo // 按 Name 建立的 schema 索引（审核评估按名取用途说明用）
	// slotContextLimit 按档位解析上下文窗口容量（token 数）的回调，由上层经 WithSlotContextLimit
	// 注入（取自模型注册表 llm_models.context_tokens）；为 nil 时容量恒为 0（不做占用展示与自动压缩）。
	slotContextLimit func(ai.Slot) int
	// runnable 编译后的四节点图执行入口（Generate 与 Stream 共用）。
	runnable compose.Runnable[AgentInput, AgentOutput]
}

// AgentOption 是 NewAgent 的可选装配项，用于注入不改变核心依赖契约的附加能力。
type AgentOption func(*Agent)

// WithSlotContextLimit 注入「按档位解析上下文窗口容量」的回调：Agent 在生成前据此计算上下文
// 占用比例并在达到阈值时自动压缩。未注入时容量恒为 0，不做占用展示与自动压缩。
func WithSlotContextLimit(fn func(ai.Slot) int) AgentOption {
	return func(a *Agent) { a.slotContextLimit = fn }
}

// contextLimit 返回指定档位的上下文窗口容量；未注入取数回调时返回 0。
func (a *Agent) contextLimit(slot ai.Slot) int {
	if a.slotContextLimit == nil {
		return 0
	}
	return a.slotContextLimit(slot)
}

// NewAgent 构造 Agent：校验依赖后组装四节点图并编译。resolver/retriever/store/params
// 任一为空时返回英文错误；memory 与 logger 允许为 nil（memory 为 nil 时跳过滚动摘要，
// logger 为 nil 时跳过告警）；resolver 与 retriever 均为函数/接口参数，测试可注入替身。
// 行为参数由上层（Runtime）从 agent_params 解析后传入。tools/toolInfos 为本次装配可用的
// 工具及其 schema 清单，二者均不做非空校验：toolInfos 为空即「无工具模式」，节点 4 不传
// 工具选项，行为与引入工具前一致。工具索引在此按 Info(ctx).Name 构建，ctx 由调用方传入
// （即 Runtime 的装配 ctx），使取消、超时与链路追踪同样覆盖构造期，而非藏在 background
// context 之后；Info 取失败的工具被跳过（该工具因此不可派发，但其 schema 若在 toolInfos
// 中仍会被模型看到——正常装配下两者同源，不会出现该情形）。
func NewAgent(ctx context.Context, resolver ChatResolver, retriever retriever.Retriever, store *SessionStore, params *Params, memory *Memory, logger *logging.Logger, tools []tool.BaseTool, toolInfos []*schema.ToolInfo, opts ...AgentOption) (*Agent, error) {
	if resolver == nil {
		return nil, errors.New("agent: nil chat resolver")
	}
	if retriever == nil {
		return nil, errors.New("agent: nil retriever")
	}
	if store == nil {
		return nil, errors.New("agent: nil session store")
	}
	if params == nil {
		return nil, errors.New("agent: nil params")
	}
	a := &Agent{
		resolver:      resolver,
		retriever:     retriever,
		store:         store,
		params:        params,
		memory:        memory,
		logger:        logger,
		tools:         tools,
		toolInfos:     toolInfos,
		toolIndex:     buildToolIndex(ctx, tools),
		toolInfoIndex: buildToolInfoIndex(toolInfos),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	g := compose.NewGraph[AgentInput, AgentOutput](
		compose.WithGenLocalState(func(ctx context.Context) *AgentState {
			return &AgentState{}
		}),
	)
	// 节点 1：上下文组装（输入校验 + 写图状态），输出检索查询串。
	if err := g.AddLambdaNode(agentNodeContextAssembly, compose.InvokableLambda(a.assembleContext)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 2：业务数据清洗 + 最终消息组装（系统提示 + 历史 + 业务现场）。
	if err := g.AddLambdaNode(agentNodeSanitize, compose.InvokableLambda(a.sanitizeBusinessData)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 3：LLM 生成（流式 lambda；按本次运行档位解析对话模型后 Stream），
	// 输出 *schema.Message 流，节点 key 与出边保持不变。
	if err := g.AddLambdaNode(agentNodeLLMGeneration, compose.StreamableLambda(a.generate)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 4：结构化校验（透传增量 + 流末尾聚合校验），输出 AgentOutput 流。
	if err := g.AddLambdaNode(agentNodeValidation, compose.TransformableLambda(a.validateOutput)); err != nil {
		return nil, errors.WithStack(err)
	}
	edges := [][2]string{
		{compose.START, agentNodeContextAssembly},
		{agentNodeContextAssembly, agentNodeSanitize},
		{agentNodeSanitize, agentNodeLLMGeneration},
		{agentNodeLLMGeneration, agentNodeValidation},
		{agentNodeValidation, compose.END},
	}
	for _, e := range edges {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	// 构造期图编译（非活动链路）：compile 只在构造 Agent 时发生一次、与任何一次运行 / 请求无关，
	// 故用 Background；运行期的上下文（含取消）由 runnable.Stream / Invoke 的入参 ctx 承载。
	runnable, err := g.Compile(context.Background())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a.runnable = runnable
	return a, nil
}

// generate 是节点 3（流式 lambda）：从图状态读取本次运行解析出的档位与工具阶段上报回调，
// 经 resolver 取回对话客户端。无启用工具（toolInfos 为空）时退化为单次流式生成，行为与
// 引入工具前完全一致；存在启用工具时执行受 agent.maxToolRounds 约束的工具调用循环——
// 每轮模型流式生成，每轮正文分块都逐块随流外发（含工具轮的前导说明文本，用户可边生成边看到
// 模型的中间过程），含 ToolCalls 的轮次处理工具、回填结果后进入下一轮。单个工具执行失败时把
// 错误文本作为结果回填，不中断整轮；达到轮数上限仍有 ToolCalls 时以该轮文本为答案（逐块随流
// 外发，与其他轮一致）并上报 toolCall 阶段 failed，不返回错误。档位未绑定或解析失败时返回英文
// 错误，由图的流错误通道向外传播。
// 工具调用归属：client 工具在具备会话与审批通道时一律登记挂起等前端执行；backend 工具（builtin /
// MCP，位于 a.toolIndex）仅在评估为 must 级审核时挂起，其余照旧行内执行；同轮内行内执行的 backend
// 工具先回填结果，需挂起的调用再统一持久化现场并下发 tool_request，无待审批调用则进入下一轮。
// 无会话或无审批通道时不改变既有行为（client 按失败回灌、backend 一律行内执行）。
func (a *Agent) generate(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	var slot ai.Slot
	var reporter EvalReporter
	var toolRequests ToolRequestReporter
	var in AgentInput
	var collector *knowledgeCollector
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		slot = st.slot
		reporter = st.toolReporter
		toolRequests = st.toolRequests
		in = st.input
		collector = st.knowledge
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	// builder 是本轮运行的块构建器：工具调用 / 结果的发起与回填在此写入，与节点 4 的思考 / 正文增量、
	// wrapPersist 的挂起建块 / 收敛共享同一构建器，故块时序单点来源在本包内成立。为 nil（旁路调用）时跳过。
	builder := in.blocks
	chat, err := a.resolver(slot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if chat == nil {
		return nil, errors.Newf("agent: no chat model resolved for slot %q", slot)
	}
	// 思考开关：对话客户端可选实现 requestOptionsProvider（*llm.ChatClient 即实现了它），
	// 按本次输入的思考模式构造请求选项；未实现时 opts 为空，两处 Stream 行为保持原样。
	var opts []model.Option
	if p, ok := chat.(requestOptionsProvider); ok {
		opts = p.RequestOptions(in.Thinking)
	}
	// 无工具模式：不传工具选项，原样走单次流式生成（真流式，行为与引入工具前一致）。
	if len(a.toolInfos) == 0 {
		reader, err := chat.Stream(ctx, messages, opts...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return reader, nil
	}

	outSR, outSW := schema.Pipe[*schema.Message](64)
	go func() {
		defer outSW.Close()
		// 轮数上限非正数时钳到 1；结果字节上限非正数时不截断。
		maxRounds := a.params.Int("agent.maxToolRounds")
		if maxRounds <= 0 {
			maxRounds = 1
		}
		maxBytes := a.params.Int("agent.maxToolResultBytes")
		// 派发工具前注入记忆与知识检索运行期信息：memory_write 据此取回写入器与本次运行的
		// 归属，knowledge_search 据此取回检索器（命名空间/文档限定）与采集器。
		toolCtx := a.memoryToolContext(ctx, in)
		toolCtx = a.knowledgeToolContext(toolCtx, in, collector)
		current := messages
		for round := 0; round < maxRounds; round++ {
			reader, err := chat.Stream(ctx, current, append([]model.Option{model.WithTools(a.toolInfos)}, opts...)...)
			if err != nil {
				outSW.Send(nil, errors.WithStack(err))
				return
			}
			// 本轮边读边发：正文分块到达即外发（真流式，边生成边可见），同时聚合全文供判定本轮
			// 是否为工具轮、并把含 ToolCalls 的消息追加进下一轮上下文。
			msg, err := collectRoundStream(reader, outSW)
			if err != nil {
				outSW.Send(nil, errors.WithStack(err))
				return
			}
			// 答案轮：无 ToolCalls，本轮正文已逐块随流外发，无需再补发。
			if len(msg.ToolCalls) == 0 {
				return
			}
			// 达到轮数上限仍有未完成的工具调用：以该轮文本为答案、上报 toolCall 阶段失败、不返回
			// 错误。该轮正文同样已逐块随流外发，故此处不再补发。
			if round == maxRounds-1 {
				err := errors.Newf("agent: reached the tool-call round bound (%d) with pending tool calls, using this round's text as the answer", maxRounds)
				if reporter != nil {
					if rerr := reporter(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusFailed, Round: round + 1, Err: err.Error()}); rerr != nil {
						outSW.Send(nil, errors.WithStack(rerr))
					}
				}
				return
			}
			// 中间轮：把含 ToolCalls 的 assistant 消息追加进下一轮上下文，并逐个处理工具、把结果
			// 作为 tool 消息回填；本轮正文（模型的前导说明文本）已随流外发，不重复补发。
			// 思考过程只在生成时随 SSE 外发给用户，不作为上下文回灌给模型：供应商侧 reasoning_content
			// 属响应字段，回传无收益，部分供应商还明确要求不要回传。故追加前清空——挂起时持久化的
			// 消息链用的正是这里的 current（见 persistSuspendedRun），恢复续跑因此同样不含思考内容。
			msg.ReasoningContent = ""
			current = append(current, msg)
			// 本轮是否存在挂起通道：有持久化会话且具备审批通道时，client 工具一律挂起，backend 工具
			// 在评估为 must 级时挂起；否则两者都按各自既有语义（client 失败回灌 / backend 行内执行）。
			canSuspend := in.SessionID != 0 && toolRequests != nil
			// 两遍处理：第一遍按原始顺序收集每条调用（记录归属与 client 在无通道时的回灌文案）并上报
			// toolCall/start；第二遍据评估重要级决定「挂起」还是「行内执行」，使同轮内行内执行的 backend
			// 工具与挂起的调用可共存。
			type toolCallEntry struct {
				tc       schema.ToolCall
				owner    ToolExecOwner
				call     *pendingToolCall // client 调用（具备挂起通道时非 nil）
				failText string           // client 工具在无会话/无通道时的回灌文案（非空即按失败回灌）
				// questionnaire 非 nil 表示该 client 调用是问卷类（ask_user）且入参合法：豁免审核
				// 重要级评估与阀门合成，挂起时据其载荷写入本回合的锚点消息行
				questionnaire *QuestionnairePayload
			}
			entries := make([]toolCallEntry, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if reporter != nil {
					if rerr := reporter(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: name, CallID: tc.ID, Round: round + 1}); rerr != nil {
						outSW.Send(nil, errors.WithStack(rerr))
						return
					}
				}
				// client 来源工具不在本地工具索引 a.toolIndex 中，须特判：仅当本次运行有持久化会话
				// 且具备审批通道时登记为待执行调用；否则记录回灌文案（稍后按工具失败回灌）。
				if isClientTool(name) {
					call, failText := prepareClientToolCall(tc, in, round+1, toolRequests)
					entry := toolCallEntry{tc: tc, owner: ToolExecOwnerClient, call: call, failText: failText}
					// 问卷类调用：解析入参为问卷载荷。入参非法时不挂起（撤销待执行登记、按工具失败回灌），
					// 让模型据错误文本修正后重试，避免给用户弹出一个残缺问卷。
					if isQuestionnaireTool(name) && call != nil {
						payload, perr := parseQuestionnaireArgs(tc.Function.Arguments)
						if perr != nil {
							entry.call = nil
							entry.failText = "invalid ask_user arguments: " + perr.Error()
						} else {
							entry.questionnaire = payload
						}
					}
					entries = append(entries, entry)
					continue
				}
				entries = append(entries, toolCallEntry{tc: tc, owner: ToolExecOwnerBackend})
			}
			// 挂起通道具备时，对本次全部调用（client + backend）做一次批量评估，产出与调用顺序对齐的
			// 审核重要级：backend 据 must 级决定是否挂起，client 据档位与会话阀门合成决定是否需人工审核。
			// 问卷类调用（ask_user）豁免该评估：它不入 evalCalls（不消耗评估），但 levels 仍与 entries
			// 逐位对齐（豁免位留空值 none），使下游继续可直接用 levels[i]。
			var levels []AuditLevel
			if canSuspend {
				evalCalls := make([]pendingToolCall, 0, len(entries))
				// evalIdx 记录 evalCalls 第 k 项对应的 entries 下标，回填时据此对齐。
				evalIdx := make([]int, 0, len(entries))
				for ei, e := range entries {
					if e.questionnaire != nil {
						continue
					}
					if e.owner == ToolExecOwnerBackend {
						evalCalls = append(evalCalls, pendingToolCall{
							CallID:    e.tc.ID,
							ToolName:  e.tc.Function.Name,
							Arguments: e.tc.Function.Arguments,
							ExecOwner: ToolExecOwnerBackend,
						})
						evalIdx = append(evalIdx, ei)
						continue
					}
					if e.call != nil {
						evalCalls = append(evalCalls, *e.call)
						evalIdx = append(evalIdx, ei)
					}
				}
				levels = make([]AuditLevel, len(entries))
				for k, level := range a.evaluateAuditLevels(ctx, slot, evalCalls) {
					if k < len(evalIdx) {
						levels[evalIdx[k]] = level
					}
				}
			}
			// pending 收集本轮需挂起等审批的调用；非空即本轮结束时挂起。阀门空值保守按「逐一审核」，
			// 避免误放行。questionnaire 记录本轮的问卷载荷：一轮内可有多条问卷类（ask_user）调用，其
			// 题目按调用顺序合并为同一张问卷（前端只弹一个面板、作答后逐条回传结果），交由 wrapPersist
			// 写入锚点行。
			var pending []pendingToolCall
			var questionnaire *QuestionnairePayload
			// seenQuestionIDs 记录已并入本轮问卷的题目 id：答案按题目 id 归属，id 重复即无法拆分，
			// 故对含重复 id 的调用改按工具失败回灌，让模型修正后重试。
			seenQuestionIDs := make(map[string]struct{})
			valve := in.ReviewValve
			if valve == "" {
				valve = ai.ReviewValveEach
			}
			for i, e := range entries {
				name := e.tc.Function.Name
				// 登记本次工具调用的发起：按轮次（round）归入同一 tool_calls 块（同轮并发调用同块）。
				// 审核重要级 / 执行归属取自本次评估与归属判定；问卷类调用豁免评估时该轮为空值 none。
				if builder != nil {
					level := AuditLevelNone
					if i < len(levels) {
						level = levels[i]
					}
					builder.AddToolCall(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: name, CallID: e.tc.ID, Round: round + 1}, e.tc.Function.Arguments, level, e.owner)
				}
				switch {
				case e.owner == ToolExecOwnerClient && e.failText != "":
					// 无持久化会话或审批通道：client 工具按失败回灌，不产生任何 ToolRequest。
					if reporter != nil {
						if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, Err: e.failText}); rerr != nil {
							outSW.Send(nil, errors.WithStack(rerr))
							return
						}
					}
					if builder != nil {
						builder.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, Err: e.failText}, "")
					}
					current = append(current, schema.ToolMessage(e.failText, e.tc.ID))
				case e.questionnaire != nil:
					// 问卷类调用：豁免审核重要级与阀门合成，恒 none / false，直接挂起等前端弹问卷。
					// 同一回合内多条 ask_user 的题目合并为同一张问卷；含重复题目 id 的调用会让回传答案
					// 无法归属，此时该条调用改按工具失败回灌，让模型修正后重试。
					if dup := markQuestionIDs(seenQuestionIDs, e.questionnaire.Questions); dup != "" {
						text := "duplicate ask_user question id: " + dup
						if reporter != nil {
							if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, Err: text}); rerr != nil {
								outSW.Send(nil, errors.WithStack(rerr))
								return
							}
						}
						if builder != nil {
							builder.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, Err: text}, "")
						}
						current = append(current, schema.ToolMessage(text, e.tc.ID))
					} else {
						call := *e.call
						call.AuditLevel = AuditLevelNone
						call.ApprovalRequired = false
						pending = append(pending, call)
						if questionnaire == nil {
							// 本轮提问前模型已外发的正文由先出现的 content 块自然承载（该轮正文分块已随流
							// 外发并归入 content 块），故问卷载荷只记题目，不再另存前导说明。
							questionnaire = &QuestionnairePayload{}
						}
						questionnaire.Questions = append(questionnaire.Questions, e.questionnaire.Questions...)
					}
				case e.owner == ToolExecOwnerClient:
					call := *e.call
					call.AuditLevel = levels[i]
					call.ApprovalRequired = NeedsHumanReview(levels[i], valve)
					pending = append(pending, call)
				case canSuspend && levels[i] == AuditLevelMust:
					// backend 工具仅在评估为 must 级时挂起等审批（与阀门无关）。
					pending = append(pending, pendingToolCall{
						CallID:           e.tc.ID,
						ToolName:         name,
						Arguments:        e.tc.Function.Arguments,
						ExecOwner:        ToolExecOwnerBackend,
						AuditLevel:       AuditLevelMust,
						ApprovalRequired: true,
						Round:            round + 1,
					})
				default:
					// backend 工具行内执行：失败把错误文本作为结果回填让模型自行纠错，绝不当作流错误
					// 上抛；成功按字节上限截断。
					start := time.Now()
					result, execErr := a.runTool(toolCtx, e.tc)
					elapsed := time.Since(start).Milliseconds()
					if execErr != nil {
						result = "tool execution failed: " + execErr.Error()
						if reporter != nil {
							if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, ElapsedMs: elapsed, Err: execErr.Error()}); rerr != nil {
								outSW.Send(nil, errors.WithStack(rerr))
								return
							}
						}
						if builder != nil {
							builder.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: e.tc.ID, Round: round + 1, ElapsedMs: elapsed, Err: execErr.Error()}, result)
						}
					} else {
						result = truncateBytes(result, maxBytes)
						if reporter != nil {
							if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: name, CallID: e.tc.ID, Round: round + 1, ElapsedMs: elapsed}); rerr != nil {
								outSW.Send(nil, errors.WithStack(rerr))
								return
							}
						}
						if builder != nil {
							builder.AddToolResult(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: name, CallID: e.tc.ID, Round: round + 1, ElapsedMs: elapsed}, result)
						}
					}
					current = append(current, schema.ToolMessage(result, e.tc.ID))
				}
			}
			// 本轮有待审批调用：挂起本次运行——先持久化运行现场，再置挂起标志，最后按原序逐条下发
			// tool_request；随后结束本次流（defer outSW.Close()）且不产出 done。无待审批调用则维持
			// 既有行为进入下一轮。问卷挂起是例外：其下发延后到 wrapPersist 写好锚点行之后（见下），
			// 以便载荷带上锚点消息 ID。
			if len(pending) > 0 {
				runID, err := a.persistSuspendedRun(ctx, in, slot, current, collector, pending)
				if err != nil {
					outSW.Send(nil, errors.WithStack(err))
					return
				}
				// 挂起：把本轮载荷与现场 ID 写入运行期共享状态，由 wrapPersist 在流以挂起结束时据此把本回合的
				// 助手消息行落为锚点（问卷挂起写入问卷载荷、其余挂起仅置 phase=suspended），并把其 ID 回写现场。
				// 本轮调用一并暂存而非即时下发：锚点消息 ID 要到 wrapPersist 落库后才可知，届时下发才能把该 ID
				// 带进 tool_request 载荷（前端实时会话据此把挂起定位到具体助手消息行）。仅当运行期共享状态存在
				// （StreamRun / ResumeRun 均会置入）时才如此；否则回落为即时下发（messageID 恒 0），与该状态引入前一致。
				stashSuspension := in.Questionnaire != nil
				if stashSuspension {
					// 各轮并列追加而非并入同一份：同一回合内模型可在拿到上一轮答案后再次提问，锚点行的
					// 问卷载荷须把本轮作为新的一轮追加到已累计的各轮之后（一轮一份，见 appendQuestionnaireRound），
					// 前端据此把各轮渲染成各自独立的问答框。追加而非覆盖的同时也保证此前的题目与答案不被抹掉；
					// 非问卷挂起（本轮载荷为空）时保留已累计的各轮，使锚点行的问卷卡片不被抹掉。
					in.Questionnaire.rounds = appendQuestionnaireRound(in.Questionnaire.rounds, questionnaire)
					in.Questionnaire.runID = runID
					in.Questionnaire.pending = pending
				}
				if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
					st.suspended = true
					return nil
				}); err != nil {
					outSW.Send(nil, errors.WithStack(err))
					return
				}
				// 未暂存（运行期共享状态缺失，无处可锚）：仍在此时即时下发，messageID 恒 0
				if !stashSuspension {
					if err := reportToolRequests(toolRequests, pending, 0); err != nil {
						outSW.Send(nil, errors.WithStack(err))
						return
					}
				}
				return
			}
		}
	}()
	return outSR, nil
}

// collectRoundStream 边读边发地抽取本轮模型流：逐块读取并聚合全部分块供上层判定本轮是否为
// 工具轮（并把含 ToolCalls 的消息追加进下一轮上下文），同时把本轮的正文分块**到达即写入**
// outSW——这是真流式的关键，避免整轮抽干后一次性下发造成的长时间无输出。含工具调用的轮次
// 同样外发其正文（模型的前导说明文本，如「我先检索一下知识库」）：中间过程不隐藏，且实时
// 增量与最终 done 正文（节点 4 对流内容做常规聚合）保持同一份文本，两者一致。
// 返回聚合后的完整消息。
func collectRoundStream(reader *schema.StreamReader[*schema.Message], outSW *schema.StreamWriter[*schema.Message]) (*schema.Message, error) {
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.WithStack(err)
		}
		chunks = append(chunks, chunk)
		if chunk.Content != "" || chunk.ReasoningContent != "" {
			outSW.Send(chunk, nil)
		}
	}
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return msg, nil
}

// buildToolIndex 按 Info(ctx).Name 为可派发工具建立索引：Info 取失败的工具被跳过（本包
// Agent 无 logger 依赖，故不记录日志，仅依赖运行期的 reporter 上报工具失败）。
func buildToolIndex(ctx context.Context, tools []tool.BaseTool) map[string]tool.BaseTool {
	index := make(map[string]tool.BaseTool, len(tools))
	for _, built := range tools {
		if built == nil {
			continue
		}
		info, err := built.Info(ctx)
		if err != nil || info == nil || info.Name == "" {
			continue
		}
		index[info.Name] = built
	}
	return index
}

// buildToolInfoIndex 按 Name 为工具 schema 清单建立索引（跳过 nil 与空名），供审核评估按工具名
// 取回用途说明，避免每次线性扫描 toolInfos。
func buildToolInfoIndex(infos []*schema.ToolInfo) map[string]*schema.ToolInfo {
	index := make(map[string]*schema.ToolInfo, len(infos))
	for _, info := range infos {
		if info == nil || info.Name == "" {
			continue
		}
		index[info.Name] = info
	}
	return index
}

// runTool 派发一次工具调用：按工具名在索引中查找并断言为 tool.InvokableTool，取不到或类型
// 不符即视为执行失败（返回英文错误），否则以 JSON 参数串调用 InvokableRun 并返回其结果。
// 入口先检查 ctx.Err()：会话被中止时立即返回取消错误，使工具阶段随取消即时感知，无需等到下一次
// 模型调用才收敛（工具执行本身也应尊重传入 ctx，见内部 tools 的 InvokableRun）。
func (a *Agent) runTool(ctx context.Context, tc schema.ToolCall) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.WithStack(err)
	}
	name := tc.Function.Name
	built, ok := a.toolIndex[name]
	if !ok || built == nil {
		return "", errors.Newf("agent: no tool named %q is available", name)
	}
	invokable, ok := built.(tool.InvokableTool)
	if !ok {
		return "", errors.Newf("agent: tool %q is not an invokable tool", name)
	}
	res, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return res, nil
}

// memoryToolContext 在派发工具前把记忆运行期信息（写入器与归属：命名空间 / Agent 类型 / 来源
// 会话）注入 ctx，供 memory_write 在工具循环中取回；记忆服务缺失或本次运行无会话（SessionID
// 为 0）时原样返回 ctx（长期记忆按会话隔离，无会话运行不生成记忆，工具因取不到运行期信息而
// 返回英文错误，不会静默写入）。
func (a *Agent) memoryToolContext(ctx context.Context, in AgentInput) context.Context {
	if a.memory == nil || in.SessionID == 0 {
		return ctx
	}
	return tools.WithMemoryRuntime(ctx, tools.MemoryRuntime{
		Writer:        memoryStoreWriter{store: a.memory.store},
		NamespaceName: in.NamespaceName,
		AgentKind:     in.AgentKind,
		SessionID:     in.SessionID,
		UserID:        in.UserID,
	})
}

// truncateBytes 把字符串按 UTF-8 rune 边界截断到不超过 max 字节（max 非正数时不截断），
// 发生截断时追加 "..."，保证不产生残缺的多字节字符。与 truncateText（rune 语义）不同，
// 本函数以字节为限，用于限制工具结果对模型上下文的占用。
func truncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// 从 max 处向前回退到 rune 边界，避免切断多字节字符。
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// resolveInputSlot 解析本次运行的档位并写回输入：优先级为显式 Slot → 调用点声明的
// DeclaredSlot → 参数 agent.defaultSlot。解析失败返回英文错误。
func (a *Agent) resolveInputSlot(input *AgentInput) error {
	fallback := ai.Slot(a.params.String("agent.defaultSlot"))
	slot, err := ai.ResolveSlot(input.Slot, input.DeclaredSlot, fallback)
	if err != nil {
		return errors.WithStack(err)
	}
	input.Slot = slot
	return nil
}

// Generate 非流式执行一次对话，返回最终结构化产物（done 项）。
func (a *Agent) Generate(ctx context.Context, input AgentInput, opts ...compose.Option) (AgentOutput, error) {
	if err := a.resolveInputSlot(&input); err != nil {
		return AgentOutput{}, errors.WithStack(err)
	}
	out, err := a.runnable.Invoke(ctx, input, opts...)
	if err != nil {
		return AgentOutput{}, errors.WithStack(err)
	}
	return out, nil
}

// Stream 流式执行一次对话，返回 AgentOutput 流（若干 delta 增量项 + 一个 done
// 完成项）。调用方负责关闭返回的 reader。
func (a *Agent) Stream(ctx context.Context, input AgentInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	if err := a.resolveInputSlot(&input); err != nil {
		return nil, errors.WithStack(err)
	}
	reader, err := a.runnable.Stream(ctx, input, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return reader, nil
}

// StreamRun 是带会话持久化的流式执行入口（SSE 管线后端）：
//  1. 会话存在时先校验会话、加载历史消息（先按 session.maxContextMessages 取最近 N 条、
//     再按 session.maxContextBytes 从最旧裁剪整条）并注入图输入；生成前按本次运行的有效档位
//     （显式 slot → DeclaredSlot（会话档位）→ agent.defaultSlot）感知上下文占用，占用比例达到
//     session.compactThresholdRatio 时自动流式压缩（折叠被挤出消息进滚动摘要并落库，失败仅告警、
//     不中断生成）；随后按 memory.summaryMinEvictedMessages 阈值推进滚动摘要（自动压缩已推进游标时
//     它是无操作，失败仅告警），摘要一并注入图输入；
//  2. 持久化本轮用户消息；
//  3. 执行流式图；
//  4. 流末尾收到 done 项时先持久化助手回复（正文 + 草稿 + 引用）再向外发送 done。
//
// SessionID 为 0 时跳过全部持久化，行为与 Stream 一致。会话不存在时返回
// ErrSessionNotFound 且不落任何消息；提问为空时返回英文错误（不落空消息）。
func (a *Agent) StreamRun(ctx context.Context, input AgentInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	// 耗时口径从进入本函数（加载历史之前）起算，覆盖整轮回答而不止模型推理；采集器经回调
	// 累加本轮内所有模型调用的 token 用量，两者都在 done 项产出时一并取出。
	metrics := newRunMetrics()
	if strings.TrimSpace(input.Query) == "" {
		return nil, errors.New("agent: empty query")
	}
	if input.SessionID != 0 {
		session, err := a.store.GetSession(ctx, input.SessionID, input.NamespaceName, input.UserID)
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				return nil, errors.WithStack(err)
			}
			return nil, errors.WithStack(err)
		}
		history, err := a.store.HistoryMessages(ctx, input.SessionID, input.UserID, a.params.Int("session.maxContextMessages"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 字节预算裁剪：条数窗口之后，再按 session.maxContextBytes 从最旧开始丢弃整条消息
		// （至少保留最新一条）；裁剪后的最旧消息 ID 即窗口下界，供滚动摘要圈定被挤出消息。
		history = trimMessagesByBytes(history, a.params.Int("session.maxContextBytes"))
		input.HistoryMessages = toSchemaMessages(history)
		// 会话所属 Agent 类型是长期记忆的隔离维度，由会话决定（调用方的显式取值不覆盖它）。
		input.AgentKind = session.AgentKind
		// 会话级审核阀门随会话注入本次运行，供挂起前与每条调用的重要级合成。
		input.ReviewValve = session.ReviewValve
		// 生成前按本次运行的有效档位感知上下文占用：显式 slot → DeclaredSlot（会话档位）→
		// agent.defaultSlot（这里的解析与 a.Stream 内的再次解析幂等）。占用比例达到阈值时先
		// 自动流式压缩——折叠被挤出消息进滚动摘要并落库；压缩失败仅告警、绝不中断生成。
		if err := a.resolveInputSlot(&input); err != nil {
			return nil, errors.WithStack(err)
		}
		if limit := a.contextLimit(input.Slot); limit > 0 {
			threshold := a.params.Float("session.compactThresholdRatio")
			contextTokens, _ := a.store.ContextTokens(ctx, session.ID, input.UserID)
			if float64(contextTokens)/float64(limit) >= threshold {
				evicted, err := a.evictedForSummary(ctx, session, history)
				if err != nil {
					a.warnf(ctx, "agent: loading evicted messages for auto compression failed: %v", err)
				} else if len(evicted) > 0 {
					a.CompressSession(ctx, session, evicted, "auto", input.Slot, input.CompressReporter)
				}
			}
		}
		// 压缩已就地推进 session 的摘要与游标，故此处取到的是压缩后摘要；未触发压缩时按
		// memory.summaryMinEvictedMessages 阈值继续静默滚动摘要。
		input.Summary = a.refreshSummary(ctx, session, history)
		if _, err := a.store.AppendMessage(ctx, &ai.AgentMessage{
			SessionID:     input.SessionID,
			NamespaceName: input.NamespaceName,
			UserID:        input.UserID,
			Role:          ai.MessageRoleUser,
			Content:       serializer.NewCrypto([]byte(strings.TrimSpace(input.Query))),
		}); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	// 挂起运行期共享状态：本指针随 input 值拷贝进入图与 wrapPersist，节点 3 在挂起时写入载荷（仅问卷
	// 挂起）与现场 ID，wrapPersist 据此把本回合的助手消息行落为「挂起锚点」（见 questionnaireState）。
	// 未挂起的运行该状态自始至终为空。
	input.Questionnaire = &questionnaireState{}
	// 块构建器：本次运行的块时序唯一来源，随 input 以指针形式在节点 3（工具调用 / 结果）、节点 4（思考 /
	// 正文增量）与 wrapPersist（挂起建块 / 收敛 / 落库）之间共享。首次运行尚无助手消息行，故 messageID
	// 为 0，待首次落库拿到行 ID 后经 SetMessageID 补正。
	input.blocks = newBlockBuilder(0, a.params.Int("agent.maxToolResultBytes"))
	reader, err := a.Stream(ctx, input, append(opts, compose.WithCallbacks(metrics.handler()))...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.wrapPersist(ctx, reader, input, metrics), nil
}

// RunDetached 是脱管执行入口：以 StreamRun 执行带会话持久化的流式图，并把输出流抽干到
// 回调——onEvent 收到每个流项（增量 / 完成），onToolRequest 收到挂起时下发的客户端工具
// 请求。调用方须传入脱管上下文（见 RunHub），使客户端断开不中止图循环、且 done 助手消息
// 仍以脱管 ctx 落库。任一回调返回错误或流报错即中止并返回该错误；正常结束返回 nil。
func (a *Agent) RunDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	in.ToolRequestReporter = onToolRequest
	reader, err := a.StreamRun(ctx, in)
	if err != nil {
		return errors.WithStack(err)
	}
	defer reader.Close()
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.WithStack(err)
		}
		if onEvent != nil {
			if err := onEvent(item); err != nil {
				return errors.WithStack(err)
			}
		}
	}
}

// ResumeDetached 以已备好的续跑输入脱管执行一次续跑并把输出抽干到回调（供 RunHub 驻留执行）：
// 抽干语义与 RunDetached 一致（onEvent 收每个流项、onToolRequest 收再次挂起时下发的客户端工具请求），
// 区别仅在于输入由 PrepareResume 备好——不重载历史、不追加用户消息。调用方须传入脱管上下文，使客户端
// 断开不中止图循环、且最终 done 仍以脱管 ctx 落库。
func (a *Agent) ResumeDetached(ctx context.Context, in AgentInput, onEvent func(AgentOutput) error, onToolRequest func(ToolRequest) error) error {
	in.ToolRequestReporter = onToolRequest
	metrics := newRunMetrics()
	reader, err := a.Stream(ctx, in, compose.WithCallbacks(metrics.handler()))
	if err != nil {
		return errors.WithStack(err)
	}
	reader = a.wrapPersist(ctx, reader, in, metrics)
	defer reader.Close()
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.WithStack(err)
		}
		if onEvent != nil {
			if err := onEvent(item); err != nil {
				return errors.WithStack(err)
			}
		}
	}
}

// trimMessagesByBytes 按字节预算从最旧开始丢弃整条消息，直到剩余消息的 Content 字节总数不
// 超过 maxBytes；至少保留最新一条（只丢整条消息，故不会切断多字节字符），maxBytes 非正数或
// 未超预算时原样返回。
func trimMessagesByBytes(messages []*ai.AgentMessage, maxBytes int) []*ai.AgentMessage {
	if maxBytes <= 0 || len(messages) == 0 {
		return messages
	}
	total := 0
	for _, msg := range messages {
		if msg != nil {
			total += len(msg.Content.Value())
		}
	}
	if total <= maxBytes {
		return messages
	}
	cut := 0
	for cut < len(messages)-1 && total > maxBytes {
		if messages[cut] != nil {
			total -= len(messages[cut].Content.Value())
		}
		cut++
	}
	return messages[cut:]
}

// evictedForSummary 返回「窗口之外（seq < 窗口最旧消息 seq）且尚未被摘要覆盖（seq > summary_upto_seq）」
// 的消息，供滚动摘要圈定被挤出的历史。
func (a *Agent) evictedForSummary(ctx context.Context, session *ai.AgentSession, history []*ai.AgentMessage) ([]*ai.AgentMessage, error) {
	var windowOldestSeq uint
	if len(history) > 0 && history[0] != nil {
		windowOldestSeq = uint(history[0].Seq)
	}
	return a.store.ListMessagesBefore(ctx, session.ID, session.UserID, windowOldestSeq, session.SummaryUptoSeq)
}

// refreshSummary 在加载窗口历史后推进滚动摘要：取「窗口外且 seq > summary_upto_seq 的
// 未摘要消息」（窗口最旧消息 seq 作上界），条数达到 memory.summaryMinEvictedMessages 且 memory
// 非 nil 时用 flash 重写摘要并落库，同时把新摘要写回会话副本使本次生成即用新摘要。摘要失败仅
// 告警、不中断生成、不推进游标；不受 memory.enabled 开关约束（该开关只管长期记忆）。
func (a *Agent) refreshSummary(ctx context.Context, session *ai.AgentSession, history []*ai.AgentMessage) string {
	if session == nil {
		return ""
	}
	existing := string(session.Summary.Value())
	if a.memory == nil {
		return existing
	}
	evicted, err := a.evictedForSummary(ctx, session, history)
	if err != nil {
		a.warnf(ctx, "agent: loading evicted messages for the rolling summary failed: %v", err)
		return existing
	}
	if len(evicted) == 0 || len(evicted) < a.params.Int("memory.summaryMinEvictedMessages") {
		return existing
	}
	summary, upto, err := a.memory.Summarize(ctx, session, evicted)
	if err != nil {
		a.warnf(ctx, "agent: rolling summary failed, keeping the previous summary: %v", err)
		return existing
	}
	// 推进前先取该时刻的上下文占用（最近一条有用量记录的助手消息的 PromptTokens，见 ContextTokens）
	// 作为度量：与是否展示无关，三种推进原因都落值；是否回显由读取侧按 reason 决定。取占用失败则放弃
	// 本次推进（保留原摘要、本轮不折叠）。注意该指标本就可能为 0——会话尚无任何一次真实测量（例如
	// 唯一一轮回复被取消）时它取不到值，0 表示「尚无测量」而非「占用为 0」。
	contextTokens, err := a.store.ContextTokens(ctx, session.ID, session.UserID)
	if err != nil {
		a.warnf(ctx, "agent: loading the context usage for the rolling summary failed, keeping the previous summary: %v", err)
		return existing
	}
	if err := a.store.UpdateSummaryAndCheckpoint(ctx, session, summary, upto, "rolling", CompactionMetrics{TokensBefore: contextTokens, FoldedCount: len(evicted)}); err != nil {
		a.warnf(ctx, "agent: persisting the rolling summary failed, keeping the previous summary: %v", err)
		return existing
	}
	session.Summary = serializer.NewCrypto([]byte(summary))
	session.SummaryUptoSeq = upto
	return summary
}

// CompressSession 执行一次流式上下文压缩：把 evicted 折叠进滚动摘要并落库，按需抽取长期记忆，
// 过程经 report 依次上报 compress_start / compress_delta / compress_done。失败仅告警（上报
// foldedCount=0 的 compress_done），不返回错误、不推进游标。reason 取 auto / manual。
// slot 为本次运行的有效档位（自动路径传入显式解析结果），为空时依次回退到会话档位
// session.TargetModel 与参数 agent.defaultSlot，据此取该档位的上下文容量。
func (a *Agent) CompressSession(ctx context.Context, session *ai.AgentSession, evicted []*ai.AgentMessage, reason string, slot ai.Slot, report CompressReporter) *CompactResult {
	if session == nil || a.memory == nil {
		return nil
	}
	// 先取压缩前的上下文占用作为检查点度量（最近一条有用量记录的助手消息的 PromptTokens，见
	// ContextTokens）。取占用失败按压缩失败同等处理——告警、上报 foldedCount=0 的 compress_done、
	// 不推进游标。该值本就可能为 0：会话尚无任何一次真实测量（例如唯一一轮回复被取消）时取不到
	// 基线，0 表示「尚无测量」，前端据此不展示前后对比（见可视化侧 compressTextOf）。
	contextTokens, err := a.store.ContextTokens(ctx, session.ID, session.UserID)
	if err != nil {
		a.warnf(ctx, "agent: loading the context usage for the session compression failed, keeping the previous summary: %v", err)
		if report != nil {
			report(CompressEvent{Type: ai.AgentRunEventCompressDone, FoldedCount: 0})
		}
		return nil
	}
	if slot == "" {
		slot = session.TargetModel
	}
	if slot == "" {
		slot = ai.Slot(a.params.String("agent.defaultSlot"))
	}
	limit := a.contextLimit(slot)
	threshold := a.params.Float("session.compactThresholdRatio")
	if report != nil {
		if err := report(CompressEvent{
			Type:      ai.AgentRunEventCompressStart,
			Reason:    reason,
			Tokens:    contextTokens,
			Limit:     limit,
			Threshold: threshold,
		}); err != nil {
			return nil
		}
	}
	if len(evicted) == 0 {
		if report != nil {
			report(CompressEvent{Type: ai.AgentRunEventCompressDone, FoldedCount: 0})
		}
		return &CompactResult{Summary: string(session.Summary.Value())}
	}
	result, err := a.memory.CompactStream(ctx, session, evicted, reason,
		CompactionMetrics{TokensBefore: contextTokens, FoldedCount: len(evicted)}, func(d string) error {
			if report == nil {
				return nil
			}
			return report(CompressEvent{Type: ai.AgentRunEventCompressDelta, Delta: d})
		})
	if err != nil {
		a.warnf(ctx, "agent: session compression failed, keeping the previous summary: %v", err)
		if report != nil {
			report(CompressEvent{Type: ai.AgentRunEventCompressDone, FoldedCount: 0})
		}
		return nil
	}
	if report != nil {
		report(CompressEvent{
			Type:           ai.AgentRunEventCompressDone,
			Reason:         reason,
			TokensBefore:   contextTokens,
			TokensAfter:    result.TokensAfter,
			FoldedCount:    len(evicted),
			SummaryUptoSeq: session.SummaryUptoSeq,
		})
	}
	return result
}

// OptimizePrompt 单轮优化用户诉求：以参数 agent.promptOptimizeSystemPrompt 为系统提示，用 flash
// 档位把 query 改写为更清晰、更易被模型正确理解的提问，返回裁剪后的纯文本。该能力无会话、无工具、
// 无记忆，不写任何库。query 裁剪后为空时不调用模型即返回英文错误；resolver 缺失、flash 档位未绑定
// 或解析失败、模型返回空内容时同样返回英文错误，绝不静默降级到其它档位（与 GenerateTopic /
// Summarize 等辅助 LLM 任务一致）。
func (a *Agent) OptimizePrompt(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("agent: empty prompt to optimize")
	}
	if a.resolver == nil {
		return "", errors.New("agent: nil chat resolver")
	}
	chat, err := a.resolver(ai.SlotFlash)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if chat == nil {
		return "", errors.Newf("agent: no chat model resolved for slot %q", ai.SlotFlash)
	}
	resp, err := chat.Generate(ctx, []*schema.Message{
		schema.SystemMessage(a.params.Text("agent.promptOptimizeSystemPrompt")),
		schema.UserMessage(query),
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	if resp == nil {
		return "", errors.New("agent: prompt optimization model returned no response")
	}
	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", errors.New("agent: prompt optimization model returned an empty response")
	}
	return out, nil
}

// warnf 在 logger 非空时输出告警。
func (a *Agent) warnf(ctx context.Context, format string, args ...any) {
	if a.logger == nil {
		return
	}
	a.logger.Ctx(ctx).Warnf(format, args...)
}

// assistantCheckpointBytes 是助手回复阶段性落库的字节阈值：本轮累积的正文与思考过程增量合计每达到
// 该阈值即写库一次，使长耗时运行的中途产物随时可回读（刷新后仍可见）。等待越久产出越多，若仅在
// done 时一次性落库，一旦中断（超时 / 取消 / 报错）整轮产物即丢失，重试成本随产出线性增长。
const assistantCheckpointBytes = 2048

// interruptedPhase 把「本轮运行被中断」的错误归类为生命周期终态阶段：取消（含退订宽限自动取消）归
// cancelled，其余（运行上界 deadline 触发、模型报错等）归 failed。取消既可能体现为承载本次运行的
// 上下文被取消（ctx.Err()），也可能体现为流自身返回包装过的 context.Canceled 错误，两者都识别。
// 该阶段随助手消息落库（agent_messages.phase），并由 RunHub 以同名阶段写入 error 事件负载，使前端
// 据阶段展示「本轮被中断」而无需依赖正文里的提示文案。
func interruptedPhase(ctx context.Context, err error) ai.AgentRunPhase {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return ai.AgentRunPhaseCancelled
	}
	return ai.AgentRunPhaseFailed
}

// wrapPersist 包装图输出流并把本轮助手回复落库：
//   - 流中累积的正文（delta）与思考过程（reasoning）增量合计每达到 assistantCheckpointBytes 即
//     阶段性写库一次——首次落库走 AppendMessage 分配消息行并记下其 ID，其后按该 ID 原地更新同一行，
//     使长耗时运行的中途产物随时可回读，而非仅在成功结束时一次性落库；阶段性落库失败只记录告警，
//     不中断正在进行的生成（DB 短暂抖动不应毁掉一轮昂贵的模型调用）；
//   - 收到 done 项时，先把本轮用量与耗时（metrics 快照）写入该 done 项，会话存在时再以同一行终态化
//     落库（写入阶段 done），最后向外发送 done——先赋值后落库、后下发，保证「落库」与「SSE 载荷」
//     取到同一份数值；此处落库失败把错误作为流错误发送；
//   - 流以非 EOF 错误结束（超时 / 取消 / 模型报错）时，先把已累积的正文与思考落库并写入中断阶段
//     （cancelled / failed），再照常把错误作为流错误下发——中断前的产物与「本轮被中断」这一事实
//     一并保留，且提示文案由前端据阶段渲染，不混入正文；
//   - 流以无 done 的 EOF 结束（挂起等待工具审批）时：问卷挂起（本轮带 ask_user 调用）保留下方锚点行
//     ——写入问卷载荷、phase 置 suspended，并把其 ID 回写运行现场；其余挂起则回收阶段性落库的那一行
//     ——挂起不是终局，续跑会把本回合已外发的正文并回最终 done，留着残篇会在续跑后形成同一回合的
//     两条助手消息。
//
// SessionID 为 0 时只跳过落库，用量与耗时仍随 done 下发（无状态运行也能展示）。
func (a *Agent) wrapPersist(ctx context.Context, in *schema.StreamReader[AgentOutput], input AgentInput, metrics *runMetrics) *schema.StreamReader[AgentOutput] {
	outSR, outSW := schema.Pipe[AgentOutput](64)
	go func() {
		defer outSW.Close()
		defer in.Close()
		// persistCtx 是落库专用上下文：与本次运行的生命周期解耦（剥离 deadline 与取消），使「运行因
		// 超时 / 取消被中断」这一刻仍能落库已产出的中间产物——这正是阶段性落库要挽救的场景。运行
		// 上下文本身带 30 分钟 deadline，触发中断时它已过期，若直接用它落库，写入必因
		// context deadline exceeded 失败，本次修复在其最需要的场景下即失效。
		// 注意：persistCtx 仅用于落库，事件下发（outSW.Send）一律在运行 ctx 的生命周期内进行，不得
		// 借用它——取消后内容事件的下发由 RunHub 的 publish 取消前置条件统一抑制。
		persistCtx := context.WithoutCancel(ctx)
		// builder 是本轮运行的块构建器：节点 3 的工具调用 / 结果、节点 4 的思考与正文增量、本层的挂起
		// 建块与收敛共享同一构建器，故块的时序唯一来源在本包内单点成立。为 nil（旁路调用）时跳过建块。
		builder := input.blocks
		// contentBuf / reasoningBuf 累积本轮尚未落库的正文与思考增量；assistantMsgID 为已落库助手
		// 消息行的 ID（0 表示本次运行尚未落库）；flushedContent / flushedReasoning 记录上次落库时的
		// 字节数，用于按增量阈值判断下次落库时机。
		var (
			contentBuf       strings.Builder
			reasoningBuf     strings.Builder
			assistantMsgID   uint
			flushedContent   int
			flushedReasoning int
			// finalized 标记本轮已按 done 终态化落库，使「无 done 的 EOF」能唯一识别为挂起。
			finalized bool
		)
		// 问卷续跑：本次运行的助手消息行就是挂起时写下的锚点行，终态化须原地写回该行（不另起新行），
		// 从而维持「同一回合只有一条助手消息」的不变式。
		if input.Questionnaire != nil && input.Questionnaire.anchorID != 0 {
			assistantMsgID = input.Questionnaire.anchorID
		}
		// checkpoint 把当前累积的正文连同给定的生命周期终态阶段落库（阶段性落库传空阶段，中断落库传
		// cancelled / failed）。尚无已落库行且正文为空时不落库，避免产生空的助手消息行。思考过程不再落
		// 消息列，一律由 reasoning 块承载（见 flushBlocks）。
		checkpoint := func(phase ai.AgentRunPhase) error {
			if input.SessionID == 0 {
				return nil
			}
			content := contentBuf.String()
			if assistantMsgID == 0 && content == "" {
				return nil
			}
			msg := &ai.AgentMessage{
				SessionID:     input.SessionID,
				NamespaceName: input.NamespaceName,
				UserID:        input.UserID,
				Role:          ai.MessageRoleAssistant,
				Content:       serializer.NewCrypto([]byte(content)),
				Phase:         phase,
			}
			// ID 由内嵌 BaseModel 提升而来，复合字面量不可直接赋值，故构造后单独置入。
			msg.ID = assistantMsgID
			if assistantMsgID == 0 {
				appended, err := a.store.AppendMessage(persistCtx, msg)
				if err != nil {
					return errors.WithStack(err)
				}
				assistantMsgID = appended.ID
			} else if err := a.store.UpdateAssistantMessage(persistCtx, msg); err != nil {
				return errors.WithStack(err)
			}
			flushedContent, flushedReasoning = contentBuf.Len(), reasoningBuf.Len()
			return nil
		}
		// flushBlocks 把构建器待落库的块写入 agent_message_blocks：尚无助手消息行时先建一行取 ID，再逐条
		// AppendBlock（新建）/ UpdateBlock（同块更新）。无待写入或本次运行无会话时为空操作（仅清空待写入集）。
		// 块的 MessageID 一律以当前 assistantMsgID 覆盖——构建器在拿到消息行 ID 前产出的写入其 MessageID
		// 仍为 0，此处统一补正，避免孤儿块。
		flushBlocks := func(ctx context.Context) error {
			if builder == nil {
				return nil
			}
			writes := builder.DrainWrites()
			if len(writes) == 0 || input.SessionID == 0 {
				return nil
			}
			if assistantMsgID == 0 {
				appended, err := a.store.AppendMessage(ctx, &ai.AgentMessage{
					SessionID:     input.SessionID,
					NamespaceName: input.NamespaceName,
					UserID:        input.UserID,
					Role:          ai.MessageRoleAssistant,
					Content:       serializer.NewCrypto([]byte(contentBuf.String())),
				})
				if err != nil {
					return errors.WithStack(err)
				}
				assistantMsgID = appended.ID
				builder.SetMessageID(assistantMsgID)
			}
			for _, w := range writes {
				block := &ai.AgentMessageBlock{
					NamespaceName: input.NamespaceName,
					UserID:        input.UserID,
					MessageID:     assistantMsgID,
					BlockSeq:      w.BlockSeq,
					Kind:          w.Kind,
					Payload:       serializer.NewCrypto(w.Payload),
				}
				if w.Created {
					if err := a.store.AppendBlock(ctx, block); err != nil {
						return errors.WithStack(err)
					}
				} else if err := a.store.UpdateBlock(ctx, block); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		}
		// converge 收敛块构建器并把待落库块写入库：终态化 / 挂起 / 中断 / 失败前调用，使尚在开放的块收敛为
		// 终态并落库（此后块序列保持不变）。落库失败仅告警——块是展示事实，不应因落库抖动毁掉已产出的一轮。
		converge := func(ctx context.Context) {
			if builder != nil {
				builder.Close()
			}
			if berr := flushBlocks(ctx); berr != nil {
				a.warnf(ctx, "agent: persisting the assistant message blocks failed: %v", berr)
			}
		}
		// syncQuestionnaire 把锚点已有各轮 questionnaire 块的作答回填为本次续跑的结果：仅在问卷续跑
		// （Questionnaire 非空且已载入既有轮次、锚点存在）时生效，供各终态路径在收敛落库后调用——续跑
		// 未再次挂起（直接 done / 中断 / 失败）时，此前挂起写下的问卷块也须补上本次作答，否则刷新后仍
		// 呈「未作答」。非问卷续跑（loadedRounds 为 0）时为空操作。失败仅告警（块是展示事实）。
		syncQuestionnaire := func(ctx context.Context) {
			if input.Questionnaire == nil || input.Questionnaire.anchorID == 0 {
				return
			}
			if berr := a.syncAnchorQuestionnaireBlocks(ctx, input, input.Questionnaire.anchorID); berr != nil {
				a.warnf(ctx, "agent: syncing the anchor questionnaire blocks failed: %v", berr)
			}
		}
		for {
			item, err := in.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					// 无 done 的 EOF 即「本轮流因待审批 / 待作答的工具调用挂起」（见节点 3 的挂起分支：只发
					// tool_request，不产出 done）。挂起一律把本回合的助手消息行落为「锚点」并保留：phase 置
					// suspended，其 ID 回写运行现场——续跑据此原地终态化该行（不再新增一行，故同一回合恒只有一条
					// 助手消息），回退 / 「重新回答」接口据此截断其后消息，用户刷新后也能据它重建现场。问卷挂起
					// 额外写入问卷载荷；其余挂起不写载荷、仅以其为锚点。锚点行落库后才逐条下发本轮挂起的调用，
					// 使 tool_request 载荷带上锚点消息 ID。落库 / 上报失败均仅告警：挂起现场已落库，本轮挂起已
					// 成立，多一行残篇（或锚点缺失）不应妨碍它。
					// 以 !finalized 为前提：已按 done 终态化的轮次其流同样以 EOF 结束，若不加此判据，续跑（或
					// 再次挂起）的 EOF 会把刚终态化的行再次覆盖为 suspended。
					// 运行期共享状态缺失（旁路调用，Questionnaire 为 nil）时无处可锚，回落为回收阶段性落库行：
					// 挂起并非终局，续跑会把本回合已外发的助手正文并回最终 done（validateOutput 的 turnEmittedText
					// 拼接），留着残篇会在续跑结束后留下同一回合的两条助手消息。
					// 额外注意：问卷载荷不再写消息列，一律由 questionnaire 块承载（见下方挂起建块）。
					if !finalized && input.Questionnaire != nil && input.Questionnaire.runID != 0 {
						q := input.Questionnaire
						// 正文并回恢复运行前已外发的助手正文（turnEmittedText），与续跑终态化时的拼接口径一致，
						// 使锚点行在续跑前就完整呈现本回合已产出的文本（刷新后不丢前导说明）。
						content := turnEmittedText(input.ResumeMessages) + contentBuf.String()
						// 挂起建块：待审批调用落为 approval 块、本轮新产生的问卷各轮落为 questionnaire 块
						// （此前各轮的问卷块已在锚点消息里，只回填其作答）。建块先于锚点行落库，使块序号与
						// 真实产生时序一致。
						if builder != nil {
							a.addSuspensionBlocks(builder, q)
						}
						if aerr := a.persistSuspensionAnchor(persistCtx, input, assistantMsgID, content); aerr != nil {
							a.warnf(ctx, "agent: persisting the suspended anchor message failed: %v", aerr)
						}
						// 锚点行落库后：把其 ID 交给构建器并收敛、落库本轮新产生的块，再回填锚点已有问卷块
						// 的作答。块须在拿到锚点 ID 之后才落库，故顺序不可颠倒。
						if q.anchorID != 0 {
							assistantMsgID = q.anchorID
							if builder != nil {
								builder.SetMessageID(q.anchorID)
							}
						}
						converge(persistCtx)
						syncQuestionnaire(persistCtx)
						// 本轮挂起产生的块事件随最后一个流项下发（此前无承载项），使前端能实时渲染。
						if builder != nil {
							if events := builder.DrainEvents(); len(events) > 0 {
								outSW.Send(AgentOutput{Blocks: events}, nil)
							}
						}
						// 锚点行已落库并回写现场：此刻才逐条下发本轮挂起的调用，使载荷带上锚点消息 ID。
						// 上报失败（客户端断开）仅告警：挂起现场与锚点行均已落库，本轮挂起已成立，不必再抛错。
						if rerr := reportToolRequests(input.ToolRequestReporter, q.pending, q.anchorID); rerr != nil {
							a.warnf(ctx, "agent: reporting the suspended tool requests failed: %v", rerr)
						}
						return
					}
					if !finalized && assistantMsgID != 0 {
						if derr := a.store.DeleteMessage(persistCtx, assistantMsgID, input.UserID); derr != nil {
							a.warnf(ctx, "agent: discarding the suspended assistant message row failed: %v", derr)
						}
					}
					// 本回合的落库行被回收（旁路调用，无处可锚）：构建器待落库的块一并丢弃——其从属消息行
					// 已不存在，留着会变成孤儿块。
					if builder != nil {
						builder.Close()
						builder.DrainWrites()
					}
					return
				}
				if cerr := checkpoint(interruptedPhase(ctx, err)); cerr != nil {
					a.warnf(ctx, "agent: persisting the interrupted assistant message failed: %v", cerr)
				}
				converge(persistCtx)
				syncQuestionnaire(persistCtx)
				outSW.Send(AgentOutput{}, errors.WithStack(err))
				return
			}
			switch {
			case item.Done:
				item.PromptTokens, item.CompletionTokens, item.TotalTokens, item.ElapsedMs = metrics.snapshot()
				// 先收敛并落库块（含回填锚点已有问卷块的本次作答），再按 done 终态化助手消息行（沿用同一条已落库行）。
				converge(persistCtx)
				syncQuestionnaire(persistCtx)
				if input.SessionID != 0 {
					finalizedID, ferr := a.finalizeAssistantMessage(persistCtx, input, item, assistantMsgID)
					if ferr != nil {
						outSW.Send(AgentOutput{}, errors.WithStack(ferr))
						return
					}
					// 随 done 项下发本回合助手消息的落库行 ID：前端实时会话据此把本地气泡绑定到该行
					item.MessageID = finalizedID
					finalized = true
				}
			case item.Delta != "":
				contentBuf.WriteString(item.Delta)
			case item.Reasoning != "":
				reasoningBuf.WriteString(item.Reasoning)
			}
			// 块事件与增量同源同序：把本流项到达前构建器新产生的块事件（创建 / 收敛）挂到本项上，供
			// run_hub 以 block 事件下发（前端据 {seq, kind} 归并增量到当前开放块）。
			if builder != nil {
				item.Blocks = builder.DrainEvents()
			}
			outSW.Send(item, nil)
			// 先下发再落库：用户能尽早看到增量，落库只在其后补做；增量未达阈值时不落库。
			if !item.Done && contentBuf.Len()-flushedContent+reasoningBuf.Len()-flushedReasoning >= assistantCheckpointBytes {
				if cerr := checkpoint(""); cerr != nil {
					a.warnf(ctx, "agent: persisting the assistant message checkpoint failed: %v", cerr)
				}
				if berr := flushBlocks(persistCtx); berr != nil {
					a.warnf(ctx, "agent: persisting the assistant message blocks failed: %v", berr)
				}
			}
			// 分块边界检查取消（放在处理完当前分块之后）：会话被中止时立即落库已产出的部分正文 / 思考
			// （cancelled 阶段）并结束本流，使取消能被及时感知。置于循环底部而非顶部，是为了不丢弃管
			// 道中已缓冲、尚未读取的分块——取消先于首个 Recv 发生时若在顶部早退，会把已外发到管道的
			// 部分正文整段丢弃、导致「保留部分正文」不成立；底部检查则先取尽已到达的分块再收尾。
			if cerr := ctx.Err(); cerr != nil {
				if perr := checkpoint(interruptedPhase(ctx, cerr)); perr != nil {
					a.warnf(ctx, "agent: persisting the interrupted assistant message failed: %v", perr)
				}
				converge(persistCtx)
				syncQuestionnaire(persistCtx)
				outSW.Send(AgentOutput{}, errors.WithStack(cerr))
				return
			}
		}
	}()
	return outSR
}

// finalizeAssistantMessage 把 done 项终态化落库为会话的助手消息：msgID 非 0 时按该 ID 原地更新本
// 运行阶段性落库的那一行（沿用其已分配好的会话内序号），为 0 时新建一行（本运行全程未触发阶段性
// 落库）。正文始终落库并写入终态阶段 done，结构化草稿与引用来源非空时随消息写入；本轮用量与耗时为
// 明文数值，随消息一并落库（用户消息不参与本函数，故其用量列保持 0）。返回该助手消息的落库行 ID，
// 供调用方回填进 done 项下发给前端（新建行时即新分配的行 ID，原地更新时即 msgID）。
func (a *Agent) finalizeAssistantMessage(ctx context.Context, input AgentInput, done AgentOutput, msgID uint) (uint, error) {
	msg := &ai.AgentMessage{
		SessionID:        input.SessionID,
		NamespaceName:    input.NamespaceName,
		UserID:           input.UserID,
		Role:             ai.MessageRoleAssistant,
		Content:          serializer.NewCrypto([]byte(done.Content)),
		PromptTokens:     done.PromptTokens,
		CompletionTokens: done.CompletionTokens,
		TotalTokens:      done.TotalTokens,
		ElapsedMs:        done.ElapsedMs,
		Phase:            ai.AgentRunPhaseDone,
	}
	// ID 由内嵌 BaseModel 提升而来，复合字面量不可直接赋值，故构造后单独置入（0 表示新建一行）。
	msg.ID = msgID
	if len(done.Draft) > 0 && string(done.Draft) != "null" {
		msg.DraftJSON = serializer.NewCrypto(done.Draft)
	}
	if len(done.Citations) > 0 {
		if b, err := json.Marshal(done.Citations); err == nil {
			msg.CitationsJSON = serializer.NewCrypto(b)
		}
	}
	if msgID == 0 {
		appended, err := a.store.AppendMessage(ctx, msg)
		if err != nil {
			return 0, errors.WithStack(err)
		}
		return appended.ID, nil
	}
	if err := a.store.UpdateAssistantMessage(ctx, msg); err != nil {
		return 0, errors.WithStack(err)
	}
	return msgID, nil
}

// persistSuspensionAnchor 把本回合的助手消息行落为「挂起锚点」：正文取本轮已外发的助手正文（已在调用处
// 并回恢复运行前的正文，可能为空）、phase 置 suspended。问卷题目 / 作答与授权清单不再写消息列，一律由
// questionnaire / approval 块承载（见 addSuspensionBlocks）。msgID 非 0 时原地更新阶段性落库的那一行
// （沿用其已分配好的会话内序号），为 0 时新建一行（无任何已外发正文时即为空行——挂起仍须有锚点以承载
// messageID 并供续跑原地终态化）。落库成功后把锚点 ID 记入运行期共享状态并回写运行现场
// （agent_runs.anchor_message_id），使续跑与回退接口都能定位它。
func (a *Agent) persistSuspensionAnchor(ctx context.Context, input AgentInput, msgID uint, content string) error {
	q := input.Questionnaire
	if q == nil || q.runID == 0 {
		return nil
	}
	msg := &ai.AgentMessage{
		SessionID:     input.SessionID,
		NamespaceName: input.NamespaceName,
		UserID:        input.UserID,
		Role:          ai.MessageRoleAssistant,
		Content:       serializer.NewCrypto([]byte(content)),
		Phase:         ai.AgentRunPhaseSuspended,
	}
	// ID 由内嵌 BaseModel 提升而来，复合字面量不可直接赋值，故构造后单独置入（0 表示新建一行）。
	msg.ID = msgID
	if msgID == 0 {
		appended, aerr := a.store.AppendMessage(ctx, msg)
		if aerr != nil {
			return aerr
		}
		q.anchorID = appended.ID
	} else {
		if aerr := a.store.UpdateAssistantMessage(ctx, msg); aerr != nil {
			return aerr
		}
		q.anchorID = msgID
	}
	return a.store.SetAgentRunAnchor(ctx, q.runID, q.anchorID)
}

// addSuspensionBlocks 把本轮挂起产生的内容落为块：待审批调用（非问卷类）落为一个 approval 块，本轮新
// 产生的每一轮问卷各落为一个 questionnaire 块（此前各轮的问卷块已在锚点消息里，不重复建块，只回填其
// 作答——见 syncAnchorQuestionnaireBlocks）。因同一回合内可同时有 backend 待审批调用与 ask_user 问卷
// 挂起，两类块可并存。
func (a *Agent) addSuspensionBlocks(builder *blockBuilder, q *questionnaireState) {
	var approvalCalls []PendingCall
	for _, call := range toPendingCalls(q.pending) {
		if isQuestionnaireTool(call.ToolName) {
			continue
		}
		approvalCalls = append(approvalCalls, call)
	}
	if len(approvalCalls) > 0 {
		builder.AddApproval(approvalCalls)
	}
	start := q.loadedRounds
	if start > len(q.rounds) {
		start = len(q.rounds)
	}
	for _, round := range q.rounds[start:] {
		if len(round.Questions) > 0 {
			builder.AddQuestionnaire(round.Questions)
		}
	}
}

// syncAnchorQuestionnaireBlocks 把锚点消息上**已有**的各轮 questionnaire 块回填本次作答：按 block_seq
// 升序与本次载入的各轮（rounds 的前 loadedRounds 轮）一一对应，更新其 Answers 并置 Awaiting=false。
// 本次运行新产生的轮次由 addSuspensionBlocks 经 AddQuestionnaire 新建块承载（Awaiting=true），不在此
// 回填。无续跑载入（loadedRounds 为 0）或锚点缺失时为空操作。
func (a *Agent) syncAnchorQuestionnaireBlocks(ctx context.Context, input AgentInput, messageID uint) error {
	q := input.Questionnaire
	if q == nil || messageID == 0 || q.loadedRounds == 0 {
		return nil
	}
	blocks, err := a.store.ListBlocksByMessage(ctx, messageID, input.NamespaceName, input.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	idx := 0
	for _, blk := range blocks {
		if idx >= q.loadedRounds {
			break
		}
		if blk.Kind != ai.AgentMessageBlockKindQuestionnaire || blk.Payload == nil {
			continue
		}
		var payload QuestionnaireBlockPayload
		if derr := decodeBlockPayload(blk.Payload.Value(), &payload); derr != nil {
			idx++
			continue
		}
		if idx < len(q.rounds) {
			payload.Answers = q.rounds[idx].Answers
		}
		payload.Awaiting = false
		data, merr := encodeBlockPayload(payload)
		if merr != nil {
			return merr
		}
		if uerr := a.store.UpdateBlock(ctx, &ai.AgentMessageBlock{
			NamespaceName: input.NamespaceName,
			UserID:        input.UserID,
			MessageID:     messageID,
			BlockSeq:      blk.BlockSeq,
			Payload:       serializer.NewCrypto(data),
		}); uerr != nil {
			return uerr
		}
		idx++
	}
	return nil
}

// assembleContext 是节点 1：校验提问非空，将完整输入写入图状态（供后续节点
// 读取业务数据/历史/校验规则），并输出检索查询串。
func (a *Agent) assembleContext(ctx context.Context, in AgentInput) (string, error) {
	// 恢复路径没有新的用户提问，跳过提问非空校验；首次运行仍要求提问非空。
	if in.ResumeMessages == nil && strings.TrimSpace(in.Query) == "" {
		return "", errors.New("agent: empty query")
	}
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		st.input = in
		if in.ResumeMessages != nil {
			// 恢复路径：把挂起时刻持久化的引用预置进采集器，供节点 4 在 done 中产出引用。
			st.knowledge = &knowledgeCollector{results: in.ResumeCitations}
		} else {
			st.knowledge = &knowledgeCollector{}
		}
		st.slot = in.Slot
		st.toolReporter = in.ToolReporter
		st.toolRequests = in.ToolRequestReporter
		st.resume = in.ResumeMessages
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}
	return in.Query, nil
}

// sanitizeBusinessData 是节点 2：从图状态读取原始输入，清洗业务数据并组装
// 发送给 LLM 的最终消息列表。
func (a *Agent) sanitizeBusinessData(ctx context.Context, query string) ([]*schema.Message, error) {
	var in AgentInput
	var resume []*schema.Message
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		in = st.input
		resume = st.resume
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	// 恢复路径短路：直接透传挂起时刻持久化的消息链，不重跑业务数据清洗与记忆召回，避免上下文漂移。
	if resume != nil {
		return resume, nil
	}
	sanitized, err := SanitizeBusinessData(in.BusinessData, a.params)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return buildAgentMessages(a.params, in.AgentKind, in.Summary, a.recallMemories(ctx, in), in.HistoryMessages, sanitized, query), nil
}

// recallMemories 取回本次运行可注入的长期记忆：记忆服务缺失、memory.enabled=false、命名空间为
// 空、本次运行无会话（SessionID 为 0）或 Agent 类型非法时返回 nil（不注入记忆分节）；检索失败
// 降级为 nil 且只记录告警，绝不中断生成——长期记忆是增强能力，缺失时行为与引入记忆前一致。
func (a *Agent) recallMemories(ctx context.Context, in AgentInput) []MemoryHit {
	if a.memory == nil || !a.params.Bool("memory.enabled") || in.SessionID == 0 {
		return nil
	}
	agentKind, err := ai.ParseAgentKind(in.AgentKind)
	if err != nil {
		a.warnf(ctx, "agent: long-term memory retrieval skipped, invalid agent kind: %v", err)
		return nil
	}
	hits, err := a.memory.Recall(ctx, in.NamespaceName, agentKind, in.SessionID, in.UserID, in.Query)
	if err != nil {
		a.warnf(ctx, "agent: long-term memory retrieval failed, skipping the memory section: %v", err)
		return nil
	}
	return hits
}

// validateOutput 是节点 4：透传 LLM 流式增量（delta 项），流结束后聚合全文并
// 执行结构化产物解析与 Rego 校验，产出最终 done 项。
func (a *Agent) validateOutput(ctx context.Context, in *schema.StreamReader[*schema.Message]) (*schema.StreamReader[AgentOutput], error) {
	outSR, outSW := schema.Pipe[AgentOutput](64)
	go func() {
		defer outSW.Close()
		defer in.Close()
		var chunks []*schema.Message
		var reasoning strings.Builder
		// 块构建器（与节点 3 / wrapPersist 共享）：思考与正文增量在真实产生时点写入，使块时序单点来源
		// 在本包内成立。为 nil（旁路调用）时跳过建块。
		var builder *blockBuilder
		if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
			builder = st.input.blocks
			return nil
		}); err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		for {
			// 分块边界检查取消：会话被中止时立即返回取消错误，使下游尽快收敛，不等下一次回收。
			if cerr := ctx.Err(); cerr != nil {
				outSW.Send(AgentOutput{}, errors.WithStack(cerr))
				return
			}
			chunk, err := in.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				outSW.Send(AgentOutput{}, errors.WithStack(err))
				return
			}
			chunks = append(chunks, chunk)
			if chunk.Content != "" {
				if builder != nil {
					builder.AddContent(chunk.Content)
				}
				outSW.Send(AgentOutput{Delta: chunk.Content}, nil)
			}
			if chunk.ReasoningContent != "" {
				reasoning.WriteString(chunk.ReasoningContent)
				if builder != nil {
					builder.AddReasoning(chunk.ReasoningContent)
				}
				outSW.Send(AgentOutput{Reasoning: chunk.ReasoningContent}, nil)
			}
		}
		// 挂起：节点 3 已把本次流置为以 tool_request 为终态，故不产出 done、不报错，直接结束。
		var suspended bool
		var resume []*schema.Message
		if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
			suspended = st.suspended
			resume = st.resume
			return nil
		}); err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		if suspended {
			return
		}
		// 聚合全文即最终正文：节点 3 已把每轮正文分块逐块外发（含工具轮的前导说明文本），
		// 故此处聚合出的正文与用户实时看到的增量是同一份文本。
		full, err := schema.ConcatMessages(chunks)
		if err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		out, err := a.finalizeOutput(ctx, full)
		if err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		// 恢复运行：本回合在上次挂起之前已随流外发的助手正文（挂起轮的前导说明文本）不在本次流里，
		// 须并回 done 正文——前端在 done 时以正文覆盖已累积的增量，缺失会让用户已经看到的文本凭空
		// 消失。必须拼接在 finalizeOutput 之后：结构化响应要先按完整原文解析出正文与草稿，先拼接
		// 会破坏其 JSON 解析。
		out.Content = turnEmittedText(resume) + out.Content
		out.Reasoning = reasoning.String()
		outSW.Send(out, nil)
	}()
	return outSR, nil
}

// turnEmittedText 返回消息链中最后一条 user 消息之后各 assistant 消息的正文拼接，即本回合在
// 本次恢复运行之前已经随流外发给用户的助手正文（挂起轮的前导说明文本）。恢复运行没有新的用户
// 提问，链中最后一条 user 消息即本回合的提问，其后只可能是本回合已产生的助手/工具消息，故该
// 区间恰好圈定「属于本回合、且已外发」的正文；历史轮次的助手正文都在该 user 消息之前，不会被
// 误并。链为空、无 user 消息或无 assistant 正文时返回空串（普通运行传入 nil 链亦然）。
func turnEmittedText(msgs []*schema.Message) string {
	lastUser := -1
	for i, m := range msgs {
		if m != nil && m.Role == schema.User {
			lastUser = i
		}
	}
	if lastUser < 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range msgs[lastUser+1:] {
		if m != nil && m.Role == schema.Assistant {
			sb.WriteString(m.Content)
		}
	}
	return sb.String()
}

// finalizeOutput 聚合全文后解析结构化产物并执行可选的 Rego 校验，产出完成项。
// 解析失败降级为把原文作为正文（见 parseStructuredReply）；Rego 规则执行出错
// 视为校验不通过并记录说明，不中断整个会话。
func (a *Agent) finalizeOutput(ctx context.Context, full *schema.Message) (AgentOutput, error) {
	out := AgentOutput{Done: true, Valid: true}
	if full == nil || strings.TrimSpace(full.Content) == "" {
		return out, errors.New("agent: empty model response")
	}
	content, draft, err := parseStructuredReply(full.Content)
	if err != nil {
		// 非结构化响应降级：正文原样返回，无草稿。
		content = full.Content
	}
	out.Content = content
	out.Draft = draft

	var citations []RetrievalResult
	var regoRule string
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		citations = st.knowledge.snapshot()
		regoRule = st.input.RegoRule
		return nil
	}); err != nil {
		return out, errors.WithStack(err)
	}
	out.Citations = citations
	if strings.TrimSpace(regoRule) != "" {
		valid, detail, verr := validateDraftWithRego(ctx, regoRule, draft)
		if verr != nil {
			out.Valid = false
			out.ValidationDetail = verr.Error()
		} else {
			out.Valid = valid
			out.ValidationDetail = detail
		}
	}
	return out, nil
}

// structuredReply 是模型结构化响应的 JSON 形状。
type structuredReply struct {
	Content string          `json:"content"`
	Draft   json.RawMessage `json:"draft"`
}

// parseStructuredReply 解析模型的结构化 JSON 响应（{"content": "...", "draft":
// {...}}），容忍 ```json 代码围栏包裹。解析失败或 content 为空时返回错误，
// 调用方降级为将原文作为正文（无结构化草稿），保证聊天能力不受结构化约束影响。
func parseStructuredReply(content string) (string, json.RawMessage, error) {
	var reply structuredReply
	if err := json.Unmarshal([]byte(stripJSONFence(content)), &reply); err != nil {
		return "", nil, errors.WithStack(err)
	}
	if strings.TrimSpace(reply.Content) == "" {
		return "", nil, errors.New("agent: structured reply has empty content")
	}
	if len(reply.Draft) == 0 || string(reply.Draft) == "null" {
		return reply.Content, nil, nil
	}
	return reply.Content, reply.Draft, nil
}

// stripJSONFence 去掉首尾的 ```json 代码围栏（若存在），返回去除首尾空白后的内容。
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
		if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return s
}

// validateDraftWithRego 用 Rego 规则校验结构化草稿：规则须在 data.agent.validate
// 处返回布尔值；draft 为 nil/空时以 null 作为输入。规则不产出布尔值或规则执行
// 出错均视为校验不通过（返回 detail，不返回 error），使上层把校验失败作为产物
// 校验信息输出而非中断整个会话。
func validateDraftWithRego(ctx context.Context, source string, draft json.RawMessage) (bool, string, error) {
	var input any
	if len(draft) > 0 {
		if err := json.Unmarshal(draft, &input); err != nil {
			return false, "", errors.WithStack(err)
		}
	}
	resultSet, err := rego.New(
		rego.Query(agentRegoQuery),
		rego.Module("agent_validation.rego", source),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		return false, "rego validation rule failed to evaluate: " + err.Error(), nil
	}
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		return false, "rego validation rule returned no result", nil
	}
	value, ok := resultSet[0].Expressions[0].Value.(bool)
	if !ok {
		return false, "rego validation rule must return a boolean", nil
	}
	if !value {
		return false, "structured draft failed the rego validation rule", nil
	}
	return true, "", nil
}

// buildAgentMessages 组装发送给 LLM 的完整消息列表：系统提示 + 会话摘要（若有）+
// 长期记忆分节（若有）+ 历史消息 + 业务现场 + 当前提问（业务段拼接在用户消息内，控制消息
// 条数）。长期记忆分节位于摘要之后、窗口历史之前。系统提示词按 Agent 类型选取：general
// 类型取 agent.generalSystemPrompt（纯 Markdown 正文），其余取 agent.systemPrompt（结构化
// JSON 信封）。摘要段标题、记忆段标题、两个分节标题与片段截断长度均经参数解析器 params 读取。
func buildAgentMessages(params *Params, agentKind ai.AgentKind, summary string, memories []MemoryHit, history []*schema.Message, businessData BusinessData, query string) []*schema.Message {
	promptKey := "agent.systemPrompt"
	if agentKind == ai.AgentKindGeneral {
		promptKey = "agent.generalSystemPrompt"
	}
	messages := make([]*schema.Message, 0, 3+len(history))
	messages = append(messages, schema.SystemMessage(params.Text(promptKey)))
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		messages = append(messages, schema.SystemMessage(agentSummarySectionTitle+"\n"+trimmed))
	}
	if section := formatMemories(params, memories); section != "" {
		messages = append(messages, schema.SystemMessage(section))
	}
	messages = append(messages, history...)
	var parts []string
	if len(businessData) > 0 {
		parts = append(parts, params.String("agent.businessSectionTitle")+"\n"+formatBusinessData(businessData))
	}
	parts = append(parts, params.String("agent.questionSectionTitle")+"\n"+strings.TrimSpace(query))
	messages = append(messages, schema.UserMessage(strings.Join(parts, "\n\n")))
	return messages
}

// formatMemories 把长期记忆命中格式化为一个注入段：段标题取参数 memory.sectionTitle，
// 每条形如 "- [kind] title: content"（无标题时省略 "title: "）；正文为空或全部为空时返回空串，
// 调用方据此跳过该分节。条数与单条长度已由 MemoryStore.Retrieve 按 memory.maxItems /
// memory.maxItemBytes 限界，本函数不再重复截断。
func formatMemories(params *Params, memories []MemoryHit) string {
	if len(memories) == 0 {
		return ""
	}
	lines := make([]string, 0, len(memories))
	for _, hit := range memories {
		content := strings.TrimSpace(hit.Content)
		if content == "" {
			continue
		}
		if title := strings.TrimSpace(hit.Title); title != "" {
			content = title + ": " + content
		}
		lines = append(lines, "- ["+string(hit.Kind)+"] "+content)
	}
	if len(lines) == 0 {
		return ""
	}
	return params.String("memory.sectionTitle") + "\n" + strings.Join(lines, "\n")
}

// formatBusinessData 将业务现场序列化为 JSON 文本。入参已经 SanitizeBusinessData
// 清洗（其结果保证可序列化），因此这里的序列化不会失败。
func formatBusinessData(data BusinessData) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return "```json\n" + string(b) + "\n```"
}

// toSchemaMessages 将持久化消息转为 eino schema.Message 列表（跳过空内容与
// 未知角色），供图输入注入历史上下文。
func toSchemaMessages(messages []*ai.AgentMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		if m == nil {
			continue
		}
		content := string(m.Content.Value())
		if strings.TrimSpace(content) == "" {
			continue
		}
		switch m.Role {
		case ai.MessageRoleUser:
			out = append(out, schema.UserMessage(content))
		case ai.MessageRoleAssistant:
			out = append(out, schema.AssistantMessage(content, nil))
		case ai.MessageRoleSystem:
			out = append(out, schema.SystemMessage(content))
		}
	}
	return out
}
