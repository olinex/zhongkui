// Package vtab（本文件）提供 Redis hash 数据源实现。
// redis_hash 表把 Redis hash 每个 field 作为一行：key 列支持精确等值/模式扫描；
// 当同时存在 key 精确值与 field 精确等值（OpEQ）→HGET 单行，否则 HGETALL 逐 field
// 一行并按 field 升序保证确定性。类型非 hash 的键不产生行。连接经共享 RedisPool
// 在 Open 借出、取数后即时归还。

package vtab

import (
	// 标准库
	"context"
	"sort"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// RedisHashDatasource 是基于 Redis hash 结构的 vtable 数据源。
type RedisHashDatasource struct {
	redisBase
	// fieldIndex 记录 field 列在表结构中的列索引。
	fieldIndex int
}

// NewRedisHashDatasource 创建 redis_hash 数据源。
func NewRedisHashDatasource(pool *RedisPool) *RedisHashDatasource {
	return &RedisHashDatasource{
		redisBase: redisBase{
			pool:     pool,
			keyIndex: 0,
			kind:     redisKindHash,
			schema: sqlitevtab.Schema{
				Columns: []sqlitevtab.Column{
					{Name: "key", ValueType: sqlitevtab.ValueTypeString},
					{Name: "field", ValueType: sqlitevtab.ValueTypeString},
					{Name: "value", ValueType: sqlitevtab.ValueTypeString},
				},
				Filterable: []int{0, 1},
			},
		},
		fieldIndex: 1,
	}
}

// Open 实现 Datasource.Open：借出连接，按 key 解析计划与可选的 field 精确等值读取
// hash 行并返回行游标。
func (d *RedisHashDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	client, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	plan := d.resolveKeyPlan(pushdown)
	field, hasField := "", false
	for _, cst := range pushdown {
		if cst.Column == d.fieldIndex && cst.Op == sqlitevtab.OpEQ {
			if s, ok := textPattern(cst.Value); ok {
				field, hasField = s, true
			}
		}
	}
	rows, err := d.fetch(ctx, client, plan, field, hasField)
	// fetch 已同步取回全部行，连接作用域到此结束，即时归还以便复用
	d.pool.Release(ctx, client)
	if err != nil {
		return nil, err
	}
	return newRedisRows(rows), nil
}

// fetch 根据计划收集 hash 行（key, field, value）。
func (d *RedisHashDatasource) fetch(ctx context.Context, client *redis.Client, plan redisKeyPlan, field string, hasField bool) ([][]any, error) {
	var rows [][]any
	appendKey := func(key string) {
		if !d.typeIs(ctx, client, key) {
			return
		}
		if hasField {
			if val, err := client.HGet(ctx, key, field).Result(); err == nil {
				rows = append(rows, []any{key, field, val})
			}
			return
		}
		pairs, err := client.HGetAll(ctx, key).Result()
		if err != nil {
			return
		}
		fields := make([]string, 0, len(pairs))
		for f := range pairs {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			rows = append(rows, []any{key, f, pairs[f]})
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
