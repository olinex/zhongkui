package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/open-policy-agent/opa/rego"
	"gorm.io/datatypes"

	// 项目内部

	"zhongkui.com/zhongkui/internal/ai/tools"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// agentNode 常量：Eino Graph 五节点管线的节点名（上下文组装 → 混合检索 →
// 业务数据清洗 → LLM 生成 → 结构化校验）。
const (
	agentNodeContextAssembly = "context_assembly"
	agentNodeHybridRetrieval = "hybrid_retrieval"
	agentNodeSanitize        = "sanitize_business_data"
	agentNodeLLMGeneration   = "llm_generation"
	agentNodeValidation      = "validation"
)

// agentRegoQuery 是结构化草稿 Rego 校验规则必须产出的查询名：规则文件声明
// package agent 且规则名为 validate（data.agent.validate），返回值须为布尔。
const agentRegoQuery = "data.agent.validate"

// agentSummarySectionTitle 是注入给 LLM 时「会话滚动摘要」段的英文标题常量（长期记忆分节的
// 标题取参数 memory.sectionTitle，分节位于摘要之后、窗口历史之前）。
const agentSummarySectionTitle = "## Conversation Summary"

// AgentInput 是一次 Agent 提问的图输入。
type AgentInput struct {
	// Query 用户本次提问文本
	Query string `json:"query"`
	// NamespaceName 检索限定的命名空间（多租户隔离）
	NamespaceName string `json:"namespaceName"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs,omitempty"`
	// SessionID 会话 ID；为 0 时不读写会话（无持久化）
	SessionID uint `json:"sessionID,omitempty"`
	// AgentKind 本次运行所属的 Agent 类型，是长期记忆按「命名空间 + Agent 类型」隔离的维度；
	// 为空时按 knowledge 处理。会话路径由 StreamRun 从会话读取后注入。
	AgentKind llm.AgentKind `json:"agentKind,omitempty"`
	// HistoryMessages 历史消息（一般由 StreamRun 从会话加载，也可由调用方直接注入）
	HistoryMessages []*schema.Message `json:"-"`
	// Summary 本次生成使用的会话滚动摘要（由 StreamRun 从会话读取/刷新后注入），
	// 非空时作为独立摘要段插在系统提示之后、窗口历史之前
	Summary string `json:"-"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData llm.BusinessData `json:"businessData,omitempty"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码；为空跳过规则校验
	RegoRule string `json:"regoRule,omitempty"`
	// Slot 显式指定的 LLM 档位；为空时依次回退到 DeclaredSlot 与
	// agent.defaultSlot 参数（见 resolveInputSlot）
	Slot llm.Slot `json:"slot,omitempty"`
	// DeclaredSlot 调用点声明的默认档位（不对外暴露），优先级低于显式 Slot
	DeclaredSlot llm.Slot `json:"-"`
	// ToolReporter 工具调用阶段事件的上报回调（不对外暴露），由视图层注入 SSE 写入器；
	// 为 nil 时不上报。与 HistoryMessages / DeclaredSlot 同为 json:"-"。
	ToolReporter EvalReporter `json:"-"`
}

// ChatResolver 按档位解析对话模型（使用者侧行为契约）：由上层装配方（Runtime）
// 实现，Agent 内部按本次运行解析出的档位调用它取回对话客户端，从而在
// 同一张图内支持按请求切换模型而不重建图。
type ChatResolver = func(slot llm.Slot) (model.BaseChatModel, error)

// AgentState 是图运行期间的共享状态，经 compose.ProcessState 在节点间传递
// 原始输入与检索引用（局部状态按次运行隔离，并发安全由框架保证）。
type AgentState struct {
	input        AgentInput        // 节点 1 写入的原始输入
	citations    []RetrievalResult // 节点 3 写入的检索引用
	slot         llm.Slot          // 节点 1 写入的本次运行档位
	toolReporter EvalReporter      // 节点 1 写入的本次运行工具阶段上报回调
}

// AgentOutput 是图输出（流式项）：流中先出现若干 delta 增量项，最后出现一个
// done 完成项（含结构化产物与校验结果）；聚合后即为单次问答的完整产物。
type AgentOutput struct {
	// Delta 流式增量文本（仅中间 delta 项）
	Delta string `json:"delta,omitempty"`
	// Content 完整回复正文（仅 done 项）
	Content string `json:"content,omitempty"`
	// Done 是否为最终完成项
	Done bool `json:"done"`
	// Citations 检索引用来源列表（仅 done 项）
	Citations []RetrievalResult `json:"citations,omitempty"`
	// Draft 结构化草稿载荷（JSON 对象）；模型未产出结构化为 null
	Draft json.RawMessage `json:"draft,omitempty"`
	// Valid 结构化产物是否通过校验（无校验规则时为 true）
	Valid bool `json:"valid"`
	// ValidationDetail 校验未通过时的说明
	ValidationDetail string `json:"validationDetail,omitempty"`
}

