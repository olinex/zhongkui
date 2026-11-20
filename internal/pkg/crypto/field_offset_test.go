package crypto

import (
	// 标准库
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
)

// TestFieldTagOffsets 是字段级密钥派生参数（crypto / hash / s3crypto 标签）的全局守卫：
// 扫描 internal/models 下全部 Go 文件，断言以下不变量：
//   - crypto 标签只能挂在 serializer.Crypto / *serializer.Crypto 字段上；
//   - hash 标签只能挂在 serializer.Hash 字段上，且不得携带 mode（算法固定 HMAC-SHA256）；
//   - s3crypto 标签只能挂在 serializer.S3Crypto 字段上，且不得携带 mode（管道固定 zstd+AES-GCM）；
//   - 同一字段不得同时携带两类及以上标签；声明了对应类型的字段不得缺少标签；
//   - offset + max(MinKeyLength, length) 不超过密码本长度（超出时 key.DeriveChildKey
//     会在 bookBytes[offset:] 处切片越界 panic）；
//   - offset 在 crypto / hash / s3crypto 三类标签之间全局唯一（复用 offset 会让不同字段派生同一条盐）。
//
// 以源码扫描而非导入模型实现：避免 pkg/crypto 反向依赖 models 造成导入环，同时自动覆盖
// 新增模型，无需维护模型清单。
func TestFieldTagOffsets(t *testing.T) {
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
			tags := reflect.StructTag(raw)
			cryptoRaw, hasCrypto := tags.Lookup(CryptoTagKey)
			hashRaw, hasHash := tags.Lookup(HashTagKey)
			s3CryptoRaw, hasS3Crypto := tags.Lookup(S3CryptoTagKey)
			if !hasCrypto && !hasHash && !hasS3Crypto {
				return true
			}
			where := rel + ":" + field.Names[0].Name
			typ := types.ExprString(field.Type)
			isCryptoField := typ == "serializer.Crypto" || typ == "*serializer.Crypto"
			isHashField := typ == "serializer.Hash"
			isS3CryptoField := typ == "serializer.S3Crypto"
			if tagKinds := countTagKinds(hasCrypto, hasHash, hasS3Crypto); tagKinds > 1 {
				t.Errorf("%s: field must carry only one of %q / %q / %q tags", where, CryptoTagKey, HashTagKey, S3CryptoTagKey)
				return true
			}
			if hasCrypto {
				if !isCryptoField {
					t.Errorf("%s: %q tag requires serializer.Crypto, got %s", where, CryptoTagKey, typ)
					return true
				}
				spec, tagErr := ParseCryptoTag(cryptoRaw)
				if tagErr != nil {
					t.Errorf("%s: malformed %s tag %q: %v", where, CryptoTagKey, cryptoRaw, tagErr)
					return true
				}
				checkOffset(t, where, CryptoTagKey, spec.Offset, spec.Length, owner)
			}
			if hasHash {
				if !isHashField {
					t.Errorf("%s: %q tag requires serializer.Hash, got %s", where, HashTagKey, typ)
					return true
				}
				// 摘要算法在代码层固定为 HMAC-SHA256，hash 标签不允许再声明 mode
				if strings.Contains(hashRaw, "mode") {
					t.Errorf("%s: %s tag must not carry a mode attribute (algorithm is fixed to hmac-sha256): %q", where, HashTagKey, hashRaw)
					return true
				}
				spec, tagErr := ParseHashTag(hashRaw)
				if tagErr != nil {
					t.Errorf("%s: malformed %s tag %q: %v", where, HashTagKey, hashRaw, tagErr)
					return true
				}
				checkOffset(t, where, HashTagKey, spec.Offset, spec.Length, owner)
			}
			if hasS3Crypto {
				if !isS3CryptoField {
					t.Errorf("%s: %q tag requires serializer.S3Crypto, got %s", where, S3CryptoTagKey, typ)
					return true
				}
				// S3 对象内容固定使用 zstd+AES-GCM 管道，s3crypto 标签不允许再声明 mode
				if strings.Contains(s3CryptoRaw, "mode") {
					t.Errorf("%s: %s tag must not carry a mode attribute (pipeline is fixed to zstd+aes-gcm): %q", where, S3CryptoTagKey, s3CryptoRaw)
					return true
				}
				spec, tagErr := ParseS3CryptoTag(s3CryptoRaw)
				if tagErr != nil {
					t.Errorf("%s: malformed %s tag %q: %v", where, S3CryptoTagKey, s3CryptoRaw, tagErr)
					return true
				}
				checkOffset(t, where, S3CryptoTagKey, spec.Offset, spec.Length, owner)
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
		t.Fatalf("no crypto/hash fields found under %s, the offset guard is not scanning the models", modelsDir)
	}
}

// countTagKinds 统计字段携带的字段级安全标签种类数。
func countTagKinds(hasCrypto, hasHash, hasS3Crypto bool) int {
	count := 0
	for _, has := range []bool{hasCrypto, hasHash, hasS3Crypto} {
		if has {
			count++
		}
	}
	return count
}

// checkOffset 校验单个字段的 offset 边界，并维护 crypto/hash/s3crypto 三类标签共享的全局唯一命名空间。
func checkOffset(t *testing.T, where, tagKey string, offset, length int, owner map[int]string) {
	t.Helper()
	span := offset + max(key.MinKeyLength, length)
	if span > key.PasswordBookSize {
		t.Errorf("%s: %s offset %d with key length %d exceeds the password book size %d (key derivation would slice out of range)",
			where, tagKey, offset, max(key.MinKeyLength, length), key.PasswordBookSize)
	}
	if prev, dup := owner[offset]; dup {
		t.Errorf("%s: %s offset %d is already used by %s (salt reuse)", where, tagKey, offset, prev)
	} else {
		owner[offset] = where
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
