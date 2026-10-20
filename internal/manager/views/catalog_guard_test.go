// 本文件是「单一声明点」守卫测试：接口元信息构造器 StructMetaGinHandler / ListMetaGinHandler
// 只允许由声明 helper（RegisterApi / RegisterListApi）在 catalog.go 中调用，其定义位于 meta.go；
// 其它非测试文件出现裸用即视为绕过了接口目录声明，本测试以 t.Errorf 指出文件与行号。
package views

import (
	// 标准库
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// bareMetaHandlers 是不允许在 helper 之外被裸用的构造器名（含包名限定写法亦可被包含匹配捕获）。
var bareMetaHandlers = []string{
	"StructMetaGinHandler(",
	"ListMetaGinHandler(",
}

// TestCatalogSingleDeclarationPoint 递归遍历本包目录下的非测试 .go 文件，
// 断言 StructMetaGinHandler / ListMetaGinHandler 仅在定义处（meta.go）与 helper 实现处
// （catalog.go）出现；其它文件若出现裸用则失败并打印文件与行号。
func TestCatalogSingleDeclarationPoint(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) 失败，无法定位守卫测试所在目录")
	}
	viewsDir := filepath.Dir(thisFile)

	// 允许出现构造器名字的文件：定义处与 helper 实现处。
	allowedFiles := map[string]bool{
		"meta.go":    true,
		"catalog.go": true,
	}

	walkErr := filepath.WalkDir(viewsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if allowedFiles[name] {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return errors.WithStack(readErr)
		}
		for lineIndex, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			// 注释行（以 // 开头）中提及构造器名不算裸用。
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			for _, handler := range bareMetaHandlers {
				if strings.Contains(trimmed, handler) {
					t.Errorf("%s:%d: 检测到对 %s 的裸用，接口元信息必须经 RegisterApi/RegisterListApi 声明",
						path, lineIndex+1, strings.TrimSuffix(handler, "("))
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历 %s 失败：%v", viewsDir, walkErr)
	}
}
