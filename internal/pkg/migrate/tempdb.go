package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

type dbDialect struct {
	urlSchemes        []string
	defaultDBName     string
	dbKeyName         string
	adminDBName       string
	quoteDB           bool
	createIfNotExists bool
}

var (
	postgresDialect = dbDialect{
		urlSchemes:        []string{"postgres://", "postgresql://"},
		defaultDBName:     "postgres",
		dbKeyName:         "dbname",
		adminDBName:       "postgres",
		quoteDB:           true,
		createIfNotExists: false,
	}
	clickhouseDialect = dbDialect{
		urlSchemes:        []string{"clickhouse://"},
		defaultDBName:     "default",
		dbKeyName:         "database",
		adminDBName:       "default",
		quoteDB:           false,
		createIfNotExists: true,
	}
)

// TempDB 表示一个已创建的临时数据库及其连接信息。
type TempDB struct {
	// cfg 迁移配置。
	cfg *Config
	// db 临时数据库连接。
	db *sql.DB
	// dsn 临时数据库的 DSN。
	dsn string
	// name 临时数据库名。
	name string
	// dialect 数据库方言。
	dialect dbDialect
}

func sqlDriverName(gormDriver string) string {
	switch gormDriver {
	case "postgres":
		return "pgx"
	default:
		return gormDriver
	}
}

// EnsureDatabase 确保目标数据库存在，不存在则创建。
// 复用 tempdb 的 dialect 与 DSN 解析逻辑，供测试助手（internal/pkg/testdb）在连接测试库前调用。
// PostgreSQL 的 CREATE DATABASE 不支持 IF NOT EXISTS，目标已存在时报错视为正常（幂等）。
func EnsureDatabase(ctx context.Context, driver, dsn string) error {
	d, err := dialectFor(driver)
	if err != nil {
		return errors.WithStack(err)
	}

	adminDSN, dbName, err := parseDSN(dsn, d)
	if err != nil {
		return errors.WithStack(err)
	}

	adminDB, err := sql.Open(sqlDriverName(driver), adminDSN)
	if err != nil {
		return errors.WithStack(err)
	}
	defer adminDB.Close()

	if _, err := adminDB.ExecContext(ctx, d.createSQL(dbName)); err != nil {
		if databaseAlreadyExists(err) {
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}

// databaseAlreadyExists 判断数据库创建失败是否因目标已存在（幂等场景的预期情况）。
func databaseAlreadyExists(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists")
}

func dialectFor(driver string) (dbDialect, error) {
	switch driver {
	case "postgres":
		return postgresDialect, nil
	case "clickhouse":
		return clickhouseDialect, nil
	default:
		return dbDialect{}, errors.Newf("unsupported driver for temp db: %s", driver)
	}
}

// CreateTempDB 按配置创建临时数据库并返回其连接句柄。
func CreateTempDB(ctx context.Context, cfg *Config) (*TempDB, error) {
	suffix, err := crypto.GenerateRandomString(8)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dialect, err := dialectFor(cfg.Driver)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return createTempDB(ctx, cfg, suffix, dialect)
}

func createTempDB(ctx context.Context, cfg *Config, suffix string, d dbDialect) (*TempDB, error) {
	adminDSN, dbName, err := parseDSN(cfg.DSN, d)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	driver := sqlDriverName(cfg.Driver)
	adminDB, err := sql.Open(driver, adminDSN)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer adminDB.Close()

	tempName := fmt.Sprintf("%s_dryrun_%s", dbName, suffix)

	if _, err = adminDB.ExecContext(ctx, d.createSQL(tempName)); err != nil {
		return nil, errors.WithStack(err)
	}

	tempDSN := replaceDSNName(cfg.DSN, tempName, d)
	tempDB, err := sql.Open(driver, tempDSN)
	if err != nil {
		adminDB.ExecContext(ctx, d.dropSQL(tempName))
		return nil, errors.WithStack(err)
	}

	if err := tempDB.PingContext(ctx); err != nil {
		tempDB.Close()
		adminDB.ExecContext(ctx, d.dropSQL(tempName))
		return nil, errors.Newf("cannot ping temp database: %v", err)
	}

	return &TempDB{
		cfg:     cfg,
		db:      tempDB,
		dsn:     tempDSN,
		name:    tempName,
		dialect: d,
	}, nil
}

// Close 关闭临时数据库连接，并在未设置保留时删除该临时数据库。
func (t *TempDB) Close(ctx context.Context) error {
	if t.db != nil {
		t.db.Close()
	}

	if t.cfg.KeepTemp {
		return nil
	}

	return t.dropDB(ctx)
}

func (t *TempDB) dropDB(ctx context.Context) error {
	adminDSN, _, err := parseDSN(t.cfg.DSN, t.dialect)
	if err != nil {
		return errors.WithStack(err)
	}

	adminDB, err := sql.Open(sqlDriverName(t.cfg.Driver), adminDSN)
	if err != nil {
		return errors.WithStack(err)
	}
	defer adminDB.Close()

	_, err = adminDB.ExecContext(ctx, t.dialect.dropSQL(t.name))
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (d dbDialect) createSQL(name string) string {
	if d.quoteDB {
		s := fmt.Sprintf(`CREATE DATABASE "%s"`, name)
		if d.createIfNotExists {
			s = fmt.Sprintf(`CREATE DATABASE IF NOT EXISTS "%s"`, name)
		}
		return s
	}
	s := fmt.Sprintf("CREATE DATABASE %s", name)
	if d.createIfNotExists {
		s = fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", name)
	}
	return s
}

func (d dbDialect) dropSQL(name string) string {
	if d.quoteDB {
		return fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, name)
	}
	return fmt.Sprintf("DROP DATABASE IF EXISTS %s", name)
}

func (d dbDialect) matchesURLScheme(dsn string) bool {
	for _, scheme := range d.urlSchemes {
		if strings.HasPrefix(dsn, scheme) {
			return true
		}
	}
	return false
}

func parseDSN(dsn string, d dbDialect) (adminDSN, dbName string, err error) {
	if d.matchesURLScheme(dsn) {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", errors.WithStack(err)
		}
		dbName = strings.TrimPrefix(u.Path, "/")
		if dbName == "" {
			dbName = d.defaultDBName
		}
		u.Path = "/" + d.adminDBName
		adminDSN = u.String()
		return adminDSN, dbName, nil
	}

	params := make(map[string]string)
	for part := range strings.FieldsSeq(dsn) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			params[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
		}
	}

	dbName = params[d.dbKeyName]
	if dbName == "" {
		dbName = d.defaultDBName
	}

	params[d.dbKeyName] = d.adminDBName
	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	adminDSN = strings.Join(parts, " ")
	return adminDSN, dbName, nil
}

func replaceDSNName(dsn, newName string, d dbDialect) string {
	if d.matchesURLScheme(dsn) {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		u.Path = "/" + newName
		return u.String()
	}

	params := make(map[string]string)
	for part := range strings.FieldsSeq(dsn) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			params[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
		}
	}
	params[d.dbKeyName] = newName
	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, " ")
}
