// 本文件提供各渠道类型配置结构体的反射注册表，供上层表单元信息抽取使用。
// 每个渠道在 init() 中注册其渠道级/目标级配置结构体类型（reflect.Type），
// 与渠道工厂强绑定、不分离；管理接口据此反射生成前端表单 schema。
// 仅暴露 reflect.Type，不依赖本项目的 routes 元信息反射逻辑，避免依赖循环。
package message

import (
	// 标准库
	"reflect"
)

// formStructs 渠道类型键 -> 配置结构体类型注册表。
var formStructs = map[string]FormStructs{}

// FormStructs 描述某渠道类型的渠道级/目标级配置结构体类型。
// 由表单元信息抽取方（routes 层）通过反射读取结构体标签生成前端表单 schema。
type FormStructs struct {
	// Channel 渠道级配置结构体类型
	Channel reflect.Type
	// Target 目标级配置结构体类型
	Target reflect.Type
}

// registerFormStructs 注册渠道类型对应的渠道级/目标级配置结构体类型。
// 各渠道在 init() 中与注册工厂一并调用。
func registerFormStructs(typ string, channel, target reflect.Type) {
	formStructs[typ] = FormStructs{Channel: channel, Target: target}
}

// FormStructsFor 返回某渠道类型的配置结构体类型；未注册时返回 false。
func FormStructsFor(typ string) (FormStructs, bool) {
	fs, ok := formStructs[typ]
	return fs, ok
}

// IsPlaceholderType 判断某渠道类型是否为「占位符」渠道。
// 占位符渠道的渠道级配置结构体字段数为 0（如 webhook：WebhookChannelConfig 为空对象），
// 仅用于标明通知目标的类型，不承载任何渠道级参数。这类渠道作为固定数据随命名空间
// 自动种子化，前端不可创建、不可删除。
func IsPlaceholderType(typ string) bool {
	fs, ok := formStructs[typ]
	return ok && fs.Channel.NumField() == 0
}

// PlaceholderTypes 返回所有已注册的占位符渠道类型键（升序）。
// 用于命名空间创建时自动种子化占位渠道，以及迁移脚本补充既有命名空间缺位数据。
func PlaceholderTypes() []string {
	types := make([]string, 0)
	for _, typ := range Types() {
		if IsPlaceholderType(typ) {
			types = append(types, typ)
		}
	}
	return types
}
