package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/depend"
	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	apimodel "zhongkui.com/zhongkui/internal/models/api"
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	netmodel "zhongkui.com/zhongkui/internal/models/network"
	tagmodel "zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
)

type MigrateParams struct {
	fx.In
	Ctx context.Context
	V   *viper.Viper
	depend.CoreDatabaseFxIn
	depend.RawDatabaseFxIn
	depend.DatabaseLoggerFxIn
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	Run:   runMigrateCmd("up"),
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback all applied migrations",
	Run:   runMigrateCmd("down"),
}

var migrateUpToCmd = &cobra.Command{
	Use:   "up-to [version]",
	Short: "Migrate up to a specific version",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("up-to"),
}

var migrateDownToCmd = &cobra.Command{
	Use:   "down-to [version]",
	Short: "Rollback down to a specific version",
	Args:  cobra.ExactArgs(1),
	Run:   runMigrateCmd("down-to"),
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Run:   runMigrateCmd("status"),
}

var migrateDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "Dry run migrations on a temp database",
	Run:   runDryRunMigrateCmd,
}

var migrateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare database schema differences",
	Run:   runDiffMigrateCmd,
}

var migrateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check model definitions against database schema",
	Run:   runCheckMigrateCmd,
}

var migrateCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a SQL migration file in the migrations directory",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbTarget := getDBTarget(cmd)
		name := args[0]

		for _, dbName := range targetDBs(dbTarget) {
			filePath, err := createMigrationFile(dbName, name)
			if err != nil {
				pterm.Error.Printf("%s: %v\n", dbName, err)
				os.Exit(1)
			}
			pterm.Success.Printf("created %s\n", filePath)
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
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) { return cmd, context.Background() },
				config.NewDefaultViperWithCmd,
				depend.DatabaseLoggerAnnotate(),
				depend.CoreDatabaseAnnotate(),
				depend.RawDatabaseAnnotate(),
			),
			fx.Invoke(func(p MigrateParams) {
				timeout := getTimeout(cmd)
				ctx, cancel := context.WithTimeout(p.Ctx, timeout)
				defer cancel()

				dbTarget := getDBTarget(cmd)

				for _, dbName := range targetDBs(dbTarget) {
					pterm.DefaultSection.Printf("%s (%s)", action, dbName)
					if err := migrateDB(ctx, p, dbName, cmd, action); err != nil {
						pterm.Error.Printf("%s %s: %v\n", dbName, action, err)
						os.Exit(1)
					}
				}
				os.Exit(0)
			}),
		).Run()
	}
}

func migrateDB(ctx context.Context, p MigrateParams, dbName string, cmd *cobra.Command, action string) error {
	m, err := createMigratorForDB(p, dbName, cmd)
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
		v := parseInt64(cmd.Flags().Arg(0))
		return m.UpTo(ctx, v)
	case "down-to":
		v := parseInt64(cmd.Flags().Arg(0))
		return m.DownTo(ctx, v)
	default:
		return errors.Newf("unknown action: %s", action)
	}
}

func runDryRunMigrateCmd(cmd *cobra.Command, args []string) {
	fx.New(
		depend.FxLogger(),
		fx.Provide(
			func() (*cobra.Command, context.Context) { return cmd, context.Background() },
			config.NewDefaultViperWithCmd,
			depend.DatabaseLoggerAnnotate(),
			depend.CoreDatabaseAnnotate(),
			depend.RawDatabaseAnnotate(),
		),
		fx.Invoke(func(p MigrateParams) {
			dbTarget := getDBTarget(cmd)
			for _, dbName := range targetDBs(dbTarget) {
				pterm.DefaultSection.Printf("dry-run %s", dbName)
				if err := migrateDryRunDB(p, dbName); err != nil {
					pterm.Error.Printf("dry-run %s: %v\n", dbName, err)
				}
			}
			os.Exit(0)
		}),
	).Run()
}

