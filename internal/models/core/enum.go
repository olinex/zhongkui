// 本文件把通知模板来源与通知发送记录状态注册为枚举类 notificationSource /
// notificationRecordStatus。两者的取值空间在本包已有唯一来源（TemplateSource* 与 RecordStatus*
// 常量），故此处只按该来源派生条目并在包内 init 注册，不重复声明取值常量；类名须与 search.toml
// 字段绑定的引用一致。
//
// This file registers the notification template sources and the notification delivery record statuses
// as the enum classes notificationSource / notificationRecordStatus. Both value spaces already have a
// single source inside this package (the TemplateSource* and RecordStatus* constants), so this file
// only derives the entries from that source and registers them in an in-package init, repeating no
// value constant; the class names must match the references in the search.toml field bindings.
package core

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// notificationSourceValues 是 notificationSource 枚举类的取值空间，取自通知模板来源常量
// （TemplateSourceDefault / TemplateSourceOverride），声明顺序即枚举类条目顺序与前端下拉的展示顺序。
// notificationSourceValues is the value space of the notificationSource enum class, taken from the
// notification template source constants (TemplateSourceDefault / TemplateSourceOverride); the
// declaration order is the entry order of the enum class and the display order of the frontend select.
var notificationSourceValues = []string{TemplateSourceDefault, TemplateSourceOverride}

// notificationRecordStatusValues 是 notificationRecordStatus 枚举类的取值空间，取自通知记录状态常量
// （RecordStatusPending / RecordStatusSuccess / RecordStatusFailed），声明顺序即枚举类条目顺序与前端
// 下拉的展示顺序。
// notificationRecordStatusValues is the value space of the notificationRecordStatus enum class, taken
// from the notification record status constants (RecordStatusPending / RecordStatusSuccess /
// RecordStatusFailed); the declaration order is the entry order of the enum class and the display order
// of the frontend select.
var notificationRecordStatusValues = []string{RecordStatusPending, RecordStatusSuccess, RecordStatusFailed}

// init 注册枚举类 notificationSource 与 notificationRecordStatus（取值空间固定，条目在注册时快照）。
// init registers the enum classes notificationSource and notificationRecordStatus (fixed value spaces,
// their entries snapshotted at registration time).
func init() {
	enum.Register(enum.Type{Name: "notificationSource", Entries: enum.EntriesOf(notificationSourceValues)})
	enum.Register(enum.Type{Name: "notificationRecordStatus", Entries: enum.EntriesOf(notificationRecordStatusValues)})
}
