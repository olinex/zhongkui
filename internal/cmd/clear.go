package cmd

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/migrations"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

var clearDatabaseCmd = &cobra.Command{
	Use:   "database",
	Short: "清空所有数据库表（回滚所有迁移）",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		coreDB := provider.MustInvokeCoreDatabase(injector)
		rawDB := provider.MustInvokeRawDatabase(injector)
		timeout := v.GetDuration("timeout")

		random, err := crypto.GenerateRandomString(6)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		err = staticInputDoubleCheck(random)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Warning.Println(cliT("cli.clear.rolling-back", "正在回滚所有迁移..."))
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		type dbInfo struct {
			name string
			db   *migrate.Migrator
		}
		var migrators []dbInfo

		if m, err := createClearMigrator(provider.CoreDbPrefix, coreDB); err != nil {
			pterm.Error.Println(cliT("cli.clear.connect-failed", "连接 {{.DB}} 数据库失败：{{.Err}}", map[string]any{"DB": provider.CoreDbPrefix, "Err": err}))
			return
		} else {
			migrators = append(migrators, dbInfo{"core", m})
		}
		if m, err := createClearMigrator(provider.RawDbPrefix, rawDB); err != nil {
			pterm.Error.Println(cliT("cli.clear.connect-failed", "连接 {{.DB}} 数据库失败：{{.Err}}", map[string]any{"DB": provider.RawDbPrefix, "Err": err}))
			return
		} else {
			migrators = append(migrators, dbInfo{"raw", m})
		}

		for _, mi := range migrators {
			pterm.Info.Println(cliT("cli.clear.rolling-back-db", "正在回滚 {{.DB}}...", map[string]any{"DB": mi.name}))
			if err := mi.db.Down(ctx); err != nil {
				pterm.Error.Println(cliT("cli.clear.rollback-failed", "{{.DB}} 回滚失败：{{.Err}}", map[string]any{"DB": mi.name, "Err": err}))
				mi.db.Close()
				return
			}
			mi.db.Close()
		}
		pterm.Success.Println(cliT("cli.clear.database-complete", "数据库清空完成"))
	},
}

func createClearMigrator(dbName string, gormDB *database.DB) (*migrate.Migrator, error) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, errors.Newf("failed to get database connection: %v", err)
	}

	dialector := gormDB.Dialector
	driverName := "unknown"
	switch dialector.Name() {
	case "postgres":
		driverName = "postgres"
	case "clickhouse":
		driverName = "clickhouse"
	}

	migrationFS := migrations.MigrationFor(dbName)
	goose.SetBaseFS(migrationFS)
	cfg := migrate.NewConfig(driverName, "")
	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

type etcdClearCategory struct {
	name   string
	prefix string
}

// etcdKeyEntry 表示 etcd 缓存中的一个键值条目及其所属类别
type etcdKeyEntry struct {
	category string
	key      string
	value    string
}

var etcdClearCategories = []etcdClearCategory{
	{name: "capturer config", prefix: core.ReporterEtcdPrefix},
	{name: "distributed watermark", prefix: etcd.ClusterEtcdPrefix},
	{name: "scheduler config", prefix: pipeline.SchedulerCfgEtcdPrefix},
}

