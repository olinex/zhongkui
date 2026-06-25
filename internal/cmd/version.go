package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/version"
	"zhongkui.com/zhongkui/internal/provider"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "show version info",
	Long:  "show version info, default output semver, --verbose output full info",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		verbose := v.GetBool("verbose")
		pterm.Println(version.PrintVersion(verbose))
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "verbose building info output")
}
