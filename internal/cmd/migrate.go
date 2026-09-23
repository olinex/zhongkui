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
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "cmd.migrate.up.short",
	Run:   runMigrateCmd("up"),
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "cmd.migrate.down.short",
	Run:   runMigrateCmd("down"),
}

var migrateUpToCmd = &cobra.Command{
	Use:   "up-to [version]",
	Short: "cmd.migrate.up-to.short",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("up-to"),
}

var migrateDownToCmd = &cobra.Command{
	Use:   "down-to [version]",
	Short: "cmd.migrate.down-to.short",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("down-to"),
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "cmd.migrate.status.short",
	Run:   runMigrateCmd("status"),
}

var migrateDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "cmd.migrate.dry-run.short",
	Run:   runDryRunMigrateCmd,
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "cmd.migrate.diff.short",
	Run:   runDiffMigrateCmd,
}

var migrateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "cmd.migrate.check.short",
	Run:   runCheckMigrateCmd,
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "cmd.migrate.create.short",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		dbTarget := targetFromViper(v)
		name := args[0]

		for _, dbName := range targetDBs(dbTarget) {
			filePath, err := createMigrationFile(dbName, name)
			if err != nil {
				pterm.Error.Println(translate.T("cli.migrate.create-failed", map[string]any{"DB": dbName, "Err": err}))
				return
			}
			pterm.Success.Println(translate.T("cli.migrate.created", map[string]any{"Path": filePath}))
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
			pterm.DefaultSection.Println(translate.T("cli.migrate.section", map[string]any{"Action": action, "DB": dbName}))

			if err := migrateDB(ctx, v, gormDB, dbName, args, action); err != nil {
				pterm.Error.Println(translate.T("cli.migrate.action-failed", map[string]any{"DB": dbName, "Action": action, "Err": err}))
				return
			}
		}
		pterm.Success.Println(translate.T("cli.migrate.completed", nil))
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
		pterm.DefaultSection.Println(translate.T("cli.migrate.dry-run-section", map[string]any{"DB": dbName}))
		if err := migrateDryRunDB(ctx, v, dbName); err != nil {
			pterm.Error.Println(translate.T("cli.migrate.dry-run-db-failed", map[string]any{"DB": dbName, "Err": err}))
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
		pterm.Success.Println(translate.T("cli.migrate.dry-run-succeeded", nil))
	} else {
		pterm.Error.Println(translate.T("cli.migrate.dry-run-failed", map[string]any{"Err": result.Error}))
	}
	return nil
}

func runDiffMigrateCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Println(translate.T("cli.migrate.diff-section", map[string]any{"DB": dbName}))
		if err := migrateDiffDB(ctx, v, dbName); err != nil {
			pterm.Error.Println(translate.T("cli.migrate.diff-failed", map[string]any{"DB": dbName, "Err": err}))
		}
	}
}

func runCheckMigrateCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	keepTemp := v.GetBool("keep-temp")
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Println(translate.T("cli.migrate.check-section", map[string]any{"DB": dbName}))
		if err := migrateCheckDB(ctx, v, dbName, keepTemp, provider.Models(dbName)); err != nil {
			pterm.Error.Println(translate.T("cli.migrate.check-failed", map[string]any{"DB": dbName, "Err": err}))
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
	case base.CoreDbName:
		return []string{base.CoreDbName}
	case base.RawDbName:
		return []string{base.RawDbName}
	default:
		return []string{base.CoreDbName, base.RawDbName}
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
		c.Flags().String("db", "all", "flag.migrate.db")
		c.Flags().Bool("verbose", false, "flag.migrate.verbose")
		c.Flags().Duration("timeout", 0, "flag.migrate.timeout")
	}

	migrateDownCmd.Flags().Bool("force", false, "flag.migrate.force")
	migrateDryRunCmd.Flags().Bool("keep-temp", false, "flag.migrate.keep-temp")
	migrateDiffCmd.Flags().Bool("keep-temp", false, "flag.migrate.keep-temp")
	migrateCheckCmd.Flags().Bool("keep-temp", false, "flag.migrate.keep-temp")
	migrateAiEmbeddingCmd.Flags().Int("dimensions", 0, "flag.migrate.ai-embedding.dimensions")
}
