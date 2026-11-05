// Package testdb 提供单元测试专用的测试数据库访问助手。
//
// 测试数据库 core_test（PostgreSQL）与 raw_test（ClickHouse）复用与生产相同的
// SQL 方言和迁移脚本，避免单测依赖 sqlite 方言，从而移除代码中对数据库类型的判断。
//
// 每个测试进程内对每个测试库执行一次"确保数据库存在 + 运行迁移"；core_test 之后
// 每个测试在独立事务（savepoint 嵌套）中运行并在结束时回滚，保留迁移基线数据，
// 保证包内单测隔离且参照完整；raw_test（ClickHouse）每次调用前对业务表做 TRUNCATE。
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"

	"zhongkuisec.com/zhongkui/internal/migrations"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/migrate"
)

// 环境变量覆盖测试库 DSN，优先级高于 config.toml
const (
	coreDSNEnv = "ZKI_DATABASE_CORE_TEST_DSN"
	rawDSNEnv  = "ZKI_DATABASE_RAW_TEST_DSN"
)

var (
	coreOnce    sync.Once
	coreDB      *database.DB
	coreInitErr error
	rawOnce     sync.Once
	rawDB       *database.DB
	rawInitErr  error

	// corePrev 记录每个测试上一个已开启的核心事务句柄。
	// goconvey 会为每个叶子 Convey 重放父 Convey 体（再次调用 Core），
	// 若上一个事务任其保持未提交就直接开启新事务，重放会以同一唯一键写入
	// 多个未提交事务并互相阻塞。故在开启新事务前回滚上一个同测试事务，释放写锁。
	corePrev sync.Map // testing.TB(可比较) -> *database.DB
)

// Core 返回已迁移的共享 PostgreSQL 测试库（core_test）的事务句柄。
// 每次调用开启一个独立事务，并在下次调用前回滚上一个为同一测试开启的事务，
// 使每次（goconvey 重放父 Convey 时）都从干净基线开始。
// 迁移种子/基线数据仅初始化一次并保持提交，测试自身的写入随回滚丢弃，
// 从而在不清空基线的前提下保证包内单测隔离，同时满足参照完整性，并避免重放入户写冲突。
func Core(t testing.TB) *database.DB {
	t.Helper()
	coreOnce.Do(func() {
		coreDB, coreInitErr = open(base.CoreTestDbName, "postgres")
		if coreInitErr == nil {
			coreInitErr = seedDefaultNamespace(coreDB)
		}
	})
	if coreInitErr != nil {
		t.Fatalf("testdb.Core: %v", coreInitErr)
	}

	// 回滚同一测试的上一个事务，释放其持有的写锁；
	// 若事务已被测试提交或回滚则 rollback 报错，忽略即可。
	if prev, ok := corePrev.LoadAndDelete(t); ok {
		if prevDB, ok := prev.(*database.DB); ok && prevDB != nil {
			_ = prevDB.Rollback().Error
		}
	}

	tx := coreDB.Begin()
	if err := tx.Error; err != nil {
		t.Fatalf("testdb.Core: begin transaction: %v", err)
	}
	corePrev.Store(t, tx)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	return tx
}

// seedDefaultNamespace 首次初始化后在测试库中种入默认命名空间（仅执行一次并提交），
// 以满足 notification_* 等表上 namespace_name 的外键约束。
func seedDefaultNamespace(db *database.DB) error {
	return db.Exec(`INSERT INTO namespaces (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, base.DefaultNamespace).Error
}

// Raw 返回已迁移的共享 ClickHouse 测试库（raw_test）。
// 每次调用会先 TRUNCATE 全部业务表，保证测试从干净状态开始。
// 连接或迁移失败时以 Fatal 终止测试。
func Raw(t testing.TB) *database.DB {
	t.Helper()
	rawOnce.Do(func() {
		rawDB, rawInitErr = open(base.RawTestDbName, "clickhouse")
	})
	if rawInitErr != nil {
		t.Fatalf("testdb.Raw: %v", rawInitErr)
	}
	if err := truncate(rawDB, "clickhouse"); err != nil {
		t.Fatalf("testdb.Raw: truncate: %v", err)
	}
	return rawDB
}

// open 确保测试库存在、建立连接并在本进程内完成一次迁移。
// 迁移由独立连接执行（不复用返回的 gorm 连接，避免迁移关闭连接池）。
func open(prefix, driver string) (*database.DB, error) {
	dsn := loadDSN(prefix)
	if dsn == "" {
		return nil, errors.Newf("no DSN configured for test database %s (config.toml [database.%s] or env %s/%s)", prefix, prefix, coreDSNEnv, rawDSNEnv)
	}

	// 确保数据库存在（幂等）
	if err := migrate.EnsureDatabase(context.Background(), driver, dsn); err != nil {
		return nil, errors.WithStack(err)
	}

	// 读取 mysql/pg/ch 场景通用日志
	logger, err := logging.NewDebugLogger()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	db, err := database.NewDatabase(&database.Config{Driver: driver, DSN: dsn}, logger)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 进程内仅迁移一次
	mc := migrate.NewConfig(driver, dsn).
		WithName(prefix).
		WithMigrations(migrations.MigrationFor(prefix), ".")
	m, err := migrate.NewMigrator(mc)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer m.Close()
	if err := m.Up(context.Background()); err != nil {
		return nil, errors.WithStack(err)
	}
	return db, nil
}

// truncate 清空全部业务表（排除 goose 版本表），保证测试隔离。仅供 ClickHouse 使用。
func truncate(db *database.DB, driver string) error {
	sqlDB, err := db.DB()
	if err != nil {
		return errors.WithStack(err)
	}
	switch driver {
	case "postgres":
		return errors.New("testdb: postgres test db uses transaction-rollback isolation; TRUNCATE is not allowed")
	case "clickhouse":
		return truncateClickHouse(sqlDB)
	default:
		return errors.Newf("testdb: unsupported driver: %s", driver)
	}
}

// truncateClickHouse 清空当前数据库下全部业务表。
func truncateClickHouse(sqlDB *sql.DB) error {
	tables, err := clickHouseTables(sqlDB)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, name := range tables {
		if _, err := sqlDB.Exec(`TRUNCATE TABLE ` + name); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// clickHouseTables 返回需要清空的业务表（排除 goose 版本表）。
func clickHouseTables(sqlDB *sql.DB) ([]string, error) {
	rows, err := sqlDB.Query(`SELECT name FROM system.tables WHERE database = currentDatabase() AND name <> 'goose_db_version'`)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, errors.WithStack(err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// loadDSN 依次尝试环境变量覆盖、config.toml，返回测试库 DSN。
// 通过从当前工作目录向上查找 config.toml 定位仓库根目录（单测工作目录为各包目录）。
func loadDSN(prefix string) string {
	envKey := coreDSNEnv
	if prefix == base.RawTestDbName {
		envKey = rawDSNEnv
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	path := repoConfigPath()
	if path == "" {
		return ""
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString(fmt.Sprintf("database.%s.dsn", prefix))
}

// repoConfigPath 从当前工作目录向上查找仓库根目录的 config.toml。
func repoConfigPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, "config.toml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
