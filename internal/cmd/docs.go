package cmd

import (
	// 标准库
	"os"

	// 第三方
	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/docs"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// createDocsCmd 依据文档注册表导出 Diátaxis 分类文档的命令。
// 通过自带 PersistentPreRunE/PostRunE 旁路根命令的全局 eager 启动，仅构建一个含
// command/viper/key-derivation/universal translator 的轻量注入器，使 translate.T 可用，
// 从而在无需数据库、etcd 等外部依赖的情况下完成文档生成。
var createDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "cmd.create.docs.short",
	Long:  "cmd.create.docs.long",
	// 旁路根命令 PersistentPreRunE 的全局 eager 启动。
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	// 旁路根命令 PersistentPostRunE：轻量注入器未落入 context，直接置空以避免空注入器断言 panic。
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		// 构建仅含零网络依赖的轻量注入器，使 translate.T 全局可用。
		i := do.New()
		provider.InjectCommand(i, cmd)
		provider.InjectViper(i)
		provider.InjectKeyDerivation(i)
		provider.InjectUniversalTranslator(ctx, i)
		_ = provider.MustInvokeUniversalTranslator(i)

		// 命令描述与参数说明经本地化后才能进入 CLI 参考页。
		localizeCmdTree()
		cwd, err := os.Getwd()
		if err != nil {
			return errors.WithStack(err)
		}
		// --check 只校验不写盘：CI 据此发现生成物漂移。
		check, err := cmd.Flags().GetBool("check")
		if err != nil {
			return errors.WithStack(err)
		}
		if check {
			if err := docs.Check(ctx, cwd, cmd.Root()); err != nil {
				return errors.WithStack(err)
			}
			pterm.Success.Println(translate.T("cli.docs.check-ok", nil))
			return nil
		}
		// 导出文档（叙述页 + 参考投影 + 参考页 + 总目录 + 导航文件），无需数据库 / Redis / etcd。
		if err := docs.Export(ctx, docs.Options{ProjectRoot: cwd, Root: docs.Root, RootCommand: cmd.Root()}); err != nil {
			return errors.WithStack(err)
		}
		pterm.Success.Println(translate.T("cli.docs.generated", map[string]any{"Dir": docs.Root}))
		return nil
	},
}

// init 注册文档导出命令的局部参数。
func init() {
	createDocsCmd.Flags().Bool("check", false, "flag.create.docs.check")
}
