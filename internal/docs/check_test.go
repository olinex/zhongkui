package docs

import (
	// 标准库
	"os"
	"path/filepath"
	"testing"
)

// TestCompareGeneratedTrees 校验漂移比对能识别缺失、内容不一致与多余文件。
func TestCompareGeneratedTrees(t *testing.T) {
	expected := t.TempDir()
	actual := t.TempDir()

	writeFile(t, expected, "index.md", "第一版")
	writeFile(t, expected, "generated/explanation/a.md", "内容 A")
	writeFile(t, expected, "generated/explanation/b.md", "内容 B")

	writeFile(t, actual, "index.md", "第一版")
	writeFile(t, actual, "generated/explanation/a.md", "内容 A 已改")
	writeFile(t, actual, "generated/explanation/c.md", "多余文件")

	diffs, err := compareGeneratedTrees(expected, actual)
	if err != nil {
		t.Fatalf("compareGeneratedTrees 返回错误: %v", err)
	}
	want := map[string]bool{
		"内容不一致: generated/explanation/a.md": true,
		"缺失: generated/explanation/b.md":    true,
		"多余: generated/explanation/c.md":    true,
	}
	if len(diffs) != len(want) {
		t.Fatalf("漂移项数 = %d, want %d: %v", len(diffs), len(want), diffs)
	}
	for _, diff := range diffs {
		if !want[diff] {
			t.Errorf("未预期的漂移项: %q", diff)
		}
	}
}

// TestCompareGeneratedTrees_NoDrift 校验完全一致时返回空列表。
func TestCompareGeneratedTrees_NoDrift(t *testing.T) {
	expected := t.TempDir()
	actual := t.TempDir()
	writeFile(t, expected, "index.md", "内容")
	writeFile(t, actual, "index.md", "内容")

	diffs, err := compareGeneratedTrees(expected, actual)
	if err != nil {
		t.Fatalf("compareGeneratedTrees 返回错误: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("无漂移时不应返回差异: %v", diffs)
	}
}

// writeFile 在 root 下写入内容，目录按需创建。
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}
