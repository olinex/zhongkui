// Package vtab（本文件）提供 Redis list 数据源实现。
// redis_list 表把 Redis list 每个元素作为一行：key 列支持精确等值/模式扫描；元素以
// LRANGE 0 -1 读取，position 为 0 起始序号（int64 亲和）、value 为元素值。类型非 list
// 的键不产生行。连接经共享 RedisPool 在 Open 借出、取数后即时归还。

package vtab

import (
	// 标准库
	"context"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// RedisListDatasource 是基于 Redis list 结构的 vtable 数据源。
type RedisListDatasource struct {
	redisBase
}

// NewRedisListDatasource 创建 redis_list 数据源。
func NewRedisListDatasource(pool *RedisPool) *RedisListDatasource {
	return &RedisListDatasource{
		redisBase: redisBase{
			pool:     pool,
			keyIndex: 0,
			kind:     redisKindList,
			schema: sqlitevtab.Schema{
				Columns: []sqlitevtab.Column{
					{Name: "key", ValueType: sqlitevtab.ValueTypeString},
					{Name: "position", ValueType: sqlitevtab.ValueTypeInt64},
					{Name: "value", ValueType: sqlitevtab.ValueTypeString},
				},
				Filterable: []int{0},
			},
		},
	}
}

// Open 实现 Datasource.Open：借出连接，按 key 解析计划读取 list 元素行并返回行游标。
func (d *RedisListDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	client, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	plan := d.resolveKeyPlan(pushdown)
	rows, err := d.fetch(ctx, client, plan)
	// fetch 已同步取回全部行，连接作用域到此结束，即时归还以便复用
	d.pool.Release(ctx, client)
	if err != nil {
		return nil, err
	}
	return newRedisRows(rows), nil
}

// fetch 根据计划收集 list 行（key, position, value）。
func (d *RedisListDatasource) fetch(ctx context.Context, client *redis.Client, plan redisKeyPlan) ([][]any, error) {
	var rows [][]any
	appendKey := func(key string) {
		if !d.typeIs(ctx, client, key) {
			return
		}
		vals, err := client.LRange(ctx, key, 0, -1).Result()
		if err != nil {
			return
		}
		for i, v := range vals {
			rows = append(rows, []any{key, int64(i), v})
		}
	}
	if !plan.scan {
		appendKey(plan.exact)
		return rows, nil
	}
	keys, err := d.scanKeys(ctx, client, plan.pattern)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		appendKey(k)
	}
	return rows, nil
}
