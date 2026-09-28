// Package migrations 统一聚合各数据库的嵌入式 SQL 迁移文件集，并按数据库名分发。
package migrations

import (
	"embed"

	migcore "zhongkui.com/zhongkui/internal/migrations/core"
	migraw "zhongkui.com/zhongkui/internal/migrations/raw"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// MigrationFor 根据数据库名返回对应数据库的嵌入式迁移文件集，未知库名将 panic。
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
