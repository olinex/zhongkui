// 本文件把渠道类型注册为枚举类 channelType。取值空间取自本包的渠道工厂注册表
// （registry，由 email.go / webhook.go 的 init() 依次填充），故经 enum.RegisterSource
// 声明来源：条目在每次读取时实时派生，完整性与包内 init 的文件顺序无关。
//
// This file registers the channel types as the enum class channelType. The value space
// comes from this package's channel factory registry (registry, filled by the init()
// functions of email.go / webhook.go), so it is declared via enum.RegisterSource: the
// entries are derived on every read, independent of the in-package init file order.
package message

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// init 注册 channelType 枚举类（来源为渠道工厂注册表，读取时派生）。
// init registers the channelType enum class (sourced from the channel factory registry,
// derived on read).
func init() {
	enum.RegisterSource("channelType", func() []enum.Entry {
		return enum.EntriesOf(Types())
	})
}
