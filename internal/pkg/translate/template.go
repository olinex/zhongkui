package translate

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// resolveLang 将可能为 nil 的语言归一化为有效语言：nil 时回退平台默认语言。
func resolveLang(lang *base.Language) *base.Language {
	if lang == nil {
		return base.DefaultLanguage
	}
	return lang
}

// EnumLabel 返回某语言下指定枚举类某 key 的 label：键为 enum.<类名>.<key>，未命中时回退
// 返回传入的 key 参数（与 Localize 未命中返回键本身语义一致）。
func EnumLabel(lang *base.Language, className, key string) string {
	l := resolveLang(lang)
	full := "enum." + className + "." + key
	if localized := Localize(l, full, nil); localized == full {
		return key
	} else {
		return localized
	}
}
