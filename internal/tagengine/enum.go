// 本文件把目标模型与标签事件维度注册为枚举类。两者的取值空间均取自本包的目标模型
// 注册表（registeredTargets）：tagRuleModel 的 Key 取 ModelKey(typeKey) 短展示键、
// Value 取完整类型键（数据库存储语义）；tagEventDimension 的 Key/Value 均取维度取值
// （snake_case 列名）。注册表由包级变量字面量构建，此处经 enum.RegisterSource 声明
// 来源，与 TypeKeys()/AllDimensions() 共用同一注册表作为单一来源。
//
// This file registers the target models and the tag event dimensions as enum classes.
// Both value spaces come from this package's target model registry (registeredTargets):
// tagRuleModel takes ModelKey(typeKey) as key (short display key) and the full type key as
// value (database storage semantics); tagEventDimension takes the dimension value
// (snake_case column name) as both key and value. The registry is built from a
// package-level variable literal, and enum.RegisterSource is used here so the enums share
// that very registry as the single source of truth.
package tagengine

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// init 注册 tagRuleModel 与 tagEventDimension 枚举类（来源为目标模型注册表，读取时派生）。
// init registers the tagRuleModel and tagEventDimension enum classes (sourced from the
// target model registry, derived on read).
func init() {
	enum.RegisterSource("tagRuleModel", func() []enum.Entry {
		typeKeys := TypeKeys()
		entries := make([]enum.Entry, 0, len(typeKeys))
		for _, typeKey := range typeKeys {
			entries = append(entries, enum.Entry{Key: ModelKey(typeKey), Value: typeKey})
		}
		return entries
	})
	enum.RegisterSource("tagEventDimension", func() []enum.Entry {
		return enum.EntriesOf(AllDimensions())
	})
}
