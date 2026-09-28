// Package vtab（本文件）提供数据源的「预检」能力：落库注册之前供视图层提前调用的
// 两个纯校验函数 TestConn 与 IntrospectSchema（sqlite 文件自省已并入 IntrospectSchema，
// 经 file 参数传入 .db 文件流）。它们复用 register.go 的 decodeDatasourceConfig 与各
// 数据源的连接池/自省逻辑，但既不落库也不注册 vtable：TestConn 仅建立连接并校验连通性；
// IntrospectSchema 建立连接后解析目标表结构并映射为列定义（列名 + 类型键），供前端在
// 创建数据源时展示。
//
// 命名空间统一用 ""（空串）：预检不持久化任何连接，故连接池按空命名空间派生临时 Key，
// 不与真实注册的命名空间池串扰。
package vtab

import (
	// 标准库
	"context"
	"io"
	"os"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// TestConn 按数据源类型建立连接并校验连通性，失败返回带上下文的原因。
// redis 经 Ping 探活、mysql/postgres/clickhouse 经 PingContext、
// etcd 经 MemberList（覆盖连通+认证）、mongo 经 Ping(Primary) 校验；
// sqlite_file 无网络连接，返回「不支持连接测试」错误。
func TestConn(ctx context.Context, typ string, cfgData []byte) error {
	cfg, err := decodeDatasourceConfig(typ, cfgData)
	if err != nil {
		return err
	}
	p, ok := lookupDatasourcePlugin(typ)
	if !ok {
		return errors.Newf("tag datasource: unsupported type %q", typ)
	}
	return p.TestConn(ctx, cfg)
}

// IntrospectSchema 按数据源类型解析表结构并返回列定义（列名 + 类型键）。
// mysql/postgres/clickhouse 经各自 SHOW COLUMNS / information_schema / DESCRIBE 自省；
// redis 经 Ping 探活后按 Struct 返回该结构的固定列；etcd 探活后返回固定 key/value 两列；
// mongo 无固定结构，返回「需手动填写 schema」错误；sqlite_file 复用同一入口，要求传入
// 前端上传的 .db 文件（io.Reader），内部经 introspectSQLiteFile 做 PRAGMA table_info 自省，
// 未传文件返回错误。非 sqlite_file 类型传入的 file 被忽略。故障返回带上下文原因。
func IntrospectSchema(ctx context.Context, typ string, cfgData []byte, file io.Reader) ([]DatasourceColumn, error) {
	cfg, err := decodeDatasourceConfig(typ, cfgData)
	if err != nil {
		return nil, err
	}
	p, ok := lookupDatasourcePlugin(typ)
	if !ok {
		return nil, errors.Newf("tag datasource: unsupported type %q", typ)
	}
	return p.IntrospectSchema(ctx, cfg, file)
}

// introspectSQLiteFile 对前端上传的 .db 文件（以 io.Reader 传入明文内容）做 PRAGMA
// table_info 自省，返回列定义。文件写入临时文件打开，结束后清理。仅由 IntrospectSchema
// 的 sqlite_file 分支调用。
func introspectSQLiteFile(ctx context.Context, table string, r io.Reader) ([]DatasourceColumn, error) {
	tmp, err := os.CreateTemp("", "sqlite-introspect-*.db")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	path := tmp.Name()
	defer tmp.Close()
	defer os.Remove(path)
	if _, err := io.Copy(tmp, r); err != nil {
		return nil, errors.WithStack(err)
	}
	pool := NewSQLiteFilePool(&SQLiteFileConfig{Path: path})
	defer pool.Close(ctx)
	ds, err := NewSQLiteFileDatasource(ctx, pool, table)
	if err != nil {
		return nil, err
	}
	return schemaColumnsOut(ds.datasourceSchema()), nil
}

// valueTypeKey 将 sqlitevtab.ValueType 反向映射为手动 schema 的类型键
// （any/integer/float/string/blob/bool），与 columnValueType 互为逆映射；未知类型
// （理论上不会发生）映射为 "any"。
func valueTypeKey(vt sqlitevtab.ValueType) string {
	switch vt {
	case sqlitevtab.ValueTypeAny:
		return "any"
	case sqlitevtab.ValueTypeInt64:
		return "integer"
	case sqlitevtab.ValueTypeFloat64:
		return "float"
	case sqlitevtab.ValueTypeString:
		return "string"
	case sqlitevtab.ValueTypeBlob:
		return "blob"
	case sqlitevtab.ValueTypeBool:
		return "bool"
	default:
		return "any"
	}
}

// schemaColumnsOut 把 sqlitevtab.Schema 的列映射为视图层可用的 []DatasourceColumn。
func schemaColumnsOut(schema sqlitevtab.Schema) []DatasourceColumn {
	out := make([]DatasourceColumn, 0, len(schema.Columns))
	for _, col := range schema.Columns {
		out = append(out, DatasourceColumn{Name: col.Name, Type: valueTypeKey(col.ValueType)})
	}
	return out
}
