package cmd

import (
	"encoding/json"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeSchedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "cmd.describe.scheduler.short",
	Long:  "cmd.describe.scheduler.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		names := v.GetStringSlice("name")
		client := provider.MustInvokeEtcdClient(i)

		if len(names) == 0 {
			rsp, err := client.Get(ctx, pipeline.SchedulerCfgEtcdPrefix, clientv3.WithPrefix())
			if err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			if len(rsp.Kvs) == 0 {
				pterm.Warning.Println(translate.T("cli.scheduler.none-found", nil))
				return
			}
			renderSchedulerTable(pipeline.SchedulerCfgEtcdPrefix, rsp)
		}
		for _, name := range names {
			rsp, err := client.Get(ctx, pipeline.SchedulerCfgEtcdPrefix+name)
			if err != nil {
				pterm.Error.Println(err.Error())
				return
			}
			if len(rsp.Kvs) == 0 {
				pterm.Warning.Println(translate.T("cli.scheduler.not-found", map[string]any{"Name": name}))
				continue
			}
			if err := renderSchedulerTable(pipeline.SchedulerCfgEtcdPrefix, rsp); err != nil {
				pterm.Error.Println(err.Error())
				return
			}
		}
	},
}

func renderSchedulerTable(prefix string, rsp *clientv3.GetResponse) error {
	data := pterm.TableData{
		{
			translate.T("cli.scheduler.col-name", nil),
			translate.T("cli.scheduler.col-cronspec", nil),
			translate.T("cli.scheduler.col-interval", nil),
			translate.T("cli.scheduler.col-queue", nil),
			translate.T("cli.scheduler.col-timeout", nil),
			translate.T("cli.scheduler.col-maxretry", nil),
			translate.T("cli.scheduler.col-enabled", nil),
		},
	}
	for _, kv := range rsp.Kvs {
		var cfg pipeline.SchedulerTaskConfig
		if err := json.Unmarshal(kv.Value, &cfg); err != nil {
			return errors.WithStack(err)
		}
		name := strings.TrimPrefix(string(kv.Key), prefix)
		data = append(data, []string{
			name,
			cfg.CronSpec,
			cfg.Interval.String(),
			cfg.Queue,
			cfg.Timeout.String(),
			pterm.Sprintf("%d", cfg.MaxRetry),
			pterm.Sprintf("%t", cfg.Enabled),
		})
	}
	return errors.WithStack(pterm.DefaultTable.WithHasHeader().WithData(data).Render())
}

func init() {
	describeSchedulerCmd.Flags().StringArrayP("name", "n", nil, "flag.describe.scheduler.name")
}
