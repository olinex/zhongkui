// Package memory 承载长期记忆与会话记忆维护：记忆写入去重、向量/关键词混合检索、会话滚动
// 摘要、压缩、主题生成与记忆抽取。本包只依赖 contract、config、port、retrieval 与
// internal/pkg，不引用 agent / runhub；记忆按「命名空间 + Agent 类型 + 会话」归属与隔离。
package memory
