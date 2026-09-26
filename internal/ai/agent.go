package ai

import (
	// stdlib
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	// third-party
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/open-policy-agent/opa/rego"
	"gorm.io/datatypes"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// agentNode 常量：Eino Graph 五节点管线的节点名（上下文组装 → 混合检索 →
// 业务数据清洗 → LLM 生成 → 结构化校验）。
// agentNode constants: the node names of the five-node Eino Graph pipeline
// (context assembly → hybrid retrieval → business-data sanitization → LLM
// generation → structured validation).
const (
	agentNodeContextAssembly = "context_assembly"
	agentNodeHybridRetrieval = "hybrid_retrieval"
	agentNodeSanitize        = "sanitize_business_data"
	agentNodeLLMGeneration   = "llm_generation"
	agentNodeValidation      = "validation"
)

// agentRegoQuery 是结构化草稿 Rego 校验规则必须产出的查询名：规则文件声明
// package agent 且规则名为 validate（data.agent.validate），返回值须为布尔。
// agentRegoQuery is the query the structured-draft Rego validation rule must
// produce: the rule file declares package agent with the rule name validate
// (data.agent.validate) whose value must be a boolean.
const agentRegoQuery = "data.agent.validate"

// AgentInput 是一次 Agent 提问的图输入。
// AgentInput is the graph input of a single agent request.
type AgentInput struct {
	// Query 用户本次提问文本
	// Query the user question text of this turn
	Query string `json:"query"`
	// NamespaceName 检索限定的命名空间（多租户隔离）
	// NamespaceName the namespace the retrieval is restricted to (multi-tenant isolation)
	NamespaceName string `json:"namespaceName"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	// DocIDs the document IDs the retrieval is restricted to, empty means no restriction
	DocIDs []uint `json:"docIDs,omitempty"`
	// SessionID 会话 ID；为 0 时不读写会话（无持久化）
	// SessionID the session ID; 0 means no session persistence
	SessionID uint `json:"sessionID,omitempty"`
	// HistoryMessages 历史消息（一般由 StreamRun 从会话加载，也可由调用方直接注入）
	// HistoryMessages the history messages (normally loaded from the session by
	// StreamRun, or injected directly by the caller)
	HistoryMessages []*schema.Message `json:"-"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	// BusinessData the frontend-passed current business scene (sanitized and
	// bounded by SanitizeBusinessData before injection)
	BusinessData llm.BusinessData `json:"businessData,omitempty"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码；为空跳过规则校验
	// RegoRule the optional Rego rule source validating the structured draft;
	// empty skips the rule validation
	RegoRule string `json:"regoRule,omitempty"`
	// Slot 显式指定的 LLM 档位；为空时依次回退到 DeclaredSlot 与
	// agent.defaultSlot 参数（见 resolveInputSlot）
	// Slot the explicitly requested LLM slot; when empty it falls back to
	// DeclaredSlot and then to the agent.defaultSlot parameter (see resolveInputSlot)
	Slot llm.Slot `json:"slot,omitempty"`
	// DeclaredSlot 调用点声明的默认档位（不对外暴露），优先级低于显式 Slot
	// DeclaredSlot the default slot declared by the call site (not exposed), with a
	// lower priority than the explicit Slot
	DeclaredSlot llm.Slot `json:"-"`
	// ToolReporter 工具调用阶段事件的上报回调（不对外暴露），由视图层注入 SSE 写入器；
	// 为 nil 时不上报。与 HistoryMessages / DeclaredSlot 同为 json:"-"。
	// ToolReporter the report callback of tool-call stage events (not exposed), injected by
	// the view layer with the SSE writer; nil reports nothing. Like HistoryMessages /
	// DeclaredSlot it is json:"-".
	ToolReporter EvalReporter `json:"-"`
}

// ChatResolver 按档位解析对话模型（使用者侧行为契约）：由上层装配方（Runtime）
// 实现，Agent 内部按本次运行解析出的档位调用它取回对话客户端，从而在
// 同一张图内支持按请求切换模型而不重建图。
// ChatResolver resolves a chat model by slot (a consumer-side behavior contract):
// implemented by the assembler (the Runtime). The Agent calls it with the
// slot resolved for the current run to obtain the chat client, so a single graph
// serves per-request model switching without being rebuilt.
type ChatResolver = func(slot llm.Slot) (model.BaseChatModel, error)

// AgentState 是图运行期间的共享状态，经 compose.ProcessState 在节点间传递
// 原始输入与检索引用（局部状态按次运行隔离，并发安全由框架保证）。
// AgentState is the per-run shared state of the graph, carrying the raw input
// and the retrieval citations between nodes via compose.ProcessState (local
// per-run state, concurrency-safe by the framework).
type AgentState struct {
	input        AgentInput        // 节点 1 写入的原始输入
	citations    []RetrievalResult // 节点 3 写入的检索引用
	slot         llm.Slot          // 节点 1 写入的本次运行档位
	toolReporter EvalReporter      // 节点 1 写入的本次运行工具阶段上报回调
}

// AgentOutput 是图输出（流式项）：流中先出现若干 delta 增量项，最后出现一个
// done 完成项（含结构化产物与校验结果）；聚合后即为单次问答的完整产物。
// AgentOutput is the graph output (a stream item): several delta items appear
// first, then a final done item carrying the structured artifact and the
// validation result; collected together they form the full reply of one turn.
type AgentOutput struct {
	// Delta 流式增量文本（仅中间 delta 项）
	// Delta the streaming delta text (delta items only)
	Delta string `json:"delta,omitempty"`
	// Content 完整回复正文（仅 done 项）
	// Content the full reply content (done item only)
	Content string `json:"content,omitempty"`
	// Done 是否为最终完成项
	// Done whether this is the final done item
	Done bool `json:"done"`
	// Citations 检索引用来源列表（仅 done 项）
	// Citations the retrieval citation sources (done item only)
	Citations []RetrievalResult `json:"citations,omitempty"`
	// Draft 结构化草稿载荷（JSON 对象）；模型未产出结构化为 null
	// Draft the structured draft payload (a JSON object); null when the model
	// produced no structure
	Draft json.RawMessage `json:"draft,omitempty"`
	// Valid 结构化产物是否通过校验（无校验规则时为 true）
	// Valid whether the structured artifact passed the validation (true when no
	// validation rule is configured)
	Valid bool `json:"valid"`
	// ValidationDetail 校验未通过时的说明
	// ValidationDetail the explanation when the validation fails
	ValidationDetail string `json:"validationDetail,omitempty"`
}

// init 注册 AgentOutput 的流块聚合函数：图 Invoke 降级路径把 [delta..., done]
// 聚合为最后一个非零项（即完成项）。该机制是 eino 为自定义流类型提供的
// RegisterStreamChunkConcatFunc 注册点，幂等覆盖。
// init registers the stream-chunk concat function of AgentOutput so the graph
// Invoke downgrade path aggregates [delta..., done] into the last non-zero item
// (the done item). This is the RegisterStreamChunkConcatFunc registration point
// eino provides for custom stream types; overwriting is idempotent.
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
// Agent is the dialogue executor of the AI assistant: it holds the compiled
// five-node Eino Graph (Runnable) and offers both non-streaming (Generate) and
// streaming (Stream) execution, depending on SessionStore for session
// persistence (see StreamRun). Every behavior parameter (prompts, section
// titles, context and truncation bounds) is read through the params resolver.
type Agent struct {
	resolver  ChatResolver             // 图内 LLM 节点（按档位解析对话模型）
	retriever retriever.Retriever      // 图内检索节点（*Retriever 天然满足）
	store     *SessionStore            // 会话持久化
	params    *llm.Params              // 行为参数解析器（提示词、标题、上限、截断长度）
	tools     []tool.BaseTool          // 本次装配可用的工具（无工具时为空）
	toolInfos []*schema.ToolInfo       // 与 tools 对应的 schema 清单（无工具时为空）
	toolIndex map[string]tool.BaseTool // 按 Info(ctx).Name 建立的工具索引（节点 4 派发用）
	runnable  compose.Runnable[AgentInput, AgentOutput]
}

// NewAgent 构造 Agent：校验依赖后组装五节点图并编译。resolver/retriever/store/params
// 任一为空时返回英文错误；resolver 与 retriever 均为函数/接口参数，测试可注入替身。
// 行为参数由上层（Runtime）从 agent_params 解析后传入。tools/toolInfos 为本次装配可用的
// 工具及其 schema 清单，二者均不做非空校验：toolInfos 为空即「无工具模式」，节点 4 不传
// 工具选项，行为与引入工具前一致。工具索引在此按 Info(ctx).Name 构建，ctx 由调用方传入
// （即 Runtime 的装配 ctx），使取消、超时与链路追踪同样覆盖构造期，而非藏在 background
// context 之后；Info 取失败的工具被跳过（该工具因此不可派发，但其 schema 若在 toolInfos
// 中仍会被模型看到——正常装配下两者同源，不会出现该情形）。
// NewAgent constructs an Agent: it validates the dependencies, assembles the
// five-node graph and compiles it. A nil resolver/retriever/store/params yields an
// English error; resolver and retriever are function/interface parameters so tests
// can inject doubles. The behavior parameters are resolved from agent_params by the
// caller (the Runtime) and passed in. tools/toolInfos are the tools available to this
// assembly and their schema list; neither is validated for emptiness: an empty
// toolInfos means "no-tool mode" and node 4 passes no tool option, behaving exactly as
// before tools existed. The tool index is built here from Info(ctx).Name — ctx comes from
// the caller (the Runtime assembly ctx) so that cancellation, deadlines and tracing cover
// the construction too, instead of being hidden behind a background context; a tool whose
// Info fails is skipped (and thus not dispatchable, though its schema in toolInfos would
// still be visible to the model — under normal assembly both come from the same source so
// this never happens).
func NewAgent(ctx context.Context, resolver ChatResolver, retriever retriever.Retriever, store *SessionStore, params *llm.Params, tools []tool.BaseTool, toolInfos []*schema.ToolInfo) (*Agent, error) {
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
// generate is node 4 (a streaming lambda): it reads the slot resolved for this run and the
// tool-stage report callback from the graph state, and resolves the chat client via the
// resolver. With no enabled tools (an empty toolInfos) it degrades to a single streaming
// generation, behaving exactly as before tools existed; with enabled tools it runs a tool-call
// loop bounded by agent.maxToolRounds — each round generates a model stream, a round carrying
// ToolCalls runs the tools and feeds the results back into the next round (its text deltas are
// never emitted), and a round without ToolCalls emits its text deltas as the final answer. A
// single tool failure feeds the error text back as the tool result without aborting the turn;
// reaching the round bound with ToolCalls still pending emits that round's text as the answer
// and reports the toolCall stage as failed without returning an error. An unbound or
// unresolvable slot yields an English error propagated through the graph's stream error
// channel.
func (a *Agent) generate(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	var slot llm.Slot
	var reporter EvalReporter
	if err := compose.ProcessState[*AgentState](ctx, func(ctx context.Context, st *AgentState) error {
		slot = st.slot
		reporter = st.toolReporter
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
	// No-tool mode: pass no tool option and take the single streaming generation path unchanged
	// (truly streaming, behaving exactly as before tools existed).
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
		// A non-positive round bound is clamped to 1; a non-positive byte bound means no
		// truncation.
		maxRounds := a.params.Int("agent.maxToolRounds")
		if maxRounds <= 0 {
			maxRounds = 1
		}
		maxBytes := a.params.Int("agent.maxToolResultBytes")
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
			// Answer round: no ToolCalls, so this round's stream is emitted chunk by chunk
			// verbatim (node 5's delta pass-through and aggregation contract is unchanged; this
			// is the only path that emits incremental text).
			if len(msg.ToolCalls) == 0 {
				for _, c := range chunks {
					outSW.Send(c, nil)
				}
				return
			}
			// 达到轮数上限仍有未完成的工具调用：以该轮文本为答案、上报 toolCall 阶段失败、不返回错误。
			// The round bound is reached with tool calls still pending: this round's text becomes
			// the answer, the toolCall stage is reported failed, and no error is returned.
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
			// Intermediate round: emit no incremental text; append the assistant message carrying
			// the ToolCalls to the next round's context, run each tool and feed the result back as
			// a tool message.
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
				result, execErr := a.runTool(ctx, tc)
				elapsed := time.Since(start).Milliseconds()
				if execErr != nil {
					// 工具执行失败：把错误文本作为工具结果回填让模型自行纠错，绝不当作流错误上抛。
					// Tool failure: feed the error text back as the tool result so the model can
					// correct itself; never raise it as a stream error.
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
// drainMessageStream drains a message stream and returns all its chunks: on a non-io.EOF receive
// error it closes the stream and returns the error, and on reaching the stream end it closes the
// stream and returns a nil error.
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
// buildToolIndex builds an index of dispatchable tools by Info(ctx).Name: a tool whose Info fails
// is skipped (this package's Agent has no logger dependency, so nothing is logged; tool failures
// are surfaced at run time through the reporter instead).
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
// runTool dispatches one tool call: it looks the tool name up in the index and asserts it to
// tool.InvokableTool, treating a miss or a type mismatch as an execution failure (returning an
// English error), otherwise it calls InvokableRun with the JSON argument string and returns its
// result.
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

// truncateBytes 把字符串按 UTF-8 rune 边界截断到不超过 max 字节（max 非正数时不截断），
// 发生截断时追加 "..."，保证不产生残缺的多字节字符。与 truncateText（rune 语义）不同，
// 本函数以字节为限，用于限制工具结果对模型上下文的占用。
// truncateBytes truncates a string to at most max bytes along UTF-8 rune boundaries (a
// non-positive max means no truncation), appending "..." when it truncates, so no malformed
// multi-byte character is ever produced. Unlike truncateText (rune semantics), this function is
// byte-bounded, keeping a tool result's footprint on the model context limited.
func truncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// 从 max 处向前回退到 rune 边界，避免切断多字节字符。
	// Step back from max to a rune boundary so no multi-byte character is cut off.
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// resolveInputSlot 解析本次运行的档位并写回输入：优先级为显式 Slot → 调用点声明的
// DeclaredSlot → 参数 agent.defaultSlot。解析失败返回英文错误。
// resolveInputSlot resolves the slot for this run and writes it back into the input:
// the priority is the explicit Slot, then the call-site DeclaredSlot, then the
// agent.defaultSlot parameter. A resolution failure yields an English error.
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
// Generate runs one conversation turn in a non-streaming way and returns the
// final structured artifact (the done item).
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
// Stream runs one conversation turn in a streaming way and returns an
// AgentOutput stream (delta items followed by a done item). The caller is
// responsible for closing the returned reader.
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
//  1. 会话存在时先校验会话、加载历史消息并注入图输入（受参数 session.maxContextMessages 约束）；
//  2. 持久化本轮用户消息；
//  3. 执行流式图；
//  4. 流末尾收到 done 项时先持久化助手回复（正文 + 草稿 + 引用）再向外发送 done。
//
// SessionID 为 0 时跳过全部持久化，行为与 Stream 一致。会话不存在时返回
// ErrSessionNotFound 且不落任何消息；提问为空时返回英文错误（不落空消息）。
// StreamRun is the session-persisting streaming entry (the backend of the SSE
// pipeline):
//  1. when the session exists, it validates the session, loads the history
//     messages and injects them into the graph input (bounded by the parameter
//     session.maxContextMessages);
//  2. persists the user message of this turn;
//  3. runs the streaming graph;
//  4. on the done item at the stream end, persists the assistant reply (content +
//     draft + citations) before emitting the done item outward.
//
// A zero SessionID skips all persistence, behaving like Stream. A missing session
// yields ErrSessionNotFound and persists nothing; an empty query yields an
// English error (no empty message is persisted).
func (a *Agent) StreamRun(ctx context.Context, input AgentInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, errors.New("agent: empty query")
	}
	if input.SessionID != 0 {
		if _, err := a.store.GetSession(ctx, input.SessionID); err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				return nil, err
			}
			return nil, errors.WithStack(err)
		}
		history, err := a.store.HistoryMessages(ctx, input.SessionID, a.params.Int("session.maxContextMessages"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		input.HistoryMessages = toSchemaMessages(history)
		if _, err := a.store.AppendMessage(ctx, &modelsai.AgentMessage{
			SessionID:     input.SessionID,
			NamespaceName: input.NamespaceName,
			Role:          MessageRoleUser,
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

// wrapPersist 包装图输出流：收到 done 项且会话存在时，先持久化助手回复再向外
// 发送 done；持久化失败把错误作为流错误发送。SessionID 为 0 时直接透传。
// wrapPersist wraps the graph output stream: on a done item with a live session it
// persists the assistant reply before emitting the done item outward; a
// persistence failure is sent as a stream error. A zero SessionID passes
// everything through untouched.
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
// persistAssistantMessage persists the done item as an assistant message of the
// session: the content is always stored, and the structured draft and the
// citation sources are stored with the message when non-empty.
func (a *Agent) persistAssistantMessage(ctx context.Context, input AgentInput, done AgentOutput) error {
	msg := &modelsai.AgentMessage{
		SessionID:     input.SessionID,
		NamespaceName: input.NamespaceName,
		Role:          MessageRoleAssistant,
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
// retrieverOptions assembles the implementation-specific retrieval options of
// this call (namespace and document IDs), routed to the in-graph retriever node
// via compose.WithRetrieverOption.
func (a *Agent) retrieverOptions(input AgentInput) compose.Option {
	return compose.WithRetrieverOption(WithNamespace(input.NamespaceName), WithDocIDs(input.DocIDs))
}

// assembleContext 是节点 1：校验提问非空，将完整输入写入图状态（供后续节点
// 读取业务数据/历史/校验规则），并输出检索查询串。
// assembleContext is node 1: it validates the non-empty question, writes the
// full input into the graph state (for later nodes to read the business data /
// history / validation rule) and outputs the retrieval query string.
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
// sanitizeBusinessData is node 3: it reads the raw input from the graph state,
// sanitizes the business data, assembles the final message list sent to the LLM,
// and writes the retrieval citations into the state for the validation node.
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
	return buildAgentMessages(a.params, in.HistoryMessages, docs, sanitized, in.Query), nil
}

// validateOutput 是节点 5：透传 LLM 流式增量（delta 项），流结束后聚合全文并
// 执行结构化产物解析与 Rego 校验，产出最终 done 项。
// validateOutput is node 5: it passes through the LLM streaming deltas (delta
// items), then on stream end aggregates the full text, runs the structured
// artifact parsing and the Rego validation, and emits the final done item.
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
// finalizeOutput parses the structured artifact from the aggregated text and
// runs the optional Rego validation, producing the done item. A parse failure
// degrades to the raw text as the content (see parseStructuredReply); a Rego
// rule evaluation error is treated as a validation failure with a detail
// message, without aborting the whole turn.
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
// structuredReply is the JSON shape of the structured model response.
type structuredReply struct {
	Content string          `json:"content"`
	Draft   json.RawMessage `json:"draft"`
}

// parseStructuredReply 解析模型的结构化 JSON 响应（{"content": "...", "draft":
// {...}}），容忍 ```json 代码围栏包裹。解析失败或 content 为空时返回错误，
// 调用方降级为将原文作为正文（无结构化草稿），保证聊天能力不受结构化约束影响。
// parseStructuredReply parses the structured JSON response of the model
// ({"content": "...", "draft": {...}}), tolerating ```json code fences. A parse
// failure or an empty content yields an error and the caller degrades to the
// raw text as the content (no structured draft), keeping the chat capability
// independent of the structured-output constraint.
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
// stripJSONFence removes the surrounding ```json code fence (when present),
// returning the trimmed content.
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
// validateDraftWithRego validates the structured draft with a Rego rule: the
// rule must return a boolean at data.agent.validate; a nil/empty draft is fed
// as null. A non-boolean rule value or a rule evaluation error is treated as a
// validation failure (returned as detail, not as error) so the caller reports
// the failure as artifact validation info instead of aborting the turn.
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

// buildAgentMessages 组装发送给 LLM 的完整消息列表：系统提示 + 历史消息 +
// 检索知识 + 业务现场 + 当前提问（检索/业务段拼接在用户消息内，控制消息条数）。
// 系统提示词、三个分节标题与片段截断长度均经参数解析器 params 读取。
// buildAgentMessages assembles the full message list sent to the LLM: system
// prompt + history messages + retrieved knowledge + business scene + the current
// question (the retrieval/business sections are joined into the user message to
// keep the message count bounded). The system prompt, the three section titles
// and the snippet truncation length are all read through the params resolver.
func buildAgentMessages(params *llm.Params, history []*schema.Message, docs []*schema.Document, businessData llm.BusinessData, query string) []*schema.Message {
	messages := make([]*schema.Message, 0, 2+len(history))
	messages = append(messages, schema.SystemMessage(params.Text("agent.systemPrompt")))
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

// formatRetrievedDocs 将检索文档格式化为带标题的引用列表（正文经 truncateText
// 限长到参数 agent.retrievedDocSnippetLimit，控制 prompt 体积）。
// formatRetrievedDocs formats the retrieved documents into a titled citation
// list (the bodies are length-capped by truncateText to the parameter
// agent.retrievedDocSnippetLimit to bound the prompt size).
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
// formatBusinessData serializes the business scene as JSON text. The input has
// been sanitized by SanitizeBusinessData (whose result is guaranteed
// serializable), so the serialization here never fails.
func formatBusinessData(data llm.BusinessData) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return "```json\n" + string(b) + "\n```"
}

// toSchemaMessages 将持久化消息转为 eino schema.Message 列表（跳过空内容与
// 未知角色），供图输入注入历史上下文。
// toSchemaMessages converts persisted messages into eino schema.Message values
// (skipping empty contents and unknown roles) for history injection into the
// graph input.
func toSchemaMessages(messages []*modelsai.AgentMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		switch m.Role {
		case MessageRoleUser:
			out = append(out, schema.UserMessage(m.Content))
		case MessageRoleAssistant:
			out = append(out, schema.AssistantMessage(m.Content, nil))
		case MessageRoleSystem:
			out = append(out, schema.SystemMessage(m.Content))
		}
	}
	return out
}
