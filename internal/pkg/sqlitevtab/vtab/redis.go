// Package vtab（本文件）提供基于 Redis 的 vtable 数据源。
// 五类核心数据结构（string/hash/list/set/zset）各为一张独立 vtable，均复用本文件的
// 键解析计划（精确等值 / 模式扫描 / 全量扫描）、SCAN+TYPE 遍历与行游标。
// 连接模型为"运行时借还，取数即还"：数据源持有对一个 redis 实例配置的 RedisPool；
// 每次查询 Open 时 acquire 借出 client（必要时惰性建连/重连），fetch 同步取回全部行后
// 立即 release 归还。同一 redis 实例的五张 vtable 共享同一个 RedisPool。
// go-redis 的 Client 内部已池化物理连接，故本层以 resourcepool 池化「client 实例」：
// 前端配置可能存在多个 redis 实例，每个配置经 Key 映射到一个相互独立的 client 池。
//
// 本文件遵循 internal/pkg/sqlitevtab 的 Datasource 超集契约：数据源头尽力过滤是优化，
// sqlite 侧以 Omit=false 复查每个返回行，故任何解析/扫描都必须返回真实匹配行的超集，
// 禁止漏行。
//
// This file provides the Redis vtable datasources. The five core data structures
// (string/hash/list/set/zset) each map to a dedicated table, all reusing the key
// resolution plan (exact / pattern scan / full scan), SCAN+TYPE traversal and row
// cursor. The connection model is "borrow on open, return after fetch": a datasource
// holds a RedisPool for one redis instance config; each query's Open acquires a client
// (lazily creating or reconnecting as needed), synchronously fetches all rows and
// immediately releases the client. The five tables of one redis instance share one
// RedisPool. Because go-redis Client already pools physical connections, this layer
// uses resourcepool to pool client instances: front-end config may declare multiple
// redis instances, each config mapping to an independent client pool by Key.
//
// This file follows the sqlitevtab Datasource superset contract: upstream filtering
// is only an optimization, and sqlite re-checks every returned row with Omit=false,
// so any resolution/scan must return a superset of the truly matching rows and never
// drop one.
package vtab

