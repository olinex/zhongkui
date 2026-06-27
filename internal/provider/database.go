package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

const (
	CORE_DB_LABEL  = "database.core"
	CORE_DB_PREFIX = "core"
	RAW_DB_LABEL   = "database.raw"
	RAW_DB_PREFIX  = "raw"
)

func newDatabaseProvider(name string) do.Provider[*database.DB] {
	return func(i do.Injector) (*database.DB, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := database.ExtractConfig(viper, name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		logger, err := InvokeDatabaseLogger(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		db, err := database.NewDatabase(cfg, logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，为 GORM 添加 OTEL 追踪插件
		if plugin := otel.GormPlugin(); plugin != nil {
			if err := db.Use(plugin); err != nil {
				return nil, errors.WithStack(err)
			}
		}
		return db, nil
	}
}

func NewCoreDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(CORE_DB_PREFIX)
}

func NewRawDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(RAW_DB_PREFIX)
}

func InjectCoreDatabase(i do.Injector) {
	do.ProvideNamed(i, CORE_DB_LABEL, NewCoreDatabase())
}
func InjectRawDatabase(i do.Injector) {
	do.ProvideNamed(i, RAW_DB_LABEL, NewRawDatabase())
}

func InvokeCoreDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, CORE_DB_LABEL)
}

func MustInvokeCoreDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, CORE_DB_LABEL)
}

func InvokeRawDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, RAW_DB_LABEL)
}

func MustInvokeRawDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, RAW_DB_LABEL)
}

func InvokeDatabase(i do.Injector, dbName string) (*database.DB, error) {
	switch dbName {
	case CORE_DB_PREFIX:
		return InvokeCoreDatabase(i)
	case RAW_DB_PREFIX:
		return InvokeRawDatabase(i)
	default:
		return nil, errors.Newf("unknown database: %s", dbName)
	}
}

func MustInvokeDatabase(i do.Injector, dbName string) *database.DB {
	switch dbName {
	case CORE_DB_PREFIX:
		return MustInvokeCoreDatabase(i)
	case RAW_DB_PREFIX:
		return MustInvokeRawDatabase(i)
	default:
		panic(errors.Newf("unknown database: %s", dbName))
	}
}

func DescribeDatabaseConfig() config.ConfigDescriptions {
	return database.DescribeConfig(CORE_DB_PREFIX, RAW_DB_PREFIX)
}