// init 注册 AgentOutput 的流块聚合函数：图 Invoke 降级路径把 [delta..., done]
// 聚合为最后一个非零项（即完成项）。该机制是 eino 为自定义流类型提供的
// RegisterStreamChunkConcatFunc 注册点，幂等覆盖。
func init() {
	compose.RegisterStreamChunkConcatFunc(func(items []AgentOutput) (AgentOutput, error) {
		if len(items) == 0 {
			return AgentOutput{}, nil
		}
		return items[len(items)-1], nil
	})
}

// Agent 是 AI 助手的对话执行器：持有编译后的五节点 Eino Graph（Runnable），
// 提供非流式（Generate）与流式（Stream）两种执行入口，并依赖 SessionStore
// 做会话持久化（见 StreamRun）。行为参数（提示词、分节标题、上下文与截断上限）
// 一律经参数解析器 params 读取。
type Agent struct {
	resolver  ChatResolver             // 图内 LLM 节点（按档位解析对话模型）
	retriever retriever.Retriever      // 图内检索节点（*Retriever 天然满足）
	store     *SessionStore            // 会话持久化
	params    *llm.Params              // 行为参数解析器（提示词、标题、上限、截断长度）
	memory    *Memory                  // 会话记忆服务（滚动摘要）；为 nil 时跳过摘要
	logger    *logging.Logger          // 日志记录器（可空：为 nil 时跳过告警）
	tools     []tool.BaseTool          // 本次装配可用的工具（无工具时为空）
	toolInfos []*schema.ToolInfo       // 与 tools 对应的 schema 清单（无工具时为空）
	toolIndex map[string]tool.BaseTool // 按 Info(ctx).Name 建立的工具索引（节点 4 派发用）
	runnable  compose.Runnable[AgentInput, AgentOutput]
}

