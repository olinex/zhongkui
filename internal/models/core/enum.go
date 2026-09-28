// 本文件把通知模板来源与通知发送记录状态注册为枚举类 notificationSource /
// notificationRecordStatus。两者的取值空间在本包已有唯一来源（TemplateSource* 与 RecordStatus*
// 常量），故此处只按该来源派生条目并在包内 init 注册，不重复声明取值常量；类名须与 search.toml
// 字段绑定的引用一致。
package core

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// notificationSourceValues 是 notificationSource 枚举类的取值空间，取自通知模板来源常量
// （TemplateSourceDefault / TemplateSourceOverride），声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var notificationSourceValues = []string{TemplateSourceDefault, TemplateSourceOverride}

// notificationRecordStatusValues 是 notificationRecordStatus 枚举类的取值空间，取自通知记录状态常量
// （RecordStatusPending / RecordStatusSuccess / RecordStatusFailed），声明顺序即枚举类条目顺序与前端
// 下拉的展示顺序。
var notificationRecordStatusValues = []string{RecordStatusPending, RecordStatusSuccess, RecordStatusFailed}

// init 注册枚举类 notificationSource 与 notificationRecordStatus（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "notificationSource", Entries: enum.EntriesOf(notificationSourceValues)})
	enum.Register(enum.Type{Name: "notificationRecordStatus", Entries: enum.EntriesOf(notificationRecordStatusValues)})
}
