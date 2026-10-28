// Package msgops 为 ADK 使用的两种 eino 消息表示提供通用辅助函数：
// *schema.Message（Chat Completions）和 *schema.AgenticMessage
// （Response API）。它有意不依赖业务包，以便 agentkit 与 turns
// 共享。
package msgops

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// APIModeOf 返回 M 所对应的模型接口形态：*schema.AgenticMessage 走 responses，
// 其余（*schema.Message）走 chat。取值直接复用 llm.APIMode，本包不另行声明。
func APIModeOf[M adk.MessageType]() llm.APIMode {
	var zero M
	switch any(zero).(type) {
	case *schema.AgenticMessage:
		return llm.APIModeResponses
	default:
		return llm.APIModeChat
	}
}
