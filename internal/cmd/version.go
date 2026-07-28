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
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		verbose := v.GetBool("verbose")
		pterm.Println(version.PrintVersion(verbose))
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "verbose building info output")
}
