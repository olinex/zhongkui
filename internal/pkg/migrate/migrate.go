// Package migrate（本文件）封装基于 goose 的迁移执行器，提供版本升降级、
// 当前版本查询与状态展示等能力。
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

var (
	grayColor  = "\033[90m"
	resetColor = "\033[00m"
)

// ptermGooseLogger 将 goose 库的日志输出转发到 pterm
// goose 是第三方库，其日志文本为英文硬编码，通过 SetLogger 注入后在此统一翻译，
// 避免逐个发现未翻译文本再修补
type ptermGooseLogger struct{}

func (l ptermGooseLogger) Printf(format string, v ...any) {
	msg := translateGooseLog(format, v...)
	if msg == "" {
		return
	}
	pterm.Info.Println(msg)
}

func (l ptermGooseLogger) Fatalf(format string, v ...any) {
	pterm.Fatal.Println(translateGooseLog(format, v...))
}

// translateGooseLog 统一翻译 goose 库日志消息
// 通过匹配 goose 库内的固定格式串（库内常量，长期稳定）实现集中翻译；
// 未匹配的消息（如 verbose 模式输出的 SQL 语句原文）原样透传
func translateGooseLog(format string, v ...any) string {
	switch format {
	case "goose: successfully migrated database to version: %d":
		return translate.T("cli.migrate.goose-migrated", map[string]any{"Version": v[0]})
	case "goose: no migrations to run. current version: %d":
		return translate.T("cli.migrate.goose-no-migrations", map[string]any{"Version": v[0]})
	case "goose: up to current file version: %d":
		return translate.T("cli.migrate.goose-up-to", map[string]any{"Version": v[0]})
	case "goose: down to current file version: %d":
		return translate.T("cli.migrate.goose-down-to", map[string]any{"Version": v[0]})
	case "goose: migration file not found for current version (%d), error: %s":
		return translate.T("cli.migrate.goose-file-not-found", map[string]any{"Version": v[0], "Err": v[1]})
	case "OK   %s (%s)":
		return translate.T("cli.migrate.goose-ok", map[string]any{"Source": v[0], "Duration": v[1]})
	case "EMPTY %s (%s)":
		return translate.T("cli.migrate.goose-empty", map[string]any{"Source": v[0], "Duration": v[1]})
	default:
		// 未匹配的消息（如 verbose 模式输出的 SQL 语句）原样透传
		msg := strings.TrimSpace(fmt.Sprintf(format, v...))
		msg = strings.TrimPrefix(msg, grayColor)
		msg = strings.TrimSuffix(msg, resetColor)
		return msg
	}
}

// Migrator 封装 goose 迁移操作与数据库连接。
type Migrator struct {
	cfg *Config
	db  *sql.DB
}

// NewMigrator 依据配置建立数据库连接并创建迁移执行器。
func NewMigrator(cfg *Config) (*Migrator, error) {
	db, err := sql.Open(sqlDriverName(cfg.Driver), cfg.DSN)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, errors.WithStack(err)
	}
	m := &Migrator{cfg: cfg, db: db}
	if err := m.init(); err != nil {
		return nil, errors.WithStack(err)
	}
	return m, nil
}

// NewMigratorFromDB 复用已有的数据库连接创建迁移执行器。
func NewMigratorFromDB(cfg *Config, db *sql.DB) (*Migrator, error) {
	if err := db.Ping(); err != nil {
		return nil, errors.WithStack(err)
	}
	m := &Migrator{cfg: cfg, db: db}
	if err := m.init(); err != nil {
		return nil, errors.WithStack(err)
	}
	return m, nil
}