import (
	// stdlib
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"

	// third-party
	"github.com/redis/go-redis/v9"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

const (
	// redisKindString string，GET 读取。
	// redisKindString maps to GET.
	redisKindString redisTypeKind = "string"
	// redisKindHash hash，HGET/HGETALL 读取。
	// redisKindHash maps to HGET/HGETALL.
	redisKindHash redisTypeKind = "hash"
	// redisKindList list，LRANGE 读取。
	// redisKindList maps to LRANGE.
	redisKindList redisTypeKind = "list"
	// redisKindSet set，SMEMBERS 读取。
	// redisKindSet maps to SMEMBERS.
	redisKindSet redisTypeKind = "set"
	// redisKindZSet zset，ZRANGE WITHSCORES 读取。
	// redisKindZSet maps to ZRANGE WITHSCORES.
	redisKindZSet redisTypeKind = "zset"
)

// redisClients 是进程级 redis client 池注册表：共享 Manager 与 Key→Options 映射。
// 前端配置经 NewRedisPoolScoped 注入后，对应 Key 的 client 即可在任意 datasource 中借出。
// redisClients is the process-wide redis client pool registry: a shared Manager plus
// a Key→Options map. Once a front-end config is injected via NewRedisPoolScoped,
// clients for that Key become borrowable from any datasource.
var redisClients = newRedisClientRegistry()

// redisTypeKind 标识一种 Redis 数据结构类型，对应命令族与之映射。
// redisTypeKind identifies a Redis data structure kind, mapping to a command family.
type redisTypeKind string

// RedisPool 是基于 resourcepool 的 redis client 池句柄，绑定一个实例配置派生的 Key。
// 底层为进程级共享的 resourcepool.Manager[*redis.Client]：按 Key 维护相互独立的
// go-redis client 池，统一驱动「惰性建连 → 借出 → 归还 → 闲置驱逐 → 优雅关闭」。
// 对 go-redis 而言一个 client 内部已池化物理连接，这里池化的是「client 实例」：
// 前端配置可能存在多个 redis 实例，每个配置对应一个 Key 一个 client 池，做到跨实例
// 复用、失效自动重建与闲置回收。go-redis client 并发安全，池内可驻留多个 client
// 以服务并发查询。
// RedisPool is a resourcepool-backed redis client pool handle bound to a Key derived
// from an instance configuration. Under the hood is a process-wide
// resourcepool.Manager[*redis.Client]: each Key maintains an independent go-redis
// client pool that uniformly drives "lazy create → borrow → return → idle eviction →
// graceful close". For go-redis one client already pools physical connections, and
// this layer pools client instances: front-end config may declare multiple redis
// instances, each mapping to a Key and a dedicated client pool that is reused,
// auto-rebuilt on staleness and reclaimed when idle. go-redis clients are
// concurrency-safe, so a pool can hold several clients to serve concurrent queries.
type RedisPool struct {
	key resourcepool.Key
}

// redisClientRegistry 按 Key 维护 redis client 池与配置。
// redisClientRegistry maintains redis client pools and their options by Key.
type redisClientRegistry struct {
	mu      sync.RWMutex
	manager *resourcepool.Manager[*redis.Client]
	options map[resourcepool.Key]*redis.Options
}

// newRedisClientRegistry 创建注册表并装配进程级 Manager。Manager 采用默认资源池
// 配置（testOnBorrow 探活、失效自动重建、闲置驱逐），与其他池一致。
// newRedisClientRegistry builds the registry and its process-wide Manager with the
// default resourcepool config (testOnBorrow probing, auto-rebuild on staleness,
// idle eviction), consistent with other pools.
func newRedisClientRegistry() *redisClientRegistry {
	r := &redisClientRegistry{options: make(map[resourcepool.Key]*redis.Options)}
	r.manager = resourcepool.NewManager(r.makeClient, resourcepool.NewDefaultConfig())
	return r
}

// makeClient 依据 Key 从注册表取配置并惰性创建 redis client（resourcepool.Make）。
// makeClient looks up the config for a Key and lazily creates a redis client
// (resourcepool.Make).
func (r *redisClientRegistry) makeClient(key resourcepool.Key) (*redis.Client, error) {
	r.mu.RLock()
	opts := r.options[key]
	r.mu.RUnlock()
	if opts == nil {
		return nil, errors.Newf("redis: options not registered for key %q", key)
	}
	return redis.NewClient(opts), nil
}

// NewRedisPool 注入一份 redis 实例配置并返回其池句柄，连接池 Key 含命名空间维度。
// client 惰性创建（借出时才建立），构造本身不触发网络连接。
// 命名空间维度用于按命名空间隔离连接会话：同一实例在不同命名空间下派生不同 Key、
// 绝不共用连接（隔离连接与凭据）。redis vtable 接入必须归属命名空间，故仅提供此带
// 命名空间的构造器。
// NewRedisPool injects one redis instance configuration and returns its pool
// handle, with the pool Key including the namespace dimension. Clients are created
// lazily on borrow, so construction initiates no network connection. The namespace
// dimension isolates connection sessions by namespace: the same instance under
// different namespaces derives different Keys and never shares connections (isolating
// connections and credentials). Redis vtable registration must belong to a namespace,
// so only this namespace-aware constructor is provided.
func NewRedisPool(opts *redis.Options, namespace string) *RedisPool {
	key := redisKey(opts.Addr, opts.DB, namespace)
	redisClients.mu.Lock()
	redisClients.options[key] = opts
	redisClients.mu.Unlock()
	return &RedisPool{key: key}
}

// redisKey 由连接参数与命名空间规范化出实例 Key。语义配置差异或命名空间差异映射
// 不同 Key，从而在 Manager 中维护相互独立的 client 池；命名空间维度保证同一实例在
// 不同命名空间下维护相互独立的 client 池。
// redisKey derives the instance Key from the connection parameters and the namespace.
// Different configurations or namespaces map to different Keys so the Manager keeps
// independent client pools; the namespace dimension guarantees the same instance keeps
// independent client pools across namespaces.
func redisKey(addr string, db int, namespace string) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s/%d", namespace, addr, db))
}

