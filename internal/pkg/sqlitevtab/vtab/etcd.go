// Package vtab（本文件）提供基于 etcd 的 vtable 数据源。
// etcd 是分布式键值存储，这里把它的一张「键值表」暴露为两张表合一的 vtable（键值对
// 即行）：仅一张 etcd_kv 虚拟表，列 key/vlue（vtable 名即该数据源的业务命名，见
// register 复用）。key 列支持精确等值（→Get 单键）或前缀/全量范围扫描
// （→Get WithPrefix / WithFromKey），value 列为键值。连接经共享资源池在 Open 借出
// clientv3.Client、取数后即时归还。
//
// 下推取舍：etcd 唯一可下推的维度是 key。模板/匹配类谓词（LIKE/%/_）按上述前缀/全量
// 范围粗略下推——范围扫描结果是真实匹配行的超集，残余模式匹配交由 sqlite 以 Omit=false
// 复查兜底，保证不漏行。value 列不可下推。
//
// This file provides the etcd vtable datasource. etcd is a distributed key-value store;
// here its key-value entries are exposed as rows of a vtable with key/value columns. The
// key column supports exact equality (→Get on a single key) or prefix/full range scans
// (→Get WithPrefix / WithFromKey), with value as the stored value. A clientv3.Client is
// borrowed from a shared pool on Open and released right after fetching.
//
// Pushdown trade-off: the only pushdown dimension in etcd is the key. Template/match
// predicates (LIKE/%/_) are roughly pushed as prefix/full range scans — the range result
// is a superset of the true matches, and residual pattern matching is re-checked by
// sqlite with Omit=false, so no match is dropped. The value column is never pushed.
package vtab

import (
	// stdlib
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	// third-party
	clientv3 "go.etcd.io/etcd/client/v3"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab"
)

// defaultEtcdDialTimeout etcd 客户端建连超时。
// defaultEtcdDialTimeout is the etcd client dial timeout.
const defaultEtcdDialTimeout = 5 * time.Second

// EtcdConfig 描述一个 etcd 集群的接入配置。
// EtcdConfig describes how to reach one etcd cluster.
type EtcdConfig struct {
	// Endpoints 端点列表（host:port）。
	// Endpoints the endpoint list (host:port).
	Endpoints []string
	// Username 用户名（可选）。
	// Username the user name (optional).
	Username string
	// Password 密码（可选）。
	// Password the password (optional).
	Password string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与端点寻址）。
	// Namespace the datasource namespace; only participates in pool-key derivation.
	Namespace string
}

// etcdClients 是 etcd 客户端池注册表。
// etcdClients is the etcd client pool registry.
var etcdClients = newEtcdClientRegistry()

// etcdClientConfig 描述一个 Key 的客户端构造。
// etcdClientConfig describes the client construction for one Key.
type etcdClientConfig struct {
	cfg clientv3.Config
}

// etcdClientRegistry 按 Key 维护 etcd 客户端池。
// etcdClientRegistry keeps etcd client pools by Key.
type etcdClientRegistry struct {
	mu       sync.RWMutex
	manager  *resourcepool.Manager[*clientv3.Client]
	cfgs     map[resourcepool.Key]*etcdClientConfig
	override map[resourcepool.Key]*clientv3.Client // 仅供测试注入 mock client
}

// newEtcdClientRegistry 创建注册表并装配进程级 Manager。
// newEtcdClientRegistry builds the registry and its process-wide Manager.
func newEtcdClientRegistry() *etcdClientRegistry {
	r := &etcdClientRegistry{
		cfgs:     make(map[resourcepool.Key]*etcdClientConfig),
		override: make(map[resourcepool.Key]*clientv3.Client),
	}
	r.manager = resourcepool.NewManager(r.makeDBFor, resourcepool.NewDefaultConfig())
	return r
}

// makeDBFor 依据 Key 惰性创建 clientv3.Client（resourcepool.Make）；测试经 override
// 注入 mock client 时直接返回之。
// makeDBFor lazily builds a *clientv3.Client for a Key (resourcepool.Make); tests bypass
// it via the override map.
func (r *etcdClientRegistry) makeDBFor(key resourcepool.Key) (*clientv3.Client, error) {
	r.mu.RLock()
	if cli, ok := r.override[key]; ok {
		r.mu.RUnlock()
		return cli, nil
	}
	cfg := r.cfgs[key]
	r.mu.RUnlock()
	if cfg == nil {
		return nil, errors.Newf("etcd: config not registered for key %q", key)
	}
	return clientv3.New(cfg.cfg)
}

// register 注入一个 Key 的客户端配置（重复注入覆盖）。
// register injects the client config for a Key (overwrites on re-injection).
func (r *etcdClientRegistry) register(key resourcepool.Key, cfg clientv3.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfgs[key] = &etcdClientConfig{cfg: cfg}
}

// overrideClient 设置/清除指定 Key 的 mock client 注入（仅供测试）。
// overrideClient sets/clears a mocked client override for a Key (test only).
func (r *etcdClientRegistry) overrideClient(key resourcepool.Key, cli *clientv3.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cli == nil {
		delete(r.override, key)
	} else {
		r.override[key] = cli
	}
}

