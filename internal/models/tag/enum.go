// 本文件把标签事件记录的告警状态注册为枚举类 tagAlertStatus。取值空间在本包已有唯一来源
// （EventStatus* 常量），故此处只按该来源派生条目并在包内 init 注册，不重复声明取值常量；类名须与
// search.toml 字段绑定的引用一致。
//
// This file registers the alert status of tag event records as the enum class tagAlertStatus. Its value
// space already has a single source inside this package (the EventStatus* constants), so this file only
// derives the entries from that source and registers them in an in-package init, repeating no value
// constant; the class name must match the references in the search.toml field bindings.
package tag

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// tagAlertStatusValues 是 tagAlertStatus 枚举类的取值空间，取自标签事件状态常量（EventStatus*），
// 声明顺序即枚举类条目顺序与前端下拉的展示顺序。
// tagAlertStatusValues is the value space of the tagAlertStatus enum class, taken from the tag event
// status constants (EventStatus*); the declaration order is the entry order of the enum class and the
// display order of the frontend select.
var tagAlertStatusValues = []string{
	EventStatusPending,
	EventStatusAlerting,
	EventStatusSuppressed,
	EventStatusClosed,
}

// init 注册枚举类 tagAlertStatus（取值空间固定，条目在注册时快照）。
// init registers the enum class tagAlertStatus (a fixed value space, its entries snapshotted at
// registration time).
func init() {
	enum.Register(enum.Type{Name: "tagAlertStatus", Entries: enum.EntriesOf(tagAlertStatusValues)})
}
