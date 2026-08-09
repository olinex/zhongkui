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
)

// Engine 基于 OPA 的标签识别引擎。
// 规则按（Namespace, TargetType, TagType）分组加载并编译缓存，对结构体实例评估产出标签。
// 引擎可选绑定 etcd 客户端：未绑定时仅提供本地识别与缓存失效能力；
// 绑定时实现 runtime.Service，可启动协程监听规则变更并对外广播变更事件。
// Engine 并发安全，rego.PreparedEvalQuery 可并发评估。
type Engine struct {
	db     *database.DB
	logger *logging.Logger
	client *etcd.EtcdClient
	cache  sync.Map // key: groupKey(string) → *compiledGroup
}

// groupKey 计算分组的缓存键。
// 命名空间参与分组键，不同租户的规则组互不串扰。
func groupKey(namespace, targetType, tagType string) string {
	return fmt.Sprintf(`%s:%s:%s`, namespace, targetType, tagType)
}

// NewEngine 创建标签识别引擎，绑定 core 数据库读取规则。
func NewEngine(db *database.DB, logger *logging.Logger, client *etcd.EtcdClient) *Engine {
	engine := &Engine{
		db:     db,
		logger: logger,
		client: client,
	}
	return engine
}

// getRulesFromDB 返回指定命名空间下目标类型与标签类型中启用的规则。
func (e *Engine) getRulesFromDB(ctx context.Context, namespace, targetType, tagType string) ([]*tag.TagRule, error) {
	var rules []*tag.TagRule
	if err := e.db.WithContext(ctx).
		Where("namespace_name = ? AND target_type = ? AND tag_type = ? AND enabled = ?", namespace, targetType, tagType, true).
		Find(&rules).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rules, nil
}

// loadGroup 加载并编译指定命名空间下目标类型与标签类型对应的规则组（带缓存）。
func (e *Engine) loadGroup(ctx context.Context, namespace, targetType, tagType string) (*compiledGroup, error) {
	key := groupKey(namespace, targetType, tagType)
	if cached, ok := e.cache.Load(key); ok {
		return cached.(*compiledGroup), nil
	}
	rules, err := e.getRulesFromDB(ctx, namespace, targetType, tagType)
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
func (e *Engine) Recognize(ctx context.Context, target any, tagType string) ([]string, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	group, err := e.loadGroup(ctx, extractNamespace(target), targetType, tagType)
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
	return group.Recognize(ctx, rego.EvalInput(input))
}

// RecognizeWithDetail 对目标结构体实例执行标签识别，并返回命中的规则名。
// 规则组按目标实例的命名空间隔离加载；逐规则评估，用于审计与调试场景。
func (e *Engine) RecognizeWithDetail(ctx context.Context, target any, tagType string) (*GroupDetailResult, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	group, err := e.loadGroup(ctx, extractNamespace(target), targetType, tagType)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	input, err := BuildInput(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return group.RecognizeWithDetail(ctx, rego.EvalInput(input)), nil
}

// Invalidate 失效指定命名空间下分组的已编译查询缓存。
// 规则写入路径（创建/更新/删除/启用切换）后必须调用以保证一致性。
func (e *Engine) Invalidate(namespace, targetType, tagType string) {
	e.cache.Delete(groupKey(namespace, targetType, tagType))
}

// InvalidateAll 失效全部分组的已编译查询缓存。
func (e *Engine) InvalidateAll() {
	e.cache.Range(func(k, _ any) bool {
		e.cache.Delete(k)
		return true
	})
}
