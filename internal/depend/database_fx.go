package depend

import (
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

const (
	CORE_DB = "core"
	RAW_DB  = "raw"
)

type DatabaseFxIn struct {
	fx.In
	DatabaseLoggerFxIn
	V *viper.Viper
}

type RawDatabaseFxIn struct {
	fx.In
	RawDB *database.DB `name:"raw"`
}

type CoreDatabaseFxIn struct {
	fx.In
	CoreDB *database.DB `name:"core"`
}

func newDatabase(name string) func(in DatabaseFxIn) *database.DB {
	return func(in DatabaseFxIn) *database.DB {
		if cfg, err := database.ExtractConfig(in.V, name); err != nil {
			panic(err)
		} else if db, err := database.NewDatabase(cfg, in.DatabaseLogger); err != nil {
			panic(err)
		} else {
			return db
		}
	}
}

func databaseAnnotate(name string) any {
	return fx.Annotate(newDatabase(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func RawDatabaseAnnotate() any {
	return databaseAnnotate(RAW_DB)
}

func CoreDatabaseAnnotate() any {
	return databaseAnnotate(CORE_DB)
}

func DescribeDatabaseConfig() config.ConfigDescriptions {
	return database.DescribeConfig(CORE_DB, RAW_DB)
}
