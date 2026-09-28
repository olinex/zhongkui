// 本文件把 LLM 模型类型、档位槽位、工具来源、分块结构单元、Agent 类型与记忆类别/状态注册为枚举类
// llmModelKind / llmSlot / llmToolSource / chunkUnitKind / agentKind / memoryKind / memoryStatus。七类
// 取值空间均以本包已有的单一来源为准，故此处不重复声明：模型类型取自 ModelKind 常量、档位取自
// AllSlots 清单、分块结构单元取自 chunker.go 的 chunkUnitKindValues、Agent 类型取自 agent.go 的
// agentKindValues、记忆类别/状态取自 memory.go 的 memoryKindValues / memoryStatusValues；工具来源
// （llm_tools.source）的取值常量在本文件声明（此前散落为字面量），与工具注册表（ToolSpecs）同包。
// 类名须与 search.toml 字段绑定的引用一致。
package llm

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// ToolSource 是工具来源的取值类型（llm_tools.source 与视图层 source 字段）。
type ToolSource string

// 工具来源常量。
const (
	// ToolSourceBuiltin 内置工具：由代码内实现注册表声明，生命周期由代码掌握
	ToolSourceBuiltin ToolSource = "builtin"
	// ToolSourceMCP MCP 工具：由所属 MCP 服务器同步而来的远端工具
	ToolSourceMCP ToolSource = "mcp"
)

// modelKindValues 是 llmModelKind 枚举类的取值空间，取自模型类型常量（ModelKind*），声明顺序即
// 枚举类条目顺序与前端下拉的展示顺序。
var modelKindValues = []ModelKind{ModelKindChat, ModelKindEmbedding}

// toolSourceValues 是 llmToolSource 枚举类的取值空间，声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var toolSourceValues = []ToolSource{ToolSourceBuiltin, ToolSourceMCP}

// init 注册枚举类 llmModelKind / llmSlot / llmToolSource / chunkUnitKind / agentKind / memoryKind /
// memoryStatus（取值空间固定，条目在注册时快照）。llmSlot 的取值空间直接复用 AllSlots 清单，档位
// 成员仍只在该清单一处声明；chunkUnitKind 复用 chunkUnitKindValues，分块结构单元成员仍只在
// chunker.go 一处声明；agentKind 复用 agentKindValues，Agent 类型成员仍只在 agent.go 一处声明；
// memoryKind / memoryStatus 复用 memory.go 的两份取值序列，记忆类别/状态成员仍只在该文件一处声明。
func init() {
	enum.Register(enum.Type{Name: "llmModelKind", Entries: enum.EntriesOf(modelKindValues)})
	enum.Register(enum.Type{Name: "llmSlot", Entries: enum.EntriesOf(AllSlots)})
	enum.Register(enum.Type{Name: "llmToolSource", Entries: enum.EntriesOf(toolSourceValues)})
	enum.Register(enum.Type{Name: "chunkUnitKind", Entries: enum.EntriesOf(chunkUnitKindValues)})
	enum.Register(enum.Type{Name: "agentKind", Entries: enum.EntriesOf(agentKindValues)})
	enum.Register(enum.Type{Name: "memoryKind", Entries: enum.EntriesOf(memoryKindValues)})
	enum.Register(enum.Type{Name: "memoryStatus", Entries: enum.EntriesOf(memoryStatusValues)})
}
