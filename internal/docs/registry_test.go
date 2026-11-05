package docs

import (
	// 标准库
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// curatedStatuses 是策展文档元信息头允许的状态取值。
var curatedStatuses = []string{"草稿（AI 起草）", "已校对"}

// curatedReferencePattern 从「依据」行中提取反引号包裹的仓库相对路径。
var curatedReferencePattern = regexp.MustCompile("`([^`]+)`")

// TestRegistry 校验文档注册表的结构不变量：分类与分区合法、分区内顺序唯一、输出路径唯一且落在
// 来源对应的根目录、来源与包字段一致、责任人非空、叙述档元信息完整、不存在死角色。
func TestRegistry(t *testing.T) {
	root := repoRoot(t)
	seenOutput := make(map[string]bool)
	seenOrder := make(map[string]bool)
	usedRoles := make(map[Role]bool)

	for _, entry := range Registry() {
		if !entry.Category.Valid() {
			t.Errorf("条目分类非法: %q（Output=%q）", entry.Category, entry.Output)
		}
		if !entry.Section.Valid() {
			t.Errorf("条目分区非法: %q（Output=%q）", entry.Section, entry.Output)
		}
		if entry.Order <= 0 {
			t.Errorf("条目 Order 必须为正数: %q", entry.Output)
		}
		orderKey := string(entry.Section) + "#" + itoa(entry.Order)
		if seenOrder[orderKey] {
			t.Errorf("分区内 Order 重复: %s（Output=%q）", orderKey, entry.Output)
		}
		seenOrder[orderKey] = true

		if !entry.Source.Valid() {
			t.Errorf("条目来源非法: %q（Output=%q）", entry.Source, entry.Output)
		}
		if !entry.AiIngest.Valid() {
			t.Errorf("条目 AI 摄入分区非法: %q（Output=%q）", entry.AiIngest, entry.Output)
		}
		if entry.Owner == "" {
			t.Errorf("条目必须标注责任人: %q", entry.Output)
		}

		if entry.Output == "" {
			t.Errorf("条目 Output 不能为空: %+v", entry)
		} else if !filepath.IsLocal(entry.Output) {
			t.Errorf("条目 Output 必须为 docs 下的相对路径且不得逃逸: %q", entry.Output)
		} else if seenOutput[entry.Output] {
			t.Errorf("条目 Output 重复: %q", entry.Output)
		} else {
			seenOutput[entry.Output] = true
		}
		if wantRoot := entry.Source.Root(); wantRoot != "" && !strings.HasPrefix(entry.Output, wantRoot+"/") {
			t.Errorf("条目 Output 必须位于 %s/ 下: %q（Source=%s）", wantRoot, entry.Output, entry.Source)
		}

		if entry.Source == SourceComment {
			if entry.Package == "" {
				t.Errorf("comment 条目必须声明 Package: %q", entry.Output)
			} else {
				rel := strings.TrimPrefix(entry.Package, base.ModulePath()+"/")
				dir := filepath.Join(root, rel)
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					t.Errorf("生成条目的包目录不存在: %q（Package=%q）", dir, entry.Package)
				}
			}
		} else if entry.Package != "" {
			t.Errorf("策展条目不得声明 Package: %q", entry.Output)
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

// TestDocs_NoOrphanPages 守卫孤儿页：docs/curated 下每个 Markdown 都必须登记；
// docs/generated 下的 Markdown 只能来自注册条目或自动导出的参考档。
func TestDocs_NoOrphanPages(t *testing.T) {
	root := repoRoot(t)
	registered := make(map[string]bool, len(Registry()))
	for _, entry := range Registry() {
		registered[entry.Output] = true
	}

	for _, rel := range markdownFiles(t, filepath.Join(root, Root, CuratedRoot)) {
		if !registered[rel] {
			t.Errorf("策展目录存在未登记页面: %s", rel)
		}
	}

	generatedPrefix := filepath.ToSlash(filepath.Join(GeneratedRoot, "reference")) + "/"
	for _, rel := range markdownFiles(t, filepath.Join(root, Root, GeneratedRoot)) {
		if registered[rel] || strings.HasPrefix(rel, generatedPrefix) {
			continue
		}
		t.Errorf("生成目录存在未登记页面: %s", rel)
	}
}

// TestDocs_GeneratedFilesCarryMarker 守卫生成目录中的每个 Markdown 都带生成标记，防止人工内容混入。
func TestDocs_GeneratedFilesCarryMarker(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range markdownFiles(t, filepath.Join(root, Root, GeneratedRoot)) {
		path := filepath.Join(root, Root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取生成文件失败: %v", err)
		}
		if !strings.HasPrefix(string(data), generatedMarker) {
			t.Errorf("生成文件缺少生成标记: %s", rel)
		}
	}
}

// TestDocs_RegisteredFilesExist 守卫注册表指向的文件真实存在：登记了却没有产物说明导出未执行。
func TestDocs_RegisteredFilesExist(t *testing.T) {
	root := repoRoot(t)
	for _, entry := range Registry() {
		path := filepath.Join(root, Root, filepath.FromSlash(entry.Output))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			t.Errorf("注册条目的产物不存在: %s（Output=%q）", path, entry.Output)
		}
	}
}

// TestDocs_CuratedMetadata 守卫策展文档的元信息头：状态、责任人、依据齐全；状态合法；
// 依据路径真实存在；模板的「起草须知」不得残留。
func TestDocs_CuratedMetadata(t *testing.T) {
	root := repoRoot(t)
	for _, entry := range Curated() {
		rel := entry.Output
		data, err := os.ReadFile(filepath.Join(root, Root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("读取策展文档失败: %v（%s）", err, rel)
			continue
		}
		content := string(data)

		status, ok := metaLine(content, "状态：")
		if !ok {
			t.Errorf("策展文档缺少「状态」元信息: %s", rel)
		} else if !slicesContains(curatedStatuses, status) {
			t.Errorf("策展文档状态非法: %q（%s）", status, rel)
		}
		owner, ok := metaLine(content, "责任人：")
		if !ok || strings.TrimSpace(owner) == "" {
			t.Errorf("策展文档缺少「责任人」元信息: %s", rel)
		}
		refs, ok := metaLine(content, "依据：")
		if !ok || strings.TrimSpace(refs) == "" {
			t.Errorf("策展文档缺少「依据」元信息: %s", rel)
			continue
		}
		paths := curatedReferencePattern.FindAllStringSubmatch(refs, -1)
		if len(paths) == 0 {
			t.Errorf("策展文档「依据」必须用反引号列出仓库相对路径: %s", rel)
			continue
		}
		for _, match := range paths {
			target := filepath.Join(root, filepath.FromSlash(match[1]))
			if _, err := os.Stat(target); err != nil {
				t.Errorf("策展文档依据路径不存在: %q（%s）", match[1], rel)
			}
		}
		if hasTemplateInstructionHeading(content) {
			t.Errorf("策展文档残留模板「起草须知」小节: %s", rel)
		}
	}
}

// hasTemplateInstructionHeading 报告内容是否仍含模板的「起草须知」小节标题。
// 只匹配标题行，避免把正文中对模板的说明误判为残留。
func hasTemplateInstructionHeading(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## 起草须知" || trimmed == "### 起草须知" {
			return true
		}
	}
	return false
}

// metaLine 在内容中查找以 `> ` 开头、含指定标签的元信息行，返回标签后的值。
func metaLine(content, key string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimPrefix(strings.TrimSpace(line), "> ")
		if strings.HasPrefix(trimmed, key) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, key)), true
		}
	}
	return "", false
}

// markdownFiles 返回 dir 下全部 Markdown 文件相对 docs 根的斜线路径，并按字典序排序。
func markdownFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(filepath.Join(repoRoot(t), Root), path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("遍历 %s 失败: %v", dir, err)
	}
	return out
}

// slicesContains 报告字符串切片是否包含指定值。
func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// itoa 是测试内使用的十进制整数转字符串，避免引入 strconv 依赖。
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
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
