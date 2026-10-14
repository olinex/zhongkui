package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/pkg/version"
	"zhongkuisec.com/zhongkui/internal/provider"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "cmd.version.short",
	Long:  "cmd.version.long",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		verbose := v.GetBool("verbose")
		pterm.Println(version.PrintVersion(verbose))
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "flag.version.verbose")
}
