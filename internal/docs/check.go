package docs

import (
	// 标准库
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	// 第三方
	"github.com/spf13/cobra"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// checkMaxReport 是漂移报告中最多列出的文件数，避免一次提交打印上百行差异。
const checkMaxReport = 20

// Check 校验工作区生成物与当前代码声明一致：先把全部生成域导出到临时目录，再与 docs/ 下的
// 实际产物逐文件比对。任何缺失、多余或内容不同都返回错误，使 CI 能拦截文档漂移。
// 只比较生成域（docs/generated、docs/index.md、docs/nav.json），不触碰策展目录。
// root 用于把 CLI 命令树一并纳入投影；为 nil 时不生成 CLI 参考页。
func Check(ctx context.Context, projectRoot string, root *cobra.Command) error {
	tmpRoot, err := os.MkdirTemp("", "zki-docs-check-*")
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() { _ = os.RemoveAll(tmpRoot) }()

	if err := Export(ctx, Options{ProjectRoot: projectRoot, Root: tmpRoot, RootCommand: root}); err != nil {
		return errors.WithStack(err)
	}

	actualRoot := filepath.Join(projectRoot, Root)
	diffs, err := compareGeneratedTrees(tmpRoot, actualRoot)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(diffs) > 0 {
		if len(diffs) > checkMaxReport {
			diffs = append(diffs[:checkMaxReport], fmt.Sprintf("... and %d more files omitted", len(diffs)-checkMaxReport))
		}
		return errors.Newf("docs: generated artifacts are out of sync with code declarations; run `zki create docs` and commit the result:\n  %s",
			strings.Join(diffs, "\n  "))
	}
	return nil
}

// compareGeneratedTrees 比对临时导出目录与工作区 docs 目录中的生成域，返回漂移描述列表。
func compareGeneratedTrees(expectedRoot, actualRoot string) ([]string, error) {
	expected, err := generatedFiles(expectedRoot)
	if err != nil {
		return nil, err
	}
	actual, err := generatedFiles(actualRoot)
	if err != nil {
		return nil, err
	}

	var diffs []string
	for _, rel := range expected {
		expectedData, err := os.ReadFile(filepath.Join(expectedRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		actualPath := filepath.Join(actualRoot, filepath.FromSlash(rel))
		actualData, err := os.ReadFile(actualPath)
		if os.IsNotExist(err) {
			diffs = append(diffs, "missing: "+rel)
			continue
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !bytes.Equal(expectedData, actualData) {
			diffs = append(diffs, "content mismatch: "+rel)
		}
	}

	expectedSet := make(map[string]bool, len(expected))
	for _, rel := range expected {
		expectedSet[rel] = true
	}
	for _, rel := range actual {
		if !expectedSet[rel] {
			diffs = append(diffs, "unexpected: "+rel)
		}
	}
	sort.Strings(diffs)
	return diffs, nil
}

// generatedFiles 返回根目录下生成域的全部文件（相对路径，斜线分隔），包含 index.md 与 nav.json。
func generatedFiles(root string) ([]string, error) {
	var out []string
	for _, rel := range []string{"index.md", "nav.json"} {
		if info, err := os.Stat(filepath.Join(root, rel)); err == nil && !info.IsDir() {
			out = append(out, rel)
		}
	}
	generatedDir := filepath.Join(root, GeneratedRoot)
	if _, err := os.Stat(generatedDir); os.IsNotExist(err) {
		return out, nil
	}
	err := filepath.WalkDir(generatedDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		out = append(out, path.Clean(filepath.ToSlash(rel)))
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sort.Strings(out)
	return out, nil
}
