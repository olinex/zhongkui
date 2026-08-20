package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// zhUsageTemplate / enUsageTemplate 是本地化的 cobra usage 模板。
// 结构复刻 cobra v1.10.2 的 defaultUsageTemplate，仅本地化固定文案。
// 模板在渲染时才由 cobra 解析（见 cobra.SetUsageTemplate），
// 因此模板中的 {{.CommandPath}} 等动作可正常执行。
const (
	zhUsageTemplate = `用法:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [命令]{{end}}{{if gt (len .Aliases) 0}}

别名:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

可用命令:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

其他命令:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

标志:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局标志:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

更多帮助主题:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

使用 "{{.CommandPath}} [命令] --help" 查看命令的更多信息。{{end}}`

	enUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}`
)

// localizedUsageTemplate 返回本地化的 cobra usage 模板。
// 模板含 Go 模板动作（{{if .Runnable}} 等），渲染数据来自 cobra 的命令上下文，
// 无法经 go-i18n 预渲染（其模板上下文无 .Runnable/.CommandPath 等字段），
// 故此处直接按默认语言返回 Go 常量模板。
func localizedUsageTemplate() string {
	if base.DefaultLanguage.String() == "en" {
		return enUsageTemplate
	}
	return zhUsageTemplate
}

// localizeCmdTree 遍历命令树，将命令描述与标志说明本地化为 CLI 语言
// 命令字段（Short/Long/Example）与标志 usage 存储的即翻译键本身（cmd.*/flag.*），
// 直接以字段值作为 key 经 translate.T 解析；未命中翻译文件时返回 key 本身，
// 缺失项由命令树键完整性测试（TestCommandTreeKeysComplete）兜底暴露
func localizeCmdTree() {
	// 本地化 cobra 内置 usage 模板（Usage:/Aliases:/Flags: 等固定文案），
	// 子命令经父链继承根命令的模板，help 命令由 SetHelpCommand 单独挂载需单独设置
	usageTemplate := localizedUsageTemplate()
	RootCmd.SetUsageTemplate(usageTemplate)
	helpCmd.SetUsageTemplate(usageTemplate)

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Short = translate.T(c.Short, nil)
		c.Long = translate.T(c.Long, nil)
		c.Example = translate.T(c.Example, nil)

		translateFlags := func(fs *pflag.FlagSet) {
			fs.VisitAll(func(f *pflag.Flag) {
				f.Usage = translate.T(f.Usage, nil)
			})
		}
		translateFlags(c.LocalFlags())
		translateFlags(c.InheritedFlags())

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
	// help 命令由 SetHelpCommand 注册（Execute 阶段才挂载到命令树），需单独本地化
	walk(helpCmd)
}
