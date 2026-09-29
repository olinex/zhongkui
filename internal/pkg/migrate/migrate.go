// Package migrate 封装基于 goose 的数据库迁移执行器，并提供模型与数据库结构的比对校验
// （check）、结构差异比对（diff）与迁移演练（dry-run）。迁移脚本经 embed 打包进二进制
// （internal/migrations/core、raw 各自 //go:embed *.sql），运行时不再读取磁盘上的 SQL 文件。
//
// 包内文件分工：migrate.go（Migrator 与 goose 执行器封装、版本升降级与状态表格）、
// config.go（Config 及其链式配置）、check.go（模型声明 vs 数据库结构校验）、diff.go
// （两套结构差异比对）、dryrun.go（迁移演练）、tempdb.go（临时库生命周期）。CLI 入口在
// internal/cmd/migrate.go。
//
// ## 一、机制概览
//
// 迁移文件是 goose 格式的 SQL，带 `-- +goose Up` / `-- +goose Down` 两段；文件名形如
// `NNN_描述.sql`（三位零填充递增版本号 + 下划线 + 描述）。goose 以 `goose_db_version` 表记录
// 每个版本是否已应用：`up` 按版本升序执行未应用的 Up 段，`down` 只回滚最近一个已应用版本。
// 文件本身经 embed 打进二进制，版本号因此是唯一且不可变的发布标识。
//
// 两个库各持一套迁移：core 库存业务表与出厂数据（`001_init.sql` 建表、`002_fixtures.sql` 写入
// 参数与 RBAC 权限点等 fixture），raw 库存抓包原始数据表。两库分别经 migrations.MigrationFor
// （internal/migrations/collect.go）按库名分发嵌入式文件系统。
//
// ## 二、分步清单：新增一次迁移
//
//  1. 生成骨架（自动取下一个版本号并创建文件）：
//
//     go run cmd/zki/main.go migrate create {描述}
//
//     它在 internal/migrations/core（以及 raw）下建出 `NNN_{描述}.sql`，含 Up/Down 两段空模板。
//     普通建表/补列优先新增独立文件承载，不要原位改写已发布的既有迁移。
//
//  2. 编写 Up 段：一律用幂等写法（CREATE TABLE IF NOT EXISTS、ADD COLUMN IF NOT EXISTS、
//     CREATE INDEX IF NOT EXISTS 等），使脚本可在结构已知/未知的库上重复安全执行；建表后
//     用 COMMENT ON COLUMN 补列注释（与 `001_init.sql` 保持一致）。
//
//  3. 编写 Down 段：与 Up 对称地撤销变更（DROP ... IF EXISTS）。首版本封闭开发阶段 schema
//     可自由重构（见 AGENTS.md「开发阶段」），但仍应保持 Down 可用，以便 diff / dry-run。
//
//  4. 递增版本号：版本号必须严格递增，且一旦文件被任何库应用就不得再原位改写其内容——既有库
//     的 `goose_db_version` 已记为已应用，改写后不会重跑，导致结构漂移；承载变更须新增一个
//     更高版本号的文件。
//
//  5. 若改动涉及模型字段，同步更新 GORM 模型与 provider.CoreModels() 等登记清单（见 check）。
//
//  6. 用下节命令验证。
//
// ## 三、验证命令（CLI）
//
// 均支持 --db=core|raw|all（默认 all）与 --verbose / --timeout：
//
//   - go run cmd/zki/main.go migrate status    查看各版本是否已应用及其应用时间；
//   - go run cmd/zki/main.go migrate dry-run   在临时库上跑一遍全部 Up，验证脚本可执行；
//   - go run cmd/zki/main.go migrate up        对目标库执行全部未应用迁移；
//   - go run cmd/zki/main.go migrate down      回滚最近一次已应用的迁移；
//   - go run cmd/zki/main.go migrate up-to N   迁移至版本 N；down-to N 回滚至版本 N；
//   - go run cmd/zki/main.go migrate diff      比对「当前库结构」与「空库跑完全部迁移后的结构」，
//     列出多出/缺失的表与列；
//   - go run cmd/zki/main.go migrate check     见下节。
//
// ## 四、migrate check 的意义
//
// check 先建临时库并跑完全部迁移，再用 GORM 解析模型的期望结构，逐表逐列比对类型不匹配、缺失
// 表/列与多余表/列（postgres 与 clickhouse 各有一套类型别名归一，见 check.go）。它保证「代码里
// 的模型声明」与「迁移脚本产出的真实结构」一致——两侧任一改动而另一侧未跟上都会被它拦下；因此
// 新增模型后必须同步登记到 provider.CoreModels() / provider.Models() 等清单，否则该表会被判为
// 多余表。--keep-temp 在失败时保留临时库供人工排查。
//
// ## 五、注意事项
//
//   - 迁移只改结构，不承载业务逻辑；数据回填也应写成幂等 SQL（如 INSERT ... ON CONFLICT DO NOTHING）。
//   - 临时库在 postgres 下需具备建库权限（tempdb.go 用管理连接 CREATE DATABASE），演练结束自动
//     清理（--keep-temp 可保留）。
//   - clickhouse 的 `goose_db_version` 由迁移器在 init 时显式建表（兼容性 workaround）。
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

// Printf 将 goose 的普通日志经翻译后由 pterm 输出。
func (l ptermGooseLogger) Printf(format string, v ...any) {
	msg := translateGooseLog(format, v...)
	if msg == "" {
		return
	}
	pterm.Info.Println(msg)
}

// Fatalf 将 goose 的致命日志经翻译后由 pterm 输出并终止程序。
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
