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

		cwd, err := os.Getwd()
		if err != nil {
			pterm.Error.Println(err.Error())
			return errors.WithStack(err)
		}
		// 导出 Diátaxis 分类文档（叙事页 + 参考页 + 索引），无需数据库 / Redis / etcd。
		if err := docs.Export(ctx, docs.Options{ProjectRoot: cwd, Root: docs.Root}); err != nil {
			pterm.Error.Println(err.Error())
			return nil
		}
		pterm.Success.Println(translate.T("cli.docs.generated", map[string]any{"Dir": docs.Root}))
		return nil
	},
}
