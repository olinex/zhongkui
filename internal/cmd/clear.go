package cmd

import (
	"context"
	"embed"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/migrate"
	"zhongkui.com/zhongkui/internal/provider"
)

var clearDatabaseCmd = &cobra.Command{
	Use:   "database",
	Short: "Clear all database tables (rollback all migrations)",
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
		pterm.Warning.Println("rolling back all migrations...")
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		type dbInfo struct {
			name string
			db   *migrate.Migrator
		}
		var migrators []dbInfo

		if m, err := createClearMigrator(coreDB, rawDB, "core", migcore.Migrations); err != nil {
			pterm.Error.Printf("failed to connect core: %v\n", err)
			return
		} else {
			migrators = append(migrators, dbInfo{"core", m})
		}
		if m, err := createClearMigrator(coreDB, rawDB, "raw", migraw.Migrations); err != nil {
			pterm.Error.Printf("failed to connect raw: %v\n", err)
			return
		} else {
			migrators = append(migrators, dbInfo{"raw", m})
		}

		for _, mi := range migrators {
			pterm.Info.Printf("rolling back %s...\n", mi.name)
			if err := mi.db.Down(ctx); err != nil {
				pterm.Error.Printf("%s rollback failed: %v\n", mi.name, err)
				mi.db.Close()
				return
			}
			mi.db.Close()
		}
		pterm.Success.Println("clear database complete")
	},
}

func createClearMigrator(coreDB, rawDB *database.DB, dbName string, migrationFS embed.FS) (*migrate.Migrator, error) {
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
	}

	goose.SetBaseFS(migrationFS)
	cfg := migrate.NewConfig(driverName, "")
	return migrate.NewMigratorFromDB(cfg, sqlDB)
}

func init() {
	flags := clearDatabaseCmd.Flags()
	flags.Duration("timeout", time.Minute, "timeout for clearing database")
}
