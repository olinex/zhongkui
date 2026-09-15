package translate

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/nicksnyder/go-i18n/v2/i18n/template"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// dataDelimParser 是 translate 层使用的全局文本解析器：将 go-i18n 的模板定界符
// 从默认的 {{ / }} 改写为 [[ / ]]。如此普通翻译消息的数据占位符统一写作 [[.X]]，
// 与 text/template 保留的 {{ }} 定界符彻底隔离，避免两层模板定界符歧义。
// notify.* 默认模板正文含 {{...}} 函数调用（走 text/template 渲染），因源串不含 [[
// 会触发 go-i18n fast-path 原样透出；普通消息则写 [[.X]] 由 go-i18n 直接数据渲染。
// dataDelimParser is the package-wide text parser for translate: it rewrites go-i18n's
// template delimiters from the default {{ / }} to [[ / ]]. Ordinary message data
// placeholders are uniformly written as [[.X]], fully isolated from the {{ }} delimiters
// kept for text/template, removing the ambiguity between the two template layers.
// notify.* default templates' body carries {{...}} function calls (rendered by text/template);
// since its source contains no [[, it hits the go-i18n fast path and is passed through
// verbatim, while ordinary messages use [[.X]] rendered directly by go-i18n.
var dataDelimParser = &template.TextParser{LeftDelim: "[[", RightDelim: "]]"}

func Localizer(lang *base.Language) *i18n.Localizer {
	return i18n.NewLocalizer(Bundle, lang.String())
}

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

func L(lang *base.Language, key string, data map[string]any) string {
	return Localize(lang, key, data)
}

func Translate(key string, data map[string]any) string {
	return Localize(base.DefaultLanguage, key, data)
}

func T(key string, data map[string]any) string {
	return Translate(key, data)
}
