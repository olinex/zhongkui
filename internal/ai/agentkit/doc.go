// Package agentkit 是 AI 助手的 eino 原生执行核心。
//
// # 使用的 eino 原生能力
//
//   - adk.TypedChatModelAgent[M] / adk.TypedRunner[M]：ReAct 循环、
//     工具执行、中断以及基于检查点的恢复；
//   - compose.ToolsNode：工具分发，包括其部分执行的恢复语义
//     （已完成的工具不会被重新执行）；
//   - tool.StatefulInterrupt / tool.GetInterruptContext：用于 ask_user、
//     api_invoke 和后端审批关卡；
//   - adk.TypedChatModelAgentMiddleware：审核批处理、审批、
//     工具结果安全/截断以及模型容错；
//   - schema/prompt 辅助函数：消息构造和提示词渲染；
//   - eino-ext openai（chat）和 eino-ext agenticopenai ResponsesModel
//     （Response API）；
//   - callbacks：token/耗时指标。
//
// # 保留在 eino 之外的业务特定行为
//
//   - 审核级别评估 + ReviewValve 策略（internal/ai/audit）；
//   - 知识引用（internal/ai/knowledge collector）；
//   - 长期记忆和滚动摘要/压缩（internal/ai/memory）；
//   - 内联 next_steps 解析和 flash 模型兜底；
//   - 提示词优化。
//
// # 有意不替换的部分
//
//   - deep.NewTyped：它捆绑了文件系统、shell、write_todos 和子 agent
//     工具以及内置提示词，这些都不属于本产品，并且会与
//     由数据库管理的提示词/工具列表冲突。
//   - adk/middlewares/summarization：会话摘要持久化在
//     agent_sessions/agent_session_compactions 中，并驱动 message.compress 事件；
//     原生中间件的作用域限于单次运行，且没有持久化出口。
//   - RunHub/TurnLoop 生命周期：会话驱动位于 internal/ai/turns
//     （adk.TurnLoop）与事件缓冲区；agentkit 只负责构建 agent。
//   - 实时事件投影：它是 UI 协议，不是 eino 组件。
//
// # 已知限制
//
// eino 没有为 *schema.AgenticMessage 模型接入取消监听或重试。本包中的
// WrapModel 容错中间件补齐了这两处缺口（感知上下文的流读取和瞬时
// 错误重试）；chat 模型还使用原生的
// adk.ModelRetryConfig。
//
// 固定版本为 eino v0.9.21 和
// eino-ext/components/model/agenticopenai v0.2.4（NewResponsesModel）。
package agentkit
