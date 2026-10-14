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

package vtab

import (
	// 标准库
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	// 第三方
	clientv3 "go.etcd.io/etcd/client/v3"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/resourcepool"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab"
)

// defaultEtcdDialTimeout etcd 客户端建连超时。
const defaultEtcdDialTimeout = 5 * time.Second

// EtcdConfig 描述一个 etcd 集群的接入配置。
type EtcdConfig struct {
	// Endpoints 端点列表（host:port）。
	Endpoints []string
	// Username 用户名（可选）。
	Username string
	// Password 密码（可选）。
	Password string
	// Namespace 数据源所属命名空间，仅参与连接池 Key 派生（不参与端点寻址）。
	Namespace string
}

// etcdClients 是 etcd 客户端池注册表。
var etcdClients = newEtcdClientRegistry()

// etcdClientConfig 描述一个 Key 的客户端构造。
type etcdClientConfig struct {
	cfg clientv3.Config
}

// etcdClientRegistry 按 Key 维护 etcd 客户端池。
type etcdClientRegistry struct {
	mu       sync.RWMutex
	manager  *resourcepool.Manager[*clientv3.Client]
	cfgs     map[resourcepool.Key]*etcdClientConfig
	override map[resourcepool.Key]*clientv3.Client // 仅供测试注入 mock client
}

// newEtcdClientRegistry 创建注册表并装配进程级 Manager。
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
func (r *etcdClientRegistry) register(key resourcepool.Key, cfg clientv3.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfgs[key] = &etcdClientConfig{cfg: cfg}
}

// overrideClient 设置/清除指定 Key 的 mock client 注入（仅供测试）。
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
type EtcdPool struct {
	// key 集群配置派生的连接池 Key。
	key resourcepool.Key
}

// Acquire 借出并返回一个可用的 *clientv3.Client（懒建连）。*clientv3.Client 并发安全。
func (p *EtcdPool) Acquire(ctx context.Context) (*clientv3.Client, error) {
	return etcdClients.manager.Acquire(ctx, p.key)
}

// Release 归还 *clientv3.Client 以便复用。
func (p *EtcdPool) Release(ctx context.Context, cli *clientv3.Client) {
	etcdClients.manager.Release(ctx, p.key, cli)
}

// Close 销毁该实例池内的闲置客户端（借出中的不受影响）。
func (p *EtcdPool) Close(ctx context.Context) {
	etcdClients.manager.Clear(ctx, p.key)
}

// CloseAllEtcdClients 关闭全部 etcd 客户端池并销毁存活对象。供进程优雅关闭调用。
func CloseAllEtcdClients(ctx context.Context) {
	etcdClients.manager.CloseAll(ctx)
}

// etcdPlugin 实现 DatasourcePlugin，把 etcd 数据源类型接入插件注册表。
type etcdPlugin struct{}

// Key 返回数据源类型键。
func (etcdPlugin) Key() string { return DatasourceTypeEtcd }

// ConfigType 返回配置值反射类型。
func (etcdPlugin) ConfigType() reflect.Type { return reflect.TypeFor[EtcdTableConfig]() }

// NewConfig 返回全新一份 *EtcdTableConfig。
func (etcdPlugin) NewConfig() any { return &EtcdTableConfig{} }

// ValidateConfig 结构已经解码校验，这里无需额外操作。
func (etcdPlugin) ValidateConfig(_ any) error { return nil }

// TestConn 借用 client、MemberList 探活（覆盖连通+认证）后归还。
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
func init() {
	RegisterDatasourceType(etcdPlugin{})
}
