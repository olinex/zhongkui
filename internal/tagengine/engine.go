package tagengine

import (
	"context"
	"fmt"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// Engine 基于 OPA 的标签识别引擎。
// 规则按（Namespace, Model）分组加载并编译缓存，对结构体实例评估产出标签。
// 引擎可选绑定 etcd 客户端：未绑定时仅提供本地识别与缓存失效能力；
// 绑定时实现 runtime.Service，可启动协程监听规则变更并对外广播变更事件。
// Engine 并发安全，rego.PreparedEvalQuery 可并发评估。
//
// 第三方数据源采用按命名空间池（vtab.Pools）：各命名空间独立持有自己的
// SQLitePool，vtable 以裸名注册。求值前引擎按目标实例的命名空间解析对应池并绑定到
// 求值 ctx（WithDatasourcePool），sqlite.query builtin 从中读取，故 rego 规则可直接
// 引用裸名、无需命名空间前缀。未绑定 pools 时引擎不提供第三方数据源查询。
type Engine struct {
	// db core 库连接，用于读取标签规则。
	db *database.DB
	// logger 日志记录器。
	logger *logging.Logger
	// client etcd 客户端（nil 时仅本地识别，不监听规则变更）。
	client *etcd.EtcdClient
	// pools 按命名空间隔离的第三方数据源池（nil 时不提供第三方数据源查询）。
	pools *vtab.Pools
	// s3Loader S3 加密对象读取器（nil 时不支持 crypto.reveal 读取 S3Crypto 字段）。
	s3Loader S3ObjectLoader
	// key: groupKey(string) → *compiledGroup
	cache sync.Map
	// watcher 规则变更监听器（etcd 通用层），nil 时 Start/Shutdown 直接跳过。
	watcher *etcd.Watcher
	// s3CryptoMaxBytes S3Crypto 字段正文读取上限（字节），超限报错。
	s3CryptoMaxBytes int64
}

// groupKey 计算分组的缓存键。
// 命名空间参与分组键，不同租户的规则组互不串扰。
func groupKey(namespace, model string) string {
	return fmt.Sprintf(`%s:%s`, namespace, model)
}

// EngineOption 是 Engine 的函数式配置选项。
type EngineOption func(*Engine)

// NewEngine 创建标签识别引擎，绑定 core 数据库读取规则。
// 可选通过 variadic 选项注入第三方数据源按命名空间池（WithDatasourcePools）等。
func NewEngine(db *database.DB, logger *logging.Logger, client *etcd.EtcdClient, opts ...EngineOption) *Engine {
	engine := &Engine{
		db:     db,
		logger: logger,
		client: client,
	}
	for _, opt := range opts {
		opt(engine)
	}
	// 构造规则变更监听器（handler 需访问 engine，故先建 engine 再建 watcher）；
	// client 为 nil 也照常构造，Watcher.Start 内部会跳过监听。
	engine.watcher = etcd.NewWatcher(client, ruleChangeKey, &ruleChangeHandler{e: engine}, logger)
	return engine
}

// WithDatasourcePools 注入按命名空间隔离的第三方数据源池，供求值路径将各命名空间池
// 绑定到求值 ctx 以供 sqlite.query 查询裸名 vtable。
func WithDatasourcePools(pools *vtab.Pools) EngineOption {
	return func(e *Engine) {
		e.pools = pools
	}
}

// WithSensitiveRuntime 注入敏感字段解密运行时：S3 对象读取器与正文大小上限。
// loader 为 nil 时不支持 crypto.reveal 读取 S3Crypto 字段，Crypto 字段不受影响。
func WithSensitiveRuntime(loader S3ObjectLoader, maxBytes int64) EngineOption {
	return func(e *Engine) {
		e.s3Loader = loader
		e.s3CryptoMaxBytes = maxBytes
	}
}

// RuntimeContext 返回绑定了敏感字段解密运行时的 ctx。
// 未注入 S3 读取器时原样返回；供引擎识别、视图 dry-run 与样例校验复用同一运行时。
func (e *Engine) RuntimeContext(ctx context.Context) context.Context {
	if e.s3Loader == nil {
		return ctx
	}
	return WithSensitiveRuntimeContext(ctx, e.s3Loader, e.s3CryptoMaxBytes)
}

// getRulesFromDB 返回指定命名空间下目标模型中启用的规则。
func (e *Engine) getRulesFromDB(ctx context.Context, namespace, model string) ([]*tag.TagRule, error) {
	var rules []*tag.TagRule
	if err := e.db.WithContext(ctx).
		Where("namespace_name = ? AND model = ? AND enabled = ?", namespace, model, true).
		Find(&rules).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rules, nil
}

// loadGroup 加载并编译指定命名空间下目标模型对应的规则组（带缓存）。
func (e *Engine) loadGroup(ctx context.Context, namespace, model string) (*compiledGroup, error) {
	key := groupKey(namespace, model)
	if cached, ok := e.cache.Load(key); ok {
		return cached.(*compiledGroup), nil
	}
	rules, err := e.getRulesFromDB(ctx, namespace, model)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	group, err := CompileGroup(ctx, rules)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	e.cache.Store(key, group)
	return group, nil
}

// Recognize 对目标结构体实例执行标签识别，返回命中的标签集合（去重排序）。
// 规则组按目标实例的命名空间隔离加载；无命名空间字段时回退到未归属命名空间规则。
// 使用组级合并查询进行单次评估，适合高吞吐热路径；不返回命中的规则名。
// 求值前按目标实例命名空间解析第三方数据源池并绑定到求值 ctx（若引擎已注入 pools）。
func (e *Engine) Recognize(ctx context.Context, target any) ([]string, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	namespace := extractNamespace(target)
	group, err := e.loadGroup(ctx, namespace, targetType)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if group.group == nil {
		return nil, nil
	}
	input, err := BuildInput(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ctx = e.withNamespacePool(ctx, namespace)
	ctx = WithSensitiveTarget(e.RuntimeContext(ctx), targetType, target)
	return group.Recognize(ctx, rego.EvalInput(input))
}

// RecognizeWithDetail 对目标结构体实例执行标签识别，并返回命中的规则名。
// 规则组按目标实例的命名空间隔离加载；逐规则评估，用于审计与调试场景。
// 求值前同样按命名空间解析并绑定第三方数据源池。
func (e *Engine) RecognizeWithDetail(ctx context.Context, target any) (*GroupDetailResult, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	namespace := extractNamespace(target)
	group, err := e.loadGroup(ctx, namespace, targetType)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	input, err := BuildInput(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ctx = e.withNamespacePool(ctx, namespace)
	ctx = WithSensitiveTarget(e.RuntimeContext(ctx), targetType, target)
	return group.RecognizeWithDetail(ctx, rego.EvalInput(input)), nil
}

// withNamespacePool 若引擎已注入按命名空间池，则把该命名空间的 SQLitePool 绑定到
// ctx，供求值 sqlite.query 查询该命名空间的裸名 vtable；未注入或解析失败时原样返回
// ctx（sqlite.query 将报未绑池）。
func (e *Engine) withNamespacePool(ctx context.Context, namespace string) context.Context {
	if e.pools == nil {
		return ctx
	}
	return WithDatasourcePool(ctx, e.pools.Pool(namespace))
}

// Invalidate 失效指定命名空间下分组的已编译查询缓存。
// 规则写入路径（创建/更新/删除/启用切换）后必须调用以保证一致性。
func (e *Engine) Invalidate(namespace, model string) {
	e.cache.Delete(groupKey(namespace, model))
}

// InvalidateAll 失效全部分组的已编译查询缓存。
func (e *Engine) InvalidateAll() {
	e.cache.Range(func(k, _ any) bool {
		e.cache.Delete(k)
		return true
	})
}
