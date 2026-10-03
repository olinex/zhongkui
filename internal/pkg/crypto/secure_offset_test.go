package crypto

import (
	// 标准库
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

// TestSecureFieldOffsets 是加密字段 offset 的全局守卫：扫描 internal/models 下全部 Go 文件，
// 解析每个 secure 标签并断言两条不变量：
//   - offset + max(MinKeyLength, length) 不超过密码本长度（超出时 key.DeriveChildKey 会在
//     bookBytes[offset:] 处切片越界 panic）；
//   - offset 全局唯一（复用 offset 会让不同字段派生同一条盐，造成密钥复用）。
//
// 以源码扫描而非导入模型实现：避免 pkg/crypto 反向依赖 models 造成导入环，同时自动覆盖
// 新增模型，无需维护模型清单。
func TestSecureFieldOffsets(t *testing.T) {
	root := cryptoModuleRoot(t)
	modelsDir := filepath.Join(root, "internal", "models")
	owner := make(map[int]string)
	err := filepath.WalkDir(modelsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		ast.Inspect(file, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok || len(field.Names) == 0 || field.Tag == nil {
				return true
			}
			raw, unquoteErr := strconv.Unquote(field.Tag.Value)
			if unquoteErr != nil {
				return true
			}
			secure := reflect.StructTag(raw).Get(SecurityTagKey)
			if secure == "" {
				return true
			}
			where := rel + ":" + field.Names[0].Name
			spec, tagErr := ParseCryptoTag(secure)
			if tagErr != nil {
				t.Errorf("%s: malformed secure tag %q: %v", where, secure, tagErr)
				return true
			}
			span := spec.Offset + max(key.MinKeyLength, spec.Length)
			if span > key.PasswordBookSize {
				t.Errorf("%s: secure offset %d with key length %d exceeds the password book size %d (key derivation would slice out of range)",
					where, spec.Offset, max(key.MinKeyLength, spec.Length), key.PasswordBookSize)
			}
			if prev, dup := owner[spec.Offset]; dup {
				t.Errorf("%s: secure offset %d is already used by %s (salt reuse)", where, spec.Offset, prev)
			} else {
				owner[spec.Offset] = where
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", modelsDir, err)
	}
	// 扫描路径失效（如模型目录改名）时必须失败，否则本守卫会静默失效
	if len(owner) == 0 {
		t.Fatalf("no secure fields found under %s, the offset guard is not scanning the models", modelsDir)
	}
}

// cryptoModuleRoot 从当前工作目录向上查找含 go.mod 的模块根目录。
func cryptoModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found in any parent directory")
		}
		dir = parent
	}
}
