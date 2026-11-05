package agentkit

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"

	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// OptimizePrompt 使用 flash 槽位把用户查询改写为更清晰的
// 提示词。它是供提示词优化接口使用的单轮、无状态
// 辅助函数。
func (r *Runtime) OptimizePrompt(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("agent: empty prompt to optimize")
	}
	chat, err := r.resolveFlashChat(modelsai.SlotFlash)
	if err != nil {
		return "", errors.WithStack(err)
	}
	resp, err := chat.Generate(ctx, []*schema.Message{
		schema.SystemMessage(r.params.Text("agent.promptOptimizeSystemPrompt")),
		schema.UserMessage(query),
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	if resp == nil {
		return "", errors.New("agent: prompt optimization model returned no response")
	}
	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", errors.New("agent: prompt optimization model returned an empty response")
	}
	return out, nil
}
