package provider

import (
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
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

// InjectViper 向依赖注入器注册 Viper 配置实例 provider。
func InjectViper(i do.Injector) {
	do.Provide(i, newViperProvider())
}

// InvokeViper 从依赖注入器解析 Viper 配置实例，错误由调用方处理。
func InvokeViper(i do.Injector) (*viper.Viper, error) {
	return do.Invoke[*viper.Viper](i)
}

// MustInvokeViper 从依赖注入器解析 Viper 配置实例，失败时 panic（仅用于启动关键依赖）。
func MustInvokeViper(i do.Injector) *viper.Viper {
	return do.MustInvoke[*viper.Viper](i)
}
