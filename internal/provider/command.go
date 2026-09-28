package provider

import (
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
)

func newCommandProvider(cmd *cobra.Command) do.Provider[*cobra.Command] {
	return func(i do.Injector) (*cobra.Command, error) {
		return cmd, nil
	}
}

// InjectCommand 向依赖注入器注册给定的 cobra 命令。
func InjectCommand(i do.Injector, cmd *cobra.Command) {
	do.Provide(i, newCommandProvider(cmd))
}

// InvokeCommand 从依赖注入器解析 cobra 命令。
func InvokeCommand(i do.Injector) (*cobra.Command, error) {
	return do.Invoke[*cobra.Command](i)
}
