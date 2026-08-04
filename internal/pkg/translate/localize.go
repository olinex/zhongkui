package translate

import (
	"context"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// NewMessageBundle 创建仅包含消息包的 I18n 实例
//
// 与 InitI18n 不同，该方法跳过验证器翻译注册（newUniversalTranslator），
// 适用于 CLI 等需要在验证器初始化之前进行消息翻译的场景。
func NewMessageBundle(ctx context.Context, cfg *Config, logger *logging.Logger) *I18n {
	return &I18n{Bundle: newBundle(ctx, cfg, logger)}
}

// Localize 按语言标签翻译消息，无对应翻译时回退到 fallback。
//
// data 为可选的模板数据，消息与 fallback 中可包含 {{.Field}} 占位符。
// 该方法是 CLI 与配置描述等场景的统一翻译入口。
func (i *I18n) Localize(lang language.Tag, key, fallback string, data ...map[string]any) string {
	lc := &i18n.LocalizeConfig{
		MessageID: key,
		DefaultMessage: &i18n.Message{
			ID:    key,
			Other: fallback,
		},
	}
	if len(data) > 0 && data[0] != nil {
		lc.TemplateData = data[0]
	}
	localized, err := i18n.NewLocalizer(i.Bundle, lang.String()).Localize(lc)
	if err != nil || localized == "" {
		return fallback
	}
	return localized
}
