// Package notify 提供通知事件模板注册表，以及消息模板校验与渲染所需的事件元信息。
//
// 本文件把通知事件注册为枚举类 notificationEvent。取值空间取自本包的事件模板注册表
// （eventTemplateSpecs / TemplateSpecs()），条目按键升序排列以保证输出顺序稳定；
// 事件注册表由包级变量字面量提供（不依赖 init 顺序），此处仍经 enum.RegisterSource
// 声明来源，与 TemplateSpecs() 共用同一注册表作为单一来源。
package notify

import (
	// 标准库
	"slices"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// init 注册 notificationEvent 枚举类（来源为事件模板注册表，读取时派生）。
func init() {
	enum.RegisterSource("notificationEvent", func() []enum.Entry {
		specs := TemplateSpecs()
		events := make([]string, 0, len(specs))
		for event := range specs {
			events = append(events, event)
		}
		slices.Sort(events)
		return enum.EntriesOf(events)
	})
}
