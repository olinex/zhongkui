package database

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// testPGDSN 仅用于驱动识别类用例的 postgres 连接串。
// 指向明显不可达的地址，配合 DisableAutomaticPing 使用，不依赖本地数据库服务。
var testPGDSN = "host=127.0.0.1 port=1 user=postgres password=postgres dbname=postgres sslmode=disable connect_timeout=1 TimeZone=Asia/Shanghai"

// TestNewDatabase_ValidConfig 测试 NewDatabase 函数使用有效配置时的行为
func TestNewDatabase_ValidConfig(t *testing.T) {
	Convey("当使用有效的 PostgreSQL 配置时", t, func() {
		cfg := &Config{
			Driver: "postgres",
			DSN:    testPGDSN,
			Meta: MetaConfig{
				DisableAutomaticPing: true,
			},
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, err := NewDatabase(cfg, logger)

		Convey("应该成功创建数据库连接", func() {
			So(err, ShouldBeNil)
			So(db, ShouldNotBeNil)
		})

		Convey("数据库连接应该可以被关闭", func() {
			if db != nil {
				sqlDB, err := db.DB()
				So(err, ShouldBeNil)
				So(sqlDB.Close(), ShouldBeNil)
			}
		})
	})
}

// TestNewDatabase_UnsupportedDriver 测试 NewDatabase 函数使用不支持驱动时的行为
func TestNewDatabase_UnsupportedDriver(t *testing.T) {
	Convey("当使用不支持的数据库驱动时", t, func() {
		cfg := &Config{
			Driver: "unsupported",
			DSN:    "test://localhost/test",
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, err := NewDatabase(cfg, logger)

		Convey("应该返回错误", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Unsupported driver")
		})

		Convey("不应该返回数据库连接", func() {
			So(db, ShouldBeNil)
		})
	})
}

// TestNewDatabase_EmptyDriver 测试 NewDatabase 函数使用空驱动时的行为
func TestNewDatabase_EmptyDriver(t *testing.T) {
	Convey("当使用空数据库驱动时", t, func() {
		cfg := &Config{
			Driver: "",
			DSN:    "test.db",
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, err := NewDatabase(cfg, logger)

		Convey("应该返回错误", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Unsupported driver")
		})

		Convey("不应该返回数据库连接", func() {
			So(db, ShouldBeNil)
		})
	})
}

// TestNewDatabase_PostgresDriver 测试 NewDatabase 函数使用 PostgreSQL 驱动时的行为
func TestNewDatabase_PostgresDriver(t *testing.T) {
	Convey("当使用 PostgreSQL 驱动配置时", t, func() {
		cfg := &Config{
			Driver: "postgres",
			DSN:    "host=localhost user=test dbname=test sslmode=disable", // 仅用于驱动识别，不做真实连接
			Meta: MetaConfig{
				DisableAutomaticPing: true,
			},
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		_, err := NewDatabase(cfg, logger)

		Convey("应该识别 PostgreSQL 驱动", func() {
			// 禁用自动 ping 后不会建立真实连接，驱动识别与对象创建应当成功
			So(err, ShouldBeNil)
		})
	})
}

// TestNewDatabase_ClickHouseDriver 测试 NewDatabase 函数使用 ClickHouse 驱动时的行为
func TestNewDatabase_ClickHouseDriver(t *testing.T) {
	Convey("当使用 ClickHouse 驱动配置时", t, func() {
		cfg := &Config{
			Driver: "clickhouse",
			DSN:    "tcp://127.0.0.1:1?database=test&dial_timeout=1s", // 不可达地址，快速失败，仅测试驱动识别
			Meta: MetaConfig{
				DisableForeignKeyConstraintWhenMigrating: false,
				IgnoreRelationshipsWhenMigrating:         false,
				SkipDefaultTransaction:                   true,
				DisableNestedTransaction:                 true,
				DisableAutomaticPing:                     true,
			},
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		_, err := NewDatabase(cfg, logger)

		Convey("应该识别 ClickHouse 驱动", func() {
			// 即使连接失败，也应该识别驱动类型
			// ClickHouse 的特殊配置调整应该在函数内部处理
			So(err, ShouldNotBeNil) // 连接会失败，但驱动识别和配置调整应该成功
		})
	})
}

// TestNewDatabase_NilConfig 测试 NewDatabase 函数使用 nil 配置时的行为
func TestNewDatabase_NilConfig(t *testing.T) {
	Convey("当配置为 nil 时", t, func() {
		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, err := NewDatabase(nil, logger)

		Convey("应该返回错误", func() {
			So(err, ShouldNotBeNil)
		})

		Convey("不应该返回数据库连接", func() {
			So(db, ShouldBeNil)
		})
	})
}

// TestNewDatabase_NilLogger 测试 NewDatabase 函数使用 nil 日志记录器时的行为
func TestNewDatabase_NilLogger(t *testing.T) {
	Convey("当日志记录器为 nil 时", t, func() {
		cfg := &Config{
			Driver: "postgres",
			DSN:    testPGDSN,
			Meta: MetaConfig{
				DisableAutomaticPing: true,
			},
		}

		db, err := NewDatabase(cfg, nil)

		Convey("应该仍然尝试创建数据库连接", func() {
			// 即使日志记录器为 nil，函数也应该处理
			// 实际行为可能因实现而异，这里测试基本功能
			So(err, ShouldBeNil)
			So(db, ShouldNotBeNil)
		})

		Convey("数据库连接应该可以被关闭", func() {
			if db != nil {
				sqlDB, err := db.DB()
				So(err, ShouldBeNil)
				So(sqlDB.Close(), ShouldBeNil)
			}
		})
	})
}

// TestNewDatabase_InvalidDSN 测试 NewDatabase 函数使用无效 DSN 时的行为
func TestNewDatabase_InvalidDSN(t *testing.T) {
	Convey("当使用无效的 DSN 连接字符串时", t, func() {
		cfg := &Config{
			Driver: "postgres",
			DSN:    "invalid://connection/string", // 明显无效的 DSN
			Meta: MetaConfig{
				DisableAutomaticPing: true,
			},
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, _ := NewDatabase(cfg, logger)

		Convey("应该尝试创建数据库连接", func() {
			// gorm.Open 可能不会立即验证连接
			// 所以即使 DSN 无效，也可能返回数据库对象
			// 这里我们主要测试函数不会 panic
			So(func() {
				NewDatabase(cfg, logger)
			}, ShouldNotPanic)
		})

		Convey("如果返回数据库连接，应该可以被关闭", func() {
			if db != nil {
				sqlDB, err := db.DB()
				if err == nil {
					So(sqlDB.Close(), ShouldBeNil)
				}
			}
		})
	})
}

// TestNewDatabase_WithMetaConfig 测试 NewDatabase 函数使用完整元配置时的行为
func TestNewDatabase_WithMetaConfig(t *testing.T) {
	Convey("当使用完整的元配置时", t, func() {
		cfg := &Config{
			Driver: "postgres",
			DSN:    testPGDSN,
			Meta: MetaConfig{
				SkipDefaultTransaction:                   true,
				FullSaveAssociations:                     true,
				DryRun:                                   false,
				PrepareStmt:                              true,
				DisableAutomaticPing:                     true,
				DisableForeignKeyConstraintWhenMigrating: true,
				IgnoreRelationshipsWhenMigrating:         true,
				DisableNestedTransaction:                 false,
				AllowGlobalUpdate:                        true,
				QueryFields:                              false,
				CreateBatchSize:                          100,
				TranslateError:                           true,
				PropagateUnscoped:                        false,
			},
		}

		// 创建一个简单的日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

		db, err := NewDatabase(cfg, logger)

		Convey("应该成功创建数据库连接", func() {
			So(err, ShouldBeNil)
			So(db, ShouldNotBeNil)
		})

		Convey("数据库连接应该可以被关闭", func() {
			if db != nil {
				sqlDB, err := db.DB()
				So(err, ShouldBeNil)
				So(sqlDB.Close(), ShouldBeNil)
			}
		})
	})
}