// NewAgent 构造 Agent：校验依赖后组装五节点图并编译。resolver/retriever/store/params
// 任一为空时返回英文错误；memory 与 logger 允许为 nil（memory 为 nil 时跳过滚动摘要，
// logger 为 nil 时跳过告警）；resolver 与 retriever 均为函数/接口参数，测试可注入替身。
// 行为参数由上层（Runtime）从 agent_params 解析后传入。tools/toolInfos 为本次装配可用的
// 工具及其 schema 清单，二者均不做非空校验：toolInfos 为空即「无工具模式」，节点 4 不传
// 工具选项，行为与引入工具前一致。工具索引在此按 Info(ctx).Name 构建，ctx 由调用方传入
// （即 Runtime 的装配 ctx），使取消、超时与链路追踪同样覆盖构造期，而非藏在 background
// context 之后；Info 取失败的工具被跳过（该工具因此不可派发，但其 schema 若在 toolInfos
// 中仍会被模型看到——正常装配下两者同源，不会出现该情形）。
func NewAgent(ctx context.Context, resolver ChatResolver, retriever retriever.Retriever, store *SessionStore, params *llm.Params, memory *Memory, logger *logging.Logger, tools []tool.BaseTool, toolInfos []*schema.ToolInfo) (*Agent, error) {
	if resolver == nil {
		return nil, errors.New("agent: nil chat resolver")
	}
	if retriever == nil {
		return nil, errors.New("agent: nil retriever")
	}
	if store == nil {
		return nil, errors.New("agent: nil session store")
	}
	if params == nil {
		return nil, errors.New("agent: nil params")
	}
	a := &Agent{
		resolver:  resolver,
		retriever: retriever,
		store:     store,
		params:    params,
		memory:    memory,
		logger:    logger,
		tools:     tools,
		toolInfos: toolInfos,
		toolIndex: buildToolIndex(ctx, tools),
	}
	g := compose.NewGraph[AgentInput, AgentOutput](
		compose.WithGenLocalState(func(ctx context.Context) *AgentState {
			return &AgentState{}
		}),
	)
	// 节点 1：上下文组装（输入校验 + 写图状态），输出检索查询串。
	if err := g.AddLambdaNode(agentNodeContextAssembly, compose.InvokableLambda(a.assembleContext)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 2：混合检索（向量 + 关键词，经 RRF/加权融合与 LLM 重排）。
	if err := g.AddRetrieverNode(agentNodeHybridRetrieval, retriever); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 3：业务数据清洗 + 最终消息组装（系统提示 + 历史 + 检索知识 + 业务现场）。
	if err := g.AddLambdaNode(agentNodeSanitize, compose.InvokableLambda(a.sanitizeBusinessData)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 4：LLM 生成（流式 lambda；按本次运行档位解析对话模型后 Stream），
	// 输出 *schema.Message 流，节点 key 与出边保持不变。
	if err := g.AddLambdaNode(agentNodeLLMGeneration, compose.StreamableLambda(a.generate)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 节点 5：结构化校验（透传增量 + 流末尾聚合校验），输出 AgentOutput 流。
	if err := g.AddLambdaNode(agentNodeValidation, compose.TransformableLambda(a.validateOutput)); err != nil {
		return nil, errors.WithStack(err)
	}
	edges := [][2]string{
		{compose.START, agentNodeContextAssembly},
		{agentNodeContextAssembly, agentNodeHybridRetrieval},
		{agentNodeHybridRetrieval, agentNodeSanitize},
		{agentNodeSanitize, agentNodeLLMGeneration},
		{agentNodeLLMGeneration, agentNodeValidation},
		{agentNodeValidation, compose.END},
	}
	for _, e := range edges {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	runnable, err := g.Compile(context.Background())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a.runnable = runnable
	return a, nil
}

// generate 是节点 4（流式 lambda）：从图状态读取本次运行解析出的档位与工具阶段上报回调，
// 经 resolver 取回对话客户端。无启用工具（toolInfos 为空）时退化为单次流式生成，行为与
// 引入工具前完全一致；存在启用工具时执行受 agent.maxToolRounds 约束的工具调用循环——
// 每轮模型流式生成，含 ToolCalls 的轮次执行工具、回填结果后进入下一轮（其文本增量不外发），
// 不含 ToolCalls 的轮次文本增量作为最终答案外发。单个工具执行失败时把错误文本作为结果回填，
// 不中断整轮；达到轮数上限仍有 ToolCalls 时以该轮文本为答案并上报 toolCall 阶段 failed，
// 不返回错误。档位未绑定或解析失败时返回英文错误，由图的流错误通道向外传播。
func (a *Agent) generate(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	var slot llm.Slot
	var reporter EvalReporter
	var in AgentInput
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		slot = st.slot
		reporter = st.toolReporter
		in = st.input
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	chat, err := a.resolver(slot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if chat == nil {
		return nil, errors.Newf("agent: no chat model resolved for slot %q", slot)
	}
	// 无工具模式：不传工具选项，原样走单次流式生成（真流式，行为与引入工具前一致）。
	if len(a.toolInfos) == 0 {
		reader, err := chat.Stream(ctx, messages)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return reader, nil
	}

	outSR, outSW := schema.Pipe[*schema.Message](64)
	go func() {
		defer outSW.Close()
		// 轮数上限非正数时钳到 1；结果字节上限非正数时不截断。
		maxRounds := a.params.Int("agent.maxToolRounds")
		if maxRounds <= 0 {
			maxRounds = 1
		}
		maxBytes := a.params.Int("agent.maxToolResultBytes")
		// 派发工具前注入记忆运行期信息：memory_write 据此取回写入器与本次运行的归属。
		toolCtx := a.memoryToolContext(ctx, in)
		current := messages
		for round := 0; round < maxRounds; round++ {
			reader, err := chat.Stream(ctx, current, model.WithTools(a.toolInfos))
			if err != nil {
				outSW.Send(nil, errors.WithStack(err))
				return
			}
			chunks, err := drainMessageStream(reader)
			if err != nil {
				outSW.Send(nil, errors.WithStack(err))
				return
			}
			msg, err := schema.ConcatMessages(chunks)
			if err != nil {
				outSW.Send(nil, errors.WithStack(err))
				return
			}
			// 答案轮：无 ToolCalls，逐条原文外发本轮流（节点 5 的 delta 透传与聚合契约不变，
			// 这是唯一外发增量文本的路径）。
			if len(msg.ToolCalls) == 0 {
				for _, c := range chunks {
					outSW.Send(c, nil)
				}
				return
			}
			// 达到轮数上限仍有未完成的工具调用：以该轮文本为答案、上报 toolCall 阶段失败、不返回错误。
			if round == maxRounds-1 {
				for _, c := range chunks {
					outSW.Send(c, nil)
				}
				err := errors.Newf("agent: reached the tool-call round bound (%d) with pending tool calls, using this round's text as the answer", maxRounds)
				if reporter != nil {
					if rerr := reporter(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusFailed, Err: err.Error()}); rerr != nil {
						outSW.Send(nil, errors.WithStack(rerr))
					}
				}
				return
			}
			// 中间轮：不外发任何增量文本，仅把含 ToolCalls 的 assistant 消息追加进下一轮上下文，
			// 并逐个执行工具、把结果作为 tool 消息回填。
			current = append(current, msg)
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if reporter != nil {
					if rerr := reporter(StageEvent{Stage: EvalStageToolCall, Status: EvalStatusStart, Tool: name}); rerr != nil {
						outSW.Send(nil, errors.WithStack(rerr))
						return
					}
				}
				start := time.Now()
				result, execErr := a.runTool(toolCtx, tc)
				elapsed := time.Since(start).Milliseconds()
				if execErr != nil {
					// 工具执行失败：把错误文本作为工具结果回填让模型自行纠错，绝不当作流错误上抛。
					result = "tool execution failed: " + execErr.Error()
					if reporter != nil {
						if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, ElapsedMs: elapsed, Err: execErr.Error()}); rerr != nil {
							outSW.Send(nil, errors.WithStack(rerr))
							return
						}
					}
				} else {
					result = truncateBytes(result, maxBytes)
					if reporter != nil {
						if rerr := reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: name, ElapsedMs: elapsed}); rerr != nil {
							outSW.Send(nil, errors.WithStack(rerr))
							return
						}
					}
				}
				current = append(current, schema.ToolMessage(result, tc.ID))
			}
		}
	}()
	return outSR, nil
}

