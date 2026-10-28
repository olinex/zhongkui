// Package assistant 定义 AI 助手 SSE 事件协议：事件信封、内容块与回合载荷。
//
// 该包只承载线协议的值类型与事件构造器，不依赖展示层，也不持有会话状态。
// 设计口径见 docs/curated/explanation/ai-assistant-sse-schema.md：服务端下发领域事实，
// 前端按块类型渲染既有组件；实时流与历史快照共用同一套块模型。
package assistant
