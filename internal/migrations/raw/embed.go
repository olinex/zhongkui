// Package raw 提供原始（未加工）数据库迁移的 SQL 脚本资源，通过 embed 将 *.sql 文件打包进二进制。
package raw

import "embed"

//go:embed *.sql
var Migrations embed.FS
