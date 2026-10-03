package llm

import (
	// 标准库
	"context"

	// 第三方
	"github.com/creasty/defaults"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ProbeEmbeddingDimensions 对 embedding 端点发起一次最小向量化请求并返回实际输出的向量维度。
// cfg.Dimensions > 0 时随请求声明该维度（OpenAI 兼容端点的 dimensions 参数，模型可据此返回
// 降维向量），否则请求模型原生维度。超时取 cfg.Timeout，未声明（0）时经 defaults.Set 回填
// ModelConfig.Timeout 的 default 标签声明的 15s；端点不可达、凭据非法或返回空向量时返回英文错误，
// 绝不静默失败。
func ProbeEmbeddingDimensions(ctx context.Context, cfg ModelConfig) (int, error) {
	// 超时缺省值由 default 标签声明，未声明（0）时回填
	if err := defaults.Set(&cfg); err != nil {
		return 0, errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	client, err := NewEmbedClient(ctx, cfg)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	vectors, err := client.EmbedStrings(ctx, []string{"ping"})
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return 0, errors.New("embedding dimension probe returned no vector")
	}
	return len(vectors[0]), nil
}
