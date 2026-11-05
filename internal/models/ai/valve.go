// 本文件声明 Agent 会话的审核阀门（ReviewValve）取值空间：前端为用户提供的会话级三档开关，
// 取值 each / auto / none，与 agent_sessions.review_valve 列的取值一致。阀门与本会话内所有
// client 工具调用的审核重要级（auditLevel）合成，决定这一次调用是否需人工审核。取值的唯一
// 来源是本文件内的类型化常量，取值序列在 enum.go 复用为枚举类 reviewValve 的条目来源。
package ai

// ReviewValve 是 Agent 会话的审核阀门标识：会话级持久化的三档开关，取值 each / auto / none，
// 依次对应「逐一审核 / 自动审核 / 无需审核」，与 agent_sessions.review_valve 列的取值一致。
// 阀门与该会话内 client 工具调用的审核重要级合成后决定调用是否需人工审核。
type ReviewValve string

// 审核阀门常量。
const (
	// ReviewValveEach 逐一审核：本会话内任何重要级的调用均需人工审核
	ReviewValveEach ReviewValve = "each"
	// ReviewValveAuto 自动审核：仅 must / suggest 重要级的调用需人工审核，其余自动执行
	ReviewValveAuto ReviewValve = "auto"
	// ReviewValveNone 无需审核：仅 must 重要级的调用需人工审核，其余自动执行
	ReviewValveNone ReviewValve = "none"
)

// DefaultReviewValve 出厂默认阀门：会话未创建或未记录阀门时的默认档位，取「逐一审核」以保持
// 引入本特性前「一律人工审核」的既有行为，与 agent_sessions.review_valve 列的默认值一致。
const DefaultReviewValve = ReviewValveEach

// reviewValveValues 是 reviewValve 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var reviewValveValues = []ReviewValve{ReviewValveEach, ReviewValveAuto, ReviewValveNone}
