package contract

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 跨层共享的错误哨兵：持久化层产生、执行层与调度层经 errors.Is 判定，归属 contract 使
// 上层无需反向依赖具体持久化实现。会话层以同值别名再导出，保持既有调用点不变。
var (
	// ErrSessionNotFound 会话不存在
	ErrSessionNotFound = errors.New("agent session not found")
	// ErrMessageNotFound 消息不存在
	ErrMessageNotFound = errors.New("agent message not found")
	// ErrAgentRunNotFound 运行现场不存在
	ErrAgentRunNotFound = errors.New("agent run not found")
	// ErrAgentRunConsumed 运行现场已被消费（重复恢复）
	ErrAgentRunConsumed = errors.New("agent run already consumed")
)
