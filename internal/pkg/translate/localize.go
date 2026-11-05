package translate

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nicksnyder/go-i18n/v2/i18n/template"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// dataDelimParser 是 translate 层使用的全局文本解析器：将 go-i18n 的模板定界符
// 从默认的双大括号改写为 [[ / ]]。如此普通翻译消息的数据占位符统一写作 [[.X]]，
// 与 text/template 保留的双大括号定界符彻底隔离，避免两层模板定界符歧义。
// notify.* 默认模板正文含函数调用动作（走 text/template 渲染），因源串不含 [[
// 会触发 go-i18n fast-path 原样透出；普通消息则写 [[.X]] 由 go-i18n 直接数据渲染。
var dataDelimParser = &template.TextParser{LeftDelim: "[[", RightDelim: "]]"}

// Localizer 为指定语言创建一个 i18n 本地化器实例。
func Localizer(lang *base.Language) *i18n.Localizer {
	return i18n.NewLocalizer(Bundle, lang.String())
}

// Localize 按指定语言对消息键进行本地化渲染，数据占位符使用 [[.X]] 语法；
// 渲染失败时回退返回原始键名。
func Localize(lang *base.Language, key string, data map[string]any) string {
	lc := &i18n.LocalizeConfig{
		MessageID: key,
		DefaultMessage: &i18n.Message{
			ID: key,
		},
		TemplateParser: dataDelimParser,
	}
	if len(data) > 0 {
		lc.TemplateData = data
	}
	localized, err := Localizer(lang).Localize(lc)
	if err != nil {
		return key
	}
	return localized
}

// L 是 Localize 的简写形式，按指定语言本地化消息键。
func L(lang *base.Language, key string, data map[string]any) string {
	return Localize(lang, key, data)
}

// Translate 使用默认语言本地化消息键。
func Translate(key string, data map[string]any) string {
	return Localize(base.DefaultLanguage, key, data)
}

// T 是 Translate 的简写形式，使用默认语言本地化消息键。
func T(key string, data map[string]any) string {
	return Translate(key, data)
}