// drainMessageStream 抽干一个消息流并返回其全部分块：遇到非 io.EOF 的接收错误时先关闭流
// 再返回该错误，正常到流末尾时关闭流并返回 nil 错误。
func drainMessageStream(reader *schema.StreamReader[*schema.Message]) ([]*schema.Message, error) {
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return chunks, nil
			}
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
}

// buildToolIndex 按 Info(ctx).Name 为可派发工具建立索引：Info 取失败的工具被跳过（本包
// Agent 无 logger 依赖，故不记录日志，仅依赖运行期的 reporter 上报工具失败）。
func buildToolIndex(ctx context.Context, tools []tool.BaseTool) map[string]tool.BaseTool {
	index := make(map[string]tool.BaseTool, len(tools))
	for _, built := range tools {
		if built == nil {
			continue
		}
		info, err := built.Info(ctx)
		if err != nil || info == nil || info.Name == "" {
			continue
		}
		index[info.Name] = built
	}
	return index
}

// runTool 派发一次工具调用：按工具名在索引中查找并断言为 tool.InvokableTool，取不到或类型
// 不符即视为执行失败（返回英文错误），否则以 JSON 参数串调用 InvokableRun 并返回其结果。
func (a *Agent) runTool(ctx context.Context, tc schema.ToolCall) (string, error) {
	name := tc.Function.Name
	built, ok := a.toolIndex[name]
	if !ok || built == nil {
		return "", errors.Newf("agent: no tool named %q is available", name)
	}
	invokable, ok := built.(tool.InvokableTool)
	if !ok {
		return "", errors.Newf("agent: tool %q is not an invokable tool", name)
	}
	res, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
	if err != nil {
		return "", err
	}
	return res, nil
}

// memoryToolContext 在派发工具前把记忆运行期信息（写入器与归属：命名空间 / Agent 类型 / 来源
// 会话）注入 ctx，供 memory_write 在工具循环中取回；记忆服务缺失或本次运行无会话（SessionID
// 为 0）时原样返回 ctx（长期记忆按会话隔离，无会话运行不生成记忆，工具因取不到运行期信息而
// 返回英文错误，不会静默写入）。
func (a *Agent) memoryToolContext(ctx context.Context, in AgentInput) context.Context {
	if a.memory == nil || in.SessionID == 0 {
		return ctx
	}
	return tools.WithMemoryRuntime(ctx, tools.MemoryRuntime{
		Writer:        memoryStoreWriter{store: a.memory.store},
		NamespaceName: in.NamespaceName,
		AgentKind:     in.AgentKind,
		SessionID:     in.SessionID,
	})
}

// truncateBytes 把字符串按 UTF-8 rune 边界截断到不超过 max 字节（max 非正数时不截断），
// 发生截断时追加 "..."，保证不产生残缺的多字节字符。与 truncateText（rune 语义）不同，
// 本函数以字节为限，用于限制工具结果对模型上下文的占用。
func truncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// 从 max 处向前回退到 rune 边界，避免切断多字节字符。
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// resolveInputSlot 解析本次运行的档位并写回输入：优先级为显式 Slot → 调用点声明的
// DeclaredSlot → 参数 agent.defaultSlot。解析失败返回英文错误。
func (a *Agent) resolveInputSlot(input *AgentInput) error {
	fallback := llm.Slot(a.params.String("agent.defaultSlot"))
	slot, err := llm.ResolveSlot(input.Slot, input.DeclaredSlot, fallback)
	if err != nil {
		return errors.WithStack(err)
	}
	input.Slot = slot
	return nil
}

// Generate 非流式执行一次对话，返回最终结构化产物（done 项）。
func (a *Agent) Generate(ctx context.Context, input AgentInput, opts ...compose.Option) (AgentOutput, error) {
	if err := a.resolveInputSlot(&input); err != nil {
		return AgentOutput{}, err
	}
	opts = append(opts, a.retrieverOptions(input))
	out, err := a.runnable.Invoke(ctx, input, opts...)
	if err != nil {
		return AgentOutput{}, errors.WithStack(err)
	}
	return out, nil
}

