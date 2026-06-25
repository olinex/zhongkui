package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

func newI18nProvider() do.Provider[*translate.I18n] {
	return func(i do.Injector) (*translate.I18n, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := translate.ExtractConfig(viper); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeApiLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return translate.InitI18n(cfg, logger), nil
		}
	}
}

func InjectI18n(i do.Injector) {
	do.Provide(i, newI18nProvider())
}

func InvokeI18n(i do.Injector) (*translate.I18n, error) {
	return do.Invoke[*translate.I18n](i)
}

func MustInvokeI18n(i do.Injector) *translate.I18n {
	return do.MustInvoke[*translate.I18n](i)
}

func DescribeTranslateConfig() config.ConfigDescriptions {
	return translate.DescribeConfig()
}
