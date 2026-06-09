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
)

var (
	grayColor  = "\033[90m"
	resetColor = "\033[00m"
)

type ptermGooseLogger struct{}

func (l ptermGooseLogger) Printf(format string, v ...any) {
	msg := strings.TrimSpace(fmt.Sprintf(format, v...))
	if msg == "" {
		return
	}
	msg = strings.TrimPrefix(msg, grayColor)
	msg = strings.TrimSuffix(msg, resetColor)

	pterm.Info.Println(msg)
}

func (l ptermGooseLogger) Fatalf(format string, v ...any) {
	msg := strings.TrimSpace(fmt.Sprintf(format, v...))
	pterm.Fatal.Println(msg)
}

type Migrator struct {
	cfg *Config
	db  *sql.DB
}

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

func (m *Migrator) Close() error {
	return m.db.Close()
}

func (m *Migrator) Up(ctx context.Context) error {
	pterm.Info.Printf("running goose up on %s\n", m.cfg.Name)
	if err := goose.UpContext(ctx, m.db, m.cfg.MigrationsDir); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	pterm.Info.Printf("running goose up-to %d on %s\n", version, m.cfg.Name)
	if err := goose.UpToContext(ctx, m.db, m.cfg.MigrationsDir, version); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) Down(ctx context.Context) error {
	pterm.Warning.Printf("running goose down on %s\n", m.cfg.Name)
	if err := goose.DownContext(ctx, m.db, m.cfg.MigrationsDir); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) DownTo(ctx context.Context, version int64) error {
	pterm.Warning.Printf("running goose down-to %d on %s\n", version, m.cfg.Name)
	if err := goose.DownToContext(ctx, m.db, m.cfg.MigrationsDir, version); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (m *Migrator) GetCurrentVersion(ctx context.Context) (int64, error) {
	return goose.GetDBVersionContext(ctx, m.db)
}
func (m *Migrator) Status(ctx context.Context) error {
	migrations, err := goose.CollectMigrations(m.cfg.MigrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return errors.WithStack(err)
	}

	appliedVersions := m.getAppliedVersions(ctx)

	data := pterm.TableData{
		{"Version", "Migration", "Status", "Applied At"},
	}

	for _, migration := range migrations {
		status := "Pending"
		appliedAt := "-"
		if record, ok := appliedVersions[migration.Version]; ok {
			status = "Applied"
			appliedAt = record.Format(time.RFC3339)
		}
		filename := filepath.Base(migration.Source)
		data = append(data, []string{
			strconv.FormatInt(migration.Version, 10), filename, status, appliedAt,
		})
	}

	if len(migrations) == 0 {
		pterm.Warning.Println("no migration file found")
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