func migrateDryRunDB(p MigrateParams, dbName string) error {
	dbCfg, err := getDBConfig(p, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(getKeepTemp(nil)).
		WithMigrations(migrationsFor(dbName), ".")

	result, err := migrate.DryRun(context.Background(), mc)
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

func runDiffMigrateCmd(cmd *cobra.Command, args []string) {
	fx.New(
		depend.FxLogger(),
		fx.Provide(
			func() (*cobra.Command, context.Context) { return cmd, context.Background() },
			config.NewDefaultViperWithCmd,
			depend.DatabaseLoggerAnnotate(),
			depend.CoreDatabaseAnnotate(),
			depend.RawDatabaseAnnotate(),
		),
		fx.Invoke(func(p MigrateParams) {
			dbTarget := getDBTarget(cmd)
			for _, dbName := range targetDBs(dbTarget) {
				pterm.DefaultSection.Printf("diff %s", dbName)
				if err := migrateDiffDB(p, dbName); err != nil {
					pterm.Error.Printf("diff %s: %v\n", dbName, err)
				}
			}
			os.Exit(0)
		}),
	).Run()
}

func runCheckMigrateCmd(cmd *cobra.Command, args []string) {
	fx.New(
		depend.FxLogger(),
		fx.Provide(
			func() (*cobra.Command, context.Context) { return cmd, context.Background() },
			config.NewDefaultViperWithCmd,
			depend.DatabaseLoggerAnnotate(),
			depend.CoreDatabaseAnnotate(),
			depend.RawDatabaseAnnotate(),
		),
		fx.Invoke(func(p MigrateParams) {
			dbTarget := getDBTarget(cmd)
			keepTemp := getKeepTemp(cmd)
			for _, dbName := range targetDBs(dbTarget) {
				pterm.DefaultSection.Printf("check %s", dbName)
				if err := migrateCheckDB(p, dbName, keepTemp); err != nil {
					pterm.Error.Printf("check %s: %v\n", dbName, err)
				}
			}
			os.Exit(0)
		}),
	).Run()
}

func migrateCheckDB(p MigrateParams, dbName string, keepTemp bool) error {
	dbCfg, err := getDBConfig(p, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(keepTemp).
		WithMigrations(migrationsFor(dbName), ".")

	return migrate.Check(context.Background(), mc, modelsForDB(dbName))
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
		// 事实上没有FusedEthernet，因为FusedEthernet在理论上并不存在，该类主要是为了后续上层协议引用
		// &netmodel.FusedEthernet{},
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

func migrateDiffDB(p MigrateParams, dbName string) error {
	dbCfg, err := getDBConfig(p, dbName)
	if err != nil {
		return err
	}

	mc := migrate.NewConfig(dbCfg.Driver, dbCfg.DSN).
		WithKeepTemp(getKeepTemp(nil)).
		WithMigrations(migrationsFor(dbName), ".")

	return migrate.Diff(context.Background(), mc, mc)
}

func createMigratorForDB(p MigrateParams, dbName string, cmd *cobra.Command) (*migrate.Migrator, error) {
	gormDB := dbForName(p, dbName)
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

	verbose := getVerbose(cmd)
	cfg := migrate.NewConfig(driverName, "").
		WithVerbose(verbose).
		WithMigrations(migrationsFor(dbName), ".")

	goose.SetBaseFS(migrationsFor(dbName))

	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

func dbForName(p MigrateParams, dbName string) *database.DB {
	switch dbName {
	case "core":
		return p.CoreDB
	case "raw":
		return p.RawDB
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

func getDBConfig(p MigrateParams, dbName string) (*database.Config, error) {
	cfg, err := database.ExtractConfig(p.V, dbName)
	if err != nil {
		return nil, errors.Newf("failed to extract %s database config: %v", dbName, err)
	}
	return cfg, nil
}

func getDBTarget(cmd *cobra.Command) string {
	target, _ := cmd.Flags().GetString("db")
	if target == "" {
		target = "all"
	}
	return target
}

func getVerbose(cmd *cobra.Command) bool {
	verbose, _ := cmd.Flags().GetBool("verbose")
	return verbose
}

func getTimeout(cmd *cobra.Command) time.Duration {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return timeout
}

func getKeepTemp(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	keep, _ := cmd.Flags().GetBool("keep-temp")
	return keep
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateUpToCmd)
	migrateCmd.AddCommand(migrateDownToCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateDryRunCmd)
	migrateCmd.AddCommand(migrateDiffCmd)
	migrateCmd.AddCommand(migrateCheckCmd)
	migrateCmd.AddCommand(migrateCreateCmd)

	for _, c := range []*cobra.Command{
		migrateUpCmd, migrateDownCmd, migrateUpToCmd, migrateDownToCmd,
		migrateStatusCmd, migrateDryRunCmd, migrateDiffCmd, migrateCheckCmd, migrateCreateCmd,
	} {
		c.Flags().String("db", "all", "target database: core, raw, all")
		c.Flags().Bool("verbose", false, "print migration SQL")
	}

	migrateDownCmd.Flags().Bool("force", false, "skip confirmation")
	migrateDryRunCmd.Flags().Bool("keep-temp", false, "keep temp database")
	migrateDiffCmd.Flags().Bool("keep-temp", false, "keep temp database")
	migrateCheckCmd.Flags().Bool("keep-temp", false, "keep temp database")
}