// Acquire 从该实例的池中借出一个可用 client（借出时经 resourcepool 校验，失效自动
// 重建）。go-redis client 并发安全，可被局部借出使用。
// Acquire borrows a usable client from this instance's pool (validated by resourcepool
// on borrow, auto-rebuilt when stale). go-redis clients are concurrency-safe and may
// be borrowed and used locally.
func (p *RedisPool) Acquire(ctx context.Context) (*redis.Client, error) {
	return redisClients.manager.Acquire(ctx, p.key)
}

// Release 将 client 归还该实例的池以便复用。
// Release returns the client to this instance's pool for reuse.
func (p *RedisPool) Release(ctx context.Context, c *redis.Client) {
	redisClients.manager.Release(ctx, p.key, c)
}

// Close 销毁该实例池内的闲置 client（借出中的不受影响），用于配置删除/连接失效场景。
// 透传 ctx 以维护取消/超时语义。进程级全部 client 池的优雅关闭由接入方统一触发 CloseAll。
// Close destroys idle clients in this instance's pool (borrowed ones are unaffected),
// for config-removal or connection-staleness scenarios. ctx is propagated to preserve
// cancel/timeout semantics. Graceful close of every process-wide client pool is
// triggered by the integrator via CloseAll.
func (p *RedisPool) Close(ctx context.Context) {
	redisClients.manager.Clear(ctx, p.key)
}

// CloseAllRedisClients 关闭全部 redis client 池并销毁存活对象，释放底层连接。
// 供接入方在进程优雅关闭阶段调用。
// CloseAllRedisClients closes every redis client pool and destroys live objects,
// releasing underlying connections. It is meant for graceful shutdown.
func CloseAllRedisClients(ctx context.Context) {
	redisClients.manager.CloseAll(ctx)
}

// redisBase 是五类 Redis vtable 数据源的共享基座：持有 Redis 连接池、schema、
// key 列索引与结构类型标识。
// redisBase is the shared base of the five Redis vtable datasources, holding the
// Redis pool, schema, key column index and structure kind.
type redisBase struct {
	pool     *RedisPool
	schema   sqlitevtab.Schema
	keyIndex int
	kind     redisTypeKind
}

// redisKeyPlan 是一次查询的键解析计划：精确键（EQ）或 SCAN 模式（否则全量）。
// redisKeyPlan is the key resolution plan of a query: an exact key (EQ) or a
// SCAN pattern (otherwise full scan).
type redisKeyPlan struct {
	exact   string
	pattern string
	scan    bool
}

// resolveKeyPlan 依据下推约束解析键计划：key 列 OpEQ→精确键；OpLIKE/OpGLOB/OpMATCH→
// SCAN 模式；无 key 约束→全量扫描（pattern "*"）。仅识别 key 列，其余约束留给 sqlite
// 以 Omit=false 复查，保证返回为超集。
// resolveKeyPlan resolves the key plan from pushdown constraints: a key-column OpEQ
// yields an exact key; OpLIKE/OpGLOB/OpMATCH yield a SCAN pattern; absence of a key
// constraint yields a full scan ("*"). Only the key column is honored here; other
// constraints are left for sqlite's Omit=false re-check to guarantee a superset.
func (b *redisBase) resolveKeyPlan(pushdown []sqlitevtab.Constraint) redisKeyPlan {
	plan := redisKeyPlan{pattern: "*", scan: true}
	for _, cst := range pushdown {
		if cst.Column != b.keyIndex {
			continue
		}
		s, ok := textPattern(cst.Value)
		if !ok {
			continue
		}
		switch cst.Op {
		case sqlitevtab.OpEQ:
			return redisKeyPlan{exact: s}
		case sqlitevtab.OpLIKE:
			plan.pattern = likeGlob(s)
		case sqlitevtab.OpGLOB:
			plan.pattern = s
		case sqlitevtab.OpMATCH:
			plan.pattern = "*" + s + "*"
		}
	}
	return plan
}

