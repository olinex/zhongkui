// Package tag 提供标签事件相关的持久化模型：标签事件规则（TagEventRule）及其版本、事件记录，
// 以及基于标签聚合的统计（tag_summing）。
//
// 本文件把标签事件记录的告警状态注册为枚举类 tagAlertStatus。取值空间在本包已有唯一来源
// （EventStatus* 常量），故此处只按该来源派生条目并在包内 init 注册，不重复声明取值常量；类名须与
// search.toml 字段绑定的引用一致。
package tag

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// tagAlertStatusValues 是 tagAlertStatus 枚举类的取值空间，取自标签事件状态常量（EventStatus*），
// 声明顺序即枚举类条目顺序与前端下拉的展示顺序。
var tagAlertStatusValues = []string{
	EventStatusPending,
	EventStatusAlerting,
	EventStatusSuppressed,
	EventStatusClosed,
}

// init 注册枚举类 tagAlertStatus（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "tagAlertStatus", Entries: enum.EntriesOf(tagAlertStatusValues)})
}
