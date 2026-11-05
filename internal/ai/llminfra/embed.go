package llminfra

import (
	// 标准库
	"context"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
)

// EmbedStringsWithUsage 向量化文本并尽力取回 token 用量，委托 port.EmbedStringsWithUsage。
// 保留本包入口供装配层与视图层按既有 import 调用。
func EmbedStringsWithUsage(ctx context.Context, emb port.Embedder, texts []string) ([][]float64, contract.EmbedUsage, error) {
	return port.EmbedStringsWithUsage(ctx, emb, texts)
}
