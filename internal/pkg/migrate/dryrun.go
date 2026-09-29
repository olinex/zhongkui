package migrate

import (
	"context"

	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// DryRunResult 描述一次迁移演练的结果。
type DryRunResult struct {
	// Success 迁移是否全部成功执行。
	Success bool
	// MigrationsApplied 已成功应用的迁移数量。
	MigrationsApplied int
	// Error 迁移失败时记录的错误。
	Error error
	// TempDBName 演练所用临时数据库的名称。
	TempDBName string
}

// DryRun 在临时数据库上执行迁移演练并返回结果。
func DryRun(ctx context.Context, cfg *Config) (*DryRunResult, error) {
	pterm.Info.Println(translate.T("cli.migrate.dry-run-creating", nil))
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

	pterm.Info.Println(translate.T("cli.migrate.dry-run-running", map[string]any{"DB": tempDB.name}))
	if err := migrator.Up(ctx); err != nil {
		result.Error = err
		result.Success = false
		pterm.Error.Println(translate.T("cli.migrate.dry-run-migration-failed", map[string]any{"Err": err}))
	} else {
		result.Success = true
		pterm.Success.Println(translate.T("cli.migrate.dry-run-completed", map[string]any{"DB": tempDB.name}))
	}

	if !cfg.KeepTemp {
		pterm.Info.Println(translate.T("cli.migrate.dry-run-cleanup", map[string]any{"DB": tempDB.name}))
		if err := tempDB.Close(context.Background()); err != nil {
			pterm.Warning.Println(translate.T("cli.migrate.dry-run-drop-failed", map[string]any{"DB": tempDB.name, "Err": err}))
		}
	} else {
		pterm.Info.Println(translate.T("cli.migrate.dry-run-preserved", map[string]any{"DB": tempDB.name}))
	}

	return result, nil
}
