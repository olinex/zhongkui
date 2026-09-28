package llm

import (
	// 标准库
	"context"
	"fmt"

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

// VerifyEmbeddingCompat 探测 cfg 指向的 embedding 端点实际输出的向量维度，并校验其与
// columnDim（core 库 knowledge_chunks.embedding 列的维度）的兼容性：兼容时返回已验证维度
// （恒等于 columnDim），否则返回指引运维调整索引的英文错误。核心不变量是「embedding 档位
// 绑定模型已验证的维度 == embedding 列维度」。
// 判定分两步探测，顺序不可颠倒：
//  1. 原生探测（不声明 dimensions）：取模型原生维度；端点不可达、凭据非法等错误如实透出，
//     此类错误说明模型本身不可用，不应被当成「不兼容」而掩盖真实原因。
//  2. 定向探测（声明 dimensions = columnDim）：text-embedding-3-small/large 这类模型的语义
//     是「原生 1536 维，但支持 dimensions 参数返回降维向量」，索引若按 1024 建依然兼容，
//     故原生维度不等于 columnDim 时须再声明 columnDim 探一次；此步失败（供应商不支持
//     dimensions 参数）不单独报错，而是落到下一步返回统一的不兼容错误。
func VerifyEmbeddingCompat(ctx context.Context, cfg ModelConfig, columnDim int) (int, error) {
	cfg.Dimensions = 0
	native, err := ProbeEmbeddingDimensions(ctx, cfg)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if native == columnDim {
		return columnDim, nil
	}
	cfg.Dimensions = columnDim
	if targeted, err := ProbeEmbeddingDimensions(ctx, cfg); err == nil && targeted == columnDim {
		return columnDim, nil
	}
	return 0, &EmbeddingDimensionMismatchError{
		Model:           cfg.Name,
		IndexDimension:  columnDim,
		NativeDimension: native,
	}
}

// EmbeddingDimensionMismatchError 表示 embedding 模型的实际输出维度与知识库索引维度不兼容。
// 这是一类「请求体合法、冲突在于系统当前索引维度」的配置性拒绝，故以专用错误类型（而非笼统的
// 请求体错误）暴露给调用方，让接口层能选用语义准确的业务错误码，并据此在文案中补齐修复前置条件。
// 端点不可达 / 凭据无效等错误不会被包装为该类型，须如实透出（见 VerifyEmbeddingCompat 注释）。
type EmbeddingDimensionMismatchError struct {
	// Model 模型本地标识
	Model string
	// IndexDimension 知识库索引（knowledge_chunks.embedding 列）当前维度
	IndexDimension int
	// NativeDimension 模型未声明 dimensions 时实际返回的维度
	NativeDimension int
}

// Error 返回英文错误详情，含完整修复步骤：先解绑 embedding 档位（若已绑定，否则重建索引会被
// reconcile 前置校验拒绝），再按模型原生维度重建索引列。
func (e *EmbeddingDimensionMismatchError) Error() string {
	return fmt.Sprintf(
		"embedding model %q is incompatible with the knowledge index dimension %d (the model returned %d dimensions); unbind the embedding slot if it is bound, then run `migrate ai-embedding --dimensions %d` before binding",
		e.Model, e.IndexDimension, e.NativeDimension, e.NativeDimension,
	)
}
