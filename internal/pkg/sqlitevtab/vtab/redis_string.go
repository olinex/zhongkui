// Package vtab（本文件）提供 Redis string 数据源实现。
// redis_string 表把 Redis string 键值键作为行集暴露：key 列支持精确等值（→GET）或
// 模式扫描（→SCAN+TYPE+GET），value 列为键值。类型非 string（WRONGTYPE）或键不存在
// 视为无该行。连接经共享 RedisPool 在 Open 借出、取数后即时归还。

package vtab

import (
	// 标准库
	"context"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// RedisStringDatasource 是基于 Redis string 结构的 vtable 数据源。
type RedisStringDatasource struct {
	redisBase
}

// NewRedisStringDatasource 创建 redis_string 数据源。pool 由调用方（如 DI provider）
// 注入，同一 redis 实例的五张表共享该池；返回的实例经 SQLitePool.RegisterDatasource
// 注册后即可查询。
func NewRedisStringDatasource(pool *RedisPool) *RedisStringDatasource {
	return &RedisStringDatasource{
		redisBase: redisBase{
			pool:     pool,
			keyIndex: 0,
			kind:     redisKindString,
			schema: sqlitevtab.Schema{
				Columns: []sqlitevtab.Column{
					{Name: "key", ValueType: sqlitevtab.ValueTypeString},
					{Name: "value", ValueType: sqlitevtab.ValueTypeString},
				},
				Filterable: []int{0},
			},
		},
	}
}

// Open 实现 Datasource.Open：借出连接，按 key 解析计划读取 string 键值并返回行游标。
func (d *RedisStringDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	client, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	plan := d.resolveKeyPlan(pushdown)
	rows, err := d.fetch(ctx, client, plan)
	// fetch 已同步取回全部行，连接作用域到此结束，即时归还以便复用
	d.pool.Release(ctx, client)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newRedisRows(rows), nil
}

// fetch 根据计划收集全部 string 行（键+值），类型不匹配/缺失的键跳过。
func (d *RedisStringDatasource) fetch(ctx context.Context, client *redis.Client, plan redisKeyPlan) ([][]any, error) {
	var rows [][]any
	appendKey := func(key string) {
		if !d.typeIs(ctx, client, key) {
			return
		}
		val, err := client.Get(ctx, key).Result()
		if err != nil {
			return
		}
		rows = append(rows, []any{key, val})
	}
	if !plan.scan {
		appendKey(plan.exact)
		return rows, nil
	}
	keys, err := d.scanKeys(ctx, client, plan.pattern)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, k := range keys {
		appendKey(k)
	}
	return rows, nil
}
