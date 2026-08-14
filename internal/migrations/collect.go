package migrations

import (
	"embed"

	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/provider"
)

func MigrationFor(dbName string) embed.FS {
	switch dbName {
	case provider.CoreDbPrefix:
		return migcore.Migrations
	case provider.RawDbPrefix:
		return migraw.Migrations
	default:
		panic(errors.Newf(`unknown db name: %s`, dbName))
	}
}
