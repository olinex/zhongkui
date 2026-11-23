package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "cmd.version.short",
	Long:  "cmd.version.long",
	// version 只读取编译期注入的版本信息，不依赖 config.toml
	Annotations: map[string]string{
		annotationSkipInjectorInit: "",
	},
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Println(version.PrintVersion())
	},
}