// Stream 流式执行一次对话，返回 AgentOutput 流（若干 delta 增量项 + 一个 done
// 完成项）。调用方负责关闭返回的 reader。
func (a *Agent) Stream(ctx context.Context, input AgentInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	if err := a.resolveInputSlot(&input); err != nil {
		return nil, err
	}
	opts = append(opts, a.retrieverOptions(input))
	reader, err := a.runnable.Stream(ctx, input, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return reader, nil
}

// StreamRun 是带会话持久化的流式执行入口（SSE 管线后端）：
//  1. 会话存在时先校验会话、加载历史消息（先按 session.maxContextMessages 取最近 N 条、
//     再按 session.maxContextBytes 从最旧裁剪整条）并注入图输入；随后按 memory.summaryMinEvictedMessages
//     阈值推进滚动摘要（失败仅告警），摘要一并注入图输入；
//  2. 持久化本轮用户消息；
//  3. 执行流式图；
//  4. 流末尾收到 done 项时先持久化助手回复（正文 + 草稿 + 引用）再向外发送 done。
//
// SessionID 为 0 时跳过全部持久化，行为与 Stream 一致。会话不存在时返回
// ErrSessionNotFound 且不落任何消息；提问为空时返回英文错误（不落空消息）。
func (a *Agent) StreamRun(ctx context.Context, input AgentInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, errors.New("agent: empty query")
	}
	if input.SessionID != 0 {
		session, err := a.store.GetSession(ctx, input.SessionID)
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				return nil, err
			}
			return nil, errors.WithStack(err)
		}
		history, err := a.store.HistoryMessages(ctx, input.SessionID, a.params.Int("session.maxContextMessages"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 字节预算裁剪：条数窗口之后，再按 session.maxContextBytes 从最旧开始丢弃整条消息
		// （至少保留最新一条）；裁剪后的最旧消息 ID 即窗口下界，供滚动摘要圈定被挤出消息。
		history = trimMessagesByBytes(history, a.params.Int("session.maxContextBytes"))
		input.HistoryMessages = toSchemaMessages(history)
		// 会话所属 Agent 类型是长期记忆的隔离维度，由会话决定（调用方的显式取值不覆盖它）。
		input.AgentKind = session.AgentKind
		input.Summary = a.refreshSummary(ctx, session, history)
		if _, err := a.store.AppendMessage(ctx, &modelsai.AgentMessage{
			SessionID:     input.SessionID,
			NamespaceName: input.NamespaceName,
			Role:          llm.MessageRoleUser,
			Content:       strings.TrimSpace(input.Query),
		}); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	reader, err := a.Stream(ctx, input, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.wrapPersist(ctx, reader, input), nil
}

// trimMessagesByBytes 按字节预算从最旧开始丢弃整条消息，直到剩余消息的 Content 字节总数不
// 超过 maxBytes；至少保留最新一条（只丢整条消息，故不会切断多字节字符），maxBytes 非正数或
// 未超预算时原样返回。
func trimMessagesByBytes(messages []*modelsai.AgentMessage, maxBytes int) []*modelsai.AgentMessage {
	if maxBytes <= 0 || len(messages) == 0 {
		return messages
	}
	total := 0
	for _, msg := range messages {
		if msg != nil {
			total += len(msg.Content)
		}
	}
	if total <= maxBytes {
		return messages
	}
	cut := 0
	for cut < len(messages)-1 && total > maxBytes {
		if messages[cut] != nil {
			total -= len(messages[cut].Content)
		}
		cut++
	}
	return messages[cut:]
}

// refreshSummary 在加载窗口历史后推进滚动摘要：取「窗口外且 id > summary_upto_message_id 的
// 未摘要消息」（窗口最旧消息 ID 作上界），条数达到 memory.summaryMinEvictedMessages 且 memory
// 非 nil 时用 flash 重写摘要并落库，同时把新摘要写回会话副本使本次生成即用新摘要。摘要失败仅
// 告警、不中断生成、不推进游标；不受 memory.enabled 开关约束（该开关只管长期记忆）。
func (a *Agent) refreshSummary(ctx context.Context, session *modelsai.AgentSession, history []*modelsai.AgentMessage) string {
	if session == nil || a.memory == nil {
		if session == nil {
			return ""
		}
		return session.Summary
	}
	var windowOldestID uint
	if len(history) > 0 && history[0] != nil {
		windowOldestID = history[0].ID
	}
	evicted, err := a.store.ListMessagesBefore(ctx, session.ID, windowOldestID, session.SummaryUptoMessageID)
	if err != nil {
		a.warnf(ctx, "agent: loading evicted messages for the rolling summary failed: %v", err)
		return session.Summary
	}
	if len(evicted) == 0 || len(evicted) < a.params.Int("memory.summaryMinEvictedMessages") {
		return session.Summary
	}
	summary, upto, err := a.memory.Summarize(ctx, session, evicted)
	if err != nil {
		a.warnf(ctx, "agent: rolling summary failed, keeping the previous summary: %v", err)
		return session.Summary
	}
	if err := a.store.UpdateSummary(ctx, session.ID, summary, upto); err != nil {
		a.warnf(ctx, "agent: persisting the rolling summary failed, keeping the previous summary: %v", err)
		return session.Summary
	}
	session.Summary = summary
	session.SummaryUptoMessageID = upto
	return summary
}

// warnf 在 logger 非空时输出告警。
func (a *Agent) warnf(ctx context.Context, format string, args ...any) {
	if a.logger == nil {
		return
	}
	a.logger.Ctx(ctx).Warnf(format, args...)
}

// wrapPersist 包装图输出流：收到 done 项且会话存在时，先持久化助手回复再向外
// 发送 done；持久化失败把错误作为流错误发送。SessionID 为 0 时直接透传。
func (a *Agent) wrapPersist(ctx context.Context, in *schema.StreamReader[AgentOutput], input AgentInput) *schema.StreamReader[AgentOutput] {
	outSR, outSW := schema.Pipe[AgentOutput](64)
	go func() {
		defer outSW.Close()
		defer in.Close()
		for {
			item, err := in.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				outSW.Send(AgentOutput{}, errors.WithStack(err))
				return
			}
			if item.Done && input.SessionID != 0 {
				if err := a.persistAssistantMessage(ctx, input, item); err != nil {
					outSW.Send(AgentOutput{}, errors.WithStack(err))
					return
				}
			}
			outSW.Send(item, nil)
		}
	}()
	return outSR
}

// persistAssistantMessage 把 done 项持久化为会话的助手消息：正文始终落库，
// 结构化草稿与引用来源非空时随消息写入。
func (a *Agent) persistAssistantMessage(ctx context.Context, input AgentInput, done AgentOutput) error {
	msg := &modelsai.AgentMessage{
		SessionID:     input.SessionID,
		NamespaceName: input.NamespaceName,
		Role:          llm.MessageRoleAssistant,
		Content:       done.Content,
	}
	if len(done.Draft) > 0 && string(done.Draft) != "null" {
		msg.DraftJSON = datatypes.JSON(done.Draft)
	}
	if len(done.Citations) > 0 {
		if b, err := json.Marshal(done.Citations); err == nil {
			msg.CitationsJSON = datatypes.JSON(b)
		}
	}
	if _, err := a.store.AppendMessage(ctx, msg); err != nil {
		return err
	}
	return nil
}

// retrieverOptions 组装本次检索的实现专属选项（命名空间与文档限定），经
// compose.WithRetrieverOption 路由到图内检索节点。
func (a *Agent) retrieverOptions(input AgentInput) compose.Option {
	return compose.WithRetrieverOption(WithNamespace(input.NamespaceName), WithDocIDs(input.DocIDs))
}

// assembleContext 是节点 1：校验提问非空，将完整输入写入图状态（供后续节点
// 读取业务数据/历史/校验规则），并输出检索查询串。
func (a *Agent) assembleContext(ctx context.Context, in AgentInput) (string, error) {
	if strings.TrimSpace(in.Query) == "" {
		return "", errors.New("agent: empty query")
	}
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		st.input = in
		st.slot = in.Slot
		st.toolReporter = in.ToolReporter
		return nil
	}); err != nil {
		return "", errors.WithStack(err)
	}
	return in.Query, nil
}

