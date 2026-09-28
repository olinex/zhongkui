package tagengine

import (
	"context"
	"fmt"
	"time"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// regoQuery 引擎评估的查询路径，取合并后的 tags 集合
	regoQuery = "data.zki.tags"
)

// RuleDetailResult 单条规则的识别结果，含标签集合、评估耗时与评估错误。
type RuleDetailResult struct {
	// Tags 规则匹配的标签集合
	Tags []string `json:"tags"`
	// Duration 规则评估耗时
	Duration serializer.DurationNS `json:"duration"`
	// Error 规则评估错误
	Error error `json:"error"`
}

// compiledRule 单条规则的已编译查询。
type compiledRule struct {
	rule  *tag.TagRule
	query *rego.PreparedEvalQuery
}

// CompileRule 将单条规则编译为可复用查询，返回模块名与已编译规则。
func CompileRule(ctx context.Context, rule *tag.TagRule) (string, *compiledRule, error) {
	moduleName := fmt.Sprintf("%s.rego", rule.Name)
	// 逐规则编译，供 RecognizeWithDetail 规则归属与 dry-run 逐规则详情
	query, err := rego.New(
		rego.Query(regoQuery),
		rego.SetRegoVersion(ast.RegoV1),
		rego.Module(moduleName, rule.Rego),
	).PrepareForEval(ctx)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	return moduleName, &compiledRule{rule: rule, query: &query}, nil
}

// Recognize 执行规则查询并返回去重排序后的标签集合。
func (r *compiledRule) Recognize(ctx context.Context, options ...rego.EvalOption) ([]string, error) {
	results, err := r.query.Eval(ctx, options...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return extractTags(results), nil
}

// RecognizeWithDetail 执行规则查询并返回含标签、耗时与错误的识别详情。
func (r *compiledRule) RecognizeWithDetail(ctx context.Context, options ...rego.EvalOption) *RuleDetailResult {
	start := time.Now()
	tags, err := r.Recognize(ctx, options...)
	return &RuleDetailResult{
		Tags:     tags,
		Duration: serializer.DurationNS(time.Since(start)),
		Error:    err,
	}
}
