package message

import (
	// stdlib
	"bytes"
	"fmt"
	"text/template"
)

// renderTemplateWithFuncs 使用 Go text/template 将 text 模板按 params 与自定义函数
// funcs 渲染为字符串。采用 text/template.Option("missingkey=zero")，使模板引用但 params
// 缺失的键渲染为空/零值而非报错；funcs 可注入自定义模板函数（如通知 label 渲染的 t），
// 为 nil 时等价于无自定义函数的普通渲染。模板解析失败（语法错误）或执行失败（如类型
// 不匹配的函数调用）时返回带上下文的错误。
// renderTemplateWithFuncs renders the Go text/template text against params with the custom
// function map funcs. It uses text/template.Option("missingkey=zero") so keys referenced by
// the template but missing from params render as empty/zero instead of erroring; funcs
// injects custom template functions (e.g. the t used for notification labels) and a nil
// funcs renders like a plain template without custom functions. A template that fails to
// parse (syntax error) or to execute (e.g. a type-mismatched call) returns an error wrapped
// with context.
func renderTemplateWithFuncs(text string, params map[string]any, funcs template.FuncMap) (string, error) {
	tmpl, err := template.New("render").Option("missingkey=zero").Funcs(funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("message: parse template %q: %w", text, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("message: execute template %q: %w", text, err)
	}
	return buf.String(), nil
}

// RenderTemplateWithFuncs 导出以给定参数与自定义函数渲染单条 Go text/template 模板字符串。
// 与 RenderTemplateString 的区别在于可注入自定义模板函数（如通知 label 渲染的 t），
// 适合按目标语言渲染带函数调用的默认/持久化模板。未知/非法模板返回带上下文的错误。
// RenderTemplateWithFuncs exported wrapper that renders a single Go text/template string
// against params with the given custom functions. Unlike RenderTemplateString it can inject
// custom template functions (e.g. the t used for notification labels), which is useful to
// render default/persisted templates that call functions in the target language. Unknown or
// invalid templates return a contextual error.
func RenderTemplateWithFuncs(text string, params map[string]any, funcs template.FuncMap) (string, error) {
	return renderTemplateWithFuncs(text, params, funcs)
}

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
	return renderTemplateWithFuncs(text, params, nil)
}

// RenderTemplateString 导出以给定参数渲染单条 Go text/template 模板字符串；
// 供试运行接口用样例参数渲染编辑器内的标题/正文。未知/非法模板返回带上下文的错误。
// RenderTemplateString exported wrapper that renders a single Go text/template string
// against params; used by the dry-run API to render the in-editor title/content with
// sample params. Unknown or invalid templates return a contextual error.
func RenderTemplateString(text string, params map[string]any) (string, error) {
	return renderTemplateText(text, params)
}
