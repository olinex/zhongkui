package tagengine

import (
	"sort"

	"github.com/open-policy-agent/opa/rego"
)

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
