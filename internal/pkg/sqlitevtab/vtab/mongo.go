// Package vtab（本文件）提供基于 MongoDB 的 vtable 数据源连接层。
// MongoDB 数据库层面无固定 schema，这里把「一个集合」暴露为一个 vtable：列（列名 +
// ValueType）必须由前端在注册时显式声明的 schema 提供，不做首文档自省（见
// mongo_table.go）。连接模型复用资源池：按 URI+DB+Collection 派生 Key，懒建
// mongo.Client（mongo-driver/v2），Open 借出、取数即还。
//
// 正确性取舍：Mongo 数值类型间等值比较存在类型强转歧义，为保证超集契约不成立时的漏
// 行风险，本数据源不做任何谓词下推（Filterable 恒空），由 sqlite 对全量集合行做本地
// 过滤。详见 mongo_table.go。
//
// This file provides the MongoDB vtable datasource connection layer. MongoDB has no
// fixed schema at the database level, so here one collection is exposed as a vtable:
// the columns (names + ValueTypes) must come from the schema explicitly declared by the
// frontend at registration; no first-document introspection is performed (see
// mongo_table.go). The connection model reuses the resource pool: a Key is derived from
// URI+DB+Collection, lazily building a mongo.Client (mongo-driver/v2), borrowed on Open
// and returned after fetching.
//
// Correctness trade-off: because Mongo numeric equality has type-coercion ambiguity, to
// avoid dropping rows and breaking the superset contract we push no predicate here
// (Filterable is always empty) and let sqlite filter the full collection locally. See
// mongo_table.go.
package vtab

import (
	// stdlib
	"context"
	"fmt"
	"sync"

	// third-party
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// MongoConfig 描述一个 MongoDB 集合的接入配置。
// MongoConfig describes how to reach one MongoDB collection.
type MongoConfig struct {
	// URI 连接串（含认证），如 mongodb://user:pass@127.0.0.1:27017。
	// URI the connection string (with auth), e.g. mongodb://user:pass@127.0.0.1:27017.
	URI string
	// DB 数据库名。
	// DB the database name.
	DB string
	// Collection 集合名。
	// Collection the collection name.
	Collection string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与寻址）。
	// Namespace the datasource namespace; only participates in pool-key derivation.
	Namespace string
}

// mongoClients 是 Mongo 客户端池注册表。
// mongoClients is the Mongo client pool registry.
var mongoClients = newMongoClientRegistry()

// mongoClientConfig 描述一个 Key 的客户端构造。
// mongoClientConfig describes the client construction for one Key.
type mongoClientConfig struct {
	uri string
}

// mongoClientRegistry 按 Key 维护 Mongo 客户端池。
// mongoClientRegistry keeps Mongo client pools by Key.
type mongoClientRegistry struct {
	mu       sync.RWMutex
	manager  *resourcepool.Manager[*mongo.Client]
	cfgs     map[resourcepool.Key]*mongoClientConfig
	override map[resourcepool.Key]*mongo.Client // 仅供测试注入 mock client
}

// newMongoClientRegistry 创建注册表并装配进程级 Manager。
// newMongoClientRegistry builds the registry and its process-wide Manager.
func newMongoClientRegistry() *mongoClientRegistry {
	r := &mongoClientRegistry{
		cfgs:     make(map[resourcepool.Key]*mongoClientConfig),
		override: make(map[resourcepool.Key]*mongo.Client),
	}
	r.manager = resourcepool.NewManager(r.makeDBFor, resourcepool.NewDefaultConfig())
	return r
}

// makeDBFor 依据 Key 惰性创建 mongo.Client（mongo-driver 不阻塞，懒建连）；测试经
// override 注入 mock client 时直接返回之。
// makeDBFor lazily builds a *mongo.Client for a Key (mongo-driver is non-blocking and
// connects lazily); tests bypass it via the override map.
func (r *mongoClientRegistry) makeDBFor(key resourcepool.Key) (*mongo.Client, error) {
	r.mu.RLock()
	if cli, ok := r.override[key]; ok {
		r.mu.RUnlock()
		return cli, nil
	}
	cfg := r.cfgs[key]
	r.mu.RUnlock()
	if cfg == nil {
		return nil, errors.Newf("mongo: config not registered for key %q", key)
	}
	return mongo.Connect(options.Client().ApplyURI(cfg.uri))
}

// register 注入一个 Key 的客户端配置（重复注入覆盖）。
// register injects the client config for a Key (overwrites on re-injection).
func (r *mongoClientRegistry) register(key resourcepool.Key, uri string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfgs[key] = &mongoClientConfig{uri: uri}
}

// overrideClient 设置/清除指定 Key 的 mock client 注入（仅供测试）。
// overrideClient sets/clears a mocked client override for a Key (test only).
func (r *mongoClientRegistry) overrideClient(key resourcepool.Key, cli *mongo.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cli == nil {
		delete(r.override, key)
	} else {
		r.override[key] = cli
	}
}

// NewMongoPool 注入一份 Mongo 集合配置并返回其池句柄。客户端惰性创建，构造本身不触发
// 网络连接。
// NewMongoPool injects one Mongo collection config and returns its pool handle. Clients
// are created lazily, so construction initiates no network connection.
func NewMongoPool(cfg *MongoConfig) *MongoPool {
	key := resourcepool.Key(fmt.Sprintf("%s@%s/%s.%s", cfg.Namespace, cfg.URI, cfg.DB, cfg.Collection))
	mongoClients.register(key, cfg.URI)
	return &MongoPool{key: key, db: cfg.DB, collection: cfg.Collection}
}

// MongoPool 是基于资源池的 Mongo 客户端池句柄，绑定一个集合配置派生的 Key。
// MongoPool is a resourcepool-backed Mongo client pool handle bound to a Key derived
// from a collection config.
type MongoPool struct {
	key        resourcepool.Key
	db         string
	collection string
}

// Acquire 借出并返回一个可用的 *mongo.Client（懒建连）。*mongo.Client 并发安全。
// Acquire borrows a usable *mongo.Client (lazily connecting). It is concurrency-safe.
func (p *MongoPool) Acquire(ctx context.Context) (*mongo.Client, error) {
	return mongoClients.manager.Acquire(ctx, p.key)
}

// Release 归还 *mongo.Client 以便复用。
// Release returns the *mongo.Client for reuse.
func (p *MongoPool) Release(ctx context.Context, cli *mongo.Client) {
	mongoClients.manager.Release(ctx, p.key, cli)
}

// Close 销毁该实例池内的闲置客户端（借出中的不受影响）。
// Close destroys idle clients in this instance's pool (borrowed ones unaffected).
func (p *MongoPool) Close(ctx context.Context) {
	mongoClients.manager.Clear(ctx, p.key)
}

// CloseAllMongoClients 关闭全部 Mongo 客户端池并销毁存活对象。供进程优雅关闭调用。
// CloseAllMongoClients closes every Mongo client pool and destroys live objects, meant
// for graceful shutdown.
func CloseAllMongoClients(ctx context.Context) {
	mongoClients.manager.CloseAll(ctx)
}
