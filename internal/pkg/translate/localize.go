package translate

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

func Localizer(lang *base.Language) *i18n.Localizer {
	return i18n.NewLocalizer(Bundle, lang.String())
}

func Localize(lang *base.Language, key string, data map[string]any) string {
	lc := &i18n.LocalizeConfig{
		MessageID: key,
		DefaultMessage: &i18n.Message{
			ID: key,
		},
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
