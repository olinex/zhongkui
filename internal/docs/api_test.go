package docs

import (
	"strings"
	"testing"
)

// TestApiDomain 校验接口路径到业务域的提取。
func TestApiDomain(t *testing.T) {
	cases := map[string]string{
		"/api/v1/tag/rule/list":         "tag",
		"/api/v1/admin/capturer/create": "admin",
		"/api/v1/ai/agent/session/list": "ai",
		"/api/v1/features/list":         "features",
		"/api/v1/":                      "root",
	}
	for path, want := range cases {
		if got := apiDomain(path); got != want {
			t.Errorf("apiDomain(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestUniqueApiSlug 校验接口页文件名生成与冲突去重。
func TestUniqueApiSlug(t *testing.T) {
	used := make(map[string]bool)
	first := uniqueApiSlug(used, "POST", "/api/v1/tag/rule/:id/delete")
	second := uniqueApiSlug(used, "POST", "/api/v1/tag/rule/:id/delete")
	if first != "post-tag-rule-id-delete" {
		t.Errorf("首个 slug = %q", first)
	}
	if second != first+"-2" {
		t.Errorf("冲突 slug = %q, want %q", second, first+"-2")
	}
}

// TestRenderApiIndex 校验索引按业务域分组并生成相对链接。
func TestRenderApiIndex(t *testing.T) {
	items := []apiIndexItem{
		{Domain: "tag", Method: "POST", Path: "/api/v1/tag/rule/list", Description: "列表", Output: "generated/reference/api/tag/post-tag-rule-list.md"},
		{Domain: "admin", Method: "POST", Path: "/api/v1/admin/user/list", Description: "用户列表", Output: "generated/reference/api/admin/post-admin-user-list.md"},
	}
	content := renderApiIndex(items)
	for _, want := range []string{"### 管理", "### 标签", "[`POST /api/v1/tag/rule/list`](./tag/post-tag-rule-list.md)", "用户列表"} {
		if !strings.Contains(content, want) {
			t.Errorf("索引缺少 %q:\n%s", want, content)
		}
	}
}
