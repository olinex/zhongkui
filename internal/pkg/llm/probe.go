package llm

import (
	// stdlib
	"context"

	// third-party
	"github.com/creasty/defaults"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ProbeEmbeddingDimensions 对 embedding 端点发起一次最小向量化请求并返回实际输出的向量维度。
// cfg.Dimensions > 0 时随请求声明该维度（OpenAI 兼容端点的 dimensions 参数，模型可据此返回
// 降维向量），否则请求模型原生维度。超时取 cfg.Timeout，未声明（0）时经 defaults.Set 回填
// ModelConfig.Timeout 的 default 标签声明的 15s；端点不可达、凭据非法或返回空向量时返回英文错误，
// 绝不静默失败。
// ProbeEmbeddingDimensions issues one minimal embedding request against the endpoint and
// returns the actual vector dimension of the output. When cfg.Dimensions > 0 the dimension is
// declared with the request (the dimensions parameter of OpenAI-compatible endpoints, letting
// the model return a reduced-dimension vector), otherwise the native dimension of the model is
// requested. The timeout is cfg.Timeout, filled with the 15s declared by the default tag of
// ModelConfig.Timeout via defaults.Set when undeclared (0); an unreachable endpoint, an invalid
// credential or an empty vector yields an English error and never fails silently.
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
//
// VerifyEmbeddingCompat probes the actual vector dimension produced by the embedding endpoint
// described by cfg and verifies its compatibility with columnDim (the dimension of the
// knowledge_chunks.embedding column on the core database): a compatible endpoint returns the
// verified dimension (always equal to columnDim), otherwise an English error directing the
// operator to resize the index is returned. The core invariant is "the verified dimension of the
// model bound to the embedding slot == the embedding column dimension".
// The decision performs two probes, and their order matters:
//  1. Native probe (no dimensions declared): reads the native dimension of the model; errors
//     such as an unreachable endpoint or an invalid credential are propagated verbatim, because
//     they mean the model itself is unusable and must not be masked as "incompatible".
//  2. Targeted probe (dimensions declared = columnDim): models such as
//     text-embedding-3-small/large are natively 1536-dimensional but accept a dimensions
//     parameter to return reduced vectors, so an index built at 1024 stays compatible; hence a
//     second probe declaring columnDim is needed when the native one differs. A failure of this
//     step (the vendor does not support the dimensions parameter) is not reported on its own but
//     falls through to the unified incompatible error below.
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
	return 0, errors.Newf(
		"embedding model %q is incompatible with the knowledge index dimension %d (the model returned %d dimensions); resize the index with the migrate ai-embedding command before binding",
		cfg.Name, columnDim, native,
	)
}
