// 本文件把文档格式注册为枚举类 documentFormat。取值空间的唯一来源是本包 docconv.go 的 Format
// 类型化常量，故此处不重复声明字面量：取值序列按常量声明顺序就地复用为条目来源，条目在包内
// init 注册时快照。类名须与消费方（视图层字段绑定、i18n 键 enum.documentFormat.<key>）一致。
//
// This file registers the document formats as the enum class documentFormat. The single source
// of the value space is the typed Format constants in docconv.go of this package, so no literal
// is repeated here: the value sequence is reused in constant declaration order as the entry
// source and the entries are snapshotted by an in-package init. The class name must match the
// consumers (view-layer field bindings and the enum.documentFormat.<key> i18n keys).
package docconv

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// formatValues 是 documentFormat 枚举类的取值空间，声明顺序即枚举类条目顺序。
// formatValues is the value space of the documentFormat enum class; the declaration order is
// the entry order of the enum class.
var formatValues = []Format{
	FormatMarkdown,
	FormatTxt,
	FormatPDF,
	FormatDocx,
}

// init 注册枚举类 documentFormat（取值空间固定，条目在注册时快照）。
// init registers the enum class documentFormat (a fixed value space whose entries are
// snapshotted at registration time).
func init() {
	enum.Register(enum.Type{Name: "documentFormat", Entries: enum.EntriesOf(formatValues)})
}
