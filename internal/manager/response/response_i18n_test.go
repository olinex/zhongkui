package response

import (
	// 标准库
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	// internal — 被测依赖
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// missingErrorKeySentinel 是必定未登记的 i18n 键，用于验证「未命中返回键本身」这一判定前提
// 成立，避免守卫测试因判定方式失效而空转（与 views/enum_i18n_test.go 的哨兵用法一致）。
const missingErrorKeySentinel = "error.__missingErrorKey__"

// msgIDFactoryArgIndex 记录「以 msgID 字符串字面量传参的错误工厂」→ msgID 参数下标。
// 新增此类工厂时必须登记，否则其调用点不会被本守卫测试覆盖（漏登记的缺口无法在构建阶段暴露）。
var msgIDFactoryArgIndex = map[string]int{
	"CreateBadQueryError":               0,
	"CreateSchedulerTaskError":          0,
	"CreateBadBodyMessageError":         0,
	"CreateLlmBindingIncompatibleError": 0,
	"CreateNotificationSendFailedError": 0,
	"CreateTagRuleSampleInputError":     0,
	"CreateBadUrlParamError":            0,
	// LocalizeDetail(localizer, msgID, args...)：msgID 是第二个参数
	"LocalizeDetail": 1,
}

// TestUserFacingErrorDetailsHaveBilingualText 守卫「面向用户的错误详情」与「双语 [error] 文案」
// 不漂移：全仓非测试 Go 文件里，凡以字符串字面量声明 msgID 的位置（DetailContext{MsgID: ...}
// 与各 Message 工厂调用），其键 error.<msgID> 在每种支持语言下都必须能解析，否则中文界面会把
// 裸 i18n 键（或回退的 msgID）返给用户。新站点忘记补语言文件时由本测试在构建阶段暴露。
//
// 说明：本测试只校验「已声明的字面量 msgID」，无法发现「本该本地化却仍用 errors.New 英文文案」
// 的站点，后者靠 AGENTS.md 的 Error Handling 约定与代码审查把关。
func TestUserFacingErrorDetailsHaveBilingualText(t *testing.T) {
	if got := translate.L(base.DefaultLanguage, missingErrorKeySentinel, nil); got != missingErrorKeySentinel {
		t.Fatalf("translate.L(%q) = %q, want the key itself; the miss check of this test is unsound",
			missingErrorKeySentinel, got)
	}
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	found := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "web", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			// 无法解析的文件不参与扫描：本守卫面向可编译源码，语法错误应由 go build 暴露
			return nil
		}
		collectLiteralMsgIDs(fset, file, found)
		return nil
	}); err != nil {
		t.Fatalf("scan repository for error msgIDs: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no literal error msgID was found: the scan is unsound (check the factory registry)")
	}
	for msgID, position := range found {
		key := "error." + msgID
		for i := range base.SupportLanguages {
			lang := base.SupportLanguages[i]
			if got := translate.L(&lang, key, nil); got == key {
				t.Errorf("translate[%s] misses %q declared in %s", lang.String(), key, position)
			}
		}
	}
}

// collectLiteralMsgIDs 从单个语法树中收集所有字符串字面量形式的 msgID，写入 out（键为 msgID，
// 值为其首次出现位置，便于失败时定位）。
func collectLiteralMsgIDs(fset *token.FileSet, file *ast.File, out map[string]string) {
	record := func(node ast.Node, value string) {
		if _, exists := out[value]; !exists {
			out[value] = fset.Position(node.Pos()).String()
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			// DetailContext{MsgID: "字面量", args: ...}
			if !isTypeNamed(node.Type, "DetailContext") {
				return true
			}
			for _, elt := range node.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "MsgID" {
					continue
				}
				if value, ok := stringLiteral(kv.Value); ok {
					record(node, value)
				}
			}
		case *ast.CallExpr:
			index, ok := msgIDFactoryArgIndex[calleeName(node.Fun)]
			if !ok || index >= len(node.Args) {
				return true
			}
			if value, ok := stringLiteral(node.Args[index]); ok {
				record(node, value)
			}
		}
		return true
	})
}

// isTypeNamed 判断类型表达式是否为指定名称（同时兼容裸标识符与包选择器形式）。
func isTypeNamed(expr ast.Expr, name string) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == name
	case *ast.SelectorExpr:
		return typed.Sel.Name == name
	default:
		return false
	}
}

// calleeName 返回调用表达式的被调函数名（兼容裸函数与包选择器形式），无法识别时返回空串。
func calleeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	default:
		return ""
	}
}

// stringLiteral 判断表达式是否为字符串字面量，是则返回其去引号后的值。
func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// repoRoot 从当前工作目录逐级上溯，返回首个包含 go.mod 的目录（即仓库根）。
func repoRoot() (string, error) {
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
