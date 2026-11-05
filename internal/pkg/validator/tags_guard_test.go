package validator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// bindingBuiltinTags 是 go-playground/validator 的内置 tag 白名单。
// 自定义 tag 统一通过 Funcs() 登记，不要重复放入此表。
var bindingBuiltinTags = map[string]struct{}{
	"required":                {},
	"required_if":             {},
	"required_unless":         {},
	"required_with":           {},
	"required_with_all":       {},
	"required_without":        {},
	"required_without_all":    {},
	"excluded":                {},
	"excluded_if":             {},
	"excluded_unless":         {},
	"excluded_with":           {},
	"excluded_with_all":       {},
	"excluded_without":        {},
	"excluded_without_all":    {},
	"omitempty":               {},
	"omitnil":                 {},
	"isdefault":               {},
	"structonly":              {},
	"nostructlevel":           {},
	"skip":                    {},
	"dive":                    {},
	"keys":                    {},
	"endkeys":                 {},
	"min":                     {},
	"max":                     {},
	"len":                     {},
	"eq":                      {},
	"ne":                      {},
	"gt":                      {},
	"gte":                     {},
	"lt":                      {},
	"lte":                     {},
	"eqfield":                 {},
	"nefield":                 {},
	"gtfield":                 {},
	"gtefield":                {},
	"ltfield":                 {},
	"ltefield":                {},
	"eqcsfield":               {},
	"necsfield":               {},
	"gtcsfield":               {},
	"gtecsfield":              {},
	"ltcsfield":               {},
	"ltecsfield":              {},
	"eqignorecase":            {},
	"neignorecase":            {},
	"alpha":                   {},
	"alphanum":                {},
	"alphanumunicode":         {},
	"alphaunicode":            {},
	"ascii":                   {},
	"boolean":                 {},
	"contains":                {},
	"containsany":             {},
	"containsrune":            {},
	"excludes":                {},
	"excludesall":             {},
	"excludesrune":            {},
	"startswith":              {},
	"startswithignorecase":    {},
	"endswith":                {},
	"endswithignorecase":      {},
	"startsnotwith":           {},
	"startsnotwithignorecase": {},
	"endsnotwith":             {},
	"endsnotwithignorecase":   {},
	"lowercase":               {},
	"uppercase":               {},
	"multibyte":               {},
	"number":                  {},
	"numeric":                 {},
	"printascii":              {},
	"e164":                    {},
	"email":                   {},
	"eth_addr":                {},
	"hexadecimal":             {},
	"hexcolor":                {},
	"hostname":                {},
	"hostname_rfc1123":        {},
	"url":                     {},
	"url_encoded":             {},
	"uri":                     {},
	"uuid":                    {},
	"uuid3":                   {},
	"uuid4":                   {},
	"uuid5":                   {},
	"uuid_rfc4122":            {},
	"ulid":                    {},
	"ip":                      {},
	"ip4_addr":                {},
	"ip6_addr":                {},
	"ip_addr":                 {},
	"ipv4":                    {},
	"ipv6":                    {},
	"mac":                     {},
	"json":                    {},
	"file":                    {},
	"dir":                     {},
	"dirpath":                 {},
	"filepath":                {},
	"base64":                  {},
	"base64url":               {},
	"base64rawurl":            {},
	"creditcard":              {},
	"ssn":                     {},
	"semver":                  {},
	"spicedb":                 {},
	"timezone":                {},
	"datetime":                {},
	"md5":                     {},
	"sha256":                  {},
	"sha384":                  {},
	"sha512":                  {},
	"btc_addr":                {},
	"btc_addr_bech32":         {},
	"oneof":                   {},
	"oneofci":                 {},
}

