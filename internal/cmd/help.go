package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

// helpCmd 显示任意命令的帮助信息
//
// 通过 RootCmd.SetHelpCommand 替换 cobra 默认的英文 help 命令，
// 命令查找与补全逻辑复刻自 cobra 的 InitDefaultHelpCmd 实现。
var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "显示任意命令的帮助信息",
	Long: `帮助命令用于显示应用程序中任意命令的帮助信息。
直接输入 zhongkui help [命令路径] 即可查看完整帮助。`,
	ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		cmd, _, e := c.Root().Find(args)
		if e != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if cmd == nil {
			// 未指定路径时展示根命令的子命令
			cmd = c.Root()
		}
		for _, subCmd := range cmd.Commands() {
			if subCmd.IsAvailableCommand() && strings.HasPrefix(subCmd.Name(), toComplete) {
				completions = append(completions, cobra.CompletionWithDesc(subCmd.Name(), subCmd.Short))
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	},
	Run: func(c *cobra.Command, args []string) {
		cmd, _, e := c.Root().Find(args)
		if cmd == nil || e != nil {
			c.Printf("未知的帮助主题 %#q\n", args)
			cobra.CheckErr(c.Root().Usage())
		} else {
			cmd.InitDefaultHelpFlag()    // 确保 help 标志可见
			cmd.InitDefaultVersionFlag() // 确保 version 标志可见
			cobra.CheckErr(cmd.Help())
		}
	},
}
