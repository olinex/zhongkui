// Package codes 提供国际标准代码的查询与转换功能。
//
// 本包通过 Go 的 embed 机制将 ISO 标准数据文件嵌入到二进制中，
// 避免了运行时对外部文件系统的依赖，支持离线使用。
//
// 目前支持的标准包括：
//   - ISO 639-2：语言代码（如 zh、en、ja）
//   - ISO 3166-1：国家/地区代码（如 CN、US、JP）
//   - ISO 4217：货币代码（如 CNY、USD、JPY）
//   - ISO 15924：文字书写系统代码（如 Hani、Latn）
//
// 此外还包含一组文件扩展名集合数据，来源于 dyne/file-extension-list 项目。
//
// 基于 ISO 标准代码，本包还组合提供了 RFC 1766（语言+国家/地区）
// 和 RFC 4646（语言+书写系统）的 locale 解析能力。
package codes

import (
	"embed"
)

// embeddedStandards 嵌入了 standards/ 目录下的所有 ISO 标准数据文件。
//
// 数据来源：iso-codes-team/iso-codes 项目
//
//	版本：Release 4.19.0（提交 0f3a862394265a31d8465fc5e627d6429bbd8394）
//	地址：https://salsa.debian.org/iso-codes-team/iso-codes/-/tree/main/data?ref_type=heads
//
// 包含的文件：
//   - iso_639-2.json   — ISO 639-2 语言代码
//   - iso_3166-1.json  — ISO 3166-1 国家/地区代码
//   - iso_4217.json    — ISO 4217 货币代码
//   - iso_15924.json   — ISO 15924 书写系统代码
//
//go:embed standards/*
var embeddedStandards embed.FS

// embeddedCollections 嵌入了 collections/ 目录下的所有集合类数据文件。
//
// 数据来源：dyne/file-extension-list 项目
//
//	提交：a42bccab78c6e3d1941883a46b6ff4f41a137ba8
//	地址：https://github.com/dyne/file-extension-list
//
// 包含的文件：
//   - filename_extensions.json — 文件扩展名与 MIME 类型的映射集合
//
//go:embed collections/*
var embeddedCollections embed.FS

const (
	// 以下为 ISO 标准数据文件的路径常量，用于从 embeddedStandards 中读取对应文件

	// LANGUAGE 指向 ISO 639-2 语言代码数据文件，包含语言的 Alpha-2、Alpha-3 代码及名称。
	LANGUAGE = "standards/iso_639-2.json"

	// COUNTRY_AREA 指向 ISO 3166-1 国家/地区代码数据文件，包含国家/地区的 Alpha-2、Alpha-3、数字代码及名称。
	COUNTRY_AREA = "standards/iso_3166-1.json"

	// CURRENCY 指向 ISO 4217 货币代码数据文件，包含货币的 Alpha-3 代码、数字代码及名称。
	CURRENCY = "standards/iso_4217.json"

	// SCRIPT_LANGUAGE 指向 ISO 15924 书写系统代码数据文件，包含书写系统的 Alpha-4 代码、数字代码及名称。
	SCRIPT_LANGUAGE = "standards/iso_15924.json"

	// 以下为集合类数据文件的路径常量，用于从 embeddedCollections 中读取对应文件

	// FILENAME_EXTENSION 指向文件扩展名集合数据文件，包含文件类型名称与其对应扩展名列表的映射。
	FILENAME_EXTENSION = "collections/filename_extensions.json"
)
