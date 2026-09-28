// 本文件实现内置工具 memory_write：Agent 在工具循环中自主把一条值得在后续轮次/会话复用的长期
// 记忆交给运行期注入的记忆写入器落库。工具本身无状态：写入器接口 MemoryWriter 与入参
// MemoryWriteInput 声明在本包，由 internal/ai 适配实现（internal/ai 单向依赖本包，故接口与类型
// 必须留在本包内以避免导入环）；归属信息（命名空间 / Agent 类型 / 来源会话）与写入器一同经 ctx
// 注入，取不到时工具返回英文错误。工具元信息（llm.ToolSpecs）与出厂行（002_fixtures.sql）由
// 另外两处提供，本文件只补实现并在 init 期登记。
//
// This file implements the builtin tool memory_write: the agent autonomously hands one long-term
// memory worth reusing in later turns or sessions to a memory writer injected at runtime during the
// tool loop. The tool itself is stateless: the writer interface MemoryWriter and the input struct
// MemoryWriteInput are declared in this package and implemented by an adapter in internal/ai
// (internal/ai depends on this package one way, so the interface and types must stay here to avoid an
// import cycle); the scope (namespace / agent kind / originating session) is injected into ctx
// together with the writer, and the tool returns an English error when it is absent. The tool
// metadata (llm.ToolSpecs) and the factory row (002_fixtures.sql) come from the other two links,
// while this file provides only the implementation and registers it at init time.
package tools

import (
	// stdlib
	"context"
	"encoding/json"
	"strings"

	// third-party
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// memoryRuntimeKey 是记忆运行期写入器与归属信息在 ctx 中的键。用私有空结构体作键可避免与其他包
// 或本包其他上下文的键冲突（只有本包能构造该类型的值）。
// memoryRuntimeKey is the ctx key of the memory runtime writer and scope. A private empty struct as
// the key avoids colliding with keys of other packages or other contexts of this package (only this
// package can construct a value of this type).
type memoryRuntimeKey struct{}

// MemoryWriteInput 是 memory_write 工具交给写入器的入参，字段即落库所需的归属与内容。本包只做
// 轻量校验，缺省与规范化（如 kind 缺省 fact、正文规范化与去重）一律由写入器实现（internal/ai
// 的适配器）负责。
// MemoryWriteInput is the input the memory_write tool hands to the writer; its fields are the scope
// and content needed to persist a memory. This package does only lightweight validation: the defaults
// and normalization (e.g. kind defaulting to fact, body normalization and dedupe) are the writer's
// responsibility (the adapter in internal/ai).
type MemoryWriteInput struct {
	// NamespaceName 所属命名空间
	// NamespaceName the owning namespace
	NamespaceName string
	// AgentKind 所属 Agent 类型
	// AgentKind the owning agent kind
	AgentKind llm.AgentKind
	// SessionID 来源会话 ID（手动来源为 0）
	// SessionID the originating session id (0 for a manual origin)
	SessionID uint
	// Kind 记忆类别（已解析为具体取值，非空）
	// Kind the memory kind (already resolved to a concrete value, non-empty)
	Kind llm.MemoryKind
	// Title 记忆标题（可选，可能为空）
	// Title the memory title (optional, may be empty)
	Title string
	// Content 记忆正文（已去除首尾空白，非空）
	// Content the memory body (trimmed, non-empty)
	Content string
}

// MemoryWriter 是记忆写入器接口：把一条记忆落库并返回其 ID。语义由 internal/ai 的适配器实现
// （向量化、精确哈希去重与语义合并等）。工具不感知实现细节。
// MemoryWriter is the memory writer interface: it persists one memory and returns its id. The
// semantics are implemented by the adapter in internal/ai (embedding, exact-hash dedupe, semantic
// merge and the like); the tool is unaware of the details.
type MemoryWriter interface {
	// WriteMemory 落库一条记忆并返回其 ID。
	// WriteMemory persists one memory and returns its id.
	WriteMemory(ctx context.Context, in MemoryWriteInput) (uint, error)
}

// MemoryRuntime 是 memory_write 运行期所需的上下文快照：写入器与本次运行的归属信息。由 Agent
// 节点 4 在派发工具前经 WithMemoryRuntime 注入 ctx。
// MemoryRuntime is the runtime context snapshot the memory_write tool needs: the writer plus the
// scope of the current run. It is injected into ctx by node 4 of the agent via WithMemoryRuntime
// before dispatching a tool.
type MemoryRuntime struct {
	// Writer 记忆写入器（nil 时工具返回英文错误）
	// Writer the memory writer (a nil value makes the tool return an English error)
	Writer MemoryWriter
	// NamespaceName 本次运行所属命名空间
	// NamespaceName the namespace of the current run
	NamespaceName string
	// AgentKind 本次运行所属 Agent 类型
	// AgentKind the agent kind of the current run
	AgentKind llm.AgentKind
	// SessionID 本次运行的来源会话 ID
	// SessionID the originating session id of the current run
	SessionID uint
}

// WithMemoryRuntime 把记忆运行期写入器与归属信息写入 ctx，供 memory_write 在工具循环中取回。
// WithMemoryRuntime stores the memory runtime writer and scope into ctx for memory_write to read back
// during the tool loop.
func WithMemoryRuntime(ctx context.Context, rt MemoryRuntime) context.Context {
	return context.WithValue(ctx, memoryRuntimeKey{}, rt)
}

// memoryRuntimeFrom 从 ctx 取回记忆运行期信息；未注入时返回 ok=false。
// memoryRuntimeFrom reads the memory runtime back from ctx; ok is false when it was never injected.
func memoryRuntimeFrom(ctx context.Context) (MemoryRuntime, bool) {
	rt, ok := ctx.Value(memoryRuntimeKey{}).(MemoryRuntime)
	return rt, ok
}

// memoryKindNames 返回记忆类别取值空间的字符串清单（唯一来源为 pkg/llm），供工具参数 schema 的
// 枚举与错误文案渲染复用。
// memoryKindNames returns the string list of the memory kind value space (the sole source is
// pkg/llm), reused by the tool parameter schema enum and the error messages.
func memoryKindNames() []string {
	kinds := llm.AllMemoryKinds()
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return names
}

// memoryWriteArgs 是 memory_write 的 JSON 入参形态：content 必填，title / kind 可选。
// memoryWriteArgs is the JSON input shape of memory_write: content is required, title / kind are
// optional.
type memoryWriteArgs struct {
	// Content 记忆正文（必填，去空白后非空）
	// Content the memory body (required, non-empty after trimming)
	Content string `json:"content"`
	// Title 记忆标题（可选）
	// Title the memory title (optional)
	Title string `json:"title"`
	// Kind 记忆类别（可选，缺省 fact）
	// Kind the memory kind (optional, defaults to fact)
	Kind string `json:"kind"`
}

// memoryWriteResult 是 memory_write 成功时返回给模型的 JSON 载荷。
// memoryWriteResult is the JSON payload returned to the model when memory_write succeeds.
type memoryWriteResult struct {
	// ID 新写入（或合并后）记忆的 ID
	// ID the id of the newly written (or merged) memory
	ID uint `json:"id"`
	// Saved 恒为 true，表示已落库
	// Saved is always true, meaning the memory has been persisted
	Saved bool `json:"saved"`
}

// memoryWriteTool 是内置工具 memory_write 的无状态实现：不持有依赖，全部归属信息与写入器均取自
// ctx。
// memoryWriteTool is the stateless implementation of the builtin tool memory_write: it holds no
// dependency and takes all scope information and the writer from ctx.
type memoryWriteTool struct{}

// Info 返回 memory_write 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（content 必填，title / kind 可选，kind 带枚举取值）。
// Info returns the metadata and the parameter JSON schema of memory_write, which the model uses to
// decide when to call it. The parameters are described through the eino ParameterInfo form (content
// required; title / kind optional; kind carries its enum values).
func (t *memoryWriteTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "memory_write",
		Desc: "Save one long-term memory worth reusing in later turns or sessions: a stable fact, a user preference, a decision or an artifact. The namespace, agent kind and session are taken from the current run.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"content": {
				Type:     schema.String,
				Desc:     "The memory body to store. Must be non-empty.",
				Required: true,
			},
			"title": {
				Type: schema.String,
				Desc: "A one-line title for the memory. Optional.",
			},
			"kind": {
				Type: schema.String,
				Desc: "The memory kind. Optional, defaults to fact.",
				Enum: memoryKindNames(),
			},
		}),
	}, nil
}

