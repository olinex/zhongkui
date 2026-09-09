package sqlitevtab

import "zhongkui.com/zhongkui/internal/pkg/errors"

// 包级错误哨兵，均带 sqlitevtab: 前缀。
// Package-level error sentinels, all prefixed with sqlitevtab:.
var (
	// ErrInvalidDatasourceName 数据源名不是合法 SQL 标识符。
	// ErrInvalidDatasourceName the datasource name is not a valid SQL identifier.
	ErrInvalidDatasourceName = errors.New("sqlitevtab: invalid datasource name")
	// ErrInvalidColumnName 列名不是合法 SQL 标识符，或 schema 没有列。
	// ErrInvalidColumnName the column name is not a valid SQL identifier, or
	// the schema has no columns.
	ErrInvalidColumnName = errors.New("sqlitevtab: invalid column name")
	// ErrDuplicateColumn 存在重复列名。
	// ErrDuplicateColumn duplicate column names.
	ErrDuplicateColumn = errors.New("sqlitevtab: duplicate column name")
	// ErrColumnIndexOutOfRange Filterable 中存在越界列索引。
	// ErrColumnIndexOutOfRange a filterable column index is out of range.
	ErrColumnIndexOutOfRange = errors.New("sqlitevtab: filterable column index out of range")
	// ErrDatasourceNotRegistered 数据源未注册或已被注销。
	// ErrDatasourceNotRegistered the datasource is not registered or has been
	// unregistered.
	ErrDatasourceNotRegistered = errors.New("sqlitevtab: datasource not registered")
	// ErrTooManyRows 查询结果行数超过上限。
	// ErrTooManyRows the query result exceeds the row limit.
	ErrTooManyRows = errors.New("sqlitevtab: too many rows")
	// ErrDuplicateResultColumn 查询结果存在重复列名。
	// ErrDuplicateResultColumn duplicate result column names.
	ErrDuplicateResultColumn = errors.New("sqlitevtab: duplicate result column")
	// ErrUnsupportedBindValue 查询参数类型不受支持。
	// ErrUnsupportedBindValue the query argument type is not supported.
	ErrUnsupportedBindValue = errors.New("sqlitevtab: unsupported bind value")
)
