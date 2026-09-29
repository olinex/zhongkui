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

package vtab

import (
	// 标准库
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"

	// 第三方
	"github.com/redis/go-redis/v9"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

const (
	// redisKindString string，GET 读取。
	redisKindString redisTypeKind = "string"
	// redisKindHash hash，HGET/HGETALL 读取。
	redisKindHash redisTypeKind = "hash"
	// redisKindList list，LRANGE 读取。
	redisKindList redisTypeKind = "list"
	// redisKindSet set，SMEMBERS 读取。
	redisKindSet redisTypeKind = "set"
	// redisKindZSet zset，ZRANGE WITHSCORES 读取。
	redisKindZSet redisTypeKind = "zset"
)

// redisClients 是进程级 redis client 池注册表：共享 Manager 与 Key→Options 映射。
// 前端配置经 NewRedisPoolScoped 注入后，对应 Key 的 client 即可在任意 datasource 中借出。
var redisClients = newRedisClientRegistry()

// redisTypeKind 标识一种 Redis 数据结构类型，对应命令族与之映射。
type redisTypeKind string

// RedisPool 是基于 resourcepool 的 redis client 池句柄，绑定一个实例配置派生的 Key。
// 底层为进程级共享的 resourcepool.Manager[*redis.Client]：按 Key 维护相互独立的
// go-redis client 池，统一驱动「惰性建连 → 借出 → 归还 → 闲置驱逐 → 优雅关闭」。
// 对 go-redis 而言一个 client 内部已池化物理连接，这里池化的是「client 实例」：
// 前端配置可能存在多个 redis 实例，每个配置对应一个 Key 一个 client 池，做到跨实例
// 复用、失效自动重建与闲置回收。go-redis client 并发安全，池内可驻留多个 client
// 以服务并发查询。
type RedisPool struct {
	// key 实例配置派生的连接池 Key。
	key resourcepool.Key
}

// redisClientRegistry 按 Key 维护 redis client 池与配置。
type redisClientRegistry struct {
	mu      sync.RWMutex
	manager *resourcepool.Manager[*redis.Client]
	options map[resourcepool.Key]*redis.Options
}

// newRedisClientRegistry 创建注册表并装配进程级 Manager。Manager 采用默认资源池
// 配置（testOnBorrow 探活、失效自动重建、闲置驱逐），与其他池一致。
func newRedisClientRegistry() *redisClientRegistry {
	r := &redisClientRegistry{options: make(map[resourcepool.Key]*redis.Options)}
	r.manager = resourcepool.NewManager(r.makeClient, resourcepool.NewDefaultConfig())
	return r
}

// makeClient 依据 Key 从注册表取配置并惰性创建 redis client（resourcepool.Make）。
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
func redisKey(addr string, db int, namespace string) resourcepool.Key {
	return resourcepool.Key(fmt.Sprintf("%s@%s/%d", namespace, addr, db))
}

// Acquire 从该实例的池中借出一个可用 client（借出时经 resourcepool 校验，失效自动
// 重建）。go-redis client 并发安全，可被局部借出使用。
func (p *RedisPool) Acquire(ctx context.Context) (*redis.Client, error) {
	return redisClients.manager.Acquire(ctx, p.key)
}

// Release 将 client 归还该实例的池以便复用。
func (p *RedisPool) Release(ctx context.Context, c *redis.Client) {
	redisClients.manager.Release(ctx, p.key, c)
}

// Close 销毁该实例池内的闲置 client（借出中的不受影响），用于配置删除/连接失效场景。
// 透传 ctx 以维护取消/超时语义。进程级全部 client 池的优雅关闭由接入方统一触发 CloseAll。
func (p *RedisPool) Close(ctx context.Context) {
	redisClients.manager.Clear(ctx, p.key)
}

// CloseAllRedisClients 关闭全部 redis client 池并销毁存活对象，释放底层连接。
// 供接入方在进程优雅关闭阶段调用。
func CloseAllRedisClients(ctx context.Context) {
	redisClients.manager.CloseAll(ctx)
}

// redisBase 是五类 Redis vtable 数据源的共享基座：持有 Redis 连接池、schema、
// key 列索引与结构类型标识。
type redisBase struct {
	pool     *RedisPool
	schema   sqlitevtab.Schema
	keyIndex int
	kind     redisTypeKind
}

// redisKeyPlan 是一次查询的键解析计划：精确键（EQ）或 SCAN 模式（否则全量）。
type redisKeyPlan struct {
	exact   string
	pattern string
	scan    bool
}

// resolveKeyPlan 依据下推约束解析键计划：key 列 OpEQ→精确键；OpLIKE/OpGLOB/OpMATCH→
// SCAN 模式；无 key 约束→全量扫描（pattern "*"）。仅识别 key 列，其余约束留给 sqlite
// 以 Omit=false 复查，保证返回为超集。
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
func (b *redisBase) typeIs(ctx context.Context, client *redis.Client, key string) bool {
	typ, err := client.Type(ctx, key).Result()
	if err != nil {
		return false
	}
	return redisTypeKind(typ) == b.kind
}

// scanKeys 在给定 client 上返回匹配 SCAN 模式且类型为本结构的全部 key，
// 升序保证确定性。
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
func (b *redisBase) datasourceSchema() sqlitevtab.Schema {
	return b.schema
}

// redisRows 是预先行集的行游标：行在 Open 阶段已同步取回内存，连接已即时归还，
// 游标自身不持有任何连接或外部资源，Close 仅释放引用（幂等）。
type redisRows struct {
	rows [][]any
	idx  int
}

// newRedisRows 构造内存行游标。
func newRedisRows(rows [][]any) *redisRows {
	return &redisRows{rows: rows}
}

// Next 返回下一行（直接引用，不拷贝；调用方按需保留时自行拷贝）。
func (r *redisRows) Next() ([]any, error) {
	if r.idx >= len(r.rows) {
		return nil, io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	return row, nil
}

// Close 释放行集引用（幂等）。
func (r *redisRows) Close() error {
	r.rows = nil
	return nil
}

// likeGlob 将 sqlite LIKE 通配模式映射为 Redis SCAN glob：%→*、_→?，其余字符原样。
// 注意 glob 匹配大小写敏感而 sqlite LIKE 不敏感，映射仅作为超集提示：对大小写一致的
// 键该映射精确；被过滤的键反正交由 sqlite 以 Omit=false 复查，映射须为超集（不丢行）。
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
type redisPlugin struct{}

// Key 返回数据源类型键。
func (redisPlugin) Key() string { return DatasourceTypeRedis }

// ConfigType 返回配置值反射类型。
func (redisPlugin) ConfigType() reflect.Type { return reflect.TypeFor[RedisConfig]() }

// NewConfig 返回全新一份 *RedisConfig。
func (redisPlugin) NewConfig() any { return &RedisConfig{} }

// ValidateConfig 校验 redis 客户端要暴露的结构类型（经 redisDatasourceFactory 分派）。
func (redisPlugin) ValidateConfig(cfg any) error {
	c := cfg.(*RedisConfig)
	if _, err := redisDatasourceFactory(nil, c.Struct); err != nil {
		return err
	}
	return nil
}

// TestConn 建池借出 client、Ping 探活后归还。
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
func (redisPlugin) Register(_ context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, namespace, _ string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*RedisConfig)
	opts := &redis.Options{Addr: fmt.Sprintf("%s:%d", c.Addr, c.Port), Password: c.Password, DB: c.DB}
	poolHandle := NewRedisPool(opts, namespace)
	return redisDatasourceFactory(poolHandle, c.Struct)
}

// init 自注册 redis 数据源类型。
func init() {
	RegisterDatasourceType(redisPlugin{})
}
