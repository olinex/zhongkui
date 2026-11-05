package docs

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRenderCLI 校验 CLI 参考输出命令层级、全局参数与本地参数，且根命令不重复列全局参数。
func TestRenderCLI(t *testing.T) {
	root := &cobra.Command{Use: "zki", Short: "根命令"}
	root.PersistentFlags().StringP("config", "c", "", "配置文件路径")
	child := &cobra.Command{Use: "run", Short: "运行服务"}
	child.Flags().String("addr", "127.0.0.1", "监听地址")
	root.AddCommand(child)

	content := renderCLI(root)
	for _, want := range []string{
		"## 全局参数",
		"`-c, --config`",
		"### zki",
		"### zki run",
		"运行服务",
		"`--addr`",
		"监听地址",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("CLI 参考缺少 %q:\n%s", want, content)
		}
	}
	if strings.Count(content, "`-c, --config`") != 1 {
		t.Errorf("全局参数在根命令处重复列出:\n%s", content)
	}
	cli := cliReferenceProjection(root)
	if cli.Output != "generated/reference/cli.md" {
		t.Errorf("CLI 输出路径 = %q", cli.Output)
	}
}
