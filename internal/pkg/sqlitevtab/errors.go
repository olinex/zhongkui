package sqlitevtab

import "zhongkuisec.com/zhongkui/internal/pkg/errors"

// 包级错误哨兵，均带 sqlitevtab: 前缀。
var (
	// ErrInvalidDatasourceName 数据源名不是合法 SQL 标识符。
	ErrInvalidDatasourceName = errors.New("sqlitevtab: invalid datasource name")
	// ErrInvalidColumnName 列名不是合法 SQL 标识符，或 schema 没有列。
	ErrInvalidColumnName = errors.New("sqlitevtab: invalid column name")
	// ErrDuplicateColumn 存在重复列名。
	ErrDuplicateColumn = errors.New("sqlitevtab: duplicate column name")
	// ErrColumnIndexOutOfRange Filterable 中存在越界列索引。
	ErrColumnIndexOutOfRange = errors.New("sqlitevtab: filterable column index out of range")
	// ErrDatasourceNotRegistered 数据源未注册或已被注销。
	ErrDatasourceNotRegistered = errors.New("sqlitevtab: datasource not registered")
	// ErrTooManyRows 查询结果行数超过上限。
	ErrTooManyRows = errors.New("sqlitevtab: too many rows")
	// ErrDuplicateResultColumn 查询结果存在重复列名。
	ErrDuplicateResultColumn = errors.New("sqlitevtab: duplicate result column")
	// ErrUnsupportedBindValue 查询参数类型不受支持。
	ErrUnsupportedBindValue = errors.New("sqlitevtab: unsupported bind value")
)
