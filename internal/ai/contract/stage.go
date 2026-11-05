package contract

// EvalStage 是评估与工具调用流程的阶段标识（检索 + 4 次评估调用 + 工具调用/结果）。
type EvalStage string

// 评估阶段常量：SSE 流上 stage / delta 事件的 stage 字段取值。
const (
	// EvalStageRetrieve 混合检索阶段
	EvalStageRetrieve EvalStage = "retrieve"
	// EvalStageAnswer 基于上下文生成回答阶段
	EvalStageAnswer EvalStage = "answer"
	// EvalStageChunkRelevance 分块相关性判定阶段
	EvalStageChunkRelevance EvalStage = "chunkRelevance"
	// EvalStageClaims 答案声明抽取与支撑判定阶段
	EvalStageClaims EvalStage = "claims"
	// EvalStageReverseQuestions 反推问题生成阶段
	EvalStageReverseQuestions EvalStage = "reverseQuestions"
	// EvalStageSuggestions 改进建议生成阶段（依据检索结果、指标与实际生效的检索参数给出）
	EvalStageSuggestions EvalStage = "suggestions"
	// EvalStageToolCall 工具调用阶段（Agent 准备/发起一次工具调用）
	EvalStageToolCall EvalStage = "toolCall"
	// EvalStageToolResult 工具结果阶段（Agent 收到一次工具执行的结果）
	EvalStageToolResult EvalStage = "toolResult"
)

// EvalStageStatus 是阶段状态标识；带 Delta 的增量事件不携带状态。
type EvalStageStatus string

// 阶段状态常量。
const (
	// EvalStatusStart 阶段开始
	EvalStatusStart EvalStageStatus = "start"
	// EvalStatusDone 阶段完成
	EvalStatusDone EvalStageStatus = "done"
	// EvalStatusFailed 阶段失败（已降级为 warning）
	EvalStatusFailed EvalStageStatus = "failed"
)

// CallUsage 是一次大模型调用的 token 用量。
type CallUsage struct {
	// PromptTokens 输入 token 数
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 输出 token 数
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 合计 token 数
	TotalTokens int `json:"totalTokens"`
}

// CallMeta 是一次大模型调用的元信息（阶段、模型名、耗时与 token 用量）。
type CallMeta struct {
	// Stage 所属评估阶段
	Stage EvalStage `json:"stage"`
	// Model 调用的模型名
	Model string `json:"model"`
	// ElapsedMs 调用耗时（毫秒）
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（模型未返回用量时为零值）
	Usage CallUsage `json:"usage"`
}

// StageEvent 是阶段上报事件：Status 非空时为阶段状态事件（start/done/failed），
// Delta 非空时为该阶段的增量输出事件。
type StageEvent struct {
	// Stage 阶段标识
	Stage EvalStage `json:"stage"`
	// Model 该阶段调用的对话模型名（评估链路各阶段共用同一档位模型；检索等非对话阶段为空）
	Model string `json:"model,omitempty"`
	// Status 阶段状态（start/done/failed）；增量事件为空
	Status EvalStageStatus `json:"status,omitempty"`
	// Delta 该阶段的模型增量原始输出；状态事件为空
	Delta string `json:"delta,omitempty"`
	// ElapsedMs 阶段耗时（毫秒，仅 done/failed 有意义）
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（仅 done 有意义，模型未返回用量时为 nil）
	Usage *CallUsage `json:"usage,omitempty"`
	// Err 阶段失败原因（仅 failed 有值）
	Err string `json:"error,omitempty"`
	// Tool 本轮事件的工具名（仅 toolCall / toolResult 阶段有值，其他阶段为空）
	Tool string `json:"tool,omitempty"`
	// CallID 本次工具调用的唯一标识（仅 toolCall / toolResult 阶段有值），供前端把同一次调用的
	// 「发起」与「结果」归并到同一行；缺失时（如达到轮数上限的整体失败）为空。
	CallID string `json:"callID,omitempty"`
	// Round 本次工具调用所属的轮次（从 1 起，仅 toolCall / toolResult 阶段有值），供前端把同一轮
	// 内并发发起的多次调用聚为一组展示（一次思考 → N 次并行调用）。
	Round int `json:"round"`
}

// EvalReporter 是评估阶段事件的实时上报回调：返回错误（如 SSE 写失败/客户端断开）
// 会中止后续阶段，避免产生无谓的大模型调用开销；为 nil 时不上报。
type EvalReporter = func(StageEvent) error