// TestBindingTagsRegistered 扫描 internal 下全部 Go 文件的 binding 标签，
// 断言每个 tag 要么是 go-playground 内置 tag，要么已在 Funcs() 中登记。
func TestBindingTagsRegistered(t *testing.T) {
	root := bindingRepoRoot(t)
	internalDir := filepath.Join(root, "internal")

	allowed := make(map[string]struct{}, len(bindingBuiltinTags))
	for tag := range bindingBuiltinTags {
		allowed[tag] = struct{}{}
	}
	for tag := range Funcs() {
		allowed[tag] = struct{}{}
	}

	type unknownTag struct {
		relPath   string
		line      int
		fieldName string
		tagName   string
	}
	var unknown []unknownTag
	fset := token.NewFileSet()

	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "docs", "web", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relPath = path
		}

		ast.Inspect(file, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}
			raw, unquoteErr := strconv.Unquote(field.Tag.Value)
			if unquoteErr != nil {
				return true
			}
			bindingValue := reflect.StructTag(raw).Get("binding")
			if bindingValue == "" {
				return true
			}

			fieldName := "<embedded>"
			if len(field.Names) > 0 {
				names := make([]string, 0, len(field.Names))
				for _, ident := range field.Names {
					names = append(names, ident.Name)
				}
				fieldName = strings.Join(names, ",")
			}
			line := fset.Position(field.Pos()).Line

			for _, expr := range strings.Split(bindingValue, ",") {
				expr = strings.TrimSpace(expr)
				if expr == "" || expr == "-" {
					continue
				}
				tagName := expr
				if i := strings.Index(expr, "="); i >= 0 {
					tagName = strings.TrimSpace(expr[:i])
				}
				if _, ok := allowed[tagName]; ok {
					continue
				}
				unknown = append(unknown, unknownTag{relPath, line, fieldName, tagName})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalDir, err)
	}

	sort.Slice(unknown, func(i, j int) bool {
		if unknown[i].relPath != unknown[j].relPath {
			return unknown[i].relPath < unknown[j].relPath
		}
		if unknown[i].line != unknown[j].line {
			return unknown[i].line < unknown[j].line
		}
		return unknown[i].tagName < unknown[j].tagName
	})
	for _, u := range unknown {
		t.Errorf("%s:%d %s: binding tag %q is not registered in validator.Funcs() and is not a go-playground built-in", u.relPath, u.line, u.fieldName, u.tagName)
	}
}

// TestJSONFrameChecksCentralized 禁止除 internal/pkg/validator/json.go 之外的包自行用
// strings.HasPrefix("{") / strings.HasSuffix("}") / strings.TrimSuffix("```") 判断 JSON 帧，
// 迫使所有裸 JSON 对象与代码围栏处理统一走 DecodeObject / DecodeObjectLenient / StripJSONFence。
func TestJSONFrameChecksCentralized(t *testing.T) {
	root := bindingRepoRoot(t)
	internalDir := filepath.Join(root, "internal")
	allowedFile := filepath.Join(internalDir, "pkg", "validator", "json.go")

	fset := token.NewFileSet()
	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "docs", "web", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relPath = path
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || path == allowedFile {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "strings" {
				return true
			}
			literal, ok := call.Args[1].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				return true
			}
			forbidden := (selector.Sel.Name == "HasPrefix" && value == "{") ||
				(selector.Sel.Name == "HasSuffix" && value == "}") ||
				(selector.Sel.Name == "TrimSuffix" && value == "```")
			if forbidden {
				t.Errorf("%s:%d: JSON frame check strings.%s(%q) must go through validator.DecodeObject / StripJSONFence",
					relPath, fset.Position(call.Pos()).Line, selector.Sel.Name, value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalDir, err)
	}
}

// bindingRepoRoot 从测试运行目录向上查找 module 为 zhongkuisec.com/zhongkui 的 go.mod。
func bindingRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil && hasModuleDirective(string(data), "zhongkuisec.com/zhongkui") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("cannot find go.mod declaring module zhongkuisec.com/zhongkui above %s", dir)
		}
		dir = parent
	}
}

func hasModuleDirective(gomod, module string) bool {
	for _, line := range strings.Split(gomod, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return fields[1] == module
		}
	}
	return false
}
