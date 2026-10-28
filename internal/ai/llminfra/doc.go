// Package llminfra 封装 LLM 接入的基础设施能力：模型端点探活、embedding 维度校准与兼容性
// 检查、带用量的向量化调用，以及 MCP 服务器探活。本包只依赖 internal/pkg 与 tools/mcp，
// 不感知会话、知识库或 Agent 的编排逻辑；上层包经此完成「端点是否可达、向量维度是否一致」
// 这类与业务无关的校验。
package llminfra
