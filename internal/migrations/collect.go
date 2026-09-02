package migrations

import (
	"embed"

	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func MigrationFor(dbName string) embed.FS {
	switch dbName {
	case base.CoreDbName, base.CoreTestDbName:
		return migcore.Migrations
	case base.RawDbName, base.RawTestDbName:
		return migraw.Migrations
	default:
		panic(errors.Newf(`unknown db name: %s`, dbName))
	}
}
