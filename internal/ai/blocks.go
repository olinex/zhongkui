// 本文件承载助手回答有序内容块（AgentMessageBlock）的运行时唯一实现：六类块（reasoning /
// tool_calls / approval / questionnaire / content / next_steps）的 JSON 载荷类型与统一编解码，
// 以及「当前开放块」追踪与消息内块序号（BlockSeq）分配。块的产生规则（思考增量开块 / 工具调用
// 按轮聚组 / 待审批建块 / 问卷建块 / 正文增量开块 / 推荐下一步建块）、开放块的收敛与待落库写入集
// 都在这里一处给出，使实时流（run_hub 的 block 事件）与落库（session store 的块读写）共用同一套
// 规则，从构造上消除「实时对、刷新错」的偏差。
// 块为展示事实，不参与模型上下文回灌（助手消息回灌模型的正文仍取 agent_messages.content）。

package ai

import (
	// 标准库
	"encoding/json"
	"sync"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// AgentBlockEvent 是块「创建 / 更新 / 收敛」时的流式事件载荷（供 run_hub 下发 block 事件）：Seq 为块
// 在所属消息内的序号、Kind 为块类型、Payload 为该块的 JSON 载荷（明文，未加密）。前端据 {seq, kind}
// 归并增量到「当前开放块」，并在收敛事件到达时以 payload 覆盖整块。
type AgentBlockEvent struct {
	// Seq 块在所属消息内的单调序号（从 1 起）
	Seq uint64 `json:"seq"`
	// Kind 块类型
	Kind ai.AgentMessageBlockKind `json:"kind"`
	// Payload 块载荷 JSON（明文；落库前由落库层加密）
	Payload json.RawMessage `json:"payload"`
	// Created 该事件是否为块的创建事件：true 表示本次为该块新建（reasoning / content 的创建载荷仅带
	// 首个增量，其后同段增量经增量事件追加），false 表示同块的收敛 / 更新（reasoning / content 的收敛
	// 载荷为该块终态全文）。前端据此区分「置空文本建块后由增量累积」与「以载荷整块覆盖」——从序号 0
	// 回放时若不分此二者，创建载荷的先导增量会与本块已有 / 随后的增量文本重复。
	Created bool `json:"created"`
}

// ---- 块载荷类型（JSON，统一经 crypto 序列化器加密落库） ----

// ReasoningBlockPayload 是 reasoning 块的载荷：一段连续思考文本，开放期内增量累积。
type ReasoningBlockPayload struct {
	// Text 累积的思考文本
	Text string `json:"text"`
}

// ContentBlockPayload 是 content 块的载荷：一段连续正文文本，开放期内增量累积。
type ContentBlockPayload struct {
	// Text 累积的正文文本
	Text string `json:"text"`
}

// ToolCallRecord 是 tool_calls 块内一条工具调用记录：字段对齐 StageEvent（Tool / CallID / Round /
// ElapsedMs / Err / Status）与挂起现场（Arguments / AuditLevel / ExecOwner）的既有定义，承载一条
// 调用从发起到结果的完整事实，供重开会话按块原样回显。
type ToolCallRecord struct {
	// CallID 本次调用的唯一标识（与模型 tool call id 一致），toolResult 据此归并回填
	CallID string `json:"callID"`
	// ToolName 工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本，可能携带凭据，随块加密落库）
	Arguments string `json:"arguments"`
	// AuditLevel 本条调用的审核重要级
	AuditLevel AuditLevel `json:"auditLevel"`
	// ExecOwner 执行归属（client / backend）
	ExecOwner ToolExecOwner `json:"execOwner"`
	// Round 本次调用所属的轮次（从 1 起），同一轮内并发发起的调用落库后据此回显为一组
	Round int `json:"round,omitempty"`
	// Status 调用状态：发起为 start、完成 done、失败 failed（取值同 EvalStageStatus）
	Status EvalStageStatus `json:"status"`
	// ElapsedMs 执行耗时（毫秒，仅 done / failed 有意义）
	ElapsedMs int64 `json:"elapsedMs"`
	// Error 失败原因（仅 failed 有值）
	Error string `json:"error,omitempty"`
	// Result 结果摘要（落库前按 agent.maxToolResultBytes 截断）
	Result string `json:"result,omitempty"`
}

// ToolCallBlockPayload 是 tool_calls 块的载荷：同一轮（round）发起的调用并列为一组。
type ToolCallBlockPayload struct {
	// Calls 该轮内按发起顺序排列的调用记录
	Calls []*ToolCallRecord `json:"calls"`
}

// ApprovalCallRecord 是 approval 块内一条待审批调用记录：以 PendingCall 的既有形状承载调用描述，
// 并附用户决策与执行结果 / 错误。
type ApprovalCallRecord struct {
	PendingCall
	// Decision 用户决策：nil 表示尚无最终决策（仍待审批 / 只读回显），非 nil 时 true 为批准、
	// false 为拒绝（语义同 ToolResult.Approved）
	Decision *bool `json:"decision,omitempty"`
	// Result 执行结果文本（批准且执行成功时有效，落库前按回灌上限截断）
	Result string `json:"result,omitempty"`
	// Error 执行错误文本（前端执行失败时有效）
	Error string `json:"error,omitempty"`
}

// ApprovalBlockPayload 是 approval 块的载荷：本轮待审批调用清单。
type ApprovalBlockPayload struct {
	// Calls 本轮待审批调用，按发起顺序排列
	Calls []*ApprovalCallRecord `json:"calls"`
}

// QuestionnaireBlockPayload 是 questionnaire 块的载荷：一轮 ask_user 问卷的题目与作答。
// 同一回合内模型可多轮提问，每一轮各自成一块（按 BlockSeq 先后排列），仅末轮可为待回答。
type QuestionnaireBlockPayload struct {
	// Questions 本轮问卷的题目列表
	Questions []QuestionnaireQuestion `json:"questions"`
	// Answers 本轮用户的作答（首次挂起时为空，续跑终态化后承载答案）
	Answers []QuestionnaireAnswer `json:"answers,omitempty"`
	// Awaiting 本轮是否待用户回答
	Awaiting bool `json:"awaiting"`
}

// NextStep 是 next_steps 块内一条推荐的下一步：Title 为简短行动标题（不超过 24 个字符），
// Reason 为推荐理由（不超过 64 个字符），Text 为可直接发送给助手的完整诉求，
// Recommended 标记该条是否为最推荐项（同一块内仅首条为 true）。
type NextStep struct {
	// Title 推荐标题（简洁的行动名称，不超过 24 个字符）
	Title string `json:"title,omitempty"`
	// Text 推荐文本（用户可直接作为下一条诉求发送）
	Text string `json:"text"`
	// Reason 推荐理由（为什么推荐该步骤，不超过 64 个字符）
	Reason string `json:"reason,omitempty"`
	// Recommended 是否为最推荐项
	Recommended bool `json:"recommended,omitempty"`
}

// NextStepsBlockPayload 是 next_steps 块的载荷：回答完成后基于整体上下文给出的 1-3 条推荐项，
// 顺序即推荐程度（首条最推荐）。
type NextStepsBlockPayload struct {
	// Steps 推荐的下一步列表
	Steps []NextStep `json:"steps"`
}

// encodeBlockPayload 把块载荷结构编码为 JSON 字节，供落库层经 crypto 序列化器写入加密列。
func encodeBlockPayload(payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

// decodeBlockPayload 把块载荷 JSON 字节解码回结构；raw 为明文载荷（落库层读出的解密值）。
func decodeBlockPayload(raw []byte, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ---- 有序分配与开放块追踪（唯一实现） ----

// blockWrite 是一次待落库的块写入请求：由构建器在块创建 / 变更 / 收敛时产出，交落库层写入
// agent_message_blocks。Created 为 true 表示该块本次为新建（落库层走 AppendBlock），否则为同块的
// 更新（走 UpdateBlock）。Payload 为编码后的 JSON 字节，落库层须经 crypto 序列化器加密。
type blockWrite struct {
	// MessageID 从属的助手消息 ID
	MessageID uint
	// BlockSeq 块在所属消息内的单调序号（从 1 起）
	BlockSeq uint64
	// Kind 块类型
	Kind ai.AgentMessageBlockKind
	// Payload 编码后的块载荷 JSON 字节
	Payload []byte
	// Created 该块本次是否为新建
	Created bool
}

// openBlock 是构建器追踪的一个块：序号与类型在创建时定死，payload 为对应块类型的载荷指针。
type openBlock struct {
	// seq 块在所属消息内的单调序号
	seq uint64
	// kind 块类型
	kind ai.AgentMessageBlockKind
	// payload 该块类型的载荷指针（*ReasoningBlockPayload / *ToolCallBlockPayload / 等）
	payload any
	// round 仅 tool_calls：该块所属的调用轮次
	round int
}

// callRef 是 tool_calls 块内一条调用记录与其所属块的引用：toolResult 据此把结果回填到记录，并对
// 该块发出一次更新写入（即使该块已收敛为终态）。
type callRef struct {
	// record 调用记录指针
	record *ToolCallRecord
	// block 该记录所属的 tool_calls 块
	block *openBlock
}

// blockBuilder 是助手回答块序列的状态机：维护「当前开放块」，在内容真实产生的时点分配 BlockSeq 并
// 产出待落库写入集。同一套规则同时服务于实时流与落库，故这里一处是块时序的唯一来源。
//
// 写入策略（避免每 token 一次 UPDATE）：思考 / 正文增量只累积到开放块，**不**产出写入；仅在块创建
// （Created=true）与块收敛（Created=false）各写一次。discrete 事件（工具调用发起 / 结果回填 / 授权
// 建块 / 问卷建块）各自产出一次写入。
type blockBuilder struct {
	// mu 保护构建器的全部可变字段：构建器由图内节点（工具调用 / 思考与正文增量）与流包装层
	// （收敛、落库）在各自 goroutine 中并发写入，故所有对外方法都在锁内执行。
	mu sync.Mutex
	// messageID 从属的助手消息 ID（首次落库前为 0，拿到消息行 ID 后经 SetMessageID 更新）
	messageID uint
	// nextSeq 下一个待分配的 BlockSeq（从 1 起，同一消息内单调）
	nextSeq uint64
	// maxBytes 工具结果落库前的字节上限（取自 agent.maxToolResultBytes；非正数不截断）
	maxBytes int
	// open 当前开放块；nil 表示无开放块（尚未开块或已收敛）
	open *openBlock
	// calls 调用记录索引（callID → 记录 + 所属块），toolResult 据此归并回填
	calls map[string]callRef
	// writes 待落库的写入集，由无参 DrainWrites 取出后清空
	writes []blockWrite
	// events 待下发的块事件集（创建 / 更新 / 收敛各一次），由无参 DrainEvents 取出后清空
	events []AgentBlockEvent
	// closed 是否已收敛；收敛后不再新增 / 变更任何块
	closed bool
}

// newBlockBuilder 构造一个块构建器；messageID 为从属的助手消息 ID（续跑时即锚点消息 ID），
// maxResultBytes 为工具结果落库前的字节上限（非正数不截断）。
func newBlockBuilder(messageID uint, maxResultBytes int) *blockBuilder {
	return &blockBuilder{
		messageID: messageID,
		nextSeq:   1,
		maxBytes:  maxResultBytes,
		calls:     make(map[string]callRef),
	}
}

// SetMessageID 更新构建器从属的助手消息 ID：运行中途首次落库拿到消息行 ID 后调用，使其后新产生的
// 块带着正确的 MessageID 落库。
func (b *blockBuilder) SetMessageID(id uint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messageID = id
}

// SeedNextSeq 把下一个待分配序号抬到 next：续跑时载入锚点已有块的最大 block_seq + 1，使本次运行新产生
// 的块接续其后的序号（避免与锚点已有块的 (message_id, block_seq) 唯一约束冲突）。
func (b *blockBuilder) SeedNextSeq(next uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if next > b.nextSeq {
		b.nextSeq = next
	}
}

// beginNew 关闭当前开放块（若存在，产出其收敛写入）并分配一个新块置为开放，返回新块；构建器已收敛
// 时返回 nil。
func (b *blockBuilder) beginNew(kind ai.AgentMessageBlockKind, payload any) *openBlock {
	if b.closed {
		return nil
	}
	b.closeOpen()
	block := &openBlock{seq: b.nextSeq, kind: kind, payload: payload}
	b.nextSeq++
	b.open = block
	return block
}

// closeOpen 收敛当前开放块：产出一次终态写入（Created=false），并把「当前开放块」清空。反复调用为
// 幂等（无开放块即无操作）。
func (b *blockBuilder) closeOpen() {
	block := b.open
	if block == nil {
		return
	}
	b.open = nil
	b.emit(block, false)
}

// emit 把某块的当前载荷编码为 JSON 并追加一次写入；created 表示该次写入是否为「新建」。
// 各块载荷均为可 JSON 序列化的结构，编码不会失败；万一失败（编程缺陷）则跳过该次落库并保持事件流
// 不中断——本包 Agent 无 logger 依赖，故不记录日志。
func (b *blockBuilder) emit(block *openBlock, created bool) {
	if block == nil {
		return
	}
	data, err := encodeBlockPayload(block.payload)
	if err != nil {
		return
	}
	b.writes = append(b.writes, blockWrite{
		MessageID: b.messageID,
		BlockSeq:  block.seq,
		Kind:      block.kind,
		Payload:   data,
		Created:   created,
	})
	b.events = append(b.events, AgentBlockEvent{
		Seq:     block.seq,
		Kind:    block.kind,
		Payload: data,
		Created: created,
	})
}

// AddReasoning 追加一段思考增量：当前开放块不是 reasoning 时新建一个 reasoning 块（分配 BlockSeq 并
// 产出创建写入），否则把文本追加进当前开放的 reasoning 块（不产出写入，避免每 token 一次 UPDATE）。
// 构建器已收敛或增量为空时无操作。
func (b *blockBuilder) AddReasoning(delta string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || delta == "" {
		return
	}
	if b.open != nil && b.open.kind == ai.AgentMessageBlockKindReasoning {
		b.open.payload.(*ReasoningBlockPayload).Text += delta
		return
	}
	block := b.beginNew(ai.AgentMessageBlockKindReasoning, &ReasoningBlockPayload{Text: delta})
	b.emit(block, true)
}

// AddContent 追加一段正文增量：当前开放块不是 content 时新建一个 content 块（分配 BlockSeq 并产出
// 创建写入），否则把文本追加进当前开放的 content 块（不产出写入）。构建器已收敛或增量为空时无操作。
func (b *blockBuilder) AddContent(delta string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || delta == "" {
		return
	}
	if b.open != nil && b.open.kind == ai.AgentMessageBlockKindContent {
		b.open.payload.(*ContentBlockPayload).Text += delta
		return
	}
	block := b.beginNew(ai.AgentMessageBlockKindContent, &ContentBlockPayload{Text: delta})
	b.emit(block, true)
}

// AddToolCall 登记一次工具调用的发起（stage=toolCall）：按 ev.Round 归入同一 tool_calls 块——当前
// 开放块即同轮的 tool_calls 块时追加记录，否则新建一个 tool_calls 块（同轮并发调用同块，先到先建块）。
// arguments / auditLevel / execOwner 由调用方从工具调用现场补充（StageEvent 未携带）。构建器已收敛时
// 无操作。
func (b *blockBuilder) AddToolCall(ev StageEvent, arguments string, auditLevel AuditLevel, execOwner ToolExecOwner) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	record := &ToolCallRecord{
		CallID:     ev.CallID,
		ToolName:   ev.Tool,
		Arguments:  arguments,
		AuditLevel: auditLevel,
		ExecOwner:  execOwner,
		Round:      ev.Round,
		Status:     EvalStatusStart,
	}
	var block *openBlock
	if b.open != nil && b.open.kind == ai.AgentMessageBlockKindToolCalls && b.open.round == ev.Round {
		block = b.open
		payload := block.payload.(*ToolCallBlockPayload)
		payload.Calls = append(payload.Calls, record)
		b.emit(block, false)
	} else {
		block = b.beginNew(ai.AgentMessageBlockKindToolCalls, &ToolCallBlockPayload{Calls: []*ToolCallRecord{record}})
		if block != nil {
			block.round = ev.Round
		}
		b.emit(block, true)
	}
	if block != nil && ev.CallID != "" {
		b.calls[ev.CallID] = callRef{record: record, block: block}
	}
}

// AddToolResult 归并一次工具结果（stage=toolResult）：按 ev.CallID 找回该调用在块内的记录，回填状态
// （done / failed）、耗时、错误与结果摘要（按回灌字节上限截断后再落库），并对其所属块产出一次更新
// 写入。callID 缺失或未登记（非本构建器发起）时无操作。
func (b *blockBuilder) AddToolResult(ev StageEvent, result string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	ref, ok := b.calls[ev.CallID]
	if !ok || ev.CallID == "" {
		return
	}
	ref.record.Status = ev.Status
	ref.record.ElapsedMs = ev.ElapsedMs
	ref.record.Error = ev.Err
	if result != "" {
		ref.record.Result = truncateBytes(result, b.maxBytes)
	}
	b.emit(ref.block, false)
}

// AddApproval 创建 approval 块，承载本轮待审批的客户端 / backend 工具调用清单（每条调用以 PendingCall
// 的既有形状落库，用户决策初始为「尚无最终决策」）。每一轮授权各成一块（按 BlockSeq 先后排列），故
// 此处总是新起一块。构建器已收敛时无操作。
func (b *blockBuilder) AddApproval(calls []PendingCall) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	records := make([]*ApprovalCallRecord, 0, len(calls))
	for _, call := range calls {
		records = append(records, &ApprovalCallRecord{PendingCall: call})
	}
	block := b.beginNew(ai.AgentMessageBlockKindApproval, &ApprovalBlockPayload{Calls: records})
	b.emit(block, true)
}

// AddQuestionnaire 创建 questionnaire 块，承载本轮 ask_user 的题目（作答与「是否待回答」随续跑落库）。
// 每一轮提问各成一块（按 BlockSeq 先后排列），故此处总是新起一块。构建器已收敛时无操作。
func (b *blockBuilder) AddQuestionnaire(questions []QuestionnaireQuestion) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	block := b.beginNew(ai.AgentMessageBlockKindQuestionnaire, &QuestionnaireBlockPayload{
		Questions: questions,
		Awaiting:  true,
	})
	b.emit(block, true)
}

