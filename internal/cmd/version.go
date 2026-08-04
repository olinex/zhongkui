package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/version"
	"zhongkui.com/zhongkui/internal/provider"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  "显示版本信息，默认输出语义化版本号，使用 --verbose 输出完整信息",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		verbose := v.GetBool("verbose")
		pterm.Println(version.PrintVersion(verbose))
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "输出详细的构建信息")
}
