// 本文件声明知识文档处理状态（KnowledgeDocStatus）的取值空间，并把该取值空间注册为枚举类
// knowledgeDocStatus。取值的唯一来源是本文件内的类型化常量，取值序列就地复用为条目来源，
// 条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的 EnumClass 成员、i18n 键
// enum.<类名>.<key> 以及消费方引用一致。包内文档注释见 document.go。
//
// This file declares the value space of the knowledge document processing status
// (KnowledgeDocStatus) and registers it as the enum class knowledgeDocStatus. The single source of the
// values is the typed constants in this file; the value sequence is reused in place as the entry source
// and the entries are snapshotted by an in-package init. The class name must match the frontend
// EnumClass members in utils/enums.ts, the enum.<class>.<key> i18n keys and the references of
// consumers. The package doc comment lives in document.go.
package ai

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// KnowledgeDocStatus 是知识文档的处理状态标识：文档从导入到可被检索的流转状态。
// KnowledgeDocStatus is the processing status identifier of a knowledge document: the transition
// states of a document from import to being retrievable.
type KnowledgeDocStatus string

// 知识文档处理状态常量。
// Knowledge document processing status constants.
const (
	// KnowledgeDocStatusPending 待处理（已入库，尚未开始解析）
	// KnowledgeDocStatusPending pending (persisted, parsing not started yet)
	KnowledgeDocStatusPending KnowledgeDocStatus = "pending"
	// KnowledgeDocStatusParsing 解析中
	// KnowledgeDocStatusParsing parsing in progress
	KnowledgeDocStatusParsing KnowledgeDocStatus = "parsing"
	// KnowledgeDocStatusReady 就绪（解析完成，可被检索）
	// KnowledgeDocStatusReady ready (parsed and retrievable)
	KnowledgeDocStatusReady KnowledgeDocStatus = "ready"
	// KnowledgeDocStatusFailed 解析失败（失败原因见 ParseError）
	// KnowledgeDocStatusFailed failed (the reason is in ParseError)
	KnowledgeDocStatusFailed KnowledgeDocStatus = "failed"
)

// knowledgeDocStatusValues 是 knowledgeDocStatus 枚举类的取值空间，声明顺序即枚举类条目顺序。
// knowledgeDocStatusValues is the value space of the knowledgeDocStatus enum class; the declaration
// order is the entry order of the enum class.
var knowledgeDocStatusValues = []KnowledgeDocStatus{
	KnowledgeDocStatusPending,
	KnowledgeDocStatusParsing,
	KnowledgeDocStatusReady,
	KnowledgeDocStatusFailed,
}

// init 注册枚举类 knowledgeDocStatus（取值空间固定，条目在注册时快照）。
// init registers the enum class knowledgeDocStatus (fixed value space, entries snapshotted at
// registration time).
func init() {
	enum.Register(enum.Type{Name: "knowledgeDocStatus", Entries: enum.EntriesOf(knowledgeDocStatusValues)})
}
