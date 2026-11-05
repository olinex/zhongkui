package contract

// EmbedUsage 是一次或多次 embedding 调用的 token 用量累计：由能力增强后的向量化链路
// 逐次汇总，供应商未返回用量时为零值。
type EmbedUsage struct {
	// PromptTokens 提示（输入）token 数累计。
	PromptTokens int
	// CompletionTokens 补全（输出）token 数累计。
	CompletionTokens int
	// TotalTokens 总 token 数累计。
	TotalTokens int
}

// 同步进度上报的动作标识：取值与 SyncProgressItem.Action 一一对应。
const (
	// SyncActionCreate 新增条目。
	SyncActionCreate = "create"
	// SyncActionUpdate 更新条目。
	SyncActionUpdate = "update"
	// SyncActionDelete 删除条目。
	SyncActionDelete = "delete"
	// SyncActionReindex 重新索引条目。
	SyncActionReindex = "reindex"
	// SyncActionMemory 长期记忆批量重嵌入。
	SyncActionMemory = "memory"
)

// SyncProgressItem 是一条同步进度明细：由各同步链路在单个条目（或记忆批）完成后上报。
type SyncProgressItem struct {
	// Title 条目名；记忆按批上报时为空。
	Title string
	// Action 动作标识，取 SyncAction* 常量之一。
	Action string
	// Count 本次完成条数；文档恒为 1，记忆为批大小。
	Count int
	// ElapsedMs 本次处理耗时（毫秒）。
	ElapsedMs int64
	// Usage 本次处理累计的 embedding token 用量。
	Usage EmbedUsage
}

// SyncReporter 是同步进度上报接口：调用顺序为一次 Start、每条目一次 Step、一次 Finish。
// 调用方传 nil 表示不上报（内部经 reportStart/reportStep/reportFinish 判空转发）。
type SyncReporter interface {
	// Start 在同步开始前上报本次待处理总条数。
	Start(total int)
	// Step 在单个条目（或记忆批）完成后上报一条进度明细。
	Step(item SyncProgressItem)
	// Finish 在同步结束时上报（无论成功或提前失败均调用一次）。
	Finish()
}
