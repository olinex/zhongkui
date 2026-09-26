// 本文件把通知事件注册为枚举类 notificationEvent。取值空间取自本包的事件模板注册表
// （eventTemplateSpecs / TemplateSpecs()），条目按键升序排列以保证输出顺序稳定；
// 事件注册表由包级变量字面量提供（不依赖 init 顺序），此处仍经 enum.RegisterSource
// 声明来源，与 TemplateSpecs() 共用同一注册表作为单一来源。
//
// This file registers the notification events as the enum class notificationEvent. The
// value space comes from this package's event template registry (eventTemplateSpecs /
// TemplateSpecs()), with entries sorted by key for a stable order; the registry is a
// package-level variable literal (no init-order dependency), and enum.RegisterSource is
// used here so the enum shares that very registry as the single source of truth.
package notify

import (
	// stdlib
	"slices"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// init 注册 notificationEvent 枚举类（来源为事件模板注册表，读取时派生）。
// init registers the notificationEvent enum class (sourced from the event template
// registry, derived on read).
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
