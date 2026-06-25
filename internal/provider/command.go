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

func InjectCommand(i do.Injector, cmd *cobra.Command) {
	do.Provide(i, newCommandProvider(cmd))
}

func InvokeCommand(i do.Injector) (*cobra.Command, error) {
	return do.Invoke[*cobra.Command](i)
}