var clearEtcdCmd = &cobra.Command{
	Use:   "etcd",
	Short: "清除 etcd 缓存（抓包器配置、水印、调度器配置）",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, injector := cmdToolBox(cmd)
		v := provider.MustInvokeViper(injector)
		client := provider.MustInvokeEtcdClient(injector)
		timeout := v.GetDuration("timeout")

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// 1. 收集所有类别的 key
		var allKeys []etcdKeyEntry
		for _, cat := range etcdClearCategories {
			rsp, err := client.Get(ctx, cat.prefix, clientv3.WithPrefix())
			if err != nil {
				pterm.Error.Println(cliT("cli.etcd.get-keys-failed", "获取 {{.Category}} 键失败：{{.Err}}", map[string]any{"Category": cat.name, "Err": err}))
				return
			}
			for _, kv := range rsp.Kvs {
				allKeys = append(allKeys, etcdKeyEntry{
					category: cat.name,
					key:      string(kv.Key),
					value:    string(kv.Value),
				})
			}
		}

		if len(allKeys) == 0 {
			pterm.Warning.Println(cliT("cli.etcd.no-keys-found", "未找到任何 etcd 缓存键"))
			return
		}

		// 2. 按类别分组打印
		pterm.Info.Println(cliT("cli.etcd.keys-header", "=== etcd 缓存键 ==="))
		currentCat := ""
		for i, entry := range allKeys {
			if entry.category != currentCat {
				currentCat = entry.category
				pterm.Println(pterm.Cyan(pterm.Sprintf("\n[%s]", currentCat)))
			}
			pterm.Printf("  [%d] %s\n", i+1, entry.key)
			if len(entry.value) > 0 {
				displayVal := truncateDisplayValue(entry.value, 120)
				pterm.Printf("      =>  %s\n", pterm.Gray(displayVal))
			}
		}
		pterm.Println()

		// 3. 交互式选择要清除的 key
		selected := selectKeysToDelete(allKeys)

		// 4. 二次确认
		if err := confirmDeleteKeys(selected, allKeys); err != nil {
			pterm.Error.Println(err.Error())
			return
		}

		// 5. 执行删除
		for _, idx := range selected {
			key := allKeys[idx].key
			if _, err := client.Delete(ctx, key); err != nil {
				pterm.Error.Println(cliT("cli.etcd.delete-failed", "删除 {{.Key}} 失败：{{.Err}}", map[string]any{"Key": key, "Err": err}))
				return
			}
			pterm.Success.Println(cliT("cli.etcd.deleted", "已删除 {{.Key}}", map[string]any{"Key": key}))
		}
		pterm.Success.Println(cliT("cli.etcd.clear-complete", "etcd 缓存清除完成"))
	},
}

// selectKeysToDelete 交互式选择要删除的 key，返回选中 key 在 allKeys 中的索引
func selectKeysToDelete(allKeys []etcdKeyEntry) []int {
	total := len(allKeys)
	prompt := cliT(
		"cli.etcd.select-keys",
		"输入要删除的键编号（如 1,3,5-{{.Total}}）或输入 'all' 删除全部 {{.Total}} 个键",
		map[string]any{"Total": total},
	)
	input, err := pterm.DefaultInteractiveTextInput.Show(prompt)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}

	input = strings.TrimSpace(input)
	if input == "" {
		pterm.Info.Println(cliT("cli.etcd.no-keys", "未选择任何键，无需删除"))
		return nil
	}

	if strings.ToLower(input) == "all" {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	// 解析输入：支持 "1,3,5-8" 格式
	parts := strings.Split(input, ",")
	var selected []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			startStr := strings.TrimSpace(rangeParts[0])
			endStr := strings.TrimSpace(rangeParts[1])
			start, err1 := strconv.Atoi(startStr)
			end, err2 := strconv.Atoi(endStr)
			if err1 != nil || err2 != nil || start < 1 || end > total || start > end {
				pterm.Error.Println(cliT("cli.etcd.invalid-range", "无效的范围：{{.Range}}", map[string]any{"Range": part}))
				return nil
			}
			for i := start; i <= end; i++ {
				selected = append(selected, i-1)
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > total {
				pterm.Error.Println(cliT("cli.etcd.invalid-number", "无效的键编号：{{.Number}}", map[string]any{"Number": part}))
				return nil
			}
			selected = append(selected, n-1)
		}
	}
	return selected
}

// confirmDeleteKeys 二次确认，打印待删除的 key 并要求用户输入随机字符串确认
func confirmDeleteKeys(selected []int, allKeys []etcdKeyEntry) error {
	if len(selected) == 0 {
		return nil
	}

	pterm.Warning.Println(cliT("cli.etcd.delete-count", "即将删除 {{.Count}} 个键：", map[string]any{"Count": len(selected)}))
	for _, idx := range selected {
		pterm.Warning.Println(cliT("cli.etcd.delete-item", "  - {{.Key}}", map[string]any{"Key": allKeys[idx].key}))
	}

	random, err := crypto.GenerateRandomString(6)
	if err != nil {
		return errors.WithStack(err)
	}
	return staticInputDoubleCheck(random)
}

// truncateDisplayValue 截断过长的显示值
func truncateDisplayValue(value string, maxLen int) string {
	// 去除换行，单行显示
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", "")
	if len(value) > maxLen {
		return value[:maxLen] + "..."
	}
	return value
}

func init() {
	flags := clearDatabaseCmd.Flags()
	flags.Duration("timeout", time.Minute, "清空数据库的超时时间")

	flags = clearEtcdCmd.Flags()
	flags.Duration("timeout", time.Minute, "清空 etcd 缓存的超时时间")
}
