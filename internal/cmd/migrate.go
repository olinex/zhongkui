package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	apimodel "zhongkui.com/zhongkui/internal/models/api"
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	netmodel "zhongkui.com/zhongkui/internal/models/network"
	tagmodel "zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/provider"
)

func coreModels() []any {
	return []any{
		&coremodel.User{},
		&coremodel.Group{},
		&coremodel.Namespace{},
		&coremodel.Capturer{},
		&coremodel.AccessKey{},
		&coremodel.RbacPolicy{},
		&coremodel.RbacPolicyTemplate{},
		&tagmodel.RuleTag{},
		&tagmodel.RuleCondition{},
	}
}

func rawModels() []any {
	return []any{
		&netmodel.PacketEthernet{},
		&netmodel.PacketIPv4{},
		&netmodel.PacketIPv6{},
		&netmodel.PacketIPv4TCP{},
		&netmodel.PacketIPv6TCP{},
		&netmodel.PacketIPv4UDP{},
		&netmodel.FusedIPv4UDP{},
		&netmodel.PacketIPv6UDP{},
		&netmodel.FusedIPv6UDP{},
		&netmodel.FusedIPv4{},
		&netmodel.FusedIPv6{},
		&netmodel.FusedIPv4TCP{},
		&netmodel.FusedIPv6TCP{},
		&netmodel.FusedIPv4UDP{},
		&netmodel.FusedIPv6UDP{},
		&netmodel.HttpTransport{},
		&apimodel.HttpApiMeta{},
		&apimodel.HttpApiNode{},
		&tagmodel.TagSumming{},
	}
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE:  runMigrateCmd("up"),
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback all applied migrations",
	RunE:  runMigrateCmd("down"),
}

var migrateUpToCmd = &cobra.Command{
	Use:   "up-to [version]",
	Short: "Migrate up to a specific version",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateCmd("up-to"),
}

var migrateDownToCmd = &cobra.Command{
	Use:   "down-to [version]",
	Short: "Rollback down to a specific version",
	Args:  cobra.ExactArgs(1),
	RunE:  runMigrateCmd("down-to"),
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	RunE:  runMigrateCmd("status"),
}

var migrateDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "Dry run migrations on a temp database",
	RunE:  runDryRunMigrateCmd,
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare database schema differences",
	RunE:  runDiffMigrateCmd,
}

var migrateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check model definitions against database schema",
	RunE:  runCheckMigrateCmd,
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a SQL migration file in the migrations directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		dbTarget := targetFromViper(v)
		name := args[0]

		for _, dbName := range targetDBs(dbTarget) {
			filePath, err := createMigrationFile(dbName, name)
			if err != nil {
				pterm.Error.Printf("%s: %v\n", dbName, err)
				return err
			}
			pterm.Success.Printf("created %s\n", filePath)
		}
		return nil
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

func runMigrateCmd(action string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		v := provider.MustInvokeViper(i)
		coreDB := provider.MustInvokeCoreDatabase(i)
		rawDB := provider.MustInvokeRawDatabase(i)

		timeout := v.GetDuration("timeout")
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		dbTarget := targetFromViper(v)

		for _, dbName := range targetDBs(dbTarget) {
			pterm.DefaultSection.Printf("%s (%s)", action, dbName)
			if err := migrateDB(ctx, v, coreDB, rawDB, dbName, args, action); err != nil {
				pterm.Error.Printf("%s %s: %v\n", dbName, action, err)
				return err
			}
		}
		return nil
	}
}

