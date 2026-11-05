package contract

// BusinessData 是前端透传的业务数据载体（JSON 对象），作为 Agent 会话中「当前业务现场」
// 注入 LLM 上下文；注入前必须经清洗与限界，避免空值与超长内容挤占会话上下文预算。
// 键与值均为 JSON 可序列化类型。
type BusinessData map[string]any