func (m *Migrator) init() error {
	goose.SetLogger(ptermGooseLogger{})
	if m.cfg.Verbose {
		goose.SetVerbose(true)
	} else {
		goose.SetVerbose(false)
	}
	goose.SetBaseFS(m.cfg.MigrationsFS)
	if m.cfg.Driver != "clickhouse" {
		return nil
	}
	// 因为 gorm 自动创建的 goose_db_version 存在兼容性问题，要确保ClickHouse数据库有表
	// TODO: 等待后续版本修复
	if err := m.ensureVersionTable(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) ensureVersionTable() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS goose_db_version (
			id Int64,
			version_id Int64,
			is_applied Bool,
			tstamp DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY id
	`)
	if err != nil {
		return errors.WithStack(err)
	}
	var count int
	if err = m.db.QueryRow("SELECT count() FROM goose_db_version").Scan(&count); err != nil {
		return errors.WithStack(err)
	}
	if count == 0 {
		_, err = m.db.Exec("INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (0, true, now())")
		return errors.WithStack(err)
	}
	var hasApplied bool
	if err = m.db.QueryRow("SELECT count() > 0 FROM goose_db_version WHERE version_id = 0 AND is_applied = true").Scan(&hasApplied); err != nil {
		return errors.WithStack(err)
	}
	if !hasApplied {
		_, err = m.db.Exec("INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (0, true, now())")
		return errors.WithStack(err)
	}
	return nil
}

// Close 关闭底层数据库连接。
func (m *Migrator) Close() error {
	return m.db.Close()
}

// Up 执行全部未应用的迁移。
func (m *Migrator) Up(ctx context.Context) error {
	pterm.Info.Println(translate.T("cli.migrate.up", map[string]any{"DB": m.cfg.Name}))
	if err := goose.UpContext(ctx, m.db, m.cfg.MigrationsDir); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// UpTo 迁移至指定版本（仅向上）。
func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	pterm.Info.Println(translate.T("cli.migrate.up-to", map[string]any{"DB": m.cfg.Name, "Version": version}))
	if err := goose.UpToContext(ctx, m.db, m.cfg.MigrationsDir, version); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Down 回滚最近一次已应用的迁移。
func (m *Migrator) Down(ctx context.Context) error {
	pterm.Warning.Println(translate.T("cli.migrate.down", map[string]any{"DB": m.cfg.Name}))
	if err := goose.DownContext(ctx, m.db, m.cfg.MigrationsDir); err != nil {
		// 数据库尚无已应用的迁移时，goose 返回 no current version found，属正常边界而非错误
		if errors.Is(err, goose.ErrNoCurrentVersion) {
			pterm.Warning.Println(translate.T("cli.migrate.down-no-current-version", map[string]any{"DB": m.cfg.Name}))
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}

// DownTo 回滚至指定版本（仅向下）。
func (m *Migrator) DownTo(ctx context.Context, version int64) error {
	pterm.Warning.Println(translate.T("cli.migrate.down-to", map[string]any{"DB": m.cfg.Name, "Version": version}))
	if err := goose.DownToContext(ctx, m.db, m.cfg.MigrationsDir, version); err != nil {
		// 数据库尚无已应用的迁移时，goose 返回 no current version found，属正常边界而非错误
		if errors.Is(err, goose.ErrNoCurrentVersion) {
			pterm.Warning.Println(translate.T("cli.migrate.down-to-no-current-version", map[string]any{"DB": m.cfg.Name}))
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}

// GetCurrentVersion 返回数据库当前已应用的最新迁移版本。
func (m *Migrator) GetCurrentVersion(ctx context.Context) (int64, error) {
	return goose.GetDBVersionContext(ctx, m.db)
}

// Status 打印全部迁移文件的应用状态表格。
func (m *Migrator) Status(ctx context.Context) error {
	migrations, err := goose.CollectMigrations(m.cfg.MigrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return errors.WithStack(err)
	}

	appliedVersions := m.getAppliedVersions(ctx)

	data := pterm.TableData{
		{
			translate.T("cli.migrate.col-version", nil),
			translate.T("cli.migrate.col-migration", nil),
			translate.T("cli.migrate.col-status", nil),
			translate.T("cli.migrate.col-applied-at", nil),
		},
	}

	for _, migration := range migrations {
		status := translate.T("cli.migrate.status-pending", nil)
		appliedAt := "-"
		if record, ok := appliedVersions[migration.Version]; ok {
			status = translate.T("cli.migrate.status-applied", nil)
			appliedAt = record.Format(time.RFC3339)
		}
		filename := filepath.Base(migration.Source)
		data = append(data, []string{
			strconv.FormatInt(migration.Version, 10), filename, status, appliedAt,
		})
	}

	if len(migrations) == 0 {
		pterm.Warning.Println(translate.T("cli.migrate.no-file", nil))
		return nil
	}

	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) getAppliedVersions(ctx context.Context) map[int64]time.Time {
	result := make(map[int64]time.Time)
	rows, err := m.db.QueryContext(ctx, "SELECT version_id, tstamp FROM goose_db_version WHERE is_applied = true ORDER BY id DESC")
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		var ts time.Time
		if err := rows.Scan(&version, &ts); err != nil {
			continue
		}
		result[version] = ts
	}
	return result
}
