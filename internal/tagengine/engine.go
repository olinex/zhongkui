package tagengine

import (
	"context"
	"fmt"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
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
// Engine is the OPA-based tag recognition engine. Rules are grouped by
// (Namespace, Model) and compiled/cached; evaluating a struct produces tags. The
// engine may optionally bind an etcd client: when unbound it only offers local
// recognition and cache invalidation; when bound it implements runtime.Service,
// strstarting a goroutine to watch rule changes and broadcast change events.
// Engine is concurrency-safe and rego.PreparedEvalQuery can be evaluated in parallel.
//
// Third-party datasources use per-namespace pools (vtab.Pools): each namespace owns
// its own SQLitePool and vtables are registered under their bare name. Before
// evaluation the engine resolves the target instance's namespace pool and binds it
// to the evaluation ctx (WithDatasourcePool), which the sqlite.query builtin reads,
// so rego rules reference bare names without a namespace prefix. With no pools
// bound the engine offers no third-party datasource querying.
type Engine struct {
	db     *database.DB
	logger *logging.Logger
	client *etcd.EtcdClient
	pools  *vtab.Pools
	cache  sync.Map // key: groupKey(string) → *compiledGroup
	// watcher 规则变更监听器（etcd 通用层），nil 时 Start/Shutdown 直接跳过。
	watcher *etcd.Watcher
}

// groupKey 计算分组的缓存键。
// 命名空间参与分组键，不同租户的规则组互不串扰。
func groupKey(namespace, model string) string {
	return fmt.Sprintf(`%s:%s`, namespace, model)
}

// EngineOption 是 Engine 的函数式配置选项。
// EngineOption is a functional option for configuring an Engine.
type EngineOption func(*Engine)

// NewEngine 创建标签识别引擎，绑定 core 数据库读取规则。
// 可选通过 variadic 选项注入第三方数据源按命名空间池（WithDatasourcePools）等。
// NewEngine creates the tag recognition engine bound to the core database for rule
// reads. Optional variadic options inject per-namespace third-party datasource pools
// (WithDatasourcePools) and more.
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
// WithDatasourcePools injects the per-namespace third-party datasource pools so the
// evaluation path binds the namespace pool to the evaluation ctx for sqlite.query to
// query bare-named vtables.
func WithDatasourcePools(pools *vtab.Pools) EngineOption {
	return func(e *Engine) {
		e.pools = pools
	}
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
		return nil, err
	}
	group, err := CompileGroup(ctx, rules)
	if err != nil {
		return nil, err
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
	return group.RecognizeWithDetail(ctx, rego.EvalInput(input)), nil
}

// withNamespacePool 若引擎已注入按命名空间池，则把该命名空间的 SQLitePool 绑定到
// ctx，供求值 sqlite.query 查询该命名空间的裸名 vtable；未注入或解析失败时原样返回
// ctx（sqlite.query 将报未绑池）。
// withNamespacePool binds the namespace's SQLitePool onto ctx for sqlite.query to
// query bare-named vtables when the engine has been injected with per-namespace pools;
// it returns ctx unchanged when no pools are injected or resolution fails.
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