// InvokableRun 执行 memory_write：解析 JSON 入参，校验 content 与 kind，从 ctx 取回写入器与归属
// 信息后落库，成功返回形如 {"id":123,"saved":true} 的 JSON 字符串。任何校验失败均返回英文错误且
// 不触发写入；落库错误以 errors.WithStack 包装后返回（英文）。
// InvokableRun runs memory_write: it parses the JSON input, validates content and kind, reads the
// writer and scope back from ctx and persists the memory, returning a JSON string like
// {"id":123,"saved":true} on success. Any validation failure returns an English error and triggers no
// write; a persistence error is wrapped with errors.WithStack and returned (English).
func (t *memoryWriteTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args memoryWriteArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	content := strings.TrimSpace(args.Content)
	if content == "" {
		return "", errors.New("memory_write: content must be non-empty")
	}
	rawKind := strings.TrimSpace(args.Kind)
	kind, err := llm.ParseMemoryKind(llm.MemoryKind(rawKind))
	if err != nil {
		return "", errors.Newf("memory_write: invalid kind %q, expected one of %s",
			rawKind, strings.Join(memoryKindNames(), ", "))
	}
	rt, ok := memoryRuntimeFrom(ctx)
	if !ok || rt.Writer == nil {
		return "", errors.New("memory_write: no memory runtime in context")
	}
	id, err := rt.Writer.WriteMemory(ctx, MemoryWriteInput{
		NamespaceName: rt.NamespaceName,
		AgentKind:     rt.AgentKind,
		SessionID:     rt.SessionID,
		Kind:          kind,
		Title:         args.Title,
		Content:       content,
	})
	if err != nil {
		return "", errors.WithStack(err)
	}
	out, err := json.Marshal(memoryWriteResult{ID: id, Saved: true})
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 memory_write；工具无状态，不消费 Deps。
// init registers memory_write into the builtin tool implementation registry; the tool is stateless and
// consumes no Deps.
func init() {
	Register("memory_write", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &memoryWriteTool{}, nil
	})
}
