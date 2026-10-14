package llm

import (
	// 标准库
	"time"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	aimodel "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// verifyEmbeddingCompat 取 core 库 knowledge_chunks.embedding 列的维度，交由
// ai.VerifyEmbeddingCompat 完成「端点实际维度 vs 列维度」的兼容性判定（两步探测的顺序
// 与判定规则见该函数），本函数只负责取列维度：列不可用时返回按请求语言本地化的 400 拒绝，
// 其余探测与判定失败转成错误。二者同为「绑定 embedding 档位」的前置校验：不兼容时由调用方
// 以 400 拒绝请求。
func verifyEmbeddingCompat(c *gin.Context, cfg pkgllm.ModelConfig) (int, error) {
	db := middlewares.GetCoreDatabase(c)
	colDim, found, err := ai.ChunkEmbeddingDimension(c, db)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if !found {
		return 0, response.CreateBadBodyMessageError("llmKnowledgeIndexUnavailable")
	}
	return ai.VerifyEmbeddingCompat(c, cfg, colDim)
}

// modelConfigOf 把模型注册表记录映射为模型端点配置（含解密后的明文凭据）。
// 供连通性测试复用：ai 只导出「档位绑定 → 端点配置」的映射，未导出的模型可直接测试。
// 高级请求配置（extra_body）与思考开关的开/关取值须为合法 JSON，解析失败即返回英文错误
// （不静默降级），由调用方拒绝请求或记为事故。
func modelConfigOf(m *aimodel.LlmModel) (pkgllm.ModelConfig, error) {
	extraBody, err := pkgllm.ParseExtraBody(m.ExtraBody)
	if err != nil {
		return pkgllm.ModelConfig{}, errors.WithStack(err)
	}
	thinkingOnValue, err := pkgllm.ParseJSONValue(m.ThinkingOnValue)
	if err != nil {
		return pkgllm.ModelConfig{}, errors.WithStack(err)
	}
	thinkingOffValue, err := pkgllm.ParseJSONValue(m.ThinkingOffValue)
	if err != nil {
		return pkgllm.ModelConfig{}, errors.WithStack(err)
	}
	return pkgllm.ModelConfig{
		Name:             m.Name,
		Kind:             m.Kind,
		BaseURL:          m.BaseURL,
		APIKey:           string(m.APIKeyData()),
		Model:            m.ModelName,
		Timeout:          time.Duration(m.TimeoutSeconds) * time.Second,
		Temperature:      m.Temperature,
		MaxTokens:        m.MaxTokens,
		ContextTokens:    m.ContextTokens,
		Dimensions:       m.Dimensions,
		ExtraBody:        extraBody,
		ThinkingField:    m.ThinkingField,
		ThinkingOnValue:  thinkingOnValue,
		ThinkingOffValue: thinkingOffValue,
	}, nil
}

// afterConfigWrite 三组写路径（模型 / 档位绑定 / Agent 参数）统一的提交后置动作：
// 先整体重建本进程的 AI 运行时（配置立即生效），再经 etcd 广播变更事件通知其他
// manager 实例失效本地快照。两步均为 best-effort：失败仅记录告警，不阻断已提交的业务
// 变更（DB 已提交；重建失败时运行时按 AI 运行时约定保留上一可用快照）。
func afterConfigWrite(c *gin.Context, kind ai.ChangeKind, name string) {
	if err := middlewares.GetAiRuntime(c).Invalidate(c); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "ai runtime rebuild failed after %s write, name=%s, err=%v", kind, name, err)
	}
	if err := ai.BroadcastChange(c, middlewares.GetEtcdClient(c), &ai.ChangeEvent{Kind: kind, Name: name}); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "broadcast ai change failed, kind=%s, name=%s, err=%v", kind, name, err)
	}
}
