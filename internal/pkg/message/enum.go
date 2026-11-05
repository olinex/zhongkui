// 本文件把渠道类型注册为枚举类 channelType。取值空间取自本包的渠道工厂注册表
// （registry，由 email.go / webhook.go 的 init() 依次填充），故经 enum.RegisterSource
// 声明来源：条目在每次读取时实时派生，完整性与包内 init 的文件顺序无关。
package message

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// init 注册 channelType 枚举类（来源为渠道工厂注册表，读取时派生）。
func init() {
	enum.RegisterSource("channelType", func() []enum.Entry {
		return enum.EntriesOf(Types())
	})
}
