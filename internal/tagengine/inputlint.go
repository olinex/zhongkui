// 本文件实现 Rego 规则源码的输入文档字段感知 lint（LintInputRefs）。
// 用户 Rego 中引用 input.<field> 的字段不在目标模型输入文档中时，
// 产出 warning 级诊断（带行列号），帮助发现字段拼写错误；
// 与 validate.go 的语法/契约校验互补，lint 不阻塞规则保存，仅提示。

package tagengine

import (
	// 第三方
	"github.com/open-policy-agent/opa/ast"
)

// LintInputRefs 对 Rego 源码做输入文档字段感知 lint。
// 解析 Rego 模块 AST，遍历 input.<field>（含 input["field"]）引用，
// 引用字段不在 fields 元信息中时产出 warning 级 RegoError（行列定位到该字段）。
// 按字段名去重；解析失败返回空列表（语法错误由既有校验负责）。
func LintInputRefs(source string, fields []InputFieldMeta) []RegoError {
	module, err := ast.ParseModuleWithOpts(regoModuleName, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil || module == nil {
		return nil
	}
	valid := make(map[string]any, len(fields))
	sensitive := make(map[string]any, len(fields))
	for _, meta := range fields {
		valid[meta.Field] = struct{}{}
		if meta.Sensitive {
			sensitive[meta.Field] = struct{}{}
		}
	}
	warnings := make([]RegoError, 0, 1)
	seen := make(map[string]struct{})
	visitor := ast.NewGenericVisitor(func(x any) bool {
		term, ok := x.(*ast.Term)
		if !ok {
			return false
		}
		ref, ok := term.Value.(ast.Ref)
		if !ok || len(ref) < 2 {
			return false
		}
		// 仅处理 input 根引用，data.zki 等其余根不参与 lint
		head, ok := ref[0].Value.(ast.Var)
		if !ok || string(head) != "input" {
			return false
		}
		field, ok := refFieldName(ref[1])
		if !ok {
			return false
		}
		// 已确认是 input 字段引用，无需再下钻该 term 的子节点
		if _, ok := valid[field]; ok {
			// 敏感字段不出现在输入文档中，只能经 crypto.reveal 获取明文
			if _, isSensitive := sensitive[field]; !isSensitive {
				return true
			}
		}
		if _, dup := seen[field]; dup {
			return true
		}
		seen[field] = struct{}{}
		warning := RegoError{
			// Message/Hint 为 i18n 消息键（不携带文案明文），字段名进入 Args，
			// 由接口层 localizeRegoErrors 按请求语言翻译后统一渲染
			Message: lintFieldNotExistKey,
			Type:    "warning",
			Hint:    lintFieldHintKey,
			Args:    []any{field},
		}
		if _, isSensitive := sensitive[field]; isSensitive {
			warning.Message = lintSensitiveFieldKey
			warning.Hint = lintSensitiveFieldHintKey
			warning.Args = []any{field, field}
		}
		if loc := ref[1].Location; loc != nil {
			warning.Line = loc.Row
			warning.Column = loc.Col
		}
		warnings = append(warnings, warning)
		return true
	})
	visitor.Walk(module)
	return warnings
}

// refFieldName 提取 input.<field>（含 input["field"]）第二段的字段名。
// 第二段为 ast.Var（点号形式）或 ast.String（方括号字符串形式）时返回字段名；
// 其余形式（如数字下标 input[0]）返回 false，跳过不 lint。
func refFieldName(t *ast.Term) (string, bool) {
	switch v := t.Value.(type) {
	case ast.Var:
		return string(v), true
	case ast.String:
		return string(v), true
	}
	return "", false
}
