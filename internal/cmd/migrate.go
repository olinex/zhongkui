package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/migrations"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "应用所有待执行的迁移",
	Run:   runMigrateCmd("up"),
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "回滚所有已应用的迁移",
	Run:   runMigrateCmd("down"),
}

var migrateUpToCmd = &cobra.Command{
	Use:   "up-to [version]",
	Short: "迁移到指定版本",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("up-to"),
}

var migrateDownToCmd = &cobra.Command{
	Use:   "down-to [version]",
	Short: "回滚到指定版本",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("down-to"),
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示迁移状态",
	Run:   runMigrateCmd("status"),
}

var migrateDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "在临时数据库上试运行迁移",
	Run:   runDryRunMigrateCmd,
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "比较数据库结构差异",
	Run:   runDiffMigrateCmd,
}

var migrateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "检查模型定义与数据库结构是否一致",
	Run:   runCheckMigrateCmd,
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "在迁移目录中创建 SQL 迁移文件",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		dbTarget := targetFromViper(v)
		name := args[0]

		for _, dbName := range targetDBs(dbTarget) {
			filePath, err := createMigrationFile(dbName, name)
			if err != nil {
				pterm.Error.Println(cliT("cli.migrate.create-failed", "{{.DB}}：{{.Err}}", map[string]any{"DB": dbName, "Err": err}))
				return
			}
			pterm.Success.Println(cliT("cli.migrate.created", "已创建 {{.Path}}", map[string]any{"Path": filePath}))
		}
	},
}

func createMigrationFile(dbName, name string) (string, error) {
	dir := filepath.Join("internal", "migrations", dbName)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", errors.Newf("failed to create migrations directory: %v", err)
	}

	nextVersion := nextMigrationVersion(dir)
	filename := fmt.Sprintf("%03d_%s.sql", nextVersion, name)
	filePath := filepath.Join(dir, filename)

	content := "-- +goose Up\n-- SQL statements...\n\n-- +goose Down\n-- SQL statements...\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", errors.Newf("failed to create migration file: %v", err)
	}

	return filePath, nil
}

func nextMigrationVersion(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}

	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		idx := strings.IndexByte(name, '_')
		if idx <= 0 {
			continue
		}
		if v, err := strconv.Atoi(name[:idx]); err == nil && v > maxVersion {
			maxVersion = v
		}
	}

	return maxVersion + 1
}

func runMigrateCmd(action string) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)

		timeout := v.GetDuration("timeout")
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		dbTarget := targetFromViper(v)

		for _, dbName := range targetDBs(dbTarget) {
			gormDB := provider.MustInvokeDatabase(i, dbName)
			pterm.DefaultSection.Println(cliT("cli.migrate.section", "{{.Action}}（{{.DB}}）", map[string]any{"Action": action, "DB": dbName}))

			if err := migrateDB(ctx, v, gormDB, dbName, args, action); err != nil {
				pterm.Error.Println(cliT("cli.migrate.action-failed", "{{.DB}} {{.Action}} 失败：{{.Err}}", map[string]any{"DB": dbName, "Action": action, "Err": err}))
				return
			}
		}
		pterm.Success.Println(cliT("cli.migrate.completed", "迁移全部执行成功"))
	}
}

func migrateDB(ctx context.Context, v *viper.Viper, gormDB *database.DB, dbName string, args []string, action string) error {
	m, err := createMigratorForDB(v, gormDB, dbName)
	if err != nil {
		return err
	}
	defer m.Close()

	switch action {
	case "up":
		return m.Up(ctx)
	case "down":
		return m.Down(ctx)
	case "status":
		return m.Status(ctx)
	case "up-to":
		v := parseInt64(args[0])
		return m.UpTo(ctx, v)
	case "down-to":
		v := parseInt64(args[0])
		return m.DownTo(ctx, v)
	default:
		return errors.Newf("unknown action: %s", action)
	}
}

func createMigratorForDB(v *viper.Viper, gormDB *database.DB, dbName string) (*migrate.Migrator, error) {
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
	case "sqlite":
		driverName = "sqlite3"
	}

	verbose := v.GetBool("verbose")
	cfg := migrate.NewConfig(driverName, "").
		WithName(dbName).
		WithVerbose(verbose).
		WithMigrations(migrations.MigrationFor(dbName), ".")

	goose.SetBaseFS(migrations.MigrationFor(dbName))

	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

func runDryRunMigrateCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Println(cliT("cli.migrate.dry-run-section", "试运行（dry-run）{{.DB}}", map[string]any{"DB": dbName}))
		if err := migrateDryRunDB(ctx, v, dbName); err != nil {
			pterm.Error.Println(cliT("cli.migrate.dry-run-db-failed", "dry-run {{.DB}}：{{.Err}}", map[string]any{"DB": dbName, "Err": err}))
		}
	}
}

func migrateDryRunDB(ctx context.Context, v *viper.Viper, dbName string) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithName(dbName).
		WithKeepTemp(false).
		WithMigrations(migrations.MigrationFor(dbName), ".")

	result, err := migrate.DryRun(ctx, mc)
	if err != nil {
		return errors.Newf("dry-run failed: %v", err)
	}
	if result.Success {
		pterm.Success.Println(cliT("cli.migrate.dry-run-succeeded", "dry-run 执行成功"))
	} else {
		pterm.Error.Println(cliT("cli.migrate.dry-run-failed", "dry-run 失败：{{.Err}}", map[string]any{"Err": result.Error}))
	}
	return nil
}

func runDiffMigrateCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Println(cliT("cli.migrate.diff-section", "结构对比（diff）{{.DB}}", map[string]any{"DB": dbName}))
		if err := migrateDiffDB(ctx, v, dbName); err != nil {
			pterm.Error.Println(cliT("cli.migrate.diff-failed", "diff {{.DB}}：{{.Err}}", map[string]any{"DB": dbName, "Err": err}))
		}
	}
}

func runCheckMigrateCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	keepTemp := v.GetBool("keep-temp")
	for _, dbName := range targetDBs(dbTarget) {
		manager := provider.MustInvokeSchemaManager(i, dbName)
		pterm.DefaultSection.Println(cliT("cli.migrate.check-section", "结构检查（check）{{.DB}}", map[string]any{"DB": dbName}))
		if err := migrateCheckDB(ctx, v, dbName, keepTemp, manager.Models()); err != nil {
			pterm.Error.Println(cliT("cli.migrate.check-failed", "check {{.DB}}：{{.Err}}", map[string]any{"DB": dbName, "Err": err}))
		}
	}
}

func migrateCheckDB(ctx context.Context, v *viper.Viper, dbName string, keepTemp bool, models []any) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithName(dbName).
		WithKeepTemp(keepTemp).
		WithMigrations(migrations.MigrationFor(dbName), ".")

	return migrate.Check(ctx, mc, models)
}

func migrateDiffDB(ctx context.Context, v *viper.Viper, dbName string) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithName(dbName).
		WithKeepTemp(false).
		WithMigrations(migrations.MigrationFor(dbName), ".")

	return migrate.Diff(ctx, mc, mc)
}

func targetDBs(target string) []string {
	switch target {
	case provider.CoreDbPrefix:
		return []string{provider.CoreDbPrefix}
	case provider.RawDbPrefix:
		return []string{provider.RawDbPrefix}
	default:
		return []string{provider.CoreDbPrefix, provider.RawDbPrefix}
	}
}

func getDBConfig(v *viper.Viper, dbName string) (*database.Config, error) {
	cfg, err := database.ExtractConfig(v, validator.Validate, dbName)
	if err != nil {
		return nil, errors.Newf("failed to extract %s database config: %v", dbName, err)
	}
	return cfg, nil
}

func targetFromViper(v *viper.Viper) string {
	target := v.GetString("db")
	return target
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func init() {
	for _, c := range []*cobra.Command{
		migrateUpCmd, migrateDownCmd, migrateUpToCmd, migrateDownToCmd,
		migrateStatusCmd, migrateDryRunCmd, migrateDiffCmd, migrateCheckCmd, migrateCreateCmd,
	} {
		c.Flags().String("db", "all", "目标数据库：core、raw、all")
		c.Flags().Bool("verbose", false, "打印迁移 SQL")
		c.Flags().Duration("timeout", 0, "迁移超时时间（0 表示不超时）")
	}

	migrateDownCmd.Flags().Bool("force", false, "跳过确认")
	migrateDryRunCmd.Flags().Bool("keep-temp", false, "保留临时数据库")
	migrateDiffCmd.Flags().Bool("keep-temp", false, "保留临时数据库")
	migrateCheckCmd.Flags().Bool("keep-temp", false, "保留临时数据库")
}
