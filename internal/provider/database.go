package provider

import (
	"github.com/samber/do/v2"
	apimodel "zhongkui.com/zhongkui/internal/models/api"
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	netmodel "zhongkui.com/zhongkui/internal/models/network"
	tagmodel "zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	_ "zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	CoreDbLabel     = "database." + base.CoreDbName
	RawDbLabel      = "database." + base.RawDbName
	CoreTestDbLabel = "database." + base.CoreTestDbName
	RawTestDbLabel  = "database." + base.RawTestDbName
)

// CoreModels 返回 core 数据库的全部打标模型清单，供迁移一致性检查使用。
func CoreModels() []any {
	return []any{
		&coremodel.User{},
		&coremodel.Group{},
		&coremodel.Namespace{},
		&coremodel.Capturer{},
		&coremodel.AccessKey{},
		&coremodel.RbacPolicy{},
		&coremodel.RbacPolicyTemplate{},
		&coremodel.NotificationChannel{},
		&coremodel.NotificationTarget{},
		&coremodel.NotificationGroup{},
		&coremodel.NotificationGroupTarget{},
		&coremodel.NotificationRecord{},
		&coremodel.NotificationTemplate{},
		&tagmodel.TagEventRule{},
		&tagmodel.TagEventCondition{},
		&tagmodel.TagEventRuleVersion{},
		&tagmodel.TagEventRecord{},
		&tagmodel.TagEventRecordHit{},
		&tagmodel.TagRule{},
		&tagmodel.TagRuleVersion{},
		&tagmodel.TagRuleSample{},
		&coremodel.LlmModel{},
		&coremodel.LlmBinding{},
		&coremodel.AgentParam{},
		&coremodel.McpServer{},
		&coremodel.LlmTool{},
	}
}

// RawModels 返回 raw 数据库的全部打标模型清单，供迁移一致性检查使用。
func RawModels() []any {
	return []any{
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
	}
}

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

func NewCoreDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(base.CoreDbName)
}

func NewRawDatabase() do.Provider[*database.DB] {
	return newDatabaseProvider(base.RawDbName)
}

// Models 按数据库名返回对应的打标模型清单。
func Models(dbName string) []any {
	switch dbName {
	case base.CoreDbName:
		return CoreModels()
	case base.RawDbName:
		return RawModels()
	default:
		panic(errors.Newf("unknown schema: %s", dbName))
	}
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
	case base.CoreDbName:
		return InvokeCoreDatabase(i)
	case base.RawDbName:
		return InvokeRawDatabase(i)
	default:
		return nil, errors.Newf("unknown database: %s", dbName)
	}
}

func MustInvokeDatabase(i do.Injector, dbName string) *database.DB {
	switch dbName {
	case base.CoreDbName:
		return MustInvokeCoreDatabase(i)
	case base.RawDbName:
		return MustInvokeRawDatabase(i)
	default:
		panic(errors.Newf("unknown database: %s", dbName))
	}
}

func DescribeDatabaseConfig() config.ConfigDescriptions {
	return database.DescribeConfig(
		base.CoreDbName,
		base.RawDbName,
		base.CoreTestDbName,
		base.RawTestDbName,
	)
}
