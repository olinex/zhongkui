package cmd

import (
	"github.com/spf13/cobra"
)

// completionNoDesc 控制生成的补全脚本是否包含描述信息
// 由各 shell 子命令的 --no-descriptions 标志控制，与 cobra 默认实现保持一致
var completionNoDesc bool

// completionCmd 生成指定 shell 的自动补全脚本
//
// 通过自定义中文命令屏蔽 cobra 默认的英文 completion 命令（cobra 检测到
// 已有同名命令时会跳过默认创建），各 shell 子命令复用 cobra 的 Gen*Completion 方法。
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "cmd.completion.short",
	Long:  "cmd.completion.long",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
}

// completionBashCmd 生成 bash shell 的自动补全脚本
var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "cmd.completion.bash.short",
	Long:  "cmd.completion.bash.long",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	ValidArgsFunction:     cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), !completionNoDesc)
	},
}

// completionZshCmd 生成 zsh shell 的自动补全脚本
var completionZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "cmd.completion.zsh.short",
	Long:  "cmd.completion.zsh.long",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		if completionNoDesc {
			return cmd.Root().GenZshCompletionNoDesc(cmd.OutOrStdout())
		}
		return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
	},
}

// completionFishCmd 生成 fish shell 的自动补全脚本
var completionFishCmd = &cobra.Command{
	Use:   "fish",
	Short: "cmd.completion.fish.short",
	Long:  "cmd.completion.fish.long",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), !completionNoDesc)
	},
}

// completionPowerShellCmd 生成 PowerShell 的自动补全脚本
var completionPowerShellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "cmd.completion.powershell.short",
	Long:  "cmd.completion.powershell.long",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		if completionNoDesc {
			return cmd.Root().GenPowerShellCompletion(cmd.OutOrStdout())
		}
		return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
	},
}

// initCompletion 注册 completion 命令及其子命令
//
// 必须在 RootCmd.AddCommand(versionCmd) 之前完成注册，确保 cobra 在
// Execute 阶段检测到同名命令后不再生成默认的英文 completion 命令。
func init() {
	completionBashCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "flag.completion.bash.no-descriptions")
	completionZshCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "flag.completion.zsh.no-descriptions")
	completionFishCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "flag.completion.fish.no-descriptions")
	completionPowerShellCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "flag.completion.powershell.no-descriptions")

	completionCmd.AddCommand(completionBashCmd, completionZshCmd, completionFishCmd, completionPowerShellCmd)
	RootCmd.AddCommand(completionCmd)
}
