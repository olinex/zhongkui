package ai

import (
	// 第三方

	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentSession 表示一个 AI 助手会话，聚合同一话题下的多轮问答消息
// （AgentMessage），并记录会话缺省档位与状态。对应表为 agent_sessions。
type AgentSession struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `gorm:"type:BIGINT;not null"`
	// Title 会话标题，默认取首条用户消息的摘要，可由用户重命名
	Title string `gorm:"type:VARCHAR(256)"`
	// TargetModel 会话缺省档位（列名沿用历史命名）：取值 flash/standard/advanced，空表示按
	// 参数 agent.defaultSlot 解析；生成请求未显式指定档位时使用该档位
	TargetModel Slot `gorm:"type:VARCHAR(255)"`
	// Status 会话状态（如 active/archived），默认 active
	Status SessionStatus `gorm:"type:VARCHAR(16)"`
	// AgentKind 会话所属 Agent 类型（如 knowledge/rego），记忆归属与检索据此隔离
	AgentKind AgentKind `gorm:"type:VARCHAR(32);default:knowledge"`
	// ReviewValve 会话级审核阀门（each/auto/none），与本会话内 client 工具调用评估出的审核
	// 重要级合成，决定该调用是否需人工审核；空表示按列默认值 each（逐一审核）
	ReviewValve ReviewValve `gorm:"type:VARCHAR(16);default:each"`
	// Summary 会话滚动摘要，压缩已折叠进其中的历史消息，无摘要时为空
	// （正文含用户原文，经 crypto 序列化器加密落库）
	Summary *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12810,length=32"`
	// MessageSeq 会话内已分配的最大消息序号（seq），供消息 seq 原子分配
	MessageSeq uint `gorm:"type:BIGINT;not null"`
	// SummaryUptoSeq 滚动摘要已覆盖的最后一条消息的会话内序号（seq），0 表示尚未摘要任何消息
	SummaryUptoSeq uint `gorm:"type:BIGINT"`
}

// AgentMessage 表示 AI 助手会话中的一条消息，记录消息角色（Role）、正文
// （Content），以及助手回复的草稿载荷（DraftJSON）、引用来源列表（CitationsJSON）
// 与思考过程（Reasoning）。对应表为 agent_messages。
type AgentMessage struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// SessionID 所属会话 ID，会话删除时消息级联删除
	SessionID uint `gorm:"type:BIGINT"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `gorm:"type:BIGINT;not null"`
	// Seq 消息在所属会话内的单调递增序号（从 1 起），与 SessionID、UserID 共同唯一，
	// 作为消息读取/游标/摘要的一致排序键
	Seq uint64 `gorm:"type:BIGINT;not null"`
	// Role 消息角色：user/assistant/system 等
	Role MessageRole `gorm:"type:VARCHAR(16)"`
	// Content 消息正文内容（含用户原文与模型输出，经 crypto 序列化器加密落库）
	Content *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12811,length=32"`
	// DraftJSON 助手回复的草稿载荷（JSON，如工具调用的中间状态），可空，经 crypto 序列化器加密落库
	DraftJSON *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12812,length=32"`
	// CitationsJSON 引用来源列表（JSON），可空，经 crypto 序列化器加密落库（引用携带知识库正文）
	CitationsJSON *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12813,length=32"`
	// Reasoning 助手回复的思考过程，加密落库、可空（模型未返回思考内容时为 NULL）
	Reasoning *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12819,length=32"`
	// QuestionnaireJSON 本回合 ask_user 问卷载荷（题目与用户答案，JSON），可空，经 crypto 序列化器
	// 加密落库。问卷挂起时写入锚点消息行，续跑终态化后保留以支撑历史回显与回退重答。
	QuestionnaireJSON *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12820,length=32"`
	// PromptTokens 本轮助手回答全部模型调用的输入 token 累加（明文数值，用户消息恒为 0）
	PromptTokens int `gorm:"type:INT"`
	// CompletionTokens 本轮助手回答全部模型调用的输出 token 累加（明文数值，用户消息恒为 0）
	CompletionTokens int `gorm:"type:INT"`
	// TotalTokens 本轮助手回答全部模型调用的 token 总量累加（明文数值，用户消息恒为 0）
	TotalTokens int `gorm:"type:INT"`
	// ElapsedMs 本轮助手回答的总耗时（毫秒，明文数值，用户消息恒为 0）
	ElapsedMs int64 `gorm:"type:BIGINT"`
	// Phase 本条助手消息所属运行的生命周期终态阶段（AgentRunPhase：done/failed/cancelled，
	// 以及问卷挂起期的 suspended），空表示尚未终结（运行在途的阶段性落库行）。运行被中断
	// （超时 / 报错 / 取消）时据此表达「本轮未正常结束」，由渲染层按阶段展示提示——不再向正文
	// 拼接提示文案（提示语若混入正文，会被喂回模型污染下一轮上下文）；用户消息恒为空。
	Phase AgentRunPhase `gorm:"type:VARCHAR(16)"`
}

// AgentSessionCompaction 表示一次滚动摘要推进（静默滚动或压缩）留下的压缩检查点：记录推进后的
// 上下文起点（UptoSeq）与该时刻的摘要全文（加密落库），与 AgentSession 的 Summary/SummaryUptoSeq
// 对应，作为回滚时「逐级回退 start bound」的可恢复起点。对应表为 agent_session_compactions。
type AgentSessionCompaction struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 所属命名空间名称，多租户隔离字段
	NamespaceName string `gorm:"type:VARCHAR(32)"`
	// SessionID 所属会话 ID，会话删除时检查点级联删除（真外键，无哨兵值）
	SessionID uint `gorm:"type:BIGINT"`
	// UserID 数据属主用户 ID（users.id），用于跨用户隔离
	UserID uint `gorm:"type:BIGINT;not null"`
	// UptoSeq 本次推进后的上下文起点（即当时的 summary_upto_seq 快照），同一会话内随推进单调递增
	UptoSeq uint64 `gorm:"type:BIGINT"`
	// Summary 本次推进后的会话摘要全文（对应 AgentSession.Summary，经 crypto 序列化器加密落库）
	Summary *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=12818,length=32"`
	// Reason 触发本次推进的原因：rolling（静默滚动）/ auto（自动压缩）/ manual（手动压缩）
	Reason string `gorm:"type:VARCHAR(16)"`
	// TokensBefore 本次推进时（推进动作发生前）的会话上下文占用 token，三种原因均赋予该时刻的
	// 真实占用（不因是否展示而留空）；是否回显由读取侧按 Reason 决定，与取值无关
	TokensBefore int `gorm:"type:INT"`
	// FoldedCount 本次推进折叠进摘要的消息条数（三种原因均为真实条数）
	FoldedCount int `gorm:"type:INT"`
}
