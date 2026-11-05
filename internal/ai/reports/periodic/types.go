package periodic

import (
	// 标准库
	"encoding/json"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 报告计划的调度参数常量。
const (
	// TaskHandler 报告计划实际投递的 asynq 任务类型
	TaskHandler = "periodic-report:plan:run"
)

// 报告生成任务的执行参数常量。
const (
	// RunQueue 报告生成任务投递的队列名称
	RunQueue = "default"
	// RunTimeout 单次报告生成任务的执行超时时间
	RunTimeout = 30 * time.Minute
	// RunMaxRetry 单次报告生成任务的最大重试次数
	RunMaxRetry = 0
	// RunRetention 报告生成任务在 asynq 中的保留时长
	RunRetention = 168 * time.Hour
)

// 报告领域层的错误哨兵，供上层 errors.Is 判断。
var (
	// ErrPlanNotFound 报告计划不存在或不属于给定命名空间
	ErrPlanNotFound = errors.New("periodic-report: plan not found")
	// ErrRunNotFound 报告运行记录不存在或不属于给定命名空间
	ErrRunNotFound = errors.New("periodic-report: run not found")
	// ErrSkillNotEligible 技能不满足周期执行资格（非自定义、停用、不可周期执行、固定参数缺少
	// 合法时间窗口或输入字段缺少唯一 nowTime）
	ErrSkillNotEligible = errors.New("periodic-report: skill is not eligible for periodic report")
	// ErrScheduleInvalid cron 表达式无法解析
	ErrScheduleInvalid = errors.New("periodic-report: invalid cron schedule")
	// ErrInputMissing 技能必填输入缺失
	ErrInputMissing = errors.New("periodic-report: missing required skill input")
	// ErrInputInvalid 计划偏置输入不合法：键不在技能固定参数与输入字段范围内、覆盖 nowTime 或值
	// 的 JSON 类型与技能字段不一致
	ErrInputInvalid = errors.New("periodic-report: invalid skill input value")
	// ErrNextRunAtChanged 分发器推进 next_run_at 的 CAS 未命中（已被其它实例推进）
	ErrNextRunAtChanged = errors.New("periodic-report: next run time has changed")
)

// PlanInput 是创建或更新报告计划时的输入内容，字段的必填、长度与格式约束由视图层 binding
// 统一负责，本类型只承载已校验的值。
type PlanInput struct {
	// Name 报告计划名称
	Name string
	// Description 报告计划说明
	Description string
	// SkillID 计划使用的技能 ID（agent_skills.id）
	SkillID uint
	// CronSpec 计划执行的 cron 表达式
	CronSpec string
	// InputValues 技能输入字段的实参（键为字段名），值保留原始 JSON 类型（数字/布尔/字符串/
	// 对象/数组），既可作为输入字段偏置，也可覆盖同名固定参数
	InputValues map[string]json.RawMessage
	// RetentionDays 报告保留天数，由调用方（视图层或命令层）完成默认值填充与范围校验后传入
	RetentionDays int
	// ExpiresAt 计划失效时间，nil 表示长期有效
	ExpiresAt *time.Time
}