// NewEtcdPool 注入一份 etcd 集群配置并返回其池句柄。客户端惰性创建，构造本身不触发
// 网络连接。
// NewEtcdPool injects one etcd cluster config and returns its pool handle. Clients are
// created lazily, so construction initiates no network connection.
func NewEtcdPool(cfg *EtcdConfig) *EtcdPool {
	key := resourcepool.Key(fmt.Sprintf("%s@%s/%s", cfg.Namespace, strings.Join(cfg.Endpoints, ","), cfg.Username))
	etcdClients.register(key, clientv3.Config{
		Endpoints:   cfg.Endpoints,
		Username:    cfg.Username,
		Password:    cfg.Password,
		DialTimeout: defaultEtcdDialTimeout,
	})
	return &EtcdPool{key: key}
}

// EtcdPool 是基于资源池的 etcd 客户端池句柄，绑定一个集群配置派生的 Key。
// EtcdPool is a resourcepool-backed etcd client pool handle bound to a Key derived from
// a cluster config.
type EtcdPool struct {
	key resourcepool.Key
}

// Acquire 借出并返回一个可用的 *clientv3.Client（懒建连）。*clientv3.Client 并发安全。
// Acquire borrows a usable *clientv3.Client (lazily connecting). It is concurrency-safe.
func (p *EtcdPool) Acquire(ctx context.Context) (*clientv3.Client, error) {
	return etcdClients.manager.Acquire(ctx, p.key)
}

// Release 归还 *clientv3.Client 以便复用。
// Release returns the *clientv3.Client for reuse.
func (p *EtcdPool) Release(ctx context.Context, cli *clientv3.Client) {
	etcdClients.manager.Release(ctx, p.key, cli)
}

// Close 销毁该实例池内的闲置客户端（借出中的不受影响）。
// Close destroys idle clients in this instance's pool (borrowed ones unaffected).
func (p *EtcdPool) Close(ctx context.Context) {
	etcdClients.manager.Clear(ctx, p.key)
}

// CloseAllEtcdClients 关闭全部 etcd 客户端池并销毁存活对象。供进程优雅关闭调用。
// CloseAllEtcdClients closes every etcd client pool and destroys live objects, meant for
// graceful shutdown.
func CloseAllEtcdClients(ctx context.Context) {
	etcdClients.manager.CloseAll(ctx)
}

// etcdPlugin 实现 DatasourcePlugin，把 etcd 数据源类型接入插件注册表。
// etcdPlugin implements DatasourcePlugin, wiring the etcd datasource type into the plugin
// registry.
type etcdPlugin struct{}

// Key 返回数据源类型键。
// Key returns the datasource type key.
func (etcdPlugin) Key() string { return DatasourceTypeEtcd }

// ConfigType 返回配置值反射类型。
// ConfigType returns the config value's reflect type.
func (etcdPlugin) ConfigType() reflect.Type { return reflect.TypeFor[EtcdTableConfig]() }

// NewConfig 返回全新一份 *EtcdTableConfig。
// NewConfig returns a fresh *EtcdTableConfig.
func (etcdPlugin) NewConfig() any { return &EtcdTableConfig{} }

// ValidateConfig 结构已经解码校验，这里无需额外操作。
// ValidateConfig does nothing extra since the structure was already validated at decode.
func (etcdPlugin) ValidateConfig(_ any) error { return nil }

// TestConn 借用 client、MemberList 探活（覆盖连通+认证）后归还。
// TestConn borrows a client, probes connectivity + auth via MemberList and returns it.
func (etcdPlugin) TestConn(ctx context.Context, cfg any) error {
	c := cfg.(*EtcdTableConfig)
	pool := NewEtcdPool(&EtcdConfig{Endpoints: splitEndpoints(c.Endpoints), Username: c.Username, Password: c.Password})
	cli, err := pool.Acquire(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	_, memberErr := cli.MemberList(ctx)
	pool.Release(ctx, cli)
	if memberErr != nil {
		return errors.WithStack(memberErr)
	}
	return nil
}

// IntrospectSchema 探活后返回固定的 key/value 两列定义。
// IntrospectSchema probes then returns the fixed key/value column definitions.
func (p etcdPlugin) IntrospectSchema(ctx context.Context, cfg any, _ io.Reader) ([]DatasourceColumn, error) {
	if err := p.TestConn(ctx, cfg); err != nil {
		return nil, errors.WithStack(err)
	}
	return []DatasourceColumn{
		{Name: "key", Type: "string"},
		{Name: "value", Type: "string"},
	}, nil
}

// Register 建池 NewEtcdPool 后构造数据源。
// Register builds the pool via NewEtcdPool and constructs the datasource.
func (etcdPlugin) Register(_ context.Context, _ *sqlitevtab.SQLitePool, _ *s3.ObjectCache, namespace, _ string, cfg any) (sqlitevtab.Datasource, error) {
	c := cfg.(*EtcdTableConfig)
	return NewEtcdDatasource(NewEtcdPool(&EtcdConfig{
		Endpoints: splitEndpoints(c.Endpoints),
		Username:  c.Username,
		Password:  c.Password,
		Namespace: namespace,
	})), nil
}

// init 自注册 etcd 数据源类型。
// init self-registers the etcd datasource type.
func init() {
	RegisterDatasourceType(etcdPlugin{})
}
