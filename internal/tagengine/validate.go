package tagengine

import (
	"github.com/open-policy-agent/opa/ast"
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
