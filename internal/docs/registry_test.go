package docs

import (
	// 标准库
	"os"
	"path/filepath"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// TestRegistry 校验文档注册表的结构不变量：分类合法、输出路径唯一且不逃逸 docs、
// 生成条目对应的包目录存在、叙述档条目具备非空标题与摘要与角色与场景、
// 并且不存在没有任何条目使用的死角色。
func TestRegistry(t *testing.T) {
	root := repoRoot(t)
	seen := make(map[string]bool)
	usedRoles := make(map[Role]bool)

	for _, entry := range Registry() {
		if !entry.Category.Valid() {
			t.Errorf("条目分类非法: %q（Output=%q）", entry.Category, entry.Output)
		}

		if entry.Output == "" {
			t.Errorf("条目 Output 不能为空: %+v", entry)
		} else if !filepath.IsLocal(entry.Output) {
			t.Errorf("条目 Output 必须为 docs 下的相对路径且不得逃逸: %q", entry.Output)
		} else if seen[entry.Output] {
			t.Errorf("条目 Output 重复: %q", entry.Output)
		} else {
			seen[entry.Output] = true
		}

		if entry.Package != "" {
			rel := strings.TrimPrefix(entry.Package, base.ModulePath()+"/")
			dir := filepath.Join(root, rel)
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				t.Errorf("生成条目的包目录不存在: %q（Package=%q）", dir, entry.Package)
			}
		}

		if entry.NarrativeOnly && (entry.Title == "" || entry.Summary == "") {
			t.Errorf("叙述档条目必须包含非空 Title 与 Summary: %+v", entry)
		}

		if len(entry.Roles) == 0 {
			t.Errorf("条目必须标注至少一个建议阅读角色: %q", entry.Output)
		}
		for _, role := range entry.Roles {
			if !role.Valid() {
				t.Errorf("条目角色非法: %q（Output=%q）", role, entry.Output)
			}
			usedRoles[role] = true
		}
		if entry.Scene == "" {
			t.Errorf("条目必须标注非空适用场景: %q", entry.Output)
		}
	}

	for _, role := range Roles() {
		if !usedRoles[role] {
			t.Errorf("角色未被任何条目使用（死角色）: %q", role)
		}
	}
}

// TestRegistry_GeneratedTitleUnique 守卫自动生成条目的标题唯一：数据库以标题（Title）作为生成
// 文档分区的稳定键，重复标题会导致同步互相覆盖。
func TestRegistry_GeneratedTitleUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, entry := range Generated() {
		if prev, ok := seen[entry.Title]; ok {
			t.Errorf("生成条目标题重复: %q（Output=%q 与 %q）", entry.Title, entry.Output, prev)
			continue
		}
		seen[entry.Title] = entry.Output
	}
}

// repoRoot 从当前测试工作目录向上查找 go.mod，返回项目根目录。
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("无法获取工作目录: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("向上查找未找到 go.mod，起始目录: %s", dir)
		}
		dir = parent
	}
}
