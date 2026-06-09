package migrate

import (
	"context"

	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type DryRunResult struct {
	Success           bool
	MigrationsApplied int
	Error             error
	TempDBName        string
}

func DryRun(ctx context.Context, cfg *Config) (*DryRunResult, error) {
	pterm.Info.Println("creating temporary database for dry-run...")
	tempDB, err := CreateTempDB(ctx, cfg)
	if err != nil {
		return nil, errors.Newf("failed to create temp database: %v", err)
	}

	tempCfg := &Config{
		Name:          cfg.Name + "_dryrun",
		Driver:        cfg.Driver,
		DSN:           tempDB.dsn,
		MigrationsFS:  cfg.MigrationsFS,
		MigrationsDir: cfg.MigrationsDir,
		Timeout:       cfg.Timeout,
		Verbose:       cfg.Verbose,
	}

	migrator, err := NewMigratorFromDB(tempCfg, tempDB.db)
	if err != nil {
		tempDB.Close(context.Background())
		return nil, errors.Newf("failed to create migrator for temp db: %v", err)
	}

	result := &DryRunResult{
		TempDBName: tempDB.name,
	}

	pterm.Info.Printf("running migrations on temporary database %s...\n", tempDB.name)
	if err := migrator.Up(ctx); err != nil {
		result.Error = err
		result.Success = false
		pterm.Error.Printf("dry-run migration failed: %v\n", err)
	} else {
		result.Success = true
		pterm.Success.Printf("dry-run migration completed successfully on %s\n", tempDB.name)
	}

	if !cfg.KeepTemp {
		pterm.Info.Printf("cleaning up temporary database %s...\n", tempDB.name)
		if err := tempDB.Close(context.Background()); err != nil {
			pterm.Warning.Printf("warning: failed to drop temp database %s: %v\n", tempDB.name, err)
		}
	} else {
		pterm.Info.Printf("temporary database %s preserved (--keep-temp)\n", tempDB.name)
	}

	return result, nil
}
