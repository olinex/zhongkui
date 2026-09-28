package message

import (
	// 标准库
	"bytes"
	"fmt"
	"text/template"
)

// renderTemplateWithFuncs 使用 Go text/template 将 text 模板按 params 与自定义函数
// funcs 渲染为字符串。采用 text/template.Option("missingkey=zero")，使模板引用但 params
// 缺失的键渲染为空/零值而非报错；funcs 可注入自定义模板函数（如通知 label 渲染的 t），
// 为 nil 时等价于无自定义函数的普通渲染。模板解析失败（语法错误）或执行失败（如类型
// 不匹配的函数调用）时返回带上下文的错误。
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
func RenderTemplateWithFuncs(text string, params map[string]any, funcs template.FuncMap) (string, error) {
	return renderTemplateWithFuncs(text, params, funcs)
}

// renderTemplateText 使用 Go text/template 将 text 模板按 params 渲染为字符串。
// 采用 text/template.Option("missingkey=zero")，使模板引用但 params 缺失的键渲染为
// 空/零值而非报错；模板解析失败（语法错误）或执行失败（如类型不匹配的函数调用）时
// 返回带上下文的错误。
func renderTemplateText(text string, params map[string]any) (string, error) {
	return renderTemplateWithFuncs(text, params, nil)
}

// RenderTemplateString 导出以给定参数渲染单条 Go text/template 模板字符串；
// 供试运行接口用样例参数渲染编辑器内的标题/正文。未知/非法模板返回带上下文的错误。
func RenderTemplateString(text string, params map[string]any) (string, error) {
	return renderTemplateText(text, params)
}
