package tagengine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

const (
	// regoQuery 引擎评估的查询路径，取合并后的 tags 集合
	regoQuery = "data.zki.tags"
)

// compiledRule 单条规则的已编译查询。
type compiledRule struct {
	rule  *tag.TagRule
	query *rego.PreparedEvalQuery
}

// compiledGroup 一个（目标类型, 标签类型）分组的已编译查询集合。
type compiledGroup struct {
	rules []*compiledRule
	group *rego.PreparedEvalQuery // 组级合并查询
}

// Engine 基于 OPA 的标签识别引擎。
// 规则按（TargetType, TagType）分组加载并编译缓存，对结构体实例评估产出标签。
// Engine 并发安全，rego.PreparedEvalQuery 可并发评估。
type Engine struct {
	store  *RuleStore
	logger *logging.Logger
	cache  sync.Map // key: groupKey(string) → *compiledGroup
}

// NewEngine 创建标签识别引擎，绑定 core 数据库读取规则。
func NewEngine(db *database.DB, logger *logging.Logger) *Engine {
	return &Engine{
		store:  NewRuleStore(db),
		logger: logger,
	}
}

// groupKey 计算分组的缓存键。
func groupKey(targetType, tagType string) string {
	return fmt.Sprintf(`%s:%s`, targetType, tagType)
}

// loadGroup 加载并编译目标类型与标签类型对应的规则组（带缓存）。
func (e *Engine) loadGroup(ctx context.Context, targetType, tagType string) (*compiledGroup, error) {
	key := groupKey(targetType, tagType)
	if cached, ok := e.cache.Load(key); ok {
		return cached.(*compiledGroup), nil
	}
	rules, err := e.store.ListByGroup(ctx, targetType, tagType)
	if err != nil {
		return nil, err
	}
	group, err := e.compileGroup(ctx, rules)
	if err != nil {
		return nil, err
	}
	e.cache.Store(key, group)
	return group, nil
}

// compileGroup 将规则组编译为查询。
// 组内无启用规则时返回空组（组级查询为 nil，调用方直接返回空结果）。
// 注意：OPA v1.19.0 的 rego 默认按 Rego v0 语法编译，契约的 tags contains
// 属于 v1 语法，必须显式指定 RegoV1。
func (e *Engine) compileGroup(ctx context.Context, rules []*tag.TagRule) (*compiledGroup, error) {
	group := &compiledGroup{rules: make([]*compiledRule, 0, len(rules))}
	if len(rules) == 0 {
		return group, nil
	}
	opts := make([]func(*rego.Rego), 0, len(rules)+2)
	opts = append(opts, rego.Query(regoQuery), rego.SetRegoVersion(ast.RegoV1))
	for _, r := range rules {
		opts = append(opts, rego.Module(moduleName(r), r.Rego))
		// 逐规则编译，供 RecognizeWithDetail 规则归属
		query, err := rego.New(
			rego.Query(regoQuery),
			rego.SetRegoVersion(ast.RegoV1),
			rego.Module(moduleName(r), r.Rego),
		).PrepareForEval(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		group.rules = append(group.rules, &compiledRule{rule: r, query: &query})
	}
	prepared, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	group.group = &prepared
	return group, nil
}

// moduleName 生成 Rego 模块文件名。
func moduleName(r *tag.TagRule) string {
	return fmt.Sprintf("%s.rego", r.Name)
}

// Recognize 对目标结构体实例执行标签识别，返回命中的标签集合（去重排序）。
// 使用组级合并查询进行单次评估，适合高吞吐热路径；不返回命中的规则名。
func (e *Engine) Recognize(ctx context.Context, target any, tagType string) (*RecognizeResult, error) {
	tags, err := e.evalGroup(ctx, target, tagType)
	if err != nil {
		return nil, err
	}
	return &RecognizeResult{Tags: tags}, nil
}

// RecognizeWithDetail 对目标结构体实例执行标签识别，并返回命中的规则名。
// 逐规则评估，用于审计与调试场景。
func (e *Engine) RecognizeWithDetail(ctx context.Context, target any, tagType string) (*RecognizeResult, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, err
	}
	group, err := e.loadGroup(ctx, targetType, tagType)
	if err != nil {
		return nil, err
	}
	input, err := BuildInput(target)
	if err != nil {
		return nil, err
	}
	result := &RecognizeResult{}
	for _, cr := range group.rules {
		results, err := cr.query.Eval(ctx, rego.EvalInput(input))
		if err != nil {
			e.logger.Ctx(ctx).Warnf("tagengine rule eval failed, rule=%s, err=%v", cr.rule.Name, err)
			continue
		}
		tags := extractTags(results)
		if len(tags) == 0 {
			continue
		}
		result.Matched = append(result.Matched, cr.rule.Name)
		result.Tags = append(result.Tags, tags...)
	}
	sort.Strings(result.Tags)
	result.Tags = dedupeSorted(result.Tags)
	sort.Strings(result.Matched)
	return result, nil
}

// evalGroup 组级评估，返回命中的标签。
func (e *Engine) evalGroup(ctx context.Context, target any, tagType string) ([]string, error) {
	targetType, err := TypeKey(target)
	if err != nil {
		return nil, err
	}
	group, err := e.loadGroup(ctx, targetType, tagType)
	if err != nil {
		return nil, err
	}
	if group.group == nil {
		return nil, nil
	}
	input, err := BuildInput(target)
	if err != nil {
		return nil, err
	}
	results, err := group.group.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return extractTags(results), nil
}

// extractTags 从评估结果中提取去重排序后的标签集合。
func extractTags(results rego.ResultSet) []string {
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil
	}
	value, ok := results[0].Expressions[0].Value.([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(value))
	for _, item := range value {
		if s, ok := item.(string); ok {
			tags = append(tags, s)
		}
	}
	sort.Strings(tags)
	return dedupeSorted(tags)
}

// dedupeSorted 对已排序切片去重（要求输入已排序）。
func dedupeSorted(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	for i, s := range in {
		if i == 0 || in[i-1] != s {
			out = append(out, s)
		}
	}
	return out
}

// Invalidate 失效指定分组的已编译查询缓存。
// 规则写入路径（创建/更新/删除/启用切换）后必须调用以保证一致性。
func (e *Engine) Invalidate(targetType, tagType string) {
	e.cache.Delete(groupKey(targetType, tagType))
}

// InvalidateAll 失效全部分组的已编译查询缓存。
func (e *Engine) InvalidateAll() {
	e.cache.Range(func(k, _ any) bool {
		e.cache.Delete(k)
		return true
	})
}
