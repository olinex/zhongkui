package contract

import (
	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// MemoryHit 是一条检索到的长期记忆。
type MemoryHit struct {
	// ID 记忆 ID
	ID uint
	// Kind 记忆类别
	Kind modelsai.MemoryKind
	// Title 记忆标题
	Title string
	// Content 记忆正文（按 memory.maxItemBytes 截断）
	Content string
	// Score 融合后的相关度分数
	Score float64
}

// CompactResult 是一次会话压缩的产物：刷新后的摘要、成功落库的长期记忆条数与不影响主结果的
// 告警（长期记忆抽取/落库失败、记忆开关关闭等）。
type CompactResult struct {
	// Summary 刷新后的会话摘要
	Summary string
	// Extracted 成功落库的长期记忆条数
	Extracted int
	// Warnings 非致命告警（英文）
	Warnings []string
	// TokensAfter 本次压缩后上下文占用的即时估算：压缩前占用减去被折叠内容占用、再加上新摘要占用，
	// 即「被折叠内容换成新摘要」后的上下文占用估算，随压缩检查点一并落库并供压缩完成即刻展示；
	// 无法估算时为 0。
	TokensAfter int
}

// CompactionMetrics 携带一次摘要推进（CompactStream / Compact 产出）的展示度量，随检查点一并
// 落库。三种推进原因（rolling / auto / manual）都必须填入真实值：取值与是否展示无关，是否回显交由
// 读取侧按检查点的 reason 决定。
type CompactionMetrics struct {
	// TokensBefore 推进动作发生前的会话上下文占用 token
	TokensBefore int
	// FoldedCount 折叠进摘要的消息条数
	FoldedCount int
	// TokensAfter 推进后上下文占用的即时估算 token：调用方入参留 0，由 CompactStream 在摘要产出后
	// 算得并随检查点落库；静默滚动不产出估算，留 0。
	TokensAfter int
}
