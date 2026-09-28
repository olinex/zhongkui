package cmd

import (
	// third-party
	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
)

// describeDepsCmd 打印外部依赖连通性状态的命令（内部 → 外部依赖方向）。
// 通过自带 PersistentPreRunE 旁路根命令的全局 eager 启动：仅构建一个含零网络
// 依赖的轻量注入器（command/viper/key-derivation/database logger/etcd logger/
// universal translator），使依赖宕机时命令仍能运行，探测结果以表格形式呈现。
//
// describeDepsCmd prints the connectivity status of external dependencies (the
// internal → external-dependency direction). It bypasses the root command's global
// eager startup via its own PersistentPreRunE: only a lightweight injector with zero
// network dependencies (command/viper/key-derivation/database logger/etcd logger/
// universal translator) is built, so the command still runs even when dependencies
// are down, and the results are rendered as a table.
var describeDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "cmd.describe.deps.short",
	// 旁路根命令 PersistentPreRunE 的全局 eager 启动，使依赖宕机时命令仍可运行。
	// Bypass the global eager startup of the root command's PersistentPreRunE, so the
	// command still runs even when dependencies are down.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	// 旁路根命令 PersistentPostRunE：轻量注入器未落入 context，无法交由根命令的
	// shutdown 流程清理，直接置空以避免空注入器断言 panic。
	// Bypass the root command's PersistentPostRunE: the lightweight injector is not
	// stored in the context, so it cannot be shut down by the root flow; override with
	// a no-op to avoid a nil-injector type-assertion panic.
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		// 构建仅含零网络依赖的轻量注入器，探测注册表按配置自行构造客户端。
		// Build a lightweight injector with zero network dependencies; the probe
		// registry constructs its own clients from config.
		i := do.New()
		provider.InjectCommand(i, cmd)
		provider.InjectViper(i)
		provider.InjectKeyDerivation(i)
		provider.InjectDatabaseLogger(i)
		provider.InjectEtcdLogger(i)
		provider.InjectUniversalTranslator(ctx, i)
		// 预解析 universal translator，使 translate.T 全局可用。
		// Resolve the universal translator so translate.T is globally available.
		_ = provider.MustInvokeUniversalTranslator(i)

		reg, err := provider.NewDependencyRegistry(ctx, i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return nil
		}
		_ = reg.Run(ctx)
		if err := reg.Render(func(key string) string { return translate.T(key, nil) }); err != nil {
			pterm.Error.Println(err.Error())
		}
		return nil
	},
}
