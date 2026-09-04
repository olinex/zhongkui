package message

import (
	// stdlib
	"bytes"
	"fmt"
	"text/template"
)

// renderTemplateText 使用 Go text/template 将 text 模板按 params 渲染为字符串。
// 采用 text/template.Option("missingkey=zero")，使模板引用但 params 缺失的键渲染为
// 空/零值而非报错；模板解析失败（语法错误）或执行失败（如类型不匹配的函数调用）时
// 返回带上下文的错误。
// renderTemplateText renders the Go text/template text against params. It uses
// text/template.Option("missingkey=zero") so keys referenced by the template but missing
// from params render as empty/zero instead of erroring; a template that fails to parse
// (syntax error) or to execute (e.g. a type-mismatched call) returns an error wrapped
// with context.
func renderTemplateText(text string, params map[string]any) (string, error) {
	tmpl, err := template.New("render").Option("missingkey=zero").Parse(text)
	if err != nil {
		return "", fmt.Errorf("message: parse template %q: %w", text, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("message: execute template %q: %w", text, err)
	}
	return buf.String(), nil
}

// RenderTemplateString 导出以给定参数渲染单条 Go text/template 模板字符串；
// 供试运行接口用样例参数渲染编辑器内的标题/正文。未知/非法模板返回带上下文的错误。
// RenderTemplateString exported wrapper that renders a single Go text/template string
// against params; used by the dry-run API to render the in-editor title/content with
// sample params. Unknown or invalid templates return a contextual error.
func RenderTemplateString(text string, params map[string]any) (string, error) {
	return renderTemplateText(text, params)
}