// 本文件把应用支持语言与 RBAC 策略动作注册为枚举类 language / policyEffect。两者的取值空间在本包
// 已有唯一来源——语言取自 SupportLanguages、策略动作取自 EffectAllow / EffectDeny 常量——故此处
// 只按该来源派生条目并在包内 init 注册，不重复声明取值常量。类名须与 search.toml 字段绑定的引用一致。
//
// This file registers the application languages and the RBAC policy actions as the enum classes
// language / policyEffect. Both value spaces already have a single source inside this package —
// the languages in SupportLanguages and the policy actions in the EffectAllow / EffectDeny
// constants — so this file only derives the entries from that source and registers them in an
// in-package init, repeating no value constant. The class names must match the references in the
// search.toml field bindings.
package base

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// languageValues 是 language 枚举类的取值空间，按 SupportLanguages 的声明顺序派生（该顺序即枚举
// 类条目顺序与前端下拉的展示顺序），取值为语言标签字符串。
// languageValues is the value space of the language enum class, derived in the declaration order
// of SupportLanguages (which is the entry order of the enum class and the display order of the
// frontend select); a value is the language tag string.
var languageValues = func() []string {
	values := make([]string, 0, len(SupportLanguages))
	for i := range SupportLanguages {
		values = append(values, SupportLanguages[i].String())
	}
	return values
}()

// policyEffectValues 是 policyEffect 枚举类的取值空间，取自 RBAC 策略动作常量
// （EffectAllow / EffectDeny），声明顺序即枚举类条目顺序与前端下拉的展示顺序。
// policyEffectValues is the value space of the policyEffect enum class, taken from the RBAC
// policy action constants (EffectAllow / EffectDeny); the declaration order is the entry order of
// the enum class and the display order of the frontend select.
var policyEffectValues = []string{EffectAllow, EffectDeny}

// init 注册枚举类 language 与 policyEffect（取值空间固定，条目在注册时快照）。
// init registers the enum classes language and policyEffect (fixed value spaces, their entries
// snapshotted at registration time).
func init() {
	enum.Register(enum.Type{Name: "language", Entries: enum.EntriesOf(languageValues)})
	enum.Register(enum.Type{Name: "policyEffect", Entries: enum.EntriesOf(policyEffectValues)})
}
