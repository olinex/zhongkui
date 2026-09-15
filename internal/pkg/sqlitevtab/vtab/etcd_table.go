// Package vtab（本文件）提供 etcd 数据源实现。
// EtcdDatasource 把 etcd 的键值对暴露为一个 vtable，固定两列：key（可下推）与 value。
// Open 借出 clientv3.Client，按 key 解析计划读取键值并返回行游标（key 形式见文件头
// 注释：精确等值 / 前缀范围 / 全量范围，均为真实匹配行的超集）。
//
// This file provides the etcd datasource. EtcdDatasource exposes etcd key-value entries
// as a vtable with two fixed columns: key (pushdownable) and value. On Open it borrows a
// clientv3.Client, reads key-values per the key plan and returns a row cursor (key plans
// are exact / prefix range / full range per the file header comment, all supersets of the
// true matches).
package vtab

import (
	// stdlib
	"context"
	"strings"

	// third-party
	clientv3 "go.etcd.io/etcd/client/v3"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// etcdKeyPlan 描述一次 etcd 数据源查询的 key 解析计划：精确等值 / 前缀范围 / 全量范围。
// etcdKeyPlan describes the key plan for an etcd datasource query: exact / prefix range /
// full range.
type etcdKeyPlan struct {
	exact  bool
	prefix string
}

// EtcdDatasource 是基于 etcd 键值对集合的 vtable 数据源。
// EtcdDatasource is the vtable datasource over the etcd key-value set.
type EtcdDatasource struct {
	// pool 数据源连接池
	pool *EtcdPool
	// schema 注册 schema
	schema sqlitevtab.Schema
}

// NewEtcdDatasource 创建 etcd 数据源。pool 由调用方（如 DI provider 或 register 分支）
// 注入；返回的实例经 SQLitePool.RegisterDatasource 注册后即可查询。
// NewEtcdDatasource creates an etcd datasource with the injected pool; the returned
// instance is queryable once registered via SQLitePool.RegisterDatasource.
func NewEtcdDatasource(pool *EtcdPool) *EtcdDatasource {
	return &EtcdDatasource{
		pool: pool,
		schema: sqlitevtab.Schema{
			Columns: []sqlitevtab.Column{
				{Name: "key", ValueType: sqlitevtab.ValueTypeString},
				{Name: "value", ValueType: sqlitevtab.ValueTypeString},
			},
			Filterable: []int{0},
		},
	}
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
// datasourceSchema returns the datasource schema for registration and testing.
func (d *EtcdDatasource) datasourceSchema() sqlitevtab.Schema {
	return d.schema
}

// resolveKeyPlan 从下推约束解析 key 计划：key 列（索引 0）精确等值→Go `exact`；LIKE
// 模式→由首通配符前字面量得前缀（非空→prefix，否则→full）；缺省→full。均为真实匹配
// 行的超集，pattern 残差交由 sqlite 复查。
// resolveKeyPlan resolves the key plan from pushdown constraints: an exact EQ on the key
// column (index 0) → exact; a LIKE pattern → a prefix derived from the literal before the
// first wildcard (non-empty → prefix, else full range); otherwise full range. All are
// supersets of the true matches; pattern residuals are re-checked by sqlite.
func (d *EtcdDatasource) resolveKeyPlan(pushdown []sqlitevtab.Constraint) etcdKeyPlan {
	for _, cst := range pushdown {
		if cst.Column != 0 {
			continue
		}
		switch cst.Op {
		case sqlitevtab.OpEQ:
			if k, ok := cst.Value.(string); ok {
				return etcdKeyPlan{exact: true, prefix: k}
			}
		case sqlitevtab.OpLIKE:
			if p, ok := cst.Value.(string); ok {
				return etcdKeyPlan{prefix: likePrefix(p)}
			}
		}
	}
	return etcdKeyPlan{}
}

// Open 实现 Datasource.Open：借出连接，按 key 计划读取键值并返回行游标。连接在 fetch
// 同步取数后即时归还。
// Open implements Datasource.Open: it borrows a connection, reads key-values per the key
// plan and returns a row cursor. The connection is released right after fetch.
func (d *EtcdDatasource) Open(ctx context.Context, pushdown []sqlitevtab.Constraint) (sqlitevtab.Rows, error) {
	cli, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.fetch(ctx, cli, d.resolveKeyPlan(pushdown))
	d.pool.Release(ctx, cli)
	if err != nil {
		return nil, err
	}
	return newRedisRows(rows), nil
}

// fetch 根据 key 计划收集全部键值行。etcd 返回值字节串，此处转为 string。
// fetch collects all key-value rows per the key plan. etcd values are byte slices,
// converted to string here.
func (d *EtcdDatasource) fetch(ctx context.Context, cli *clientv3.Client, plan etcdKeyPlan) ([][]any, error) {
	var (
		resp *clientv3.GetResponse
		err  error
	)
	switch {
	case plan.exact:
		resp, err = cli.Get(ctx, plan.prefix)
	case plan.prefix != "":
		resp, err = cli.Get(ctx, plan.prefix, clientv3.WithPrefix())
	default:
		resp, err = cli.Get(ctx, "", clientv3.WithFromKey())
	}
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		rows = append(rows, []any{string(kv.Key), string(kv.Value)})
	}
	return rows, nil
}

// likePrefix 取 LIKE 模式首通配符（%/_）前的字面量前缀；无字面量前缀返回 ""（→全量）。
// likePrefix extracts the literal prefix of a LIKE pattern before the first
// wildcard (%/_); returns "" when there is no literal prefix (→ full range).
func likePrefix(pattern string) string {
	if i := strings.IndexAny(pattern, "%_"); i > 0 {
		return pattern[:i]
	}
	return ""
}
