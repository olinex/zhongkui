package docs

import (
	// 标准库
	"sort"
	"strings"

	// 第三方
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cliRootName 是文档中展示的命令名（与发布二进制 bin/zki 一致）。
const cliRootName = "zki"

// cliReferenceProjection 生成 CLI 参考页：按命令树列出用途、别名、示例与参数。
func cliReferenceProjection(root *cobra.Command) Projection {
	return Projection{
		Title:       "CLI 参考",
		Summary:     "按命令树列出全部子命令、用途、别名、示例与参数。",
		Roles:       []Role{RoleOps, RoleDev},
		Scene:       "需要查命令用法、参数默认值或子命令层级时阅读。",
		Output:      GeneratedRoot + "/reference/cli.md",
		SourceLabel: "internal/cmd",
		Content:     renderCLI(root),
	}
}

// renderCLI 生成 CLI 参考正文：全局参数 + 逐命令小节。
func renderCLI(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("## 全局参数\n\n")
	b.WriteString("以下参数对所有命令生效。\n\n")
	writeFlagTable(&b, collectFlags(nil, root.PersistentFlags()))

	b.WriteString("\n## 命令\n\n")
	writeCommand(&b, root, cliRootName)
	for _, child := range sortedCommands(root) {
		writeCommandTree(&b, child, cliRootName+" "+child.Name())
	}
	return b.String()
}

// writeCommandTree 递归写出命令及其子命令。
func writeCommandTree(b *strings.Builder, cmd *cobra.Command, path string) {
	writeCommand(b, cmd, path)
	for _, child := range sortedCommands(cmd) {
		writeCommandTree(b, child, path+" "+child.Name())
	}
}

// writeCommand 写出单个命令的用途、别名、示例与本地参数。
func writeCommand(b *strings.Builder, cmd *cobra.Command, path string) {
	b.WriteString("### ")
	b.WriteString(path)
	b.WriteString("\n\n")
	if cmd.Short != "" {
		b.WriteString(cmd.Short)
		b.WriteString("\n\n")
	}
	if cmd.Long != "" && cmd.Long != cmd.Short {
		b.WriteString(cmd.Long)
		b.WriteString("\n\n")
	}
	if len(cmd.Aliases) > 0 {
		b.WriteString("别名：`")
		b.WriteString(strings.Join(cmd.Aliases, "`, `"))
		b.WriteString("`\n\n")
	}
	if cmd.Example != "" {
		b.WriteString("示例：\n\n```bash\n")
		b.WriteString(cmd.Example)
		b.WriteString("\n```\n\n")
	}
	// 根命令的持久参数已在「全局参数」中列出，此处不重复。
	if path == cliRootName {
		return
	}
	flags := collectFlags(cmd.LocalNonPersistentFlags(), cmd.PersistentFlags())
	if len(flags) > 0 {
		b.WriteString("参数：\n\n")
		writeFlagTable(b, flags)
		b.WriteString("\n")
	}
}

// sortedCommands 返回未隐藏的子命令，按命令名排序。
func sortedCommands(cmd *cobra.Command) []*cobra.Command {
	children := cmd.Commands()
	out := make([]*cobra.Command, 0, len(children))
	for _, child := range children {
		if child.Hidden {
			continue
		}
		out = append(out, child)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// collectFlags 合并两组参数并按参数名排序去重。
func collectFlags(first, second *pflag.FlagSet) []*pflag.Flag {
	seen := make(map[string]bool)
	var out []*pflag.Flag
	add := func(fs *pflag.FlagSet) {
		if fs == nil {
			return
		}
		fs.VisitAll(func(flag *pflag.Flag) {
			if flag.Hidden || seen[flag.Name] {
				return
			}
			seen[flag.Name] = true
			out = append(out, flag)
		})
	}
	add(first)
	add(second)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// writeFlagTable 写出参数表：参数、默认值、说明。
func writeFlagTable(b *strings.Builder, flags []*pflag.Flag) {
	if len(flags) == 0 {
		b.WriteString("无。\n")
		return
	}
	b.WriteString("| 参数 | 默认值 | 说明 |\n| --- | --- | --- |\n")
	for _, flag := range flags {
		name := "--" + flag.Name
		if flag.Shorthand != "" {
			name = "-" + flag.Shorthand + ", " + name
		}
		defaultValue := flag.DefValue
		if defaultValue == "" {
			defaultValue = "-"
		}
		b.WriteString("| `")
		b.WriteString(mdCell(name))
		b.WriteString("` | ")
		b.WriteString(mdCell(defaultValue))
		b.WriteString(" | ")
		b.WriteString(mdCell(flag.Usage))
		b.WriteString(" |\n")
	}
}