// sanitizeBusinessData 是节点 3：从图状态读取原始输入，清洗业务数据并组装
// 发送给 LLM 的最终消息列表，同时把检索引用写入状态供校验节点输出。
func (a *Agent) sanitizeBusinessData(ctx context.Context, docs []*schema.Document) ([]*schema.Message, error) {
	var in AgentInput
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		in = st.input
		st.citations = ToRetrievalResults(docs)
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	sanitized, err := llm.SanitizeBusinessData(in.BusinessData, a.params)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return buildAgentMessages(a.params, in.Summary, a.recallMemories(ctx, in), in.HistoryMessages, docs, sanitized, in.Query), nil
}

// recallMemories 取回本次运行可注入的长期记忆：记忆服务缺失、memory.enabled=false、命名空间为
// 空、本次运行无会话（SessionID 为 0）或 Agent 类型非法时返回 nil（不注入记忆分节）；检索失败
// 降级为 nil 且只记录告警，绝不中断生成——长期记忆是增强能力，缺失时行为与引入记忆前一致。
func (a *Agent) recallMemories(ctx context.Context, in AgentInput) []MemoryHit {
	if a.memory == nil || !a.params.Bool("memory.enabled") || in.SessionID == 0 {
		return nil
	}
	agentKind, err := llm.ParseAgentKind(in.AgentKind)
	if err != nil {
		a.warnf(ctx, "agent: long-term memory retrieval skipped, invalid agent kind: %v", err)
		return nil
	}
	hits, err := a.memory.Recall(ctx, in.NamespaceName, agentKind, in.SessionID, in.Query)
	if err != nil {
		a.warnf(ctx, "agent: long-term memory retrieval failed, skipping the memory section: %v", err)
		return nil
	}
	return hits
}

