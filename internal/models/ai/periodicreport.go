// 本文件定义周期性报告计划的数据模型：报告计划（PeriodicReportPlan）描述一个按 cron 周期执行的技能
// 任务及其实参，报告运行记录（PeriodicReportRun）记录每次触发的时间点、状态与产出对象键，报告计划关注
// 关系（PeriodicReportPlanWatch）记录用户对计划的关注。三张表按命名空间隔离，计划以 cron 表达式驱动，
// 运行记录以 (plan_id, trigger_at) 唯一约束保证同一计划同一触发点只落一条记录。
package ai

import (
	// 标准库
	"time"

	// 第三方
	"gorm.io/datatypes"
	"gorm.io/gorm"

	// 项目内部
	coremodel "zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// PeriodicReportRunStatus 是报告运行记录状态的标识：运行中（running）、成功（success）与失败（failed）。
// 取值与 periodic_report_runs.status 列的取值一致，取值空间在包内 init 注册为枚举类 periodicReportRunStatus。
type PeriodicReportRunStatus string

// 报告运行记录状态常量。
const (
	// PeriodicReportRunStatusRunning 运行中：报告任务已触发，正在取数与生成
	PeriodicReportRunStatusRunning PeriodicReportRunStatus = "running"
	// PeriodicReportRunStatusSuccess 成功：报告已生成并落对象存储
	PeriodicReportRunStatusSuccess PeriodicReportRunStatus = "success"
	// PeriodicReportRunStatusFailed 失败：报告生成失败，失败原因见 PeriodicReportRun.Error
	PeriodicReportRunStatusFailed PeriodicReportRunStatus = "failed"
)

// PeriodicReportPlan 表示一个周期性报告计划：按 cron 表达式周期性执行指定技能，携带渲染输入所需的
// 实参，并按保留天数清理历史报告。对应表为 periodic_report_plans。
type PeriodicReportPlan struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace coremodel.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32);index:idx_periodic_report_plans_namespace_name"`
	// Name 报告计划名称
	Name string `gorm:"type:VARCHAR(128);not null"`
	// Description 报告计划说明，可为空
	Description string `gorm:"type:VARCHAR(512)"`
	// SkillID 计划使用的技能 ID（agent_skills.id，受外键约束）
	SkillID uint `gorm:"type:BIGINT;not null"`
	// CronSpec 计划执行的 cron 表达式
	CronSpec string `gorm:"type:VARCHAR(64);not null"`
	// InputValues 技能输入字段的实参（JSON 对象，键为字段名），无实参时为空对象
	InputValues datatypes.JSON `gorm:"type:JSONB;not null;default:'{}'"`
	// Enabled 计划是否启用，默认 true；停用的计划不再触发
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
	// UserID 计划属主用户 ID（users.id），用于跨用户隔离
	UserID uint `gorm:"type:BIGINT;not null"`
	// RetentionDays 报告保留天数，默认 90
	RetentionDays int `gorm:"type:INT;not null;default:90"`
	// ExpiresAt 计划失效时间，null 表示长期有效
	ExpiresAt *time.Time `gorm:"type:TIMESTAMPTZ"`
	// NextRunAt 下一次计划执行时间，NULL 表示待分发器计算
	NextRunAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}

// PeriodicReportRun 表示一次报告运行的记录：由计划在 cron 触发点创建，记录触发时间、运行状态与产出
// （对象存储键、标题、大小）以及失败原因。对应表为 periodic_report_runs。
type PeriodicReportRun struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// PlanID 所属报告计划 ID
	PlanID uint `gorm:"type:BIGINT;not null;index:idx_periodic_report_runs_plan_id;uniqueIndex:uq_periodic_report_runs_plan_id_trigger_at,priority:1,where:deleted_at IS NULL"`
	// TriggerAt 计划触发时间点；同一计划同一触发点唯一（未删除行范围内）
	TriggerAt time.Time `gorm:"type:TIMESTAMPTZ;not null;uniqueIndex:uq_periodic_report_runs_plan_id_trigger_at,priority:2,where:deleted_at IS NULL"`
	// Status 运行状态，取值来自枚举类 periodicReportRunStatus（running/success/failed）
	Status PeriodicReportRunStatus `gorm:"type:VARCHAR(16);not null;index:idx_periodic_report_runs_status"`
	// ObjectKey 生成的报告在对象存储中的键（经 s3crypto 序列化器封装），无产出时为空
	ObjectKey serializer.S3Crypto `gorm:"type:VARCHAR(512);serializer:s3crypto" s3crypto:"offset=38549,length=32"`
	// Title 生成报告的标题
	Title string `gorm:"type:VARCHAR(256)"`
	// SizeBytes 生成的报告字节大小
	SizeBytes int64 `gorm:"type:BIGINT"`
	// Error 运行失败原因，成功时为空
	Error string `gorm:"type:VARCHAR(2048)"`
	// SessionID 本次运行专属的 Agent 会话 ID（agent_sessions.id 外键，会话删除时置空）；
	// nil 表示尚未创建。每次运行独立建会话，使同一计划的多份报告可以并发生成。
	SessionID *uint `gorm:"type:BIGINT"`
	// StartedAt 实际开始执行时间，null 表示尚未开始
	StartedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
	// FinishedAt 执行结束时间，null 表示尚未结束
	FinishedAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}

// ObjectCryptoKey 派生 object_key 字段的 S3 加密子密钥。
// 模型无 json 标签，故按 Go 字段名匹配 s3crypto 标签。
func (r *PeriodicReportRun) ObjectCryptoKey() ([]byte, error) {
	return crypto.S3CryptoKeyForField(r, "ObjectKey", "")
}

// PeriodicReportPlanWatch 表示用户对某个报告计划的关注关系：关注后该用户的报告中心展示该计划的运行
// 记录。对应表为 periodic_report_plan_watches。
type PeriodicReportPlanWatch struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// UserID 关注者用户 ID（users.id）
	UserID uint `gorm:"type:BIGINT;not null;uniqueIndex:uq_periodic_report_plan_watches_user_id_namespace_name_plan_id,where:deleted_at IS NULL"`
	// NamespaceName 计划所属命名空间名称，用户隔离维度
	NamespaceName string `gorm:"type:VARCHAR(32);not null;uniqueIndex:uq_periodic_report_plan_watches_user_id_namespace_name_plan_id,where:deleted_at IS NULL"`
	// PlanID 被关注的报告计划 ID
	PlanID uint `gorm:"type:BIGINT;not null;uniqueIndex:uq_periodic_report_plan_watches_user_id_namespace_name_plan_id,where:deleted_at IS NULL"`
}

// periodicReportRunStatusValues 是 periodicReportRunStatus 枚举类的取值空间，声明顺序即枚举类条目顺序与前端展示顺序。
var periodicReportRunStatusValues = []PeriodicReportRunStatus{
	PeriodicReportRunStatusRunning,
	PeriodicReportRunStatusSuccess,
	PeriodicReportRunStatusFailed,
}

// init 注册枚举类 periodicReportRunStatus：取值空间固定，条目在注册时快照，成员仍只在本文件一处声明。
func init() {
	enum.Register(enum.Type{Name: "periodicReportRunStatus", Entries: enum.EntriesOf(periodicReportRunStatusValues)})
}
