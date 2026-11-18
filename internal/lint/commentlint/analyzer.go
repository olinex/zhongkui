// Package commentlint 实现注释完备性检查的 go/analysis Analyzer，供 make lint 与 CI 调用。
//
// 检查范围与 AGENTS.md「Comments」条目一致：
//
//   - 包注释：按目录聚合，同一包内至少一个文件带包注释即视为通过；被构建约束排除的文件
//     同样计入，测试变体（含外部测试包 `xxx_test`）不在其列（包注释由主包承载）；
//   - 导出类型、导出函数、导出方法、导出常量与导出变量必须带注释；
//   - 导出结构体的字段必须带注释（含非导出字段），字段 struct tag 含 `comment:"` 视为已有注释；
//   - 导出接口的方法必须带注释；
//   - panic 参数中的字符串字面量必须使用英文（不得包含汉字）。
//
// 豁免规则：
//
//   - 生成文件（包子句前的注释含生成标记，含非标准写法）整体跳过；
//   - 测试文件中 `Test` / `Benchmark` / `Example` / `Fuzz` 前缀的导出函数跳过；
//   - 内嵌字段、内嵌接口与匿名成员不检查；
//   - 多名字声明中「部分导出」的字段（如 `A, b string`）不检查。
package commentlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

// Analyzer 检查包注释、导出声明与结构体字段注释的完备性，并约束 panic 消息使用英文。
var Analyzer = &analysis.Analyzer{
	Name: "commentlint",
	Doc:  "检查包注释、导出声明、结构体字段注释的完备性，并约束 panic 消息使用英文",
	Run:  run,
}

// run 过滤生成文件后，按测试文件与非测试文件分组检查。
func run(pass *analysis.Pass) (any, error) {
	var files, testFiles []*ast.File
	for _, f := range pass.Files {
		if isGenerated(f) {
			continue
		}
		if isTestFile(pass, f) {
			testFiles = append(testFiles, f)
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 && len(testFiles) == 0 {
		return nil, nil
	}

	// 包注释按目录聚合：任一文件带包注释即视为该包已有包注释。只在非测试文件这一趟检查，
	// 避免测试变体（含外部测试包 xxx_test）重复报告同一处缺口。
	if len(testFiles) == 0 && !hasPackageComment(pass, files) {
		pass.Reportf(files[0].Package, "包 %s 缺少包注释", pass.Pkg.Name())
	}

	for _, f := range files {
		checkFile(pass, f, false)
	}
	for _, f := range testFiles {
		checkFile(pass, f, true)
	}
	return nil, nil
}

// isTestFile 报告文件是否为测试文件（文件名以 _test.go 结尾）。
func isTestFile(pass *analysis.Pass, f *ast.File) bool {
	return strings.HasSuffix(pass.Fset.Position(f.Pos()).Filename, "_test.go")
}

// isGenerated 报告文件是否为生成文件。
//
// 判定依据是包子句之前的注释：`ast.IsGenerated` 只认标准写法 `// Code generated <id> DO NOT EDIT.`，
// 故再按标记子串兜底，兼容 `DO NOT EDIT!!!` 这类非标准生成头；正文里提到的标记不算生成文件。
func isGenerated(f *ast.File) bool {
	if ast.IsGenerated(f) {
		return true
	}
	for _, group := range f.Comments {
		if group.Pos() > f.Package {
			break
		}
		text := group.Text()
		if strings.Contains(text, "Code generated") || strings.Contains(text, "DO NOT EDIT") {
			return true
		}
	}
	return false
}

// hasPackageComment 报告包注释是否存在。
//
// 包注释允许落在被构建约束排除的同目录源文件里（如 `//go:build linux` 的实现文件），
// 故先看本次参与编译的文件，再回退检查 IgnoredFiles 中的 Go 文件。
func hasPackageComment(pass *analysis.Pass, files []*ast.File) bool {
	for _, f := range files {
		if f.Doc != nil {
			return true
		}
	}
	for _, name := range pass.IgnoredFiles {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		src, err := pass.ReadFile(name)
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.PackageClauseOnly|parser.ParseComments)
		if err == nil && f.Doc != nil && !isGenerated(f) {
			return true
		}
	}
	return false
}

// checkFile 检查单个文件中的全部声明。
func checkFile(pass *analysis.Pass, f *ast.File, isTest bool) {
	checkPanicMessages(pass, f)
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			checkFuncDecl(pass, d, isTest)
		case *ast.GenDecl:
			checkGenDecl(pass, d)
		}
	}
}

