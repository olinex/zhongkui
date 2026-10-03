// Package vtab（本文件）提供 MongoDB 数据源实现。
// MongoDB 无固定 schema，MongoDatasource 以「一个集合」为一张 vtable：
//   - schema 必须由前端在注册时完整提供（不做首文档自省）：每个集合配置显式声明列名与
//     ValueType，注册/校验时缺省 schema 直接报错（区别于可自省触发的 SQL 数据源）。查询
//     只能引用 schema 中声明的列——未声明字段不可见、不可过滤，实现「按 schema 约束查询」。
//   - Filterable 恒空——Mongo 数值等值比较存在类型强转歧义，任何下推都可能漏行、违反
//     超集契约，故一律由 sqlite 对全量行做本地过滤。
//   - Open 借出 *mongo.Client，Find({}) 取回全部文档，逐文档按 schema 列归一化输出单元格；
//     缺失字段→nil；嵌套文档/数组等复杂值输出为可读文本（ValueTypeAny，仅展示、不参与过滤）。

package vtab

import (
	// 标准库
	"context"
	"fmt"
	"time"

	// 第三方
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// mongoColumn 描述数据源的一列：字段名与 ValueType（来自前端注册时显式声明的 schema）。
type mongoColumn struct {
	name string
	vt   sqlitevtab.ValueType
}

// MongoDatasource 是一个 Mongo 集合的 vtable 数据源。
type MongoDatasource struct {
	// pool 数据源连接池。
	pool *MongoPool
	// schema 注册 schema。
	schema sqlitevtab.Schema
	// valueTypes 各列 ValueType，输出单元格时归一化。
	valueTypes []sqlitevtab.ValueType
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
func (d *MongoDatasource) datasourceSchema() sqlitevtab.Schema {
	return d.schema
}

// NewMongoDatasourceWithSchema 以手动指定的列构建数据源（Mongo 唯一合法建 schema 路径）。
// 语义同 NewMySQLDatasourceWithSchema。
func NewMongoDatasourceWithSchema(pool *MongoPool, columns []sqlitevtab.Column) (*MongoDatasource, error) {
	if len(columns) == 0 {
		return nil, errors.New("mongo: manual schema requires at least one column")
	}
	cols := make([]mongoColumn, len(columns))
	for i, c := range columns {
		cols[i] = mongoColumn{name: c.Name, vt: c.ValueType}
	}
	return newMongoDatasource(pool, cols), nil
}

// newMongoDatasource 以给定列构建数据源（也供测试直接注入列）。
func newMongoDatasource(pool *MongoPool, columns []mongoColumn) *MongoDatasource {
	schema := sqlitevtab.Schema{Columns: make([]sqlitevtab.Column, 0, len(columns))}
	valueTypes := make([]sqlitevtab.ValueType, 0, len(columns))
	for _, c := range columns {
		schema.Columns = append(schema.Columns, sqlitevtab.Column{Name: c.name, ValueType: c.vt})
		valueTypes = append(valueTypes, c.vt)
	}
	return &MongoDatasource{pool: pool, schema: schema, valueTypes: valueTypes}
}

// Open 实现 Datasource.Open：借出客户端，Find({}) 取回全部文档并按 schema 列归一化
// 输出为行游标；客户端在 fetch 同步取数后即时归还。Filterable 为空故不产生下推谓词。
func (d *MongoDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	_ = pushdown
	cli, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	coll := cli.Database(d.pool.db).Collection(d.pool.collection)
	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		d.pool.Release(ctx, cli)
		return nil, errors.WithStack(err)
	}
	out, err := d.collect(ctx, cursor)
	d.pool.Release(ctx, cli)
	if err != nil {
		return nil, err
	}
	return newMySQLRows(out), nil
}

// collect 遍历游标，把每个文档解码为 bson.M 并按 schema 列归一化。
func (d *MongoDatasource) collect(ctx context.Context, cursor *mongo.Cursor) ([][]any, error) {
	defer cursor.Close(ctx)
	var out [][]any
	for cursor.Next(ctx) {
		doc := make(map[string]any)
		if err := cursor.Decode(&doc); err != nil {
			return nil, errors.WithStack(err)
		}
		row := make([]any, len(d.schema.Columns))
		for i, col := range d.schema.Columns {
			row[i] = d.cell(doc, col.Name, d.valueTypes[i])
		}
		out = append(out, row)
	}
	if err := cursor.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}

// cell 取文档中指定字段的归一化单元格；缺失字段返回 nil。
func (d *MongoDatasource) cell(doc map[string]any, name string, vt sqlitevtab.ValueType) any {
	v, ok := doc[name]
	if !ok {
		return nil
	}
	switch vt {
	case sqlitevtab.ValueTypeInt64:
		return mongoInt64(v)
	case sqlitevtab.ValueTypeFloat64:
		return mongoFloat(v)
	case sqlitevtab.ValueTypeBool:
		b, _ := v.(bool)
		return b
	case sqlitevtab.ValueTypeBlob:
		if b, ok := v.([]byte); ok {
			return b
		}
		return fmt.Sprint(v)
	default:
		return mongoText(v)
	}
}

// mongoInt64 尽量把任意数值归一化为 int64。
func mongoInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// mongoFloat 尽量把任意数值归一化为 float64。
func mongoFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// mongoText 把字段值格式化为文本（字符串/时间/对象ID/字节；复杂值退化为 fmt.Sprint）。
func mongoText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case bson.DateTime:
		return time.UnixMilli(int64(t)).UTC().Format(time.RFC3339)
	case bson.ObjectID:
		return t.Hex()
	case bson.Decimal128:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}
