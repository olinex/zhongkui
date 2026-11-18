package ai

// 本文件集中声明模型对应的数据库表名，供 GORM 与文档生成统一读取。

// TableName 返回 AgentCheckpoint 对应的数据库表名。
func (m *AgentCheckpoint) TableName() string {
	return "agent_checkpoints"
}

// TableName 返回 AgentInteraction 对应的数据库表名。
func (m *AgentInteraction) TableName() string {
	return "agent_interactions"
}

// TableName 返回 AgentMemory 对应的数据库表名。
func (m *AgentMemory) TableName() string {
	return "agent_memories"
}

// TableName 返回 AgentMessage 对应的数据库表名。
func (m *AgentMessage) TableName() string {
	return "agent_messages"
}

// TableName 返回 AgentParam 对应的数据库表名。
func (m *AgentParam) TableName() string {
	return "agent_params"
}

// TableName 返回 AgentRun 对应的数据库表名。
func (m *AgentRun) TableName() string {
	return "agent_runs"
}

// TableName 返回 AgentSession 对应的数据库表名。
func (m *AgentSession) TableName() string {
	return "agent_sessions"
}

// TableName 返回 AgentSessionCompaction 对应的数据库表名。
func (m *AgentSessionCompaction) TableName() string {
	return "agent_session_compactions"
}

// TableName 返回 AgentSkill 对应的数据库表名。
func (m *AgentSkill) TableName() string {
	return "agent_skills"
}

// TableName 返回 KnowledgeChunk 对应的数据库表名。
func (m *KnowledgeChunk) TableName() string {
	return "knowledge_chunks"
}

// TableName 返回 KnowledgeDocument 对应的数据库表名。
func (m *KnowledgeDocument) TableName() string {
	return "knowledge_documents"
}

// TableName 返回 LlmBinding 对应的数据库表名。
func (m *LlmBinding) TableName() string {
	return "llm_bindings"
}

// TableName 返回 LlmModel 对应的数据库表名。
func (m *LlmModel) TableName() string {
	return "llm_models"
}

// TableName 返回 LlmTool 对应的数据库表名。
func (m *LlmTool) TableName() string {
	return "llm_tools"
}

// TableName 返回 McpServer 对应的数据库表名。
func (m *McpServer) TableName() string {
	return "mcp_servers"
}

// TableName 返回 PeriodicReportPlan 对应的数据库表名。
func (m *PeriodicReportPlan) TableName() string {
	return "periodic_report_plans"
}

// TableName 返回 PeriodicReportPlanWatch 对应的数据库表名。
func (m *PeriodicReportPlanWatch) TableName() string {
	return "periodic_report_plan_watches"
}

// TableName 返回 PeriodicReportRun 对应的数据库表名。
func (m *PeriodicReportRun) TableName() string {
	return "periodic_report_runs"
}
