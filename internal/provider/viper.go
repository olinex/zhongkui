package provider

import (
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

func newViperProvider() do.Provider[*viper.Viper] {
	return func(i do.Injector) (*viper.Viper, error) {
		if cmd, err := do.Invoke[*cobra.Command](i); err != nil {
			return nil, err
		} else {
			return config.NewDefaultViperWithCmd(cmd), nil
		}
	}
}

func InjectViper(i do.Injector) {
	do.Provide(i, newViperProvider())
}

func InvokeViper(i do.Injector) (*viper.Viper, error) {
	return do.Invoke[*viper.Viper](i)
}

func MustInvokeViper(i do.Injector) *viper.Viper {
	return do.MustInvoke[*viper.Viper](i)
}
