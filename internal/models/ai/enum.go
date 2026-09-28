// 本文件声明知识文档处理状态（KnowledgeDocStatus）的取值空间，并把该取值空间注册为枚举类
// knowledgeDocStatus。取值的唯一来源是本文件内的类型化常量，取值序列就地复用为条目来源，
// 条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的 EnumClass 成员、i18n 键
// enum.<类名>.<key> 以及消费方引用一致。包内文档注释见 document.go。
package ai

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// KnowledgeDocStatus 是知识文档的处理状态标识：文档从导入到可被检索的流转状态。
type KnowledgeDocStatus string

// 知识文档处理状态常量。
const (
	// KnowledgeDocStatusPending 待处理（已入库，尚未开始解析）
	KnowledgeDocStatusPending KnowledgeDocStatus = "pending"
	// KnowledgeDocStatusParsing 解析中
	KnowledgeDocStatusParsing KnowledgeDocStatus = "parsing"
	// KnowledgeDocStatusReady 就绪（解析完成，可被检索）
	KnowledgeDocStatusReady KnowledgeDocStatus = "ready"
	// KnowledgeDocStatusFailed 解析失败（失败原因见 ParseError）
	KnowledgeDocStatusFailed KnowledgeDocStatus = "failed"
)

// knowledgeDocStatusValues 是 knowledgeDocStatus 枚举类的取值空间，声明顺序即枚举类条目顺序。
var knowledgeDocStatusValues = []KnowledgeDocStatus{
	KnowledgeDocStatusPending,
	KnowledgeDocStatusParsing,
	KnowledgeDocStatusReady,
	KnowledgeDocStatusFailed,
}

// init 注册枚举类 knowledgeDocStatus（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "knowledgeDocStatus", Entries: enum.EntriesOf(knowledgeDocStatusValues)})
}
