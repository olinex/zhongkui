package provider

import (
	"github.com/samber/do/v2"
	apimodel "zhongkui.com/zhongkui/internal/models/api"
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	netmodel "zhongkui.com/zhongkui/internal/models/network"
	tagmodel "zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	_ "zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	CoreDbLabel            = "database.core"
	CoreDbPrefix           = "core"
	RawDbLabel             = "database.raw"
	RawDbPrefix            = "raw"
	CoreSchemaManagerLabel = "schema.core"
	RawSchemaManagerLabel  = "schema.raw"
)

func newDatabaseProvider(name string) do.Provider[*database.DB] {
	return func(i do.Injector) (*database.DB, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := database.ExtractConfig(viper, validator.Validate, name); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if db, err := database.NewDatabase(cfg, logger); err != nil {
			return nil, errors.WithStack(err)
		} else if plugin := otel.GormPlugin(); plugin != nil {
			if err := db.Use(plugin); err != nil {
				return nil, errors.WithStack(err)
			}
			return db, nil
		} else {
			return db, nil
		}
	}
}

func newSchemaManagerProvider(name string, models ...any) do.Provider[*database.SchemaManager] {
	return func(i do.Injector) (*database.SchemaManager, error) {
		manager := database.NewSchemaManager()
		for _, model := range models {
			manager.Register(model)
		}
		if db, err := InvokeDatabase(i, name); err != nil {
			return nil, errors.WithStack(err)
		} else {
			manager.Sync(db)
			return manager, nil
		}
	}
}

func NewCoreDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(CoreDbPrefix)
}

func NewCoreSchemaManager() do.Provider[*database.SchemaManager] {
	return newSchemaManagerProvider(
		CoreDbPrefix,
		&coremodel.User{},
		&coremodel.Group{},
		&coremodel.Namespace{},
		&coremodel.Capturer{},
		&coremodel.AccessKey{},
		&coremodel.RbacPolicy{},
		&coremodel.RbacPolicyTemplate{},
		&tagmodel.TagEventRule{},
		&tagmodel.TagEventCondition{},
		&tagmodel.TagEventRuleVersion{},
		&tagmodel.TagRule{},
		&tagmodel.TagRuleVersion{},
		&tagmodel.TagRuleSample{},
	)
}

func NewRawDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(RawDbPrefix)
}

func NewRawSchemaManager() do.Provider[*database.SchemaManager] {
	return newSchemaManagerProvider(RawDbPrefix,
		&netmodel.PacketIPv4{},
		&netmodel.PacketIPv6{},
		&netmodel.PacketIPv4TCP{},
		&netmodel.PacketIPv6TCP{},
		&netmodel.PacketIPv4UDP{},
		&netmodel.PacketIPv6UDP{},
		&netmodel.HttpTransport{},
		&netmodel.NetworkStack{},
		&apimodel.HttpApiMeta{},
		&apimodel.HttpApiNode{},
		&tagmodel.TagSumming{},
	)
}

func InjectCoreDatabase(i do.Injector) {
	do.ProvideNamed(i, CoreDbLabel, NewCoreDatabase())
}

func InjectCoreSchemaManager(i do.Injector) {
	do.ProvideNamed(i, CoreSchemaManagerLabel, NewCoreSchemaManager())
}

func InjectRawDatabase(i do.Injector) {
	do.ProvideNamed(i, RawDbLabel, NewRawDatabase())
}

func InjectRawSchemaManager(i do.Injector) {
	do.ProvideNamed(i, RawSchemaManagerLabel, NewRawSchemaManager())
}

func InvokeCoreDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, CoreDbLabel)
}

func InvokeCoreSchemaManager(i do.Injector) (*database.SchemaManager, error) {
	return do.InvokeNamed[*database.SchemaManager](i, CoreSchemaManagerLabel)
}

func MustInvokeCoreDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, CoreDbLabel)
}

func MustInvokeCoreSchemaManager(i do.Injector) *database.SchemaManager {
	return do.MustInvokeNamed[*database.SchemaManager](i, CoreSchemaManagerLabel)
}

func InvokeRawDatabase(i do.Injector) (*database.DB, error) {
	return do.InvokeNamed[*database.DB](i, RawDbLabel)
}

func InvokeRawSchemaManager(i do.Injector) (*database.SchemaManager, error) {
	return do.InvokeNamed[*database.SchemaManager](i, RawSchemaManagerLabel)
}

func MustInvokeRawDatabase(i do.Injector) *database.DB {
	return do.MustInvokeNamed[*database.DB](i, RawDbLabel)
}

func MustInvokeRawSchemaManager(i do.Injector) *database.SchemaManager {
	return do.MustInvokeNamed[*database.SchemaManager](i, RawSchemaManagerLabel)
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

func InvokeSchemaManager(i do.Injector, dbName string) (*database.SchemaManager, error) {
	switch dbName {
	case CoreDbPrefix:
		return InvokeCoreSchemaManager(i)
	case RawDbPrefix:
		return InvokeRawSchemaManager(i)
	default:
		return nil, errors.Newf("unknown schema manager: %s", dbName)
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

func MustInvokeSchemaManager(i do.Injector, dbName string) *database.SchemaManager {
	switch dbName {
	case CoreDbPrefix:
		return MustInvokeCoreSchemaManager(i)
	case RawDbPrefix:
		return MustInvokeRawSchemaManager(i)
	default:
		panic(errors.Newf("unknown schema manager: %s", dbName))
	}
}

func DescribeDatabaseConfig() config.ConfigDescriptions {
	return database.DescribeConfig(CoreDbPrefix, RawDbPrefix)
}