// validateOutput 是节点 5：透传 LLM 流式增量（delta 项），流结束后聚合全文并
// 执行结构化产物解析与 Rego 校验，产出最终 done 项。
func (a *Agent) validateOutput(ctx context.Context, in *schema.StreamReader[*schema.Message]) (*schema.StreamReader[AgentOutput], error) {
	outSR, outSW := schema.Pipe[AgentOutput](64)
	go func() {
		defer outSW.Close()
		defer in.Close()
		var chunks []*schema.Message
		for {
			chunk, err := in.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				outSW.Send(AgentOutput{}, errors.WithStack(err))
				return
			}
			chunks = append(chunks, chunk)
			if chunk.Content != "" {
				outSW.Send(AgentOutput{Delta: chunk.Content}, nil)
			}
		}
		full, err := schema.ConcatMessages(chunks)
		if err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		out, err := a.finalizeOutput(ctx, full)
		if err != nil {
			outSW.Send(AgentOutput{}, errors.WithStack(err))
			return
		}
		outSW.Send(out, nil)
	}()
	return outSR, nil
}

// finalizeOutput 聚合全文后解析结构化产物并执行可选的 Rego 校验，产出完成项。
// 解析失败降级为把原文作为正文（见 parseStructuredReply）；Rego 规则执行出错
// 视为校验不通过并记录说明，不中断整个会话。
func (a *Agent) finalizeOutput(ctx context.Context, full *schema.Message) (AgentOutput, error) {
	out := AgentOutput{Done: true, Valid: true}
	if full == nil || strings.TrimSpace(full.Content) == "" {
		return out, errors.New("agent: empty model response")
	}
	content, draft, err := parseStructuredReply(full.Content)
	if err != nil {
		// 非结构化响应降级：正文原样返回，无草稿。
		content = full.Content
	}
	out.Content = content
	out.Draft = draft

	var citations []RetrievalResult
	var regoRule string
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		citations = append([]RetrievalResult(nil), st.citations...)
		regoRule = st.input.RegoRule
		return nil
	}); err != nil {
		return out, errors.WithStack(err)
	}
	out.Citations = citations
	if strings.TrimSpace(regoRule) != "" {
		valid, detail, verr := validateDraftWithRego(ctx, regoRule, draft)
		if verr != nil {
			out.Valid = false
			out.ValidationDetail = verr.Error()
		} else {
			out.Valid = valid
			out.ValidationDetail = detail
		}
	}
	return out, nil
}

// structuredReply 是模型结构化响应的 JSON 形状。
type structuredReply struct {
	Content string          `json:"content"`
	Draft   json.RawMessage `json:"draft"`
}

// parseStructuredReply 解析模型的结构化 JSON 响应（{"content": "...", "draft":
// {...}}），容忍 ```json 代码围栏包裹。解析失败或 content 为空时返回错误，
// 调用方降级为将原文作为正文（无结构化草稿），保证聊天能力不受结构化约束影响。
func parseStructuredReply(content string) (string, json.RawMessage, error) {
	var reply structuredReply
	if err := json.Unmarshal([]byte(stripJSONFence(content)), &reply); err != nil {
		return "", nil, errors.WithStack(err)
	}
	if strings.TrimSpace(reply.Content) == "" {
		return "", nil, errors.New("agent: structured reply has empty content")
	}
	if len(reply.Draft) == 0 || string(reply.Draft) == "null" {
		return reply.Content, nil, nil
	}
	return reply.Content, reply.Draft, nil
}

// stripJSONFence 去掉首尾的 ```json 代码围栏（若存在），返回去除首尾空白后的内容。
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
		if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return s
}

