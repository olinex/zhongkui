package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	_ "zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	CoreDbLabel  = "database.core"
	CoreDbPrefix = "core"
	RawDbLabel   = "database.raw"
	RawDbPrefix  = "raw"
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
	return newDatabaseProvider(CoreDbPrefix)
}

func NewRawDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(RawDbPrefix)
}

func InjectCoreDatabase(i do.Injector) {
	do.ProvideNamed(i, CoreDbLabel, NewCoreDatabase())
}
func InjectRawDatabase(i do.Injector) {
	do.ProvideNamed(i, RawDbLabel, NewRawDatabase())
}

func InvokeCoreDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, CoreDbLabel)
}

func MustInvokeCoreDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, CoreDbLabel)
}

func InvokeRawDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, RawDbLabel)
}

func MustInvokeRawDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, RawDbLabel)
}

func InvokeDatabase(i do.Injector, dbName string) (*database.DB, error) {
	switch dbName {
	case CoreDbPrefix:
		return InvokeCoreDatabase(i)
	case RawDbPrefix:
		return InvokeRawDatabase(i)
	default:
		return nil, errors.Newf("unknown database: %s", dbName)
	}
}

func MustInvokeDatabase(i do.Injector, dbName string) *database.DB {
	switch dbName {
	case CoreDbPrefix:
		return MustInvokeCoreDatabase(i)
	case RawDbPrefix:
		return MustInvokeRawDatabase(i)
	default:
		panic(errors.Newf("unknown database: %s", dbName))
	}
}

func DescribeDatabaseConfig() config.ConfigDescriptions {
	return database.DescribeConfig(CoreDbPrefix, RawDbPrefix)
}
