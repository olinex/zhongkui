package cmd

import (
	"encoding/json"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeSchedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "Print scheduled task configurations from etcd",
	Long: `Print the etcd-stored configuration of scheduled tasks. 
Use --name to specify one or more task types. If no --name is provided, all tasks are listed.

Examples:
  zhongkui describe scheduler
  zhongkui describe scheduler --name network:periodic:ipv4reassemble
  zhongkui describe scheduler --name network:periodic:ipv4reassemble --name network:periodic:ipv6reassemble`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		names := v.GetStringSlice("name")
		client := provider.MustInvokeEtcdClient(i)
		prefix := etcd.WithPrefix("scheduler/cfg/")

		if len(names) == 0 {
			rsp, err := client.Get(ctx, prefix, clientv3.WithPrefix())
			if err != nil {
				return errors.WithStack(err)
			}
			if len(rsp.Kvs) == 0 {
				pterm.Warning.Println("no scheduled task configs found in etcd")
				return nil
			}
			return renderSchedulerTable(prefix, rsp)
		}
		for _, name := range names {
			rsp, err := client.Get(ctx, prefix+name)
			if err != nil {
				return errors.WithStack(err)
			}
			if len(rsp.Kvs) == 0 {
				pterm.Warning.Printf("scheduled task config not found: %s\n", name)
				continue
			}
			if err := renderSchedulerTable(prefix, rsp); err != nil {
				return err
			}
		}
		return nil
	},
}

func renderSchedulerTable(prefix string, rsp *clientv3.GetResponse) error {
	data := pterm.TableData{
		{"name", "cronSpec", "queue", "timeout", "maxRetry", "enabled"},
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
			cfg.Queue,
			cfg.Timeout.String(),
			pterm.Sprintf("%d", cfg.MaxRetry),
			pterm.Sprintf("%t", cfg.Enabled),
		})
	}
	return errors.WithStack(pterm.DefaultTable.WithHasHeader().WithData(data).Render())
}

func init() {
	describeSchedulerCmd.Flags().StringArrayP("name", "n", nil, "task type name(s) to print (can be specified multiple times)")
}
