// Package vtab（本文件）提供 Redis zset 数据源实现。
// redis_zset 表把 Redis zset 每个成员作为一行：key 列支持精确等值/模式扫描；成员以
// ZRANGE 0 -1 WITHSCORES 读取，score 解析为 float64（ValueTypeFloat64）。类型非 zset
// 的键不产生行。连接经共享 RedisPool 在 Open 借出、取数后即时归还。

package vtab

import (
	// 标准库
	"context"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// RedisZSetDatasource 是基于 Redis zset 结构的 vtable 数据源。
type RedisZSetDatasource struct {
	redisBase
}

// NewRedisZSetDatasource 创建 redis_zset 数据源。
func NewRedisZSetDatasource(pool *RedisPool) *RedisZSetDatasource {
	return &RedisZSetDatasource{
		redisBase: redisBase{
			pool:     pool,
			keyIndex: 0,
			kind:     redisKindZSet,
			schema: sqlitevtab.Schema{
				Columns: []sqlitevtab.Column{
					{Name: "key", ValueType: sqlitevtab.ValueTypeString},
					{Name: "member", ValueType: sqlitevtab.ValueTypeString},
					{Name: "score", ValueType: sqlitevtab.ValueTypeFloat64},
				},
				Filterable: []int{0},
			},
		},
	}
}

// Open 实现 Datasource.Open：借出连接，按 key 解析计划读取 zset 成员行并返回行游标。
func (d *RedisZSetDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
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

// fetch 根据计划收集 zset 行（key, member, score）。
func (d *RedisZSetDatasource) fetch(ctx context.Context, client *redis.Client, plan redisKeyPlan) ([][]any, error) {
	var rows [][]any
	appendKey := func(key string) {
		if !d.typeIs(ctx, client, key) {
			return
		}
		pairs, err := client.ZRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			return
		}
		for _, p := range pairs {
			member, ok := p.Member.(string)
			if !ok {
				continue
			}
			rows = append(rows, []any{key, member, p.Score})
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
