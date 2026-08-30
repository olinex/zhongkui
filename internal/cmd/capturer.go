package cmd

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/cliinput"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

var createCapturerCmd = &cobra.Command{
	Use:   "capturer",
	Short: "cmd.create.capturer.short",
	Long:  "cmd.create.capturer.long",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)

		timeout := v.GetDuration("timeout")

		var in views.CapturerCreateIn
		if err := cliinput.Extract(v, validator.Validate, &in, ""); err != nil {
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
			pterm.Error.Println(translate.T("cli.capturer.create-failed", map[string]any{"Err": err}))
			return
		}

		pterm.Success.Println(translate.T("cli.capturer.created", map[string]any{"Namespace": in.Namespace, "Name": in.Name, "Version": in.Version}))
	},
}

func init() {
	flags := createCapturerCmd.Flags()
	flags.String("namespace", "", "flag.create.capturer.namespace")
	flags.String("name", "", "flag.create.capturer.name")
	flags.String("version", "", "flag.create.capturer.version")
	// CapturerConfig fields
	flags.String("device", "", "flag.create.capturer.device")
	flags.Uint32("snap-len", 0, "flag.create.capturer.snap-len")
	flags.Bool("promisc", false, "flag.create.capturer.promisc")
	flags.Int32("read-timeout", 0, "flag.create.capturer.read-timeout")
	flags.String("bpf-expr", "", "flag.create.capturer.bpf-expr")
	flags.Bool("enabled", false, "flag.create.capturer.enabled")
	// Meta flags
	flags.Duration("timeout", time.Minute, "flag.create.capturer.timeout")
}
