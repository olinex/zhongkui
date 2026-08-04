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
	Short: "生成指定 shell 的自动补全脚本",
	Long: `生成 zhongkui 在指定 shell 下的自动补全脚本。

各子命令的帮助信息中说明了生成脚本的使用方法。`,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
}

// completionBashCmd 生成 bash shell 的自动补全脚本
var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "生成 bash shell 的自动补全脚本",
	Long: `生成 bash shell 的自动补全脚本。

该脚本依赖 'bash-completion' 软件包。
如果尚未安装，可以通过操作系统自带的包管理器进行安装。

要在当前 shell 会话中加载补全：

	source <(zhongkui completion bash)

要为每个新的会话加载补全，请执行一次：

#### Linux:

	zhongkui completion bash > /etc/bash_completion.d/zhongkui

#### macOS:

	zhongkui completion bash > $(brew --prefix)/etc/bash_completion.d/zhongkui

重启 shell 后配置生效。`,
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
	Short: "生成 zsh shell 的自动补全脚本",
	Long: `生成 zsh shell 的自动补全脚本。

如果当前环境尚未启用 shell 补全，需要先执行一次：

	echo "autoload -U compinit; compinit" >> ~/.zshrc

要在当前 shell 会话中加载补全：

	source <(zhongkui completion zsh)

要为每个新的会话加载补全，请执行一次：

#### Linux:

	zhongkui completion zsh > "${fpath[1]}/_zhongkui"

#### macOS:

	zhongkui completion zsh > $(brew --prefix)/share/zsh/site-functions/_zhongkui

重启 shell 后配置生效。`,
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
	Short: "生成 fish shell 的自动补全脚本",
	Long: `生成 fish shell 的自动补全脚本。

要在当前 shell 会话中加载补全：

	zhongkui completion fish | source

要为每个新的会话加载补全，请执行一次：

	zhongkui completion fish > ~/.config/fish/completions/zhongkui.fish

重启 shell 后配置生效。`,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), !completionNoDesc)
	},
}

// completionPowerShellCmd 生成 PowerShell 的自动补全脚本
var completionPowerShellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "生成 PowerShell 的自动补全脚本",
	Long: `生成 PowerShell 的自动补全脚本。

要在当前 shell 会话中加载补全：

	zhongkui completion powershell | Out-String | Invoke-Expression

要为每个新的会话加载补全，请将上述命令的输出添加到 PowerShell 配置文件中。`,
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
	completionBashCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "禁用补全描述")
	completionZshCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "禁用补全描述")
	completionFishCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "禁用补全描述")
	completionPowerShellCmd.Flags().BoolVar(&completionNoDesc, "no-descriptions", false, "禁用补全描述")

	completionCmd.AddCommand(completionBashCmd, completionZshCmd, completionFishCmd, completionPowerShellCmd)
	RootCmd.AddCommand(completionCmd)
}