// checkPanicMessages 检查 panic 调用参数中的字符串字面量是否包含汉字。
// 仅检查 panic 调用内部（含 fmt.Sprintf/fmt.Errorf/errors.Newf 等嵌套包装）的字面量，
// 普通业务字符串不受影响。
func checkPanicMessages(pass *analysis.Pass, f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "panic" {
			return true
		}
		for _, arg := range call.Args {
			ast.Inspect(arg, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(lit.Value)
				if err == nil && containsHan(text) {
					pass.Reportf(lit.Pos(), "panic 消息必须使用英文")
				}
				return true
			})
		}
		return true
	})
}

// containsHan 报告字符串是否包含汉字。
func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// checkFuncDecl 检查导出函数与导出方法；测试文件中的测试入口函数豁免。
func checkFuncDecl(pass *analysis.Pass, d *ast.FuncDecl, isTest bool) {
	if d.Doc != nil || !d.Name.IsExported() {
		return
	}
	if isTest && isTestEntry(d.Name.Name) {
		return
	}
	kind := "导出函数"
	if d.Recv != nil {
		kind = "导出方法"
	}
	pass.Reportf(d.Pos(), "%s %s 缺少注释", kind, d.Name.Name)
}

// isTestEntry 报告函数名是否为 go test 消费的测试入口（Test/Benchmark/Example/Fuzz 前缀）。
func isTestEntry(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Example", "Fuzz"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// checkGenDecl 检查类型、常量与变量声明。
func checkGenDecl(pass *analysis.Pass, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			checkTypeSpec(pass, d, s)
		case *ast.ValueSpec:
			checkValueSpec(pass, d, s)
		}
	}
}

// checkTypeSpec 检查导出类型，并递归检查其结构体字段与接口方法。
func checkTypeSpec(pass *analysis.Pass, d *ast.GenDecl, s *ast.TypeSpec) {
	if !s.Name.IsExported() {
		return
	}
	if s.Doc == nil && d.Doc == nil {
		pass.Reportf(s.Pos(), "导出类型 %s 缺少注释", s.Name.Name)
	}
	switch t := s.Type.(type) {
	case *ast.StructType:
		for _, fld := range t.Fields.List {
			checkStructField(pass, s.Name.Name, fld)
		}
	case *ast.InterfaceType:
		for _, m := range t.Methods.List {
			checkIfaceMethod(pass, s.Name.Name, m)
		}
	}
}

// checkStructField 检查结构体字段：导出字段与非导出字段均须带注释。
func checkStructField(pass *analysis.Pass, typeName string, fld *ast.Field) {
	if len(fld.Names) == 0 || fld.Doc != nil || fld.Comment != nil || hasCommentTag(fld) {
		return
	}
	names := fieldNames(fld)
	// 多名字声明中「部分导出」的写法（如 A, b string）不纳入检查。
	if !allExported(names) && ast.IsExported(names[0]) {
		return
	}
	pass.Reportf(fld.Pos(), "字段 %s.%s 缺少注释", typeName, strings.Join(names, ", "))
}

// checkIfaceMethod 检查接口方法；内嵌接口不检查。
func checkIfaceMethod(pass *analysis.Pass, typeName string, m *ast.Field) {
	if len(m.Names) == 0 || m.Doc != nil || m.Comment != nil {
		return
	}
	names := fieldNames(m)
	if !allExported(names) {
		return
	}
	pass.Reportf(m.Pos(), "接口方法 %s.%s 缺少注释", typeName, strings.Join(names, ", "))
}

// checkValueSpec 检查导出常量与导出变量。
func checkValueSpec(pass *analysis.Pass, d *ast.GenDecl, s *ast.ValueSpec) {
	if s.Doc != nil || d.Doc != nil || s.Comment != nil {
		return
	}
	kind := "导出变量"
	if d.Tok == token.CONST {
		kind = "导出常量"
	}
	for _, n := range s.Names {
		if n.IsExported() {
			pass.Reportf(n.Pos(), "%s %s 缺少注释", kind, n.Name)
		}
	}
}

// fieldNames 返回字段声明的全部名字（内嵌成员返回空切片）。
func fieldNames(f *ast.Field) []string {
	names := make([]string, 0, len(f.Names))
	for _, n := range f.Names {
		names = append(names, n.Name)
	}
	return names
}

// allExported 报告名字集合是否全部导出。
func allExported(names []string) bool {
	for _, n := range names {
		if !ast.IsExported(n) {
			return false
		}
	}
	return true
}

// hasCommentTag 报告字段的 struct tag 是否带 comment 标签（配置结构体字段说明的单一事实来源）。
func hasCommentTag(f *ast.Field) bool {
	return f.Tag != nil && strings.Contains(f.Tag.Value, `comment:"`)
}
