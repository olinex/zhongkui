package tagengine

import (
	"context"
	"time"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

type GroupDetailResult struct {
	// Tags 组内所有规则识别到的去重排序后的标签集合。
	Tags []string `json:"tags"`
	// Duration 组级合并查询耗时。
	Duration serializer.DurationNS `json:"duration"`
	// Error 组级合并查询错误。
	Error error `json:"error"`
	// Rules 组内所有规则的识别结果。
	Rules map[string]*RuleDetailResult `json:"rules"`
}

// compiledGroup 一个（目标类型）分组的已编译查询集合。
type compiledGroup struct {
	rules map[string]*compiledRule
	group *rego.PreparedEvalQuery // 组级合并查询
}

// CompileGroup 将一组规则编译为查询集合。
// 组内无规则时返回空组（组级查询为 nil，调用方直接返回空结果）。
// 同时生成组级合并查询与逐规则查询，供引擎识别与 dry-run/组级校验复用。
// extraOpts 允许调用方追加 rego 编译选项（如运行期样例校验启用
// rego.StrictBuiltinErrors(true)，将 builtin 类型错误浮出为评估错误）。
// 注意：OPA v1.19.0 的 rego 默认按 Rego v0 语法编译，契约的 tags contains
// 属于 v1 语法，必须显式指定 RegoV1。
func CompileGroup(ctx context.Context, rules []*tag.TagRule, extraOpts ...func(*rego.Rego)) (*compiledGroup, error) {
	if len(rules) == 0 {
		return &compiledGroup{rules: make(map[string]*compiledRule)}, nil
	}
	opts := make([]func(*rego.Rego), 0, len(rules)+2+len(extraOpts))
	opts = append(opts, rego.Query(regoQuery), rego.SetRegoVersion(ast.RegoV1))
	opts = append(opts, extraOpts...)
	compiled := make(map[string]*compiledRule)
	for _, r := range rules {
		moduleName, cr, err := CompileRule(ctx, r)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		opts = append(opts, rego.Module(moduleName, r.Rego))
		compiled[r.Name] = cr
	}
	prepared, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &compiledGroup{rules: compiled, group: &prepared}, nil
}

func (g *compiledGroup) Recognize(ctx context.Context, options ...rego.EvalOption) ([]string, error) {
	results, err := g.group.Eval(ctx, options...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return extractTags(results), nil
}

func (g *compiledGroup) RecognizeWithDetail(ctx context.Context, options ...rego.EvalOption) *GroupDetailResult {
	start := time.Now()
	tags, err := g.Recognize(ctx, options...)
	duration := time.Since(start)
	rules := make(map[string]*RuleDetailResult, len(g.rules))
	for name, rule := range g.rules {
		rules[name] = rule.RecognizeWithDetail(ctx, options...)
	}
	return &GroupDetailResult{
		Tags:     tags,
		Duration: serializer.DurationNS(duration),
		Error:    err,
		Rules:    rules,
	}
}
