package depend

import (
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

type I18nFxIn struct {
	fx.In
	V *viper.Viper
	APILoggerFxIn
}

type TranslateFxIn struct {
	fx.In
	Translate *translate.I18n
}

func NewI18n(in I18nFxIn) *translate.I18n {
	cfg, err := translate.ExtractConfig(in.V)
	if err != nil {
		panic(err)
	}
	return translate.InitI18n(cfg, in.APILogger)
}

func DescribeTranslateConfig() config.ConfigDescriptions {
	return translate.DescribeConfig()
}
