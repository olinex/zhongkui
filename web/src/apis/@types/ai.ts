// 知识库文档（KnowledgeDocument）
export interface KnowledgeDocumentItem {
  id: number;
  title: string;
  chunkCount: number;
  createdAt: string;
  updatedAt: string;
}
export interface KnowledgeDocumentDetail extends KnowledgeDocumentItem {
  content: string;
  hash: string;
}
export interface KnowledgeDocumentCreateIn {
  title: string;
  content: string;
}
export interface KnowledgeDocumentUpdateIn {
  id: number;
  title: string;
  content: string;
}
export interface KnowledgeDocumentReindexOut {
  id: number;
  chunkCount: number;
  hash: string;
}

// AI 会话（AgentSession）
export interface AgentSessionItem {
  id: number;
  title: string;
  targetModel: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}
export interface AgentSessionCreateIn {
  title: string;
  targetModel?: string;
}
export interface AgentSessionRenameIn {
  id: number;
  title: string;
}

// 检索引用（RetrievalResult，来自 done 事件的 citations）
export interface RetrievalResult {
  documentID: string;
  title: string;
  chunkIndex: number;
  content: string;
  score: number;
}

// 会话消息（AgentMessageItem）
export interface AgentMessageItem {
  id: number;
  role: string; // user | assistant | system
  content: string;
  draft?: unknown; // 助手消息的结构化草稿 JSON 对象（自由形态）
  citations?: RetrievalResult[];
  createdAt: string;
}

// 流式生成请求体（AgentStreamIn）
export interface AgentStreamIn {
  query: string;
  docIDs?: number[];
  businessData?: Record<string, unknown>;
  regoRule?: string;
}

// SSE done 事件负载（后端 AgentOutput）
export interface AgentOutput {
  content?: string;
  done: boolean;
  citations?: RetrievalResult[];
  draft?: unknown;
  valid: boolean;
  validationDetail?: string;
}

// AI 状态（AgentStatusOut）
export interface AgentStatusOut {
  enabled: boolean;
  chatModel: string;
  embeddingModel: string;
  embeddingDimensions: number;
  dimensionMismatch: boolean;
  dimensionDetail: string;
}
