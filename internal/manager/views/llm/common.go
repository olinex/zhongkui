package llm

import (
	// stdlib
	"time"

	// third-party
	"github.com/gin-gonic/gin"

	// internal

	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
)

// verifyEmbeddingCompat 取 core 库 knowledge_chunks.embedding 列的维度，交由
// pkgllm.VerifyEmbeddingCompat 完成「端点实际维度 vs 列维度」的兼容性判定（两步探测的顺序
// 与判定规则见该函数），本函数只负责取列维度、把探测与判定失败转成英文错误。二者同为
// 「绑定 embedding 档位」的前置校验：不兼容时由调用方以 400 拒绝请求。
// verifyEmbeddingCompat reads the dimension of the knowledge_chunks.embedding column on the core
// database and hands it to pkgllm.VerifyEmbeddingCompat for the compatibility decision between
// the actual endpoint dimension and the column dimension (the order of the two probes and the
// decision rules live in that function); this function only reads the column dimension and turns
// probe/decision failures into English errors. Both are preconditions of binding the embedding
// slot: an incompatible pair is rejected by the caller with 400.
func verifyEmbeddingCompat(c *gin.Context, cfg pkgllm.ModelConfig) (int, error) {
	db := middlewares.GetCoreDatabase(c)
	colDim, found, err := pkgllm.ChunkEmbeddingDimension(c, db)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if !found {
		return 0, errors.New("the knowledge index column is not available; run migrate up before binding an embedding model")
	}
	return pkgllm.VerifyEmbeddingCompat(c, cfg, colDim)
}

// modelConfigOf 把模型注册表记录映射为模型端点配置（含解密后的明文凭据）。
// 供连通性测试复用：ai 只导出「档位绑定 → 端点配置」的映射，未导出的模型可直接测试。
// modelConfigOf maps a model registry record into a model endpoint config (with the decrypted
// plaintext credential). It lets the connectivity test cover a not-yet-bound model, since
// ai only exports the "slot binding → endpoint config" mapping.
func modelConfigOf(m *core.LlmModel) pkgllm.ModelConfig {
	return pkgllm.ModelConfig{
		Name:        m.Name,
		Kind:        pkgllm.ModelKind(m.Kind),
		BaseURL:     m.BaseURL,
		APIKey:      string(m.APIKeyData()),
		Model:       m.ModelName,
		Timeout:     time.Duration(m.TimeoutSeconds) * time.Second,
		Temperature: m.Temperature,
		MaxTokens:   m.MaxTokens,
		Dimensions:  m.Dimensions,
	}
}

// afterConfigWrite 三组写路径（模型 / 档位绑定 / Agent 参数）统一的提交后置动作：
// 先整体重建本进程的 AI 运行时（配置立即生效），再经 etcd 广播变更事件通知其他
// manager 实例失效本地快照。两步均为 best-effort：失败仅记录告警，不阻断已提交的业务
// 变更（DB 已提交；重建失败时运行时按 AI 运行时约定保留上一可用快照）。
// afterConfigWrite is the unified post-commit action of the three write paths (model / slot
// binding / Agent parameter): it first wholly rebuilds the local AI runtime (the config
// takes effect immediately) and then broadcasts the change event over etcd so other manager
// instances invalidate their local snapshot. Both steps are best-effort: a failure only logs a
// warning and never blocks the committed business change (the DB has committed; on a failed
// rebuild the runtime keeps the previous usable snapshot per the AI runtime contract).
func afterConfigWrite(c *gin.Context, kind ai.ChangeKind, name string) {
	ctx := c.Request.Context()
	if err := middlewares.GetAiRuntime(c).Invalidate(ctx); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "ai runtime rebuild failed after %s write, name=%s, err=%v", kind, name, err)
	}
	if err := ai.BroadcastChange(ctx, middlewares.GetEtcdClient(c), &ai.ChangeEvent{Kind: kind, Name: name}); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "broadcast ai change failed, kind=%s, name=%s, err=%v", kind, name, err)
	}
}
