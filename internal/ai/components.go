// Package ai 是 AI 助手（RAG + Agent）的完整实现层：知识库索引与混合检索、会话持久化、
// 对话执行器（Agent）与质量评估，以及运行时装配（以数据库模型注册表 `llm_models` 与档位绑定
// `llm_bindings` 作为 LLM 接入配置的唯一事实来源，收敛「档位 → 模型端点配置」的映射、行为参数
// （`agent_params`，出厂值由 migrate fixture 注入）、可用组件的组装、变更热重载与端点探活），
// 供 HTTP 视图层与依赖状态探测共用。本包不读取任何 LLM 配置文件。
//
// ## 运行时与分层
//
// 本包围绕进程级运行时 Runtime 组织：Runtime 以原子快照保存「已装配的组件与 LLM 接入配置」，
// 启动期与配置变更时整体重建，使调用方（视图层、worker 的解析处理器、功能开关）每次取用的是
// 当前快照而非启动瞬间固化的指针——旧指针在重建后即过期。快照中的 Components 聚合对话执行器
// Agent、知识库索引 Knowledge、混合检索器 Retriever、会话存储 Sessions、记忆服务 Memory、
// 行为参数 Params 与已启用工具 Tools；Components.Enabled 是「AI 是否完整可用」的统一判据，
// 未启用时执行组件为 nil，而 Params 始终非 nil。Runtime.Enabled / Runtime.DisabledReason 向
// internal/manager/features 报告可用性与原因，本包只陈述事实、不解读文案。
//
// ## 配置来源与枚举
//
// 档位（llm.Slot）、模型类型（llm.ModelKind）、工具来源（llm.ToolSource）、记忆种类与状态
// （llm.MemoryKind / llm.MemoryStatus）、会话状态（llm.SessionStatus）、消息角色
// （llm.MessageRole）等枚举单点声明于 internal/pkg/llm，本包只引用、不重定义；「档位 → 模型」
// 的绑定取自 `llm_bindings`，行为参数（agent.maxToolRounds、agent.maxToolResultBytes、`memory.*`、
// `knowledge.chunking.*` 等）取自 `agent_params` 并经 llm.Params 解析，出厂值随 `002_fixtures.sql`
// 注入。因此本包行为完全由数据库驱动，不存在「配置文件 + 数据库」两份事实。
//
// ## Agent 编排与记忆注入
//
// Agent 以 Eino 的 compose.Graph 编排五个节点：上下文装配、混合检索、业务数据脱敏、LLM 生成
// 与输出校验。生成节点内驱动工具循环：每轮调用可用工具并把结果回灌对话，轮数受
// agent.maxToolRounds 约束、单次工具结果受 agent.maxToolResultBytes 截断；最终输出可经 Rego
// 校验（data.agent.validate）判定是否放行。
//
// 送入模型的消息按固定顺序装配：系统提示 → 会话摘要 → 长期记忆分节 → 窗口历史 → 用户消息；
// 用户消息自身由「检索段 + 业务数据段 + 提问段」拼成。短期记忆以「条数 + 字节」双预算维护窗口，
// 超窗内容滚动压缩为摘要写回会话；检索失败一律降级告警而绝不中断生成。
//
// ## RAG 检索
//
// Retriever.Retrieve 走「向量路 + 关键词路」双通道：向量路按 embedding 的余弦距离（<=>）召回，
// 关键词路按 tsvector 全文检索（@@ plainto_tsquery）；两路结果经 RRF（fuseRRF，rrfK = 60）或
// 加权（fuseWeighted）融合，随后按内容做 4-gram Jaccard 相似度去重（dedupeSimilarResults，
// 阈值 0.8，保留排序更靠前者），再经重排与 topK 截断返回。去重只拦截内容实质相同的分块，不影响
// 分块接缝处的正常重叠。DryRun 提供不落库的检索预览；命中后异步回写 `use_count` / `last_used_at`
// （TouchUsage），失败仅告警不影响检索结果。Knowledge.IndexDocument 按 knowledge.chunking.size /
// overlap 切块并向量化，Reindex 重建、DeleteDocument 清理，BuildChunkOverview 为分块明细页
// 汇总概览。
//
// ## 长期记忆
//
// MemoryStore.Write 先按规范化正文的 sha256 精确去重，再对近邻做余弦语义合并
// （memory.dedupeThreshold），仅当两者都不命中时新增一行，并同步向量列与全文检索列；
// MemoryStore.Retrieve 复用与 RAG 相同的「向量 + 关键词 → RRF → 内容去重 → topK」管线，受
// memory.maxItems / memory.maxItemBytes 约束。记忆按「命名空间 + Agent 类型 + 会话」归属与
// 隔离，SessionID 为 0 的写入被拒绝，避免产生无会话归属的记忆。Memory 另提供 Summarize /
// Compact / GenerateTopic / Recall，均取 flash 档位，保证记忆维护的低成本。
//
// ## 会话与工具
//
// SessionStore 持久化会话与消息（创建 / 重命名 / 启停 / 删除、追加与分页读取、会话摘要回写），
// 以 ErrSessionNotFound / ErrMessageNotFound 区分未找到场景；StreamRun 在流式生成前后完成历史
// 加载、用户消息落库与助手消息落库。Store.LoadTools 依据数据库中的启用工具装配 ToolSet（含
// mcp 分组拨号与内置工具），工具契约与内置实现见 internal/ai/tools。
package ai

import (
	"github.com/cloudwego/eino/components/tool"

	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// Components 聚合 AI 助手（RAG + Agent）运行所需的全部组件：对话执行器（Agent）、
// 知识库索引（Knowledge）、知识库检索器（Retriever）、会话持久化（SessionStore）、
// 行为参数解析器（Params）与已启用的工具清单（Tools）。由本包的运行时（Runtime）依据数据库
// 模型注册表装配，并经请求中间件注入视图层；AI 未启用时各执行组件为 nil，视图层以 Enabled
// 统一判断。
type Components struct {
	// Agent 对话执行器（AI 未启用时为 nil）
	Agent *Agent
	// Knowledge 知识库索引（AI 未启用时为 nil）
	Knowledge *Knowledge
	// Retriever 知识库混合检索器（AI 未启用时为 nil）
	Retriever *Retriever
	// Sessions 会话持久化（AI 未启用时为 nil）
	Sessions *SessionStore
	// Memory 会话记忆服务（滚动摘要 / 会话压缩 / 长期记忆检索；AI 未启用时为 nil）
	Memory *Memory
	// Params 行为参数解析器（始终非 nil）
	Params *llm.Params
	// Tools 已启用的工具（AI 未启用时为空），视图层 / Agent 由它取得工具清单与 schema；
	// AI 启用但无启用工具时为空切片（非 nil）
	Tools []tool.BaseTool
}

// Enabled 报告 AI 能力是否已完整装配（未启用或装配不完整时返回 false）。
func (c *Components) Enabled() bool {
	return c != nil && c.Agent != nil && c.Knowledge != nil && c.Sessions != nil && c.Memory != nil && c.Params != nil && c.Tools != nil
}
