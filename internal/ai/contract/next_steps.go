package contract

// NextStep 的字段长度上限（rune）：与结构体字段的 binding 标签一致，供生成侧截断与校验共用。
const (
	// NextStepTitleMaxLength 推荐标题的最大长度
	NextStepTitleMaxLength = 24
	// NextStepTextMaxLength 推荐文本的最大长度
	NextStepTextMaxLength = 200
	// NextStepReasonMaxLength 推荐理由的最大长度
	NextStepReasonMaxLength = 64
)

// NextStep 是推荐下一步的一条候选：Title 为简短行动标题（不超过 24 个字符），Reason 为推荐理由
// （不超过 64 个字符），Text 为可直接发送给助手的完整诉求（不超过 200 个字符），Recommended 标记
// 该条是否为最推荐项（同一组内仅首条为 true）。长度约束由 binding 标签声明，经 validator.Check 校验。
type NextStep struct {
	// Title 推荐标题（简洁的行动名称，不超过 24 个字符；可为空）
	Title string `json:"title,omitempty" binding:"omitempty,max=24"`
	// Text 推荐文本（用户可直接作为下一条诉求发送，必填且不超过 200 个字符）
	Text string `json:"text" binding:"required,max=200"`
	// Reason 推荐理由（为什么推荐该步骤，必填且不超过 64 个字符）
	Reason string `json:"reason,omitempty" binding:"required,max=64"`
	// Recommended 是否为最推荐项
	Recommended bool `json:"recommended,omitempty"`
}
