package ai

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// AgentCheckpoint 保存 eino 图一次运行的可恢复现场：挂起时由 checkpoint 存储写入，续跑按同一键恢复图
// 现场并跳过已完成节点，运行收敛后随之删除。对应表为 agent_checkpoints。
//
// 与 AgentRun（agent_runs）是「引擎现场 ↔ 业务现场」的关系：agent_runs.checkpoint_id 指向本表的
// checkpoint_id。本表保存 eino 直接序列化的 graphState（消息链、档位、轮次等），续跑优先据此恢复并
// 跳过已完成节点；agent_runs 另存 messages / slot 作为 checkpoint 缺失时的回退，并保存 checkpoint
// 无法表达的领域数据（pending_calls、citations 与审批锚点）。本表不设过期时间——挂起现场在等待授权 /
// 作答期间与 agent_runs 同寿命，随会话删除清理。
type AgentCheckpoint struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// CheckpointID eino checkpoint 键（运行分配的 UUID），全局唯一，运行现场据此定位
	CheckpointID string `json:"checkpointID" gorm:"column:checkpoint_id;type:VARCHAR(64);not null;uniqueIndex:uq_agent_checkpoints_checkpoint_id"`
	// Payload 序列化后的图运行现场字节（含消息链、工具调用与运行状态，可能携带用户内容与凭据），
	// 经 crypto 序列化器加密落库
	Payload *serializer.Crypto `json:"payload" gorm:"column:payload;type:BYTEA;serializer:crypto" crypto:"mode=\"aes-gcm\",offset=12820,length=32"`
}
