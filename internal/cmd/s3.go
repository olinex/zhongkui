package cmd

import (
	"context"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
)

var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "cmd.s3.short",
	Long:  "cmd.s3.long",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var s3LifecycleCmd = &cobra.Command{
	Use:   "lifecycle",
	Short: "cmd.s3.lifecycle.short",
	Long:  "cmd.s3.lifecycle.long",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// s3LifecycleApplyCmd 交互式采集生命周期规则并整体应用到默认存储桶（全量替换平台既有规则）
var s3LifecycleApplyCmd = &cobra.Command{
	Use:     "apply",
	Short:   "cmd.s3.lifecycle.apply.short",
	Long:    "cmd.s3.lifecycle.apply.long",
	Example: "cmd.s3.lifecycle.apply.example",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		client := provider.MustInvokeS3(i)
		if err := runLifecycleApply(ctx, client); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

// s3LifecycleStatusCmd 查看默认存储桶当前的生命周期规则
var s3LifecycleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "cmd.s3.lifecycle.status.short",
	Long:  "cmd.s3.lifecycle.status.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		client := provider.MustInvokeS3(i)
		cfg, err := client.GetLifecycle(ctx)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		if cfg == nil || len(cfg.Rules) == 0 {
			pterm.Info.Println(translate.T("cli.s3.lifecycle.no-rules", map[string]any{"Bucket": client.Bucket()}))
			return
		}
		printLifecycleRules(cfg)
	},
}

// s3LifecycleRemoveCmd 移除默认存储桶的全部生命周期规则
var s3LifecycleRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "cmd.s3.lifecycle.remove.short",
	Long:  "cmd.s3.lifecycle.remove.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		client := provider.MustInvokeS3(i)
		if err := client.RemoveLifecycle(ctx); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(translate.T("cli.s3.lifecycle.removed", map[string]any{"Bucket": client.Bucket()}))
	},
}

// runLifecycleApply 交互式采集规则清单：前缀从代码内置枚举选取，过期天数交互输入。
// 采集过程中随时可在动作列表中选择"保存并应用"结束采集并整体替换桶规则，
// 或选择"退出"放弃本次修改。
func runLifecycleApply(ctx context.Context, client *s3.S3Client) error {
	saveAction := translate.T("cli.s3.lifecycle.action-save", nil)
	exitAction := translate.T("cli.s3.lifecycle.action-exit", nil)
	selected := make(map[string]bool)
	var rules []s3.LifecycleRuleConfig
	for {
		action, err := promptLifecycleAction(saveAction, exitAction)
		if err != nil {
			return errors.WithStack(err)
		}
		switch action {
		case saveAction:
			if len(rules) == 0 {
				pterm.Warning.Println(translate.T("cli.s3.lifecycle.no-rules-selected", nil))
				continue
			}
			pterm.Info.Println(translate.T("cli.s3.lifecycle.rule-preview", map[string]any{"Count": len(rules), "Bucket": client.Bucket()}))
			printLifecycleRuleConfigs(rules)
			confirmed, err := pterm.DefaultInteractiveConfirm.Show(translate.T("cli.s3.lifecycle.prompt-confirm-apply", map[string]any{"Count": len(rules), "Bucket": client.Bucket()}))
			if err != nil {
				return errors.WithStack(err)
			}
			if !confirmed {
				pterm.Info.Println(translate.T("cli.s3.lifecycle.cancelled", nil))
				return nil
			}
			if err := client.SetLifecycle(ctx, &s3.LifecycleConfig{Rules: rules}); err != nil {
				return errors.WithStack(err)
			}
			pterm.Success.Println(translate.T("cli.s3.lifecycle.applied", map[string]any{"Count": len(rules), "Bucket": client.Bucket()}))
			return nil
		case exitAction:
			pterm.Info.Println(translate.T("cli.s3.lifecycle.cancelled", nil))
			return nil
		default:
			if selected[action] {
				pterm.Warning.Println(translate.T("cli.s3.lifecycle.duplicate-prefix", map[string]any{"Prefix": action}))
				continue
			}
			days, err := promptExpireDays()
			if err != nil {
				return err
			}
			selected[action] = true
			rules = append(rules, s3.LifecycleRuleConfig{
				ID:         lifecycleRuleID(action),
				Status:     "Enabled",
				Prefix:     action,
				ExpireDays: days,
			})
		}
	}
}

// promptLifecycleAction 交互式选择下一个动作：选择前缀继续添加规则，
// 或选择"保存并应用"/"退出"结束采集。
func promptLifecycleAction(saveAction, exitAction string) (string, error) {
	options := make([]string, 0, len(s3.LifecyclePrefixes)+2)
	options = append(options, s3.LifecyclePrefixes...)
	options = append(options, saveAction, exitAction)
	return pterm.DefaultInteractiveSelect.
		WithDefaultText(translate.T("cli.s3.lifecycle.prompt-prefix", nil)).
		WithOptions(options).
		Show()
}

// lifecycleRuleID 依据路径前缀自动生成稳定的规则 ID，如 "pcap/" → "pcap-expire"
func lifecycleRuleID(prefix string) string {
	return strings.ReplaceAll(strings.TrimSuffix(prefix, "/"), "/", "-") + "-expire"
}

// promptExpireDays 交互式输入正整数过期天数，非法输入循环重试
func promptExpireDays() (int, error) {
	for {
		raw, err := pterm.DefaultInteractiveTextInput.Show(translate.T("cli.s3.lifecycle.prompt-expire-days", nil))
		if err != nil {
			return 0, errors.WithStack(err)
		}
		days, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || days <= 0 {
			pterm.Warning.Println(translate.T("cli.s3.lifecycle.invalid-days", nil))
			continue
		}
		return days, nil
	}
}

// lifecycleTableHeader 返回规则表格表头行（ID / STATUS / PREFIX / EXPIRE DAYS）
func lifecycleTableHeader() [][]string {
	return [][]string{
		{
			translate.T("cli.s3.lifecycle.table-id", nil),
			translate.T("cli.s3.lifecycle.table-status", nil),
			translate.T("cli.s3.lifecycle.table-prefix", nil),
			translate.T("cli.s3.lifecycle.table-expire-days", nil),
		},
	}
}

// printLifecycleRules 以表格形式输出已应用的规则（status 命令）
func printLifecycleRules(cfg *lifecycle.Configuration) {
	rows := lifecycleTableHeader()
	for _, rule := range cfg.Rules {
		days := "-"
		if rule.Expiration.Days > 0 {
			days = strconv.Itoa(int(rule.Expiration.Days))
		}
		rows = append(rows, []string{rule.ID, rule.Status, rulePrefix(rule), days})
	}
	_ = pterm.DefaultTable.WithHasHeader().WithData(rows).Render()
}

// rulePrefix 取规则的匹配前缀：优先 Filter.Prefix（现代表示），回退顶层 Prefix（legacy）
func rulePrefix(rule lifecycle.Rule) string {
	if rule.RuleFilter.Prefix != "" {
		return rule.RuleFilter.Prefix
	}
	return rule.Prefix
}

// printLifecycleRuleConfigs 以表格形式输出待应用的规则清单（apply 预览）
func printLifecycleRuleConfigs(rules []s3.LifecycleRuleConfig) {
	rows := lifecycleTableHeader()
	for _, rule := range rules {
		days := "-"
		if rule.ExpireDays > 0 {
			days = strconv.Itoa(rule.ExpireDays)
		}
		rows = append(rows, []string{rule.ID, rule.Status, rule.Prefix, days})
	}
	_ = pterm.DefaultTable.WithHasHeader().WithData(rows).Render()
}
