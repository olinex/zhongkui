package notify

import (
	// 标准库
	"slices"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
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
