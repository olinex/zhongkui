// Package contract 承载 internal/ai 各能力包共享的线协议与轻量值对象。
//
// 本包只允许依赖 internal/models 与 internal/pkg，禁止 import 任何 internal/ai 子包或根包；
// 包内类型是 SSE 事件载荷、加密列 JSON 形状与接口出参三处契约的唯一权威定义，新增或调整字段
// 必须同步核对上述三处消费方。
package contract