// typeIs 判断 key 在给定 client 上是否为当前结构类型；key 不存在或命令出错一律
// 返回 false，由调用方据此跳过而不产生行（类型不匹配语义）。
// typeIs reports whether key holds the kind this datasource serves on the given
// client; either a missing key or a command error yields false so callers skip the
// row (the type-mismatch semantics).
func (b *redisBase) typeIs(ctx context.Context, client *redis.Client, key string) bool {
	typ, err := client.Type(ctx, key).Result()
	if err != nil {
		return false
	}
	return redisTypeKind(typ) == b.kind
}

// scanKeys 在给定 client 上返回匹配 SCAN 模式且类型为本结构的全部 key，
// 升序保证确定性。
// scanKeys returns all keys matching the SCAN pattern and of this kind on the given
// client, sorted ascending for determinism.
func (b *redisBase) scanKeys(ctx context.Context, client *redis.Client, pattern string) ([]string, error) {
	var keys []string
	var cursor uint64
	for {
		batch, next, err := client.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, k := range batch {
			if b.typeIs(ctx, client, k) {
				keys = append(keys, k)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// datasourceSchema 返回数据源 schema，供注册与测试使用。
// datasourceSchema returns the datasource schema for registration and testing.
func (b *redisBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// redisRows 是预先行集的行游标：行在 Open 阶段已同步取回内存，连接已即时归还，
// 游标自身不持有任何连接或外部资源，Close 仅释放引用（幂等）。
// redisRows is a precomputed row-set cursor: rows are synchronously fetched into
// memory during Open and the connection is released immediately, so the cursor holds
// no connection or external resource; Close merely drops references (idempotent).
type redisRows struct {
	rows [][]any
	idx  int
}

// newRedisRows 构造内存行游标。
// newRedisRows builds an in-memory row cursor.
func newRedisRows(rows [][]any) *redisRows {
	return &redisRows{rows: rows}
}

// Next 返回下一行（直接引用，不拷贝；调用方按需保留时自行拷贝）。
// Next returns the next row (by reference, not a copy; callers copy if needed).
func (r *redisRows) Next() ([]any, error) {
	if r.idx >= len(r.rows) {
		return nil, io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	return row, nil
}

// Close 释放行集引用（幂等）。
// Close drops the row-set reference (idempotent).
func (r *redisRows) Close() error {
	r.rows = nil
	return nil
}

// likeGlob 将 sqlite LIKE 通配模式映射为 Redis SCAN glob：%→*、_→?，其余字符原样。
// 注意 glob 匹配大小写敏感而 sqlite LIKE 不敏感，映射仅作为超集提示：对大小写一致的
// 键该映射精确；被过滤的键反正交由 sqlite 以 Omit=false 复查，映射须为超集（不丢行）。
// likeGlob maps an sqlite LIKE wildcard pattern to a Redis SCAN glob: %→*, _→?,
// other characters pass through. Glob matching is case-sensitive while sqlite LIKE
// is not, so this mapping is only a superset hint: it is exact for case-consistent
// keys, and any key it filters out is anyway re-checked by sqlite with Omit=false;
// the mapping must remain a superset (never drops a true match).
func likeGlob(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '%':
			b.WriteByte('*')
		case '_':
			b.WriteByte('?')
		default:
			b.WriteByte(pattern[i])
		}
	}
	return b.String()
}

// redisPlugin 实现 DatasourcePlugin，把 redis 数据源类型接入插件注册表。
// registerInto / TestConn / IntrospectSchema / ValidateConfigJSON 统一经接口驱动，宿主
// 无需感知具体类型。
// redisPlugin implements DatasourcePlugin, wiring the redis datasource type into the
// plugin registry. registerInto / TestConn / IntrospectSchema / ValidateConfigJSON drive
// it uniformly through the interface, so the host need not know the concrete type.
type redisPlugin struct{}

// Key 返回数据源类型键。
// Key returns the datasource type key.
func (redisPlugin) Key() string { return DatasourceTypeRedis }

// ConfigType 返回配置值反射类型。
// ConfigType returns the config value's reflect type.
func (redisPlugin) ConfigType() reflect.Type { return reflect.TypeFor[RedisConfig]() }

// NewConfig 返回全新一份 *RedisConfig。
// NewConfig returns a fresh *RedisConfig.
func (redisPlugin) NewConfig() any { return &RedisConfig{} }

// ValidateConfig 校验 redis 客户端要暴露的结构类型（经 redisDatasourceFactory 分派）。
// ValidateConfig validates the structure type the redis client exposes (dispatched via
// redisDatasourceFactory).
func (redisPlugin) ValidateConfig(cfg any) error {
	c := cfg.(*RedisConfig)
	if _, err := redisDatasourceFactory(nil, c.Struct); err != nil {
		return err
	}
	return nil
}

// TestConn 建池借出 client、Ping 探活后归还。
// TestConn borrows a client from the pool, probes it via Ping and returns it.
func (redisPlugin) TestConn(ctx context.Context, cfg any) error {
	c := cfg.(*RedisConfig)
	pool := NewRedisPool(&redis.Options{Addr: fmt.Sprintf("%s:%d", c.Addr, c.Port), Password: c.Password, DB: c.DB}, "")
	client, err := pool.Acquire(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	pingErr := client.Ping(ctx).Err()
	pool.Release(ctx, client)
	if pingErr != nil {
		return errors.WithStack(pingErr)
	}
	return nil
}

// IntrospectSchema 建池 Ping 探活后按 Struct 返回该结构的固定列定义。
// IntrospectSchema probes via Ping then returns the fixed column definitions of the
// configured Struct.
func (redisPlugin) IntrospectSchema(ctx context.Context, cfg any, _ io.Reader) ([]DatasourceColumn, error) {
	c := cfg.(*RedisConfig)
	pool := NewRedisPool(&redis.Options{Addr: fmt.Sprintf("%s:%d", c.Addr, c.Port), Password: c.Password, DB: c.DB}, "")
	client, err := pool.Acquire(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if pingErr := client.Ping(ctx).Err(); pingErr != nil {
		pool.Release(ctx, client)
		return nil, errors.WithStack(pingErr)
	}
	pool.Release(ctx, client)
	ds, err := redisDatasourceFactory(pool, c.Struct)
	if err != nil {
		return nil, err
	}
	schema, err := schemaOf(ds)
	if err != nil {
		return nil, err
	}
	return schemaColumnsOut(schema), nil
}

// Register 建池 NewRedisPool(opts, namespace) 并按 Struct 构造数据源。
// Register builds the pool via NewRedisPool(opts, namespace) and constructs the datasource
// by Struct.
func (redisPlugin) Register(_ context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, namespace, _ string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*RedisConfig)
	opts := &redis.Options{Addr: fmt.Sprintf("%s:%d", c.Addr, c.Port), Password: c.Password, DB: c.DB}
	poolHandle := NewRedisPool(opts, namespace)
	return redisDatasourceFactory(poolHandle, c.Struct)
}

// init 自注册 redis 数据源类型。
// init self-registers the redis datasource type.
func init() {
	RegisterDatasourceType(redisPlugin{})
}
