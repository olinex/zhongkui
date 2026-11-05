// Package vtab（本文件）提供 Redis set 数据源实现。
// redis_set 表把 Redis set 每个成员作为一行：key 列支持精确等值/模式扫描；成员以
// SMEMBERS 读取并按 member 升序保证确定性。类型非 set 的键不产生行。连接经共享
// RedisPool 在 Open 借出、取数后即时归还。

package vtab

import (
	// 标准库
	"context"
	"sort"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// RedisSetDatasource 是基于 Redis set 结构的 vtable 数据源。
type RedisSetDatasource struct {
	redisBase
}

// NewRedisSetDatasource 创建 redis_set 数据源。
func NewRedisSetDatasource(pool *RedisPool) *RedisSetDatasource {
	return &RedisSetDatasource{
		redisBase: redisBase{
			pool:     pool,
			keyIndex: 0,
			kind:     redisKindSet,
			schema: sqlitevtab.Schema{
				Columns: []sqlitevtab.Column{
					{Name: "key", ValueType: sqlitevtab.ValueTypeString},
					{Name: "member", ValueType: sqlitevtab.ValueTypeString},
				},
				Filterable: []int{0},
			},
		},
	}
}

// Open 实现 Datasource.Open：借出连接，按 key 解析计划读取 set 成员行并返回行游标。
func (d *RedisSetDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
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

// fetch 根据计划收集 set 行（key, member），成员升序。
func (d *RedisSetDatasource) fetch(ctx context.Context, client *redis.Client, plan redisKeyPlan) ([][]any, error) {
	var rows [][]any
	appendKey := func(key string) {
		if !d.typeIs(ctx, client, key) {
			return
		}
		members, err := client.SMembers(ctx, key).Result()
		if err != nil {
			return
		}
		sort.Strings(members)
		for _, m := range members {
			rows = append(rows, []any{key, m})
		}
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
