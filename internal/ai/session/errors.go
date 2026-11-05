// Package session 承载 AI 会话聚合的持久化：会话、消息、持久化交互、挂起运行现场与压缩检查点
// 的读写，以及回滚 / 分叉等多表事务。本包只依赖 contract 与 models，不感知 Agent 执行与运行调度。
package session

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
)

// 包级错误哨兵：运行现场的失败类别，供上层 errors.Is 映射业务码。定义归 contract
// （跨层共享），此处保留同值别名以兼容既有调用点。
var (
	// ErrAgentRunNotFound 运行现场不存在
	ErrAgentRunNotFound = contract.ErrAgentRunNotFound
	// ErrAgentRunConsumed 运行现场已被消费（重复恢复）
	ErrAgentRunConsumed = contract.ErrAgentRunConsumed
)