func migrateDB(ctx context.Context, v *viper.Viper, coreDB, rawDB *database.DB, dbName string, args []string, action string) error {
	m, err := createMigratorForDB(v, coreDB, rawDB, dbName)
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

func createMigratorForDB(v *viper.Viper, coreDB, rawDB *database.DB, dbName string) (*migrate.Migrator, error) {
	gormDB := dbForName(coreDB, rawDB, dbName)
	if gormDB == nil {
		return nil, errors.Newf("unknown database: %s", dbName)
	}

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
		WithVerbose(verbose).
		WithMigrations(migrationsFor(dbName), ".")

	goose.SetBaseFS(migrationsFor(dbName))

	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

func runDryRunMigrateCmd(cmd *cobra.Command, args []string) error {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Printf("dry-run %s", dbName)
		if err := migrateDryRunDB(ctx, v, dbName); err != nil {
			pterm.Error.Printf("dry-run %s: %v\n", dbName, err)
		}
	}
	return nil
}

func migrateDryRunDB(ctx context.Context, v *viper.Viper, dbName string) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(false).
		WithMigrations(migrationsFor(dbName), ".")

	result, err := migrate.DryRun(ctx, mc)
	if err != nil {
		return errors.Newf("dry-run failed: %v", err)
	}
	if result.Success {
		pterm.Success.Println("dry-run succeeded")
	} else {
		pterm.Error.Printf("dry-run failed: %v\n", result.Error)
	}
	return nil
}

func runDiffMigrateCmd(cmd *cobra.Command, args []string) error {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Printf("diff %s", dbName)
		if err := migrateDiffDB(ctx, v, dbName); err != nil {
			pterm.Error.Printf("diff %s: %v\n", dbName, err)
		}
	}
	return nil
}

func runCheckMigrateCmd(cmd *cobra.Command, args []string) error {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)

	dbTarget := targetFromViper(v)
	keepTemp := v.GetBool("keep-temp")
	for _, dbName := range targetDBs(dbTarget) {
		pterm.DefaultSection.Printf("check %s", dbName)
		if err := migrateCheckDB(ctx, v, dbName, keepTemp); err != nil {
			pterm.Error.Printf("check %s: %v\n", dbName, err)
		}
	}
	return nil
}

func migrateCheckDB(ctx context.Context, v *viper.Viper, dbName string, keepTemp bool) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(keepTemp).
		WithMigrations(migrationsFor(dbName), ".")

	return migrate.Check(ctx, mc, modelsForDB(dbName))
}

func modelsForDB(dbName string) []any {
	switch dbName {
	case "core":
		return coreModels()
	case "raw":
		return rawModels()
	default:
		return nil
	}
}

func migrateDiffDB(ctx context.Context, v *viper.Viper, dbName string) error {
	dbCfg, err := getDBConfig(v, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(false).
		WithMigrations(migrationsFor(dbName), ".")

	return migrate.Diff(ctx, mc, mc)
}

func dbForName(coreDB, rawDB *database.DB, dbName string) *database.DB {
	switch dbName {
	case "core":
		return coreDB
	case "raw":
		return rawDB
	default:
		return nil
	}
}

func targetDBs(target string) []string {
	switch target {
	case "core":
		return []string{"core"}
	case "raw":
		return []string{"raw"}
	default:
		return []string{"core", "raw"}
	}
}

func migrationsFor(dbName string) embed.FS {
	switch dbName {
	case "core":
		return migcore.Migrations
	case "raw":
		return migraw.Migrations
	default:
		panic("unknown db: " + dbName)
	}
}

func getDBConfig(v *viper.Viper, dbName string) (*database.Config, error) {
	cfg, err := database.ExtractConfig(v, dbName)
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
		c.Flags().String("db", "all", "target database: core, raw, all")
		c.Flags().Bool("verbose", false, "print migration SQL")
		c.Flags().Duration("timeout", 0, "migration timeout (0 means no timeout)")
	}

	migrateDownCmd.Flags().Bool("force", false, "skip confirmation")
	migrateDryRunCmd.Flags().Bool("keep-temp", false, "keep temp database")
	migrateDiffCmd.Flags().Bool("keep-temp", false, "keep temp database")
	migrateCheckCmd.Flags().Bool("keep-temp", false, "keep temp database")
}
