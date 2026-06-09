package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/depend"
	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear zhongkui data",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var clearDatabaseCmd = &cobra.Command{
	Use:   "database",
	Short: "Clear all database tables (rollback all migrations)",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		timeout, err := flags.GetDuration("timeout")
		if err != nil {
			panic(fmt.Sprintf("invalid timeout: %v", err))
		}
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				config.NewDefaultViperWithCmd,
				depend.DatabaseLoggerAnnotate(),
				depend.CoreDatabaseAnnotate(),
				depend.RawDatabaseAnnotate(),
			),
			fx.Invoke(validator.ValidatorRegister),
			fx.Invoke(func(p MigrateParams) {
				random, err := crypto.GenerateRandomString(6)
				if err != nil {
					pterm.Error.Println(err.Error())
					os.Exit(1)
				}
				err = staticInputDoubleCheck(random)
				if err != nil {
					pterm.Error.Println(err.Error())
					os.Exit(1)
				}
				pterm.Warning.Println("rolling back all migrations...")
				ctx, cancel := context.WithTimeout(p.Ctx, timeout)
				defer cancel()

				type dbInfo struct {
					name string
					db   *migrate.Migrator
				}
				var migrators []dbInfo

				if m, err := createClearMigrator(p, "core", migcore.Migrations); err != nil {
					pterm.Error.Printf("failed to connect core: %v\n", err)
					os.Exit(1)
				} else {
					migrators = append(migrators, dbInfo{"core", m})
				}
				if m, err := createClearMigrator(p, "raw", migraw.Migrations); err != nil {
					pterm.Error.Printf("failed to connect raw: %v\n", err)
					os.Exit(1)
				} else {
					migrators = append(migrators, dbInfo{"raw", m})
				}

				for _, mi := range migrators {
					pterm.Info.Printf("rolling back %s...\n", mi.name)
					if err := mi.db.Down(ctx); err != nil {
						pterm.Error.Printf("%s rollback failed: %v\n", mi.name, err)
						mi.db.Close()
						os.Exit(1)
					}
					mi.db.Close()
				}
				pterm.Success.Println("clear database complete")
				os.Exit(0)
			}),
		).Run()
	},
}

func createClearMigrator(p MigrateParams, dbName string, migrationFS embed.FS) (*migrate.Migrator, error) {
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
	}

	goose.SetBaseFS(migrationFS)
	cfg := migrate.NewConfig(driverName, "")
	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

var clearTempFilesCmd = &cobra.Command{
	Use:   "temp-files",
	Short: "Clear temporary files from tasks",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		timeout, err := flags.GetDuration("timeout")
		if err != nil {
			panic(fmt.Sprintf("invalid timeout: %v", err))
		}
		if err != nil {
			panic(fmt.Sprintf("invalid force continue: %v", err))
		}
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				config.NewDefaultViperWithCmd,
				depend.ETCDLoggerAnnotate(),
				depend.DatabaseLoggerAnnotate(),
				depend.PipelineLoggerAnnotate(),
				depend.CoreDatabaseAnnotate(),
				depend.RawDatabaseAnnotate(),
				depend.ManageEtcdAnnotate(),
				depend.PipelineRedisAnnotate(),
				depend.CaptureS3Annotate(),
				workers.TaskContext,
			),
			fx.Invoke(func(p pipeline.ContextFunc) {
				random, err := crypto.GenerateRandomString(6)
				if err != nil {
					pterm.Error.Println(err.Error())
					os.Exit(1)
				}
				err = staticInputDoubleCheck(random)
				if err != nil {
					pterm.Error.Println(err.Error())
					os.Exit(1)
				}
				ctx := p()
				in := workers.GetFxIn(ctx)
				in.PipelineLogger.Info("clearing temp files")
				timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				wg, ctx := errgroup.WithContext(timeoutCtx)
				wg.Go(func() error {
					return nil
				})
				if err := wg.Wait(); err != nil {
					in.PipelineLogger.Errorf(`clear temp files error: %+v`, err)
					os.Exit(1)
				} else {
					in.PipelineLogger.Info("clear temp files complete")
					os.Exit(0)
				}
			}),
		).Run()
	},
}

func init() {
	flags := clearDatabaseCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for clearing database")

	flags = clearTempFilesCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for clearing temp files")
	flags.Bool("forceContinue", false, "force continue when error received")

	clearCmd.AddCommand(clearDatabaseCmd)
	clearCmd.AddCommand(clearTempFilesCmd)
}
