// 本文件把 LLM 模型类型与分块结构单元注册为枚举类 llmModelKind / chunkUnitKind。两者取值空间均
// 以本包已有的单一来源为准，故此处不重复声明：模型类型取自 ModelKind 常量、分块结构单元取自
// chunker.go 的 chunkUnitKindValues；类名须与 search.toml 字段绑定的引用一致。模型档位枚举
// （llmSlot）的取值空间 Slot / AllSlots 已迁出本包，现声明于 internal/models/core（类名与其
// search.toml 绑定一致）；其余业务枚举亦已迁出：工具来源见 internal/models/core，Agent 类型、
// 记忆类别/状态与运行现场状态见 internal/models/ai。
package llm

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// modelKindValues 是 llmModelKind 枚举类的取值空间，取自模型类型常量（ModelKind*），声明顺序即
// 枚举类条目顺序与前端下拉的展示顺序。
var modelKindValues = []ModelKind{ModelKindChat, ModelKindEmbedding}

// init 注册枚举类 llmModelKind / chunkUnitKind（取值空间固定，条目在注册时快照）。
// chunkUnitKind 复用 chunkUnitKindValues，分块结构单元成员仍只在 chunker.go 一处声明。
func init() {
	enum.Register(enum.Type{Name: "llmModelKind", Entries: enum.EntriesOf(modelKindValues)})
	enum.Register(enum.Type{Name: "chunkUnitKind", Entries: enum.EntriesOf(chunkUnitKindValues)})
}