// AddNextSteps 创建 next_steps 块，承载回答完成后基于整体上下文给出的 1-3 条推荐的下一步。
// 该块总是落在最终正文块之后（beginNew 先收敛当前开放的 content 块），故每轮回答至多一块。
// 构建器已收敛或列表为空时无操作。
func (b *blockBuilder) AddNextSteps(steps []NextStep) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(steps) == 0 {
		return
	}
	block := b.beginNew(ai.AgentMessageBlockKindNextSteps, &NextStepsBlockPayload{Steps: steps})
	b.emit(block, true)
}

// Close 收敛构建器：把尚在开放的块收敛为终态并产出其写入，此后不再新增 / 变更任何块（挂起 / 终态化 /
// 中止 / 失败时调用）。反复调用为幂等。
func (b *blockBuilder) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closeOpen()
	b.closed = true
}

// DrainWrites 取出并清空待落库写入集：调用方据此把「本次新产生的块」交给落库层（新建走 AppendBlock，
// 更新走 UpdateBlock）。无待写入时返回 nil。
func (b *blockBuilder) DrainWrites() []blockWrite {
	b.mu.Lock()
	defer b.mu.Unlock()
	writes := b.writes
	b.writes = nil
	return writes
}

// DrainEvents 取出并清空待下发的块事件集（创建 / 更新 / 收敛）：调用方据此把「本次新产生的块事件」
// 随流下发（run_hub 的 block 事件）。无待下发时返回 nil。
func (b *blockBuilder) DrainEvents() []AgentBlockEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := b.events
	b.events = nil
	return events
}

// OpenSeq 返回当前开放块的 BlockSeq；无开放块时返回 0。供 run_hub 在增量事件中指明「当前开放块」使
// 前端把 delta / reasoning 增量追加进同类块。
func (b *blockBuilder) OpenSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.open == nil {
		return 0
	}
	return b.open.seq
}

// OpenKind 返回当前开放块的类型；无开放块时返回空串。
func (b *blockBuilder) OpenKind() ai.AgentMessageBlockKind {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.open == nil {
		return ""
	}
	return b.open.kind
}