// validateDraftWithRego 用 Rego 规则校验结构化草稿：规则须在 data.agent.validate
// 处返回布尔值；draft 为 nil/空时以 null 作为输入。规则不产出布尔值或规则执行
// 出错均视为校验不通过（返回 detail，不返回 error），使上层把校验失败作为产物
// 校验信息输出而非中断整个会话。
func validateDraftWithRego(ctx context.Context, source string, draft json.RawMessage) (bool, string, error) {
	var input any
	if len(draft) > 0 {
		if err := json.Unmarshal(draft, &input); err != nil {
			return false, "", errors.WithStack(err)
		}
	}
	resultSet, err := rego.New(
		rego.Query(agentRegoQuery),
		rego.Module("agent_validation.rego", source),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		return false, "rego validation rule failed to evaluate: " + err.Error(), nil
	}
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		return false, "rego validation rule returned no result", nil
	}
	value, ok := resultSet[0].Expressions[0].Value.(bool)
	if !ok {
		return false, "rego validation rule must return a boolean", nil
	}
	if !value {
		return false, "structured draft failed the rego validation rule", nil
	}
	return true, "", nil
}

// buildAgentMessages 组装发送给 LLM 的完整消息列表：系统提示 + 会话摘要（若有）+
// 长期记忆分节（若有）+ 历史消息 + 检索知识 + 业务现场 + 当前提问（检索/业务段拼接在用户
// 消息内，控制消息条数）。长期记忆分节位于摘要之后、窗口历史之前。系统提示词、摘要段标题、
// 记忆段标题、三个分节标题与片段截断长度均经参数解析器 params 读取。
func buildAgentMessages(params *llm.Params, summary string, memories []MemoryHit, history []*schema.Message, docs []*schema.Document, businessData llm.BusinessData, query string) []*schema.Message {
	messages := make([]*schema.Message, 0, 3+len(history))
	messages = append(messages, schema.SystemMessage(params.Text("agent.systemPrompt")))
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		messages = append(messages, schema.SystemMessage(agentSummarySectionTitle+"\n"+trimmed))
	}
	if section := formatMemories(params, memories); section != "" {
		messages = append(messages, schema.SystemMessage(section))
	}
	messages = append(messages, history...)
	var parts []string
	if len(docs) > 0 {
		parts = append(parts, params.String("agent.retrievalSectionTitle")+"\n"+formatRetrievedDocs(params, docs))
	}
	if len(businessData) > 0 {
		parts = append(parts, params.String("agent.businessSectionTitle")+"\n"+formatBusinessData(businessData))
	}
	parts = append(parts, params.String("agent.questionSectionTitle")+"\n"+strings.TrimSpace(query))
	messages = append(messages, schema.UserMessage(strings.Join(parts, "\n\n")))
	return messages
}

// formatMemories 把长期记忆命中格式化为一个注入段：段标题取参数 memory.sectionTitle，
// 每条形如 "- [kind] title: content"（无标题时省略 "title: "）；正文为空或全部为空时返回空串，
// 调用方据此跳过该分节。条数与单条长度已由 MemoryStore.Retrieve 按 memory.maxItems /
// memory.maxItemBytes 限界，本函数不再重复截断。
func formatMemories(params *llm.Params, memories []MemoryHit) string {
	if len(memories) == 0 {
		return ""
	}
	lines := make([]string, 0, len(memories))
	for _, hit := range memories {
		content := strings.TrimSpace(hit.Content)
		if content == "" {
			continue
		}
		if title := strings.TrimSpace(hit.Title); title != "" {
			content = title + ": " + content
		}
		lines = append(lines, "- ["+string(hit.Kind)+"] "+content)
	}
	if len(lines) == 0 {
		return ""
	}
	return params.String("memory.sectionTitle") + "\n" + strings.Join(lines, "\n")
}

// formatRetrievedDocs 将检索文档格式化为带标题的引用列表（正文经 truncateText
// 限长到参数 agent.retrievedDocSnippetLimit，控制 prompt 体积）。
func formatRetrievedDocs(params *llm.Params, docs []*schema.Document) string {
	snippetLimit := params.Int("agent.retrievedDocSnippetLimit")
	var sb strings.Builder
	for i, d := range docs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- ")
		if d.MetaData != nil {
			if title, ok := d.MetaData["title"].(string); ok && title != "" {
				sb.WriteString(title)
				sb.WriteString(": ")
			}
		}
		sb.WriteString(truncateText(d.Content, snippetLimit))
	}
	return sb.String()
}

// formatBusinessData 将业务现场序列化为 JSON 文本。入参已经 SanitizeBusinessData
// 清洗（其结果保证可序列化），因此这里的序列化不会失败。
func formatBusinessData(data llm.BusinessData) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return "```json\n" + string(b) + "\n```"
}

// toSchemaMessages 将持久化消息转为 eino schema.Message 列表（跳过空内容与
// 未知角色），供图输入注入历史上下文。
func toSchemaMessages(messages []*modelsai.AgentMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		switch m.Role {
		case llm.MessageRoleUser:
			out = append(out, schema.UserMessage(m.Content))
		case llm.MessageRoleAssistant:
			out = append(out, schema.AssistantMessage(m.Content, nil))
		case llm.MessageRoleSystem:
			out = append(out, schema.SystemMessage(m.Content))
		}
	}
	return out
}
