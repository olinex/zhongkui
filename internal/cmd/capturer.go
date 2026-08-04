package cmd

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	"zhongkui.com/zhongkui/internal/provider"
)

var createCapturerCmd = &cobra.Command{
	Use:   "capturer",
	Short: "创建网络抓包器",
	Long:  "创建带有设备配置的新的网络抓包器。参数可以通过命令行参数提供，或通过交互式提示输入。",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		timeout := v.GetDuration("timeout")

		var in routes.CapturerCreateIn
		if err := cliinput.Extract(v, &in, ""); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()

		db := provider.MustInvokeCoreDatabase(i)
		client := provider.MustInvokeEtcdClient(i)

		instance := core.Capturer{
			NamespaceName: in.Namespace,
			Name:          in.Name,
			Version:       in.Version,
		}

		if err := instance.CreateWithConfig(db.WithContext(ctx), client, in.Config.ToGRPC()); err != nil {
			pterm.Error.Printf("cannot create capturer: %v\n", err)
			return
		}

		pterm.Success.Printf("capturer %s/%s (%s) created successfully\n", in.Namespace, in.Name, in.Version)
	},
}

func init() {
	flags := createCapturerCmd.Flags()
	flags.String("namespace", "", "抓包器所属命名空间")
	flags.String("name", "", "抓包器名称")
	flags.String("version", "", "抓包器版本")
	// CapturerConfig fields
	flags.String("device", "", "网络设备名称（如 eth0）")
	flags.Uint32("snap-len", 0, "抓包长度（字节）")
	flags.Bool("promisc", false, "启用混杂模式")
	flags.Int32("read-timeout", 0, "读取超时时间（毫秒）")
	flags.String("bpf-expr", "", "BPF 过滤表达式（如 tcp port 80）")
	flags.Bool("enabled", false, "启用该抓包器")
	// Meta flags
	flags.Duration("timeout", time.Minute, "数据库操作超时时间")
}
