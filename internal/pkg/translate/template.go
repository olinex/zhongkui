package translate

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// resolveLang 将可能为 nil 的语言归一化为有效语言：nil 时回退平台默认语言。
// resolveLang normalizes a possibly-nil language to a valid one, falling back to the
// platform default when it is nil.
func resolveLang(lang *base.Language) *base.Language {
	if lang == nil {
		return base.DefaultLanguage
	}
	return lang
}

// EnumLabel 返回某语言下指定枚举类某 key 的 label：键为 enum.<类名>.<key>，未命中时回退
// 返回传入的 key 参数（与 Localize 未命中返回键本身语义一致）。
// EnumLabel returns the label of a key in an enum class for a language, resolved from the key
// enum.<className>.<key>; on a miss it falls back to the passed-in key parameter, matching
// the Localize contract of returning the key itself when untranslated.
func EnumLabel(lang *base.Language, className, key string) string {
	l := resolveLang(lang)
	full := "enum." + className + "." + key
	if localized := Localize(l, full, nil); localized == full {
		return key
	} else {
		return localized
	}
}
