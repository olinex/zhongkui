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
	Short: "Create a network capturer",
	Long:  "Create a new network capturer with device configuration. Parameters can be provided via command-line flags or prompted interactively.",
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
	flags.String("namespace", "", "namespace for the capturer")
	flags.String("name", "", "name of the capturer")
	flags.String("version", "", "version of the capturer")
	// CapturerConfig fields
	flags.String("device", "", "network device name (e.g. eth0)")
	flags.Uint32("snap-len", 0, "snap length in bytes")
	flags.Bool("promisc", false, "enable promiscuous mode")
	flags.Int32("read-timeout", 0, "read timeout in milliseconds")
	flags.String("bpf-expr", "", "BPF filter expression (e.g. tcp port 80)")
	flags.Bool("enabled", false, "enable this capturer")
	// Meta flags
	flags.Duration("timeout", time.Minute, "timeout for database operations")
}
