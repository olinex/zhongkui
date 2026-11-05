package errors

import (
	// 标准库
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// guardErrorMessageArgIndex 记录「错误构造调用」到消息参数下标的映射。
// 下标所指参数内的全部字符串字面量都会参与检查，以覆盖 `errors.New(fmt.Sprintf(...))` 等写法。
var guardErrorMessageArgIndex = map[string]int{
	"New":          0,
	"Newf":         0,
	"Errorf":       0,
	"Wrap":         1,
	"Wrapf":        1,
	"WithMessage":  1,
	"WithMessagef": 1,
	"Annotate":     1,
}

// TestErrorMessagesAreEnglish 守卫「后端错误信息必须为英文」：扫描仓库内全部非测试 Go 文件，
// 检查 errors.New/Newf/Errorf、errors.Wrap/Wrapf/WithMessage/Annotate、fmt.Errorf、panic 的消息
// 参数，以及 Error() 方法的 return 字面量，出现汉字即失败。
func TestErrorMessagesAreEnglish(t *testing.T) {
	root, err := guardRepoRoot()
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}

	var scanned int
	var violations []string
	report := func(pos token.Position, literal string) {
		violations = append(violations, pos.String()+": error message must be English, got "+strconv.Quote(literal))
	}

	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// 非后端源码目录与生成产物目录不参与扫描
			switch d.Name() {
			case ".git", "vendor", "node_modules", "web", "docs", "tmp",
				".trae", ".trae-html-share-packages", ".screenshots", ".vscode", ".gitee", ".github":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, p, nil, 0)
		if parseErr != nil {
			// 语法错误由 go build 暴露，本守卫只检查可解析源码
			return nil
		}
		imports := guardImports(file)
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if index, ok := guardErrorMessageIndex(node.Fun, imports); ok && index < len(node.Args) {
					scanned++
					guardHanLiterals(node.Args[index], func(lit *ast.BasicLit, value string) {
						report(fset.Position(lit.Pos()), value)
					})
				}
			case *ast.FuncDecl:
				if node.Recv != nil && node.Name.Name == "Error" && node.Body != nil {
					guardErrorMethodReturns(fset, node.Body, report)
				}
			}
			return true
		})
		return nil
	}); err != nil {
		t.Fatalf("scan repository for error messages: %v", err)
	}

	if scanned == 0 {
		t.Fatal("no error constructor was scanned: the scan is unsound (check the call registry)")
	}
	for _, violation := range violations {
		t.Errorf("%s", violation)
	}
}

// guardErrorMessageIndex 判断调用表达式是否为错误构造调用，是则返回其消息参数下标。
// 仅识别以 errors / fmt 包为限定符的调用与内置 panic，避免把其它包的 New 误判为错误构造。
func guardErrorMessageIndex(fun ast.Expr, imports map[string]string) (int, bool) {
	switch callee := fun.(type) {
	case *ast.Ident:
		if callee.Name == "panic" {
			return 0, true
		}
	case *ast.SelectorExpr:
		qualifier, ok := callee.X.(*ast.Ident)
		if !ok {
			return 0, false
		}
		importPath, known := imports[qualifier.Name]
		if !known {
			return 0, false
		}
		switch path.Base(importPath) {
		case "fmt":
			return 0, callee.Sel.Name == "Errorf"
		case "errors":
			index, ok := guardErrorMessageArgIndex[callee.Sel.Name]
			return index, ok
		}
	}
	return 0, false
}

// guardImports 返回当前文件的「导入名 → 导入路径」映射，未显式起别名的按路径末段命名。
func guardImports(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		imports[name] = importPath
	}
	return imports
}

// guardErrorMethodReturns 检查 Error() 方法中的 return 字面量，避免错误文案绕过错误构造调用。
func guardErrorMethodReturns(fset *token.FileSet, body *ast.BlockStmt, report func(token.Position, string)) {
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			guardHanLiterals(result, func(lit *ast.BasicLit, value string) {
				report(fset.Position(lit.Pos()), value)
			})
		}
		return true
	})
}

// guardHanLiterals 遍历表达式内全部字符串字面量，含汉字时回调上报。
func guardHanLiterals(node ast.Node, report func(*ast.BasicLit, string)) {
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if guardHasHan(value) {
			report(lit, value)
		}
		return true
	})
}

// guardHasHan 判断字符串是否包含汉字。
func guardHasHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// guardRepoRoot 从当前工作目录逐级上溯，返回首个包含 go.mod 的目录。
func guardRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
