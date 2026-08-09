package tagengine

import (
	"context"

	"github.com/open-policy-agent/opa/ast"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// regoPackageRef 契约包名对应的完整引用，OPA 解析 package zki 后路径为 data.zki
	regoPackageRef = "data." + base.GlobalPrefix
	// regoTagsRuleName 规则契约要求的标签规则名
	regoTagsRuleName = "tags"
	// regoModuleName 校验使用的 Rego 模块文件名
	regoModuleName = "rule.rego"
)

type unwrapper interface{ Unwrap() error }

// ValidateRego 校验 Rego 规则源码是否符合标签识别契约。
//
// 校验内容:
//  1. Rego 语法正确，解析失败返回带行号定位的错误；
//  2. 包名必须为 zki；
//  3. 模块必须产出 tags 规则（contains / 整体赋值 / 部分集赋值均可）；
//  4. 语义合法，例如同一规则名不得同时存在完整赋值与部分定义（CompileModulesWithOpt）。
//
// 注意：OPA v1.19.0 的 ast.ParseModule 默认按 Rego v0 语法解析，
// 而契约要求的 tags contains 属于 Rego v1 语法，因此这里显式指定
// RegoV1 模式解析，以支持 contains 规则形式；语义编译同样需显式 RegoV1。
func ValidateRego(source string) error {
	module, err := ast.ParseModuleWithOpts(regoModuleName, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil {
		return errors.WithStack(err)
	}
	if module == nil || module.Package == nil || module.Package.Path.String() != regoPackageRef {
		return ErrInvalidRego
	}
	hasTags := false
	for _, rule := range module.Rules {
		if rule.Head.Name.String() == regoTagsRuleName {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return ErrInvalidRego
	}
	// 语义编译：捕获解析阶段检测不到的冲突，
	// 如 tags := {...} 与 tags contains ... 同名完整/部分定义并存。
	if _, err := ast.CompileModulesWithOpt(map[string]string{regoModuleName: source}, ast.CompileOpts{
		ParserOptions: ast.ParserOptions{RegoVersion: ast.RegoV1},
	}); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// RegoError 描述 Rego 规则源码中的一处校验错误，带行号/列号定位。
type RegoError struct {
	// Line 错误所在行号（1 起），无法定位时为 0
	Line int `json:"line"`
	// Column 错误所在列号（1 起），无法定位时为 0
	Column int `json:"column"`
	// Message 错误信息
	Message string `json:"message"`
}

// InspectRego 校验 Rego 规则源码是否符合标签识别契约，返回结构化错误列表与是否合法。
// 与 ValidateRego 的校验语义一致，但错误带行号/列号，供 validate / dry-run 接口输出。
func InspectRego(source string) ([]RegoError, bool) {
	module, err := ast.ParseModuleWithOpts(regoModuleName, source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil {
		return extractRegoErrors(err), false
	}
	if module == nil || module.Package == nil || module.Package.Path.String() != regoPackageRef {
		return []RegoError{{Message: ErrInvalidRego.Error()}}, false
	}
	hasTags := false
	for _, rule := range module.Rules {
		if rule.Head.Name.String() == regoTagsRuleName {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return []RegoError{{Message: ErrInvalidRego.Error()}}, false
	}
	if _, err := ast.CompileModulesWithOpt(map[string]string{regoModuleName: source}, ast.CompileOpts{
		ParserOptions: ast.ParserOptions{RegoVersion: ast.RegoV1},
	}); err != nil {
		return extractRegoErrors(err), false
	}
	return nil, true
}

// extractRegoErrors 将 OPA 解析/编译错误提取为结构化错误列表。
// ast.Errors 是 []*ast.Error 切片类型，errors.As 不支持切片目标，需类型断言；
// 同时沿错误链向上遍历，兼容 errors.WithStack 等包装。
func extractRegoErrors(err error) []RegoError {
	for e := err; e != nil; {
		if errs, ok := e.(ast.Errors); ok && len(errs) > 0 {
			out := make([]RegoError, 0, len(errs))
			for _, item := range errs {
				if item == nil {
					continue
				}
				out = append(out, newRegoError(item))
			}
			if len(out) > 0 {
				return out
			}
		} else if item, ok := e.(*ast.Error); ok && item != nil {
			return []RegoError{newRegoError(item)}
		}
		if u, ok := e.(unwrapper); ok {
			e = u.Unwrap()
			continue
		}
		break
	}
	return []RegoError{{Message: err.Error()}}
}

// newRegoError 将单个 ast.Error 转换为 RegoError，提取行列定位。
func newRegoError(e *ast.Error) RegoError {
	re := RegoError{Message: e.Message}
	if e.Location != nil {
		re.Line = e.Location.Row
		re.Column = e.Location.Col
	}
	return re
}

// ValidateGroup 校验一组规则能否作为一个整体语义编译通过。
// 用于拦截跨规则冲突（如两条规则同时对 tags 整体赋值，或
// 一条规则 tags := 与另一条 tags contains 并存），
// 在规则增删改/启停/回滚前基于"变更后的组"调用。
func ValidateGroup(ctx context.Context, rules []*tag.TagRule) error {
	if _, err := CompileGroup(ctx, rules); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ExtractRegoErrors 从 OPA 解析/编译错误中提取结构化校验错误列表（带行列号）。
// 供路由层在组级校验（ValidateGroup）失败后输出带行列号的错误详情。
func ExtractRegoErrors(err error) []RegoError {
	return extractRegoErrors(err)
}
