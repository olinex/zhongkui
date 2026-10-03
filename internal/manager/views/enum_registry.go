// Package views 提供 HTTP API 视图层处理器与 In/Out 结构定义。
// 本文件提供视图层结构体枚举字段的运行时类绑定注册入口（AddEnumBinding）。
//
// 背景：枚举取值空间的声明与注册全部下沉到视图层之外，且一律由「取值空间的归属包」就地声明——
// 归属包即该取值空间的定义处：取值空间固定者声明在其常量/清单所在的包（base 的 language /
// policyEffect、models/network 的 httpMethod / httpVersion、models/core 的 notificationSource /
// notificationRecordStatus / llmSlot、models/tag 的 tagAlertStatus、vtab 的 datasourceStruct、pkg/llm
// 的 llmModelKind / chunkUnitKind、models/core 的 llmToolSource、models/ai 的 agentKind /
// agentRunStatus / memoryKind / memoryStatus、ai/tools/mcp 的 mcpTransport，均在包内 init 经
// enum.Register 注册）；取值空间取自业务注册表者由所属业务包（message / notify / vtab /
// tagengine）经 enum.RegisterSource 声明条目来源。视图层只保留「字段 → 枚举类名」的绑定：
// search.toml [[enums]] 生成 enumBindings（见 enum_bindings.go），运行时注册的字段经
// AddEnumBinding 在对应模块 init() 时补充。
package views

// AddEnumBinding 注册某视图结构体单个枚举字段与枚举类的绑定。
// 供"类名引用"的运行时字段（如 NotificationTemplateCreateIn.event、
// NotificationRecordListOut.channelType 等）在对应模块 init() 时写入 enumBindings；
// typeName 为结构体 reflect.Type.Name()，与 search.toml [[enums]] 声明的类型名一致；
// 同结构体同字段重复注册时覆盖。init() 串行执行、包初始化在单线程完成，
// 原地修改包级 map 安全。
func AddEnumBinding(typeName, field, className string) {
	m := enumBindings[typeName]
	if m == nil {
		m = make(map[string]string)
		enumBindings[typeName] = m
	}
	m[field] = className
}
