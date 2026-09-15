// Package views 提供 HTTP API 视图层处理器与 In/Out 结构定义。
// 本文件提供视图层结构体枚举字段的运行时类绑定注册入口（AddEnumBinding），
// 并组装"值空间取自业务注册表"的枚举类注册（channelType / notificationEvent /
// datasourceType / tagRuleModel）。
//
// 背景：所有枚举的取值空间在运行生命周期内固定（无"动态枚举"）。
// 少数枚举类虽然不能静态声明进 search.toml（如渠道类型随 message.Types()、
// 通知事件随 notify.TemplateSpecs()、数据源类型随 vtab 常量、目标模型随
// tagengine 注册表扩展），但值空间仍是代码内固定的——它们在本包 init() 时
// 从对应业务注册表组装进 enum 注册表，与 /channel/types、/template/events、
// /datasource/types 等接口共用同一业务注册表作为单一来源。
package views

import (
	// stdlib
	"sort"

	// internal
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/enum"
	message "zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkui.com/zhongkui/internal/tagengine"
)

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

// typeKeysToEntries 将类型键切片转换为 {Key, Value} 一致的枚举条目列表。
func typeKeysToEntries(keys []string) []enum.Entry {
	entries := make([]enum.Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, enum.Entry{Key: key, Value: key})
	}
	return entries
}

// eventEntries 构造通知事件枚举类条目，唯一来源为 notify.TemplateSpecs() 的事件
// 注册表，与 /notification/template/events 返回的事件集合一致（该接口已由
// 统一枚举接口取代）。Key/Value 均取事件键。
func eventEntries() []enum.Entry {
	specs := notify.TemplateSpecs()
	events := make([]string, 0, len(specs))
	for event := range specs {
		events = append(events, event)
	}
	sort.Strings(events)
	return typeKeysToEntries(events)
}

// tagRuleModelEntries 构造目标模型枚举类条目，唯一来源为 tagengine 注册表：
// Key 取 ModelKey(typeKey) 短展示键（如 httpTransport），Value 取完整类型键
// （数据库存储语义，如 zhongkui.com/.../HttpTransport）。
func tagRuleModelEntries() []enum.Entry {
	typeKeys := tagengine.TypeKeys()
	entries := make([]enum.Entry, 0, len(typeKeys))
	for _, typeKey := range typeKeys {
		entries = append(entries, enum.Entry{Key: tagengine.ModelKey(typeKey), Value: typeKey})
	}
	return entries
}

// tagEventDimensionEntries 构造标签事件维度枚举类条目，唯一来源为 tagengine 注册表
// 各目标模型登记的合法标签维度取值（tagengine.AllDimensions 跨模型去重并集）。
// Key/Value 均取维度取值（snake_case 列名，数据库存储语义）。
func tagEventDimensionEntries() []enum.Entry {
	dims := tagengine.AllDimensions()
	entries := make([]enum.Entry, 0, len(dims))
	for _, dim := range dims {
		entries = append(entries, enum.Entry{Key: dim, Value: dim})
	}
	return entries
}

// init 组装"值空间取自业务注册表"的枚举类并注册进 enum 注册表。
// 值集合与对应 schema 接口（/channel/types、/template/events、/datasource/types、
// tagengine.TypeKeys）共用同一业务注册表，保证单一来源；此时业务注册表已由
// 各依赖包 init() 填充完毕（views 包依赖它们），故可安全遍历。
func init() {
	enum.Register(enum.Type{Name: "channelType", Entries: typeKeysToEntries(message.Types())})
	enum.Register(enum.Type{Name: "notificationEvent", Entries: eventEntries()})
	enum.Register(enum.Type{Name: "tagRuleModel", Entries: tagRuleModelEntries()})
	enum.Register(enum.Type{Name: "tagEventDimension", Entries: tagEventDimensionEntries()})
	enum.Register(enum.Type{Name: "datasourceType", Entries: typeKeysToEntries([]string{
		vtab.DatasourceTypeRedis,
		vtab.DatasourceTypeMySQL,
		vtab.DatasourceTypePostgres,
		vtab.DatasourceTypeClickHouse,
		vtab.DatasourceTypeSQLiteFile,
		vtab.DatasourceTypeEtcd,
		vtab.DatasourceTypeMongo,
	})})
}
