package cmd

import (
	"encoding/json"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeSchedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "从 etcd 打印定时任务配置",
	Long: `从 etcd 打印定时任务的配置。
使用 --name 指定一个或多个任务类型。未提供 --name 时，列出所有任务。

示例：
  zhongkui describe scheduler
  zhongkui describe scheduler --name network:periodic:ipv4reassemble
  zhongkui describe scheduler --name network:periodic:ipv4reassemble --name network:periodic:ipv6reassemble`,
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
				pterm.Warning.Println(cliT("cli.scheduler.none-found", "etcd 中未找到任何定时任务配置"))
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
				pterm.Warning.Println(cliT("cli.scheduler.not-found", "未找到定时任务配置：{{.Name}}", map[string]any{"Name": name}))
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
			cliT("cli.scheduler.col-name", "name"),
			cliT("cli.scheduler.col-cronspec", "cronSpec"),
			cliT("cli.scheduler.col-interval", "interval"),
			cliT("cli.scheduler.col-queue", "queue"),
			cliT("cli.scheduler.col-timeout", "timeout"),
			cliT("cli.scheduler.col-maxretry", "maxRetry"),
			cliT("cli.scheduler.col-enabled", "enabled"),
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
	describeSchedulerCmd.Flags().StringArrayP("name", "n", nil, "要打印的任务类型名称（可多次指定）")
}
