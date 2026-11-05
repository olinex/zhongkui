// Package knowledge 实现知识库能力：文档切块与向量化落库（索引）、向量路 + 关键词路的混合检索
// （含融合、去重、LLM 重排与 small-to-big 展开）、检索试运行（RAGAS 风格评估与改进建议）、
// 生成文档的分区同步，以及分块构建明细的读时聚合。本包只依赖 contract / port / config /
// llminfra / retrieval 等下层包与数据模型，不导入 internal/ai 根包，保证依赖单向。
package knowledge
