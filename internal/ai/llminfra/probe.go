package llminfra

import (
	// 标准库
	"context"
	"net/http"
	"strings"

	// 第三方
	"github.com/creasty/defaults"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// TestModel 对给定模型端点配置做一次轻量探活：chat 模型以 GET {baseURL}/models 判定
// 端点可达（OpenAI 兼容端点的标准能力，任何 HTTP 响应即视为已响应，仅传输层错误视为
// 不可达），避免以完整生成请求探活；embedding 模型执行一次最小向量化请求并校验返回
// 向量非空。配置不完整时返回英文错误。
func TestModel(ctx context.Context, cfg llm.ModelConfig) error {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return errors.New("incomplete model config: baseURL/apiKey/model must all be non-empty")
	}
	// 超时缺省值由 ModelConfig.Timeout 的 default 标签声明，未声明（0）时回填
	if err := defaults.Set(&cfg); err != nil {
		return errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if cfg.Kind == llm.ModelKindEmbedding {
		if _, err := llm.ProbeEmbeddingDimensions(ctx, cfg); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	resp.Body.Close()
	return nil
}
